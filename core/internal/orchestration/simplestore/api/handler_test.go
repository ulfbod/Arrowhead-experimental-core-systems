package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"arrowhead/core/internal/orchestration/simplestore/api"
	"arrowhead/core/internal/orchestration/simplestore/model"
	"arrowhead/core/internal/orchestration/simplestore/repository"
	"arrowhead/core/internal/orchestration/simplestore/service"
	orchmodel "arrowhead/core/internal/orchestration/model"
)

func newTestHandler() http.Handler {
	orch := service.NewSimpleStoreOrchestrator(repository.NewMemoryRepository())
	return api.NewHandler(orch, "")
}

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

// ---- ErrorResponse shape ----

func TestSimpleStoreRulesMissingFieldReturnsExceptionType(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/serviceorchestration/orchestration/simplestore/rules", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var body struct {
		ExceptionType string `json:"exceptionType"`
	}
	json.NewDecoder(w.Body).Decode(&body)
	if body.ExceptionType == "" {
		t.Errorf("exceptionType is empty — response: %s", w.Body.String())
	}
}

var validRuleBody = map[string]any{
	"consumerSystemName": "consumer-app",
	"serviceDefinition":  "temperature-service",
	"provider": map[string]any{
		"systemName": "sensor-1",
		"address":    "10.0.0.1",
		"port":       9000,
	},
	"serviceUri": "/temperature",
	"interfaces": []string{"HTTP-INSECURE-JSON"},
}

var validOrchestrateBody = map[string]any{
	"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
	"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
}

// createRuleAndGetID creates a rule via the legacy path and returns its UUID.
func createRuleAndGetID(t *testing.T, h http.Handler) string {
	t.Helper()
	w := postJSON(t, h, "/serviceorchestration/orchestration/simplestore/rules", validRuleBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create rule failed: %d %s", w.Code, w.Body.String())
	}
	var rule model.StoreRule
	json.NewDecoder(w.Body).Decode(&rule)
	return rule.ID
}

// ---- Legacy rules CRUD ----

func TestHandlerCreateRuleValid(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/serviceorchestration/orchestration/simplestore/rules", validRuleBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var rule model.StoreRule
	json.NewDecoder(w.Body).Decode(&rule)
	if rule.ID == "" {
		t.Error("expected non-empty UUID ID")
	}
	if len(rule.ID) != 36 {
		t.Errorf("id len = %d, want 36 (UUID)", len(rule.ID))
	}
}

