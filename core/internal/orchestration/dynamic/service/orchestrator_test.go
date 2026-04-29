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

func newOrchestrator(srURL, caURL string, checkAuth bool) *service.DynamicOrchestrator {
	return service.NewDynamicOrchestrator(srURL, caURL, checkAuth)
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
	resp, err := orch.Orchestrate(validRequest())
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
	resp, err := orch.Orchestrate(validRequest())
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
	resp, err := orch.Orchestrate(validRequest())
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
	resp, err := orch.Orchestrate(validRequest())
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
	resp, err := orch.Orchestrate(validRequest())
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
	resp, err := orch.Orchestrate(validRequest())
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
	_, err := orch.Orchestrate(validRequest())
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
	})
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
	})
	if err == nil {
		t.Fatal("expected error for missing serviceDefinition")
	}
}
