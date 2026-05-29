package orchestration

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSRClient_QuerySR_OK verifies that srClient.QuerySR parses a valid SR response.
func TestSRClient_QuerySR_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/serviceregistry/query" {
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(srQueryResponse{
			ServiceQueryData: []srServiceInstance{
				{
					ServiceDefinition: "telemetry",
					ProviderSystem:    srSystem{SystemName: "prov1", Address: "10.0.0.1", Port: 9000},
					ServiceUri:        "/tel",
					Interfaces:        []string{"HTTP-SECURE-JSON"},
					Version:           3,
					Metadata:          map[string]string{"zone": "B"},
				},
			},
			UnfilteredHits: 1,
		})
	}))
	defer srv.Close()

	c := NewSRClient(srv.URL)
	instances, err := c.QuerySR(ServiceFilter{ServiceDefinition: "telemetry"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	inst := instances[0]
	if inst.Provider.SystemName != "prov1" {
		t.Errorf("provider: got %q", inst.Provider.SystemName)
	}
	if inst.Version != 3 {
		t.Errorf("version: got %d", inst.Version)
	}
	if inst.Metadata["zone"] != "B" {
		t.Errorf("metadata: got %v", inst.Metadata)
	}
}

// TestSRClient_QuerySR_NetworkError verifies that a connection failure returns an error.
func TestSRClient_QuerySR_NetworkError(t *testing.T) {
	c := NewSRClient("http://127.0.0.1:1") // nothing listening
	_, err := c.QuerySR(ServiceFilter{ServiceDefinition: "x"})
	if err == nil {
		t.Fatal("expected error on network failure")
	}
}

// TestSRClient_QuerySR_BadJSON verifies that malformed JSON returns an error.
func TestSRClient_QuerySR_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{not valid json")
	}))
	defer srv.Close()

	c := NewSRClient(srv.URL)
	_, err := c.QuerySR(ServiceFilter{ServiceDefinition: "x"})
	if err == nil {
		t.Fatal("expected error on bad JSON")
	}
}

// TestSRClient_QuerySR_EmptyResponse verifies that an empty result list is handled.
func TestSRClient_QuerySR_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(srQueryResponse{ServiceQueryData: []srServiceInstance{}})
	}))
	defer srv.Close()

	c := NewSRClient(srv.URL)
	instances, err := c.QuerySR(ServiceFilter{ServiceDefinition: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instances) != 0 {
		t.Errorf("expected empty, got %d", len(instances))
	}
}

// TestHandler_InternalError_Returns500 covers the non-validation error branch.
type errOrchestrator struct{}

func (e *errOrchestrator) Orchestrate(_ OrchestrationRequest) (OrchestrationResponse, error) {
	return OrchestrationResponse{}, errors.New("registry unreachable")
}

func TestHandler_InternalError_Returns500(t *testing.T) {
	h := NewHandler(&errOrchestrator{})
	body := `{"requesterSystem":{"systemName":"c1"},"requestedService":{"serviceDefinition":"svc"}}`
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/pull", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d want 500", w.Code)
	}
}
