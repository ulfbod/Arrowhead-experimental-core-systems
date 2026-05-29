package orchestration

import (
	"bytes"
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

// ---- helpers ----------------------------------------------------------------

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func getReq(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// ---- Existing pull-orchestration tests (paths updated to AH5) ---------------

const pullPath = "/serviceorchestration/orchestration/pull"

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
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader(body))
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
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader(body))
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
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader("{bad json"))
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
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader(body))
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
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader(body))
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
	req := httptest.NewRequest(http.MethodGet, pullPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d want 405", w.Code)
	}
}

// ---- Health -----------------------------------------------------------------

func TestHealthEndpoints(t *testing.T) {
	h := NewHandler(&stubOrchestrator{})
	for _, path := range []string{"/health", pullPath + "/health"} {
		w := getReq(t, h, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s: got %d want 200", path, w.Code)
		}
	}
}

// ---- Status (backward compat) -----------------------------------------------

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

// ---- Step 18: Lock management -----------------------------------------------

func TestLockCreate(t *testing.T) {
	h := NewHandler(&stubOrchestrator{})
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"orchestrationJobId": "job-1",
		"serviceInstanceId":  "svc-1",
		"owner":              "sys-a",
		"temporary":          true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var lock Lock
	json.NewDecoder(w.Body).Decode(&lock)
	if lock.ID == 0 {
		t.Error("expected non-zero lock ID")
	}
}

func TestLockQuery(t *testing.T) {
	h := NewHandler(&stubOrchestrator{})
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"owner": "sys-a",
	})
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp LockQueryResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("expected at least 1 lock")
	}
}

func TestLockRemoveByOwner(t *testing.T) {
	h := NewHandler(&stubOrchestrator{})
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"owner": "owner-x",
	})
	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/mgmt/lock/remove/owner-x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	// Query should now be empty.
	w2 := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/query", map[string]any{})
	var resp LockQueryResponse
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp.Count != 0 {
		t.Errorf("expected 0 locks after remove, got %d", resp.Count)
	}
}

func TestHistoryRecordedOnPull(t *testing.T) {
	h := NewHandler(&stubOrchestrator{
		resp: OrchestrationResponse{Response: []OrchestrationResult{}},
	})
	body := `{"requesterSystem":{"systemName":"c1"},"requestedService":{"serviceDefinition":"svc1"}}`
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	httptest.NewRecorder() // discard response
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	w2 := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/history/query", map[string]any{})
	var resp HistoryQueryResponse
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("expected history entry after pull orchestration")
	}
}

// ---- Step 19: Subscribe / unsubscribe ---------------------------------------

var validSubscribeBody = map[string]any{
	"ownerSystemName":  "consumer-app",
	"targetSystemName": "consumer-app",
	"orchestrationRequest": map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app"},
		"requestedService": map[string]any{"serviceDefinition": "svc"},
	},
}

func TestSubscribeReturns201(t *testing.T) {
	h := NewHandler(&stubOrchestrator{})
	w := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(w.Body).Decode(&sub)
	if len(sub.ID) != 36 {
		t.Errorf("id = %q, not a UUID", sub.ID)
	}
}

func TestSubscribeDuplicateReturns200(t *testing.T) {
	h := NewHandler(&stubOrchestrator{})
	postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	w := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on duplicate, got %d", w.Code)
	}
}

func TestUnsubscribeFound200(t *testing.T) {
	h := NewHandler(&stubOrchestrator{})
	sw := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(sw.Body).Decode(&sub)

	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/unsubscribe/"+sub.ID, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUnsubscribeNotFound204(t *testing.T) {
	h := NewHandler(&stubOrchestrator{})
	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/unsubscribe/no-such-id", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestPushMgmtSubscribeAndQuery(t *testing.T) {
	h := NewHandler(&stubOrchestrator{})
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/subscribe", validSubscribeBody)
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp SubscriptionQueryResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("expected at least 1 subscription")
	}
}

func TestTriggerCreatesPendingHistory(t *testing.T) {
	h := NewHandler(&stubOrchestrator{})
	sw := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(sw.Body).Decode(&sub)

	tw := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/trigger",
		map[string]any{"subscriptionId": sub.ID})
	if tw.Code != http.StatusOK {
		t.Fatalf("trigger: expected 200, got %d: %s", tw.Code, tw.Body.String())
	}

	hw := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/history/query", map[string]any{})
	var hist HistoryQueryResponse
	json.NewDecoder(hw.Body).Decode(&hist)
	found := false
	for _, e := range hist.Entries {
		if e.Status == "PENDING" && e.Type == "PUSH" {
			found = true
		}
	}
	if !found {
		t.Errorf("no PENDING PUSH history entry found: %+v", hist.Entries)
	}
}

func TestTriggerNotFoundReturns404(t *testing.T) {
	h := NewHandler(&stubOrchestrator{})
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/trigger",
		map[string]any{"subscriptionId": "no-such-id"})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
