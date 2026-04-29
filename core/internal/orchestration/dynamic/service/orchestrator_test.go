package service_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/dynamic/service"
)

// srResponse builds a fake ServiceRegistry query response with the given providers.
func srResponse(providers ...string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		type system struct {
			SystemName string `json:"systemName"`
			Address    string `json:"address"`
			Port       int    `json:"port"`
		}
		type instance struct {
			ServiceDefinition string   `json:"serviceDefinition"`
			ProviderSystem    system   `json:"providerSystem"`
			ServiceUri        string   `json:"serviceUri"`
			Interfaces        []string `json:"interfaces"`
			Version           int      `json:"version"`
		}
		type response struct {
			ServiceQueryData []instance `json:"serviceQueryData"`
			UnfilteredHits   int        `json:"unfilteredHits"`
		}
		var instances []instance
		for i, p := range providers {
			instances = append(instances, instance{
				ServiceDefinition: "temperature-service",
				ProviderSystem:    system{SystemName: p, Address: "10.0.0.1", Port: 9000 + i},
				ServiceUri:        "/temperature",
				Interfaces:        []string{"HTTP"},
				Version:           1,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response{ServiceQueryData: instances, UnfilteredHits: len(instances)})
	}
}

// caResponse builds a fake ConsumerAuthorization verify response.
func caAlwaysAuthorized() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"authorized": true})
	}
}

func caAlwaysDenied() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"authorized": false})
	}
}

// fakeAuthSys builds a fake Authentication identity/verify response.
func fakeAuthSys(valid bool, systemName string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"valid": valid, "systemName": systemName})
	}
}

// newOrchestrator creates a DynamicOrchestrator with no identity check.
func newOrchestrator(srURL, caURL string, checkAuth bool) *service.DynamicOrchestrator {
	return service.NewDynamicOrchestrator(srURL, caURL, "", checkAuth, false)
}

func validRequest() orchmodel.OrchestrationRequest {
	return orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "consumer-app", Address: "localhost", Port: 0},
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "temperature-service"},
	}
}

// ---- Without auth ----

func TestOrchestrateDynamicNoAuth(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1", "sensor-2")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, false)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 2 {
		t.Errorf("expected 2 results (auth disabled), got %d", len(resp.Response))
	}
}

func TestOrchestrateDynamicEmptySR(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse()))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, false)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 0 {
		t.Errorf("expected 0 results for empty SR, got %d", len(resp.Response))
	}
}

// ---- With auth ----

func TestOrchestrateDynamicAuthAllAllowed(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1", "sensor-2")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, true)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 2 {
		t.Errorf("expected 2 results (all authorized), got %d", len(resp.Response))
	}
}

func TestOrchestrateDynamicAuthAllDenied(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1", "sensor-2")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(caAlwaysDenied()))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, true)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 0 {
		t.Errorf("expected 0 results (all denied), got %d", len(resp.Response))
	}
}

