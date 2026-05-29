package service_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/dynamic/service"
)

// srResponse builds a fake AH5 ServiceRegistry service-discovery/lookup response.
func srResponse(providers ...string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		type provider struct {
			Name string `json:"name"`
		}
		type iface struct {
			TemplateName string `json:"templateName"`
		}
		type instance struct {
			InstanceID            string   `json:"instanceId"`
			Provider              provider `json:"provider"`
			ServiceDefinitionName string   `json:"serviceDefinitionName"`
			Interfaces            []iface  `json:"interfaces"`
			Version               string   `json:"version"`
		}
		type response struct {
			Entries []instance `json:"entries"`
			Count   int        `json:"count"`
		}
		var instances []instance
		for i, p := range providers {
			instances = append(instances, instance{
				InstanceID:            p + "|temperature-service|1",
				Provider:              provider{Name: p},
				ServiceDefinitionName: "temperature-service",
				Interfaces:            []iface{{TemplateName: "HTTP"}},
				Version:               fmt.Sprintf("%d", i+1),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response{Entries: instances, Count: len(instances)})
	}
}

// caResponse builds a fake ConsumerAuthorization verify response (plain JSON Boolean).
func caAlwaysAuthorized() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(true)
	}
}

func caAlwaysDenied() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(false)
	}
}

// fakeAuthSys builds a fake Authentication identity/verify response.
func fakeAuthSys(valid bool, systemName string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"verified": valid, "systemName": systemName})
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
	if len(resp.Results) != 2 {
		t.Errorf("expected 2 results (auth disabled), got %d", len(resp.Results))
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
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results for empty SR, got %d", len(resp.Results))
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
	if len(resp.Results) != 2 {
		t.Errorf("expected 2 results (all authorized), got %d", len(resp.Results))
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
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results (all denied), got %d", len(resp.Results))
	}
}

func TestOrchestrateDynamicAuthPartial(t *testing.T) {
	// CA authorizes only "sensor-1", denies "sensor-2".
	allowedProvider := "sensor-1"
	ca := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Provider string `json:"provider"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		authorized := req.Provider == allowedProvider
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(authorized)
	}))
	defer ca.Close()

	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1", "sensor-2")))
	defer sr.Close()

	orch := newOrchestrator(sr.URL, ca.URL, true)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 authorized result, got %d", len(resp.Results))
	}
	if resp.Results[0].ProviderName != "sensor-1" {
		t.Errorf("expected sensor-1, got %q", resp.Results[0].ProviderName)
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
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results when CA unreachable (fail-closed), got %d", len(resp.Results))
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
			Consumer string `json:"consumer"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		authorized := req.Consumer == "consumer-app"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(authorized)
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
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result (verified name used for CA), got %d", len(resp.Results))
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
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result when identity check disabled, got %d", len(resp.Results))
	}
}

// ---- Malformed upstream responses ----

func TestOrchestrateSRMalformedJSON(t *testing.T) {
	// SR returns malformed JSON — querySR decode error should propagate as SR error.
	sr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{not valid json"))
	}))
	defer sr.Close()

	orch := newOrchestrator(sr.URL, "", false)
	_, err := orch.Orchestrate(validRequest(), "")
	if err == nil {
		t.Error("expected error when SR returns malformed JSON")
	}
}

func TestOrchestrateIdentityAuthSysMalformedJSON(t *testing.T) {
	// Auth system returns malformed JSON — decode error → fail-closed.
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	authSys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{not valid json"))
	}))
	defer authSys.Close()

	orch := service.NewDynamicOrchestrator(sr.URL, "", authSys.URL, false, true)
	_, err := orch.Orchestrate(validRequest(), "some-token")
	if err != service.ErrIdentityInvalid {
		t.Errorf("expected ErrIdentityInvalid when auth sys returns malformed JSON, got %v", err)
	}
}

func TestOrchestrateIdentityInvalidAuthURL(t *testing.T) {
	// An authSysURL containing a control character causes http.NewRequest to fail.
	// The orchestrator must still return ErrIdentityInvalid (fail-closed).
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()

	orch := service.NewDynamicOrchestrator(sr.URL, "", "http://invalid\nurl", false, true)
	_, err := orch.Orchestrate(validRequest(), "some-token")
	if err != service.ErrIdentityInvalid {
		t.Errorf("expected ErrIdentityInvalid for malformed authSysURL, got %v", err)
	}
}

func TestOrchestrateCAMalformedJSONExcludesProvider(t *testing.T) {
	// CA returns malformed JSON — checkAuthorized decode error should exclude provider (fail-closed, D4).
	sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1")))
	defer sr.Close()
	ca := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{not valid json"))
	}))
	defer ca.Close()

	orch := newOrchestrator(sr.URL, ca.URL, true)
	resp, err := orch.Orchestrate(validRequest(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Fail-closed: malformed CA response means provider is excluded.
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results when CA returns malformed JSON (fail-closed), got %d", len(resp.Results))
	}
}

// ─── Cycle 17.3 — DynamicOrch calls AH5 lookup endpoint ─────────────────────

func TestDynamicOrchCallsAH5LookupEndpoint(t *testing.T) {
	called := false
	fakeSR := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/serviceregistry/service-discovery/lookup" {
			called = true
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"entries": []any{}, "count": 0})
	}))
	defer fakeSR.Close()

	orch := service.NewDynamicOrchestrator(fakeSR.URL, "", "", false, false)
	orch.Orchestrate(orchmodel.OrchestrationRequest{
		RequesterSystem:  orchmodel.System{SystemName: "C"},
		RequestedService: orchmodel.ServiceRequirement{ServiceDefinition: "temp"},
	}, "")
	if !called {
		t.Error("DynamicOrchestrator did not call AH5 lookup endpoint")
	}
}