func TestHandlerCreateRuleValidation(t *testing.T) {
	tests := []struct {
		name string
		body map[string]any
	}{
		{"missing consumer", map[string]any{"serviceDefinition": "s", "provider": map[string]any{"systemName": "p", "address": "a", "port": 1}, "serviceUri": "/s", "interfaces": []string{"HTTP"}}},
		{"missing service", map[string]any{"consumerSystemName": "c", "provider": map[string]any{"systemName": "p", "address": "a", "port": 1}, "serviceUri": "/s", "interfaces": []string{"HTTP"}}},
		{"missing serviceUri", map[string]any{"consumerSystemName": "c", "serviceDefinition": "s", "provider": map[string]any{"systemName": "p", "address": "a", "port": 1}, "interfaces": []string{"HTTP"}}},
		{"empty interfaces", map[string]any{"consumerSystemName": "c", "serviceDefinition": "s", "provider": map[string]any{"systemName": "p", "address": "a", "port": 1}, "serviceUri": "/s", "interfaces": []string{}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(t, newTestHandler(), "/serviceorchestration/orchestration/simplestore/rules", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestHandlerListRulesEmpty(t *testing.T) {
	h := newTestHandler()
	w := getReq(t, h, "/serviceorchestration/orchestration/simplestore/rules")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.RulesResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Rules == nil {
		t.Error("expected non-nil rules slice")
	}
}

func TestHandlerListRulesWithEntries(t *testing.T) {
	h := newTestHandler()
	createRuleAndGetID(t, h)
	w := getReq(t, h, "/serviceorchestration/orchestration/simplestore/rules")
	var resp model.RulesResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("expected 1, got %d", resp.Count)
	}
}

func TestHandlerDeleteRuleValid(t *testing.T) {
	h := newTestHandler()
	id := createRuleAndGetID(t, h)
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/serviceorchestration/orchestration/simplestore/rules/%s", id), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandlerDeleteRuleNotFound(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodDelete, "/serviceorchestration/orchestration/simplestore/rules/00000000-0000-0000-0000-000000000000", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlerDeleteRuleInvalidID(t *testing.T) {
	h := newTestHandler()
	// "nonexistent-id" is a valid non-empty string but does not exist → 404
	req := httptest.NewRequest(http.MethodDelete, "/serviceorchestration/orchestration/simplestore/rules/nonexistent-id", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---- Orchestrate ----

func TestHandlerOrchestrateMatch(t *testing.T) {
	h := newTestHandler()
	createRuleAndGetID(t, h)
	w := postJSON(t, h, "/serviceorchestration/orchestration/pull", validOrchestrateBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].ProviderName != "sensor-1" {
		t.Errorf("unexpected provider: %q", resp.Results[0].ProviderName)
	}
}

func TestHandlerOrchestrateNoMatch(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/serviceorchestration/orchestration/pull", validOrchestrateBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 0 {
		t.Errorf("expected empty response, got %d", len(resp.Results))
	}
}

func TestHandlerOrchestrateInvalidJSON(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/pull", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerOrchestrateWrongMethod(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/serviceorchestration/orchestration/pull", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---- Health ----

func TestHandlerHealth(t *testing.T) {
	h := newTestHandler()
	for _, path := range []string{"/health", "/serviceorchestration/orchestration/pull/health"} {
		w := getReq(t, h, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, w.Code)
		}
	}
}

// ---- New AH5 mgmt endpoints (Step 18.1) ----

func TestSimpleStoreMgmtCreate(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/simple-store/create", validRuleBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ID == "" {
		t.Error("id is empty — expected UUID")
	}
	if len(resp.ID) != 36 {
		t.Errorf("id len = %d, want 36 (UUID)", len(resp.ID))
	}
}

func TestSimpleStoreMgmtQuery(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/simple-store/create", validRuleBody)
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/simple-store/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count int `json:"count"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("count = 0 after create")
	}
}

func TestSimpleStoreMgmtModifyPriorities(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/simple-store/create", validRuleBody)
	var created model.StoreRule
	json.NewDecoder(w.Body).Decode(&created)

	w2 := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/simple-store/modify-priorities", map[string]any{
		"priorities": map[string]int{created.ID: 5},
	})
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var resp model.RulesResponse
	json.NewDecoder(w2.Body).Decode(&resp)
	if len(resp.Rules) != 1 || resp.Rules[0].Priority != 5 {
		t.Errorf("priority not updated: %+v", resp.Rules)
	}
}

func TestSimpleStoreMgmtCreateValidation(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/simple-store/create", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing fields, got %d", w.Code)
	}
}

// ---- Step 19.1: Subscribe / Unsubscribe ----

var validSSSubscribeBody = map[string]any{
	"ownerSystemName":  "consumer-app",
	"targetSystemName": "consumer-app",
	"orchestrationRequest": map[string]any{
		"requesterSystem":    map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"serviceRequirement": map[string]any{"serviceDefinition": "temperature-service"},
	},
}

func TestSimpleStoreSubscribeReturnsUUID(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSSSubscribeBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ ID string `json:"id"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.ID) != 36 {
		t.Errorf("id = %q, not a UUID", resp.ID)
	}
}

func TestSimpleStoreSubscribeDuplicateReturns200(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSSSubscribeBody)
	w := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSSSubscribeBody)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on duplicate subscribe, got %d", w.Code)
	}
}

func TestSimpleStoreUnsubscribeFound200(t *testing.T) {
	h := newTestHandler()
	sw := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSSSubscribeBody)
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(sw.Body).Decode(&sub)

	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/unsubscribe/"+sub.ID, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on found unsubscribe, got %d", w.Code)
	}
}

func TestSimpleStoreUnsubscribeNotFound204(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/unsubscribe/no-such-id", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

// ─── Step B: Tests for Steps 24 + 25 ─────────────────────────────────────────

func TestOrchestrationResultForwardsInterfaces(t *testing.T) {
	h := newTestHandler()
	// Create a rule with interfaces
	postJSON(t, h, "/serviceorchestration/orchestration/simplestore/rules", validRuleBody)
	w := postJSON(t, h, "/serviceorchestration/orchestration/pull", validOrchestrateBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if len(resp.Results[0].Interfaces) == 0 {
		t.Errorf("Interfaces is empty, expected rule interfaces to be forwarded")
	}
}

func TestAllowInterclouReturns501(t *testing.T) {
	h := newTestHandler()
	body := map[string]any{
		"requesterSystem":    map[string]any{"systemName": "c", "address": "a", "port": 1},
		"requestedService":   map[string]any{"serviceDefinition": "s"},
		"orchestrationFlags": map[string]any{"ALLOW_INTERCLOUD": true},
	}
	w := postJSON(t, h, "/serviceorchestration/orchestration/pull", body)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOnlyInterclouReturns501(t *testing.T) {
	h := newTestHandler()
	body := map[string]any{
		"requesterSystem":    map[string]any{"systemName": "c", "address": "a", "port": 1},
		"requestedService":   map[string]any{"serviceDefinition": "s"},
		"orchestrationFlags": map[string]any{"ONLY_INTERCLOUD": true},
	}
	w := postJSON(t, h, "/serviceorchestration/orchestration/pull", body)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", w.Code, w.Body.String())
	}
}
