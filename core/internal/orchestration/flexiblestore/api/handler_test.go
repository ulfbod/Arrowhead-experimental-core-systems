package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"arrowhead/core/internal/orchestration/flexiblestore/api"
	"arrowhead/core/internal/orchestration/flexiblestore/model"
	"arrowhead/core/internal/orchestration/flexiblestore/repository"
	"arrowhead/core/internal/orchestration/flexiblestore/service"
	orchmodel "arrowhead/core/internal/orchestration/model"
)

func newTestHandler() http.Handler {
	orch := service.NewFlexibleStoreOrchestrator(repository.NewMemoryRepository())
	return api.NewHandler(orch)
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
	"priority":   1,
}

var validOrchestrateBody = map[string]any{
	"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
	"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
}

func createRuleAndGetID(t *testing.T, h http.Handler) int64 {
	t.Helper()
	w := postJSON(t, h, "/orchestration/flexiblestore/rules", validRuleBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create rule failed: %d %s", w.Code, w.Body.String())
	}
	var rule model.FlexibleRule
	json.NewDecoder(w.Body).Decode(&rule)
	return rule.ID
}

// ---- Rules CRUD ----

func TestHandlerCreateRuleValid(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/orchestration/flexiblestore/rules", validRuleBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var rule model.FlexibleRule
	json.NewDecoder(w.Body).Decode(&rule)
	if rule.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if rule.Priority != 1 {
		t.Errorf("Priority = %d, want 1", rule.Priority)
	}
}

func TestHandlerCreateRuleWithMetadataFilter(t *testing.T) {
	h := newTestHandler()
	body := map[string]any{
		"consumerSystemName": "consumer-app",
		"serviceDefinition":  "temperature-service",
		"provider":           map[string]any{"systemName": "eu-sensor", "address": "10.0.0.1", "port": 9000},
		"serviceUri":         "/temperature",
		"interfaces":         []string{"HTTP"},
		"priority":           1,
		"metadataFilter":     map[string]string{"region": "eu"},
	}
	w := postJSON(t, h, "/orchestration/flexiblestore/rules", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var rule model.FlexibleRule
	json.NewDecoder(w.Body).Decode(&rule)
	if rule.MetadataFilter["region"] != "eu" {
		t.Errorf("MetadataFilter region = %q, want eu", rule.MetadataFilter["region"])
	}
}

func TestHandlerListRulesEmpty(t *testing.T) {
	h := newTestHandler()
	w := getReq(t, h, "/orchestration/flexiblestore/rules")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.RulesResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Rules == nil {
		t.Error("expected non-nil rules slice")
	}
}

func TestHandlerDeleteRuleValid(t *testing.T) {
	h := newTestHandler()
	id := createRuleAndGetID(t, h)
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/orchestration/flexiblestore/rules/%d", id), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandlerDeleteRuleNotFound(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodDelete, "/orchestration/flexiblestore/rules/999", nil)
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
	w := postJSON(t, h, "/orchestration/flexiblestore", validOrchestrateBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Response) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Response))
	}
	if resp.Response[0].Provider.SystemName != "sensor-1" {
		t.Errorf("unexpected provider: %q", resp.Response[0].Provider.SystemName)
	}
}

func TestHandlerOrchestratePriorityOrdering(t *testing.T) {
	h := newTestHandler()

	for _, body := range []map[string]any{
		{"consumerSystemName": "consumer-app", "serviceDefinition": "temperature-service",
			"provider": map[string]any{"systemName": "low", "address": "a", "port": 1},
			"serviceUri": "/t", "interfaces": []string{"HTTP"}, "priority": 10},
		{"consumerSystemName": "consumer-app", "serviceDefinition": "temperature-service",
			"provider": map[string]any{"systemName": "high", "address": "a", "port": 2},
			"serviceUri": "/t", "interfaces": []string{"HTTP"}, "priority": 1},
	} {
		postJSON(t, h, "/orchestration/flexiblestore/rules", body)
	}

	w := postJSON(t, h, "/orchestration/flexiblestore", validOrchestrateBody)
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Response) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Response))
	}
	if resp.Response[0].Provider.SystemName != "high" {
		t.Errorf("expected high-priority provider first, got %q", resp.Response[0].Provider.SystemName)
	}
}

func TestHandlerOrchestrateNoMatch(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/orchestration/flexiblestore", validOrchestrateBody)
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Response) != 0 {
		t.Errorf("expected empty response, got %d", len(resp.Response))
	}
}

// ---- Health ----

func TestHandlerHealth(t *testing.T) {
	h := newTestHandler()
	for _, path := range []string{"/health", "/orchestration/flexiblestore/health"} {
		w := getReq(t, h, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, w.Code)
		}
	}
}
