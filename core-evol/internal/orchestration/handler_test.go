package orchestration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubOrchestrator wraps a fixed response for handler tests.
type stubOrchestrator struct {
	resp OrchestrationResponse
	err  error
}

func (s *stubOrchestrator) Orchestrate(_ OrchestrationRequest) (OrchestrationResponse, error) {
	return s.resp, s.err
}

func TestHandler_ValidRequest_Returns200(t *testing.T) {
	stub := &stubOrchestrator{
		resp: OrchestrationResponse{
			Response: []OrchestrationResult{
				{
					Provider: System{SystemName: "prov", Address: "10.0.0.1", Port: 9000},
					Service:  ServiceInfo{ServiceDefinition: "telemetry", ServiceUri: "/t", Interfaces: []string{"HTTP-SECURE-JSON"}, Version: 1},
				},
			},
		},
	}
	h := NewHandler(stub)
	body := `{"requesterSystem":{"systemName":"c1","address":"1.2.3.4","port":8000},"requestedService":{"serviceDefinition":"telemetry"}}`
	req := httptest.NewRequest(http.MethodPost, "/orchestration/dynamic", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d want 200", w.Code)
	}
	var resp OrchestrationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Response) != 1 {
		t.Errorf("results: got %d want 1", len(resp.Response))
	}
}

func TestHandler_DenyReturnsEmptyResults(t *testing.T) {
	stub := &stubOrchestrator{
		resp: OrchestrationResponse{Response: []OrchestrationResult{}},
	}
	h := NewHandler(stub)
	body := `{"requesterSystem":{"systemName":"c1"},"requestedService":{"serviceDefinition":"telemetry"}}`
	req := httptest.NewRequest(http.MethodPost, "/orchestration/dynamic", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d want 200", w.Code)
	}
	var resp OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Response) != 0 {
		t.Errorf("expected empty response on deny")
	}
}

func TestHandler_InvalidJSON_Returns400(t *testing.T) {
	stub := &stubOrchestrator{}
	h := NewHandler(stub)
	req := httptest.NewRequest(http.MethodPost, "/orchestration/dynamic", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", w.Code)
	}
}

func TestHandler_MissingRequester_Returns400(t *testing.T) {
	stub := &stubOrchestrator{err: ErrMissingRequester}
	h := NewHandler(stub)
	body := `{"requesterSystem":{"systemName":""},"requestedService":{"serviceDefinition":"telemetry"}}`
	req := httptest.NewRequest(http.MethodPost, "/orchestration/dynamic", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", w.Code)
	}
}

func TestHandler_MissingService_Returns400(t *testing.T) {
	stub := &stubOrchestrator{err: ErrMissingService}
	h := NewHandler(stub)
	body := `{"requesterSystem":{"systemName":"c1"},"requestedService":{"serviceDefinition":""}}`
	req := httptest.NewRequest(http.MethodPost, "/orchestration/dynamic", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", w.Code)
	}
}

func TestHandler_WrongMethod_Returns405(t *testing.T) {
	stub := &stubOrchestrator{}
	h := NewHandler(stub)
	req := httptest.NewRequest(http.MethodGet, "/orchestration/dynamic", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d want 405", w.Code)
	}
}

func TestStatusHandler_Returns200(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, &stubOrchestrator{}, "test-domain", true)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "UP" {
		t.Errorf("status field: got %v", body["status"])
	}
}