func TestOrchestrateDynamicAuthPartial(t *testing.T) {
	// CA authorizes only "sensor-1", denies "sensor-2".
	allowedProvider := "sensor-1"
	ca := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ProviderSystemName string `json:"providerSystemName"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		authorized := req.ProviderSystemName == allowedProvider
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"authorized": authorized})
	}))
	defer ca.Close()

	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1", "sensor-2")))
	defer sr.Close()

	orch := newOrchestrator(sr.URL, ca.URL, true)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 1 {
		t.Fatalf("expected 1 authorized result, got %d", len(resp.Response))
	}
	if resp.Response[0].Provider.SystemName != "sensor-1" {
		t.Errorf("expected sensor-1, got %q", resp.Response[0].Provider.SystemName)
	}
}

func TestOrchestrateDynamicCAUnreachableFailsClosed(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	// Point CA at a closed port to simulate unreachable service.
	orch := newOrchestrator(sr.URL, "http://127.0.0.1:1", true)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Fail-closed: unreachable CA means provider is excluded.
	if len(resp.Response) != 0 {
		t.Errorf("expected 0 results when CA unreachable (fail-closed), got %d", len(resp.Response))
	}
}

func TestOrchestrateDynamicSRUnreachable(t *testing.T) {
	orch := newOrchestrator("http://127.0.0.1:1", "http://127.0.0.1:1", false)
	_, err := orch.Orchestrate(validRequest(), "")
	if err == nil {
		t.Error("expected error when SR is unreachable")
	}
}

// ---- Validation ----

func TestOrchestrateDynamicMissingRequester(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse()))
	defer sr.Close()
	orch := newOrchestrator(sr.URL, "", false)
	_, err := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "svc"},
	}, "")
	if err == nil {
		t.Fatal("expected error for missing requesterSystem.systemName")
	}
}

func TestOrchestrateDynamicMissingService(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse()))
	defer sr.Close()
	orch := newOrchestrator(sr.URL, "", false)
	_, err := orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequesterSystem: orchmodel.System{SystemName: "consumer"},
	}, "")
	if err == nil {
		t.Fatal("expected error for missing serviceDefinition")
	}
}

// ---- Identity check ----

func TestOrchestrateIdentityRequired(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	authSys := httptest.NewServer(http.HandlerFunc(fakeAuthSys(true, "consumer-app")))
	defer authSys.Close()

	orch := service.NewDynamicOrchestrator(sr.URL, "", authSys.URL, false, true)
	_, err := orch.Orchestrate(validRequest(), "") // empty token
	if err != service.ErrIdentityRequired {
		t.Errorf("expected ErrIdentityRequired, got %v", err)
	}
}

func TestOrchestrateIdentityInvalidToken(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	authSys := httptest.NewServer(http.HandlerFunc(fakeAuthSys(false, "")))
	defer authSys.Close()

	orch := service.NewDynamicOrchestrator(sr.URL, "", authSys.URL, false, true)
	_, err := orch.Orchestrate(validRequest(), "bad-token")
	if err != service.ErrIdentityInvalid {
		t.Errorf("expected ErrIdentityInvalid, got %v", err)
	}
}

func TestOrchestrateIdentityAuthSysUnreachable(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	// Auth system unreachable — fail-closed.
	orch := service.NewDynamicOrchestrator(sr.URL, "", "http://127.0.0.1:1", false, true)
	_, err := orch.Orchestrate(validRequest(), "some-token")
	if err == nil {
		t.Error("expected error when auth system unreachable (fail-closed)")
	}
}

func TestOrchestrateIdentityVerifiedNameUsedForCACheck(t *testing.T) {
	// The requester sends systemName "impersonator" in the body, but the
	// verified token belongs to "consumer-app". The CA only authorizes
	// "consumer-app" — if the orchestrator correctly uses the verified name,
	// the result should be non-empty; if it uses the self-reported name, it would be empty.
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	authSys := httptest.NewServer(http.HandlerFunc(fakeAuthSys(true, "consumer-app")))
	defer authSys.Close()

	ca := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ConsumerSystemName string `json:"consumerSystemName"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		authorized := req.ConsumerSystemName == "consumer-app"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"authorized": authorized})
	}))
	defer ca.Close()

	orch := service.NewDynamicOrchestrator(sr.URL, ca.URL, authSys.URL, true, true)

	// Self-reported name is "impersonator" — should be overridden to "consumer-app".
	req := orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "impersonator", Address: "localhost", Port: 0},
		RequestedService: orchmodel.ServiceFilter{ServiceDefinition: "temperature-service"},
	}
	resp, err := orch.Orchestrate(req, "valid-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 1 {
		t.Errorf("expected 1 result (verified name used for CA), got %d", len(resp.Response))
	}
}

func TestOrchestrateIdentityDisabledNoTokenNeeded(t *testing.T) {
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	// checkIdentity=false — no token needed, should work fine.
	orch := service.NewDynamicOrchestrator(sr.URL, "", "http://127.0.0.1:1", false, false)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Response) != 1 {
		t.Errorf("expected 1 result when identity check disabled, got %d", len(resp.Response))
	}
}
