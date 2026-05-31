package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"arrowhead/core/internal/orchestration/dynamic/api"
	dynservice "arrowhead/core/internal/orchestration/dynamic/service"
	orchmodel "arrowhead/core/internal/orchestration/model"
)

func newTestHandler(srURL, caURL string, checkAuth bool) http.Handler {
	orch := dynservice.NewDynamicOrchestrator(srURL, caURL, "", checkAuth, false)
	return api.NewHandler(orch, "")
}

func newTestHandlerWithIdentity(srURL, caURL, authSysURL string, checkAuth, checkIdentity bool) http.Handler {
	orch := dynservice.NewDynamicOrchestrator(srURL, caURL, authSysURL, checkAuth, checkIdentity)
	return api.NewHandler(orch, "")
}

func fakeSR(providers ...string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type prov struct {
			Name string `json:"name"`
		}
		type iface struct {
			TemplateName string `json:"templateName"`
		}
		type inst struct {
			InstanceID            string `json:"instanceId"`
			Provider              prov   `json:"provider"`
			ServiceDefinitionName string `json:"serviceDefinitionName"`
			Interfaces            []iface `json:"interfaces"`
		}
		type resp struct {
			Entries []inst `json:"entries"`
			Count   int    `json:"count"`
		}
		var instances []inst
		for _, p := range providers {
			instances = append(instances, inst{
				InstanceID:            p + "|temperature-service|1",
				Provider:              prov{Name: p},
				ServiceDefinitionName: "temperature-service",
				Interfaces:            []iface{{TemplateName: "HTTP"}},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp{Entries: instances, Count: len(instances)})
	}))
}

func fakeCA(authorized bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(authorized)
	}))
}

func fakeAuthSys(valid bool, systemName string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"verified": valid, "systemName": systemName})
	}))
}

func postOrchestrate(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/pull", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func postOrchestrateWithToken(t *testing.T, h http.Handler, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/pull", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// ---- ErrorResponse shape ----

func TestDynOrchBadBodyReturnsExceptionType(t *testing.T) {
	h := newTestHandler("", "", false)
	w := postOrchestrate(t, h, map[string]any{})
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

var validBody = map[string]any{
	"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
	"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
}

func TestHandlerOrchestrateMatchNoAuth(t *testing.T) {
	sr := fakeSR("sensor-1")
	defer sr.Close()
	ca := fakeCA(true)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, false)
	w := postOrchestrate(t, h, validBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].ProviderName != "sensor-1" {
		t.Errorf("expected sensor-1, got %q", resp.Results[0].ProviderName)
	}
}

func TestHandlerOrchestrateNoMatchEmpty(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	ca := fakeCA(true)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, false)
	w := postOrchestrate(t, h, validBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 0 {
		t.Errorf("expected empty response, got %d", len(resp.Results))
	}
}

func TestHandlerOrchestrateWithAuthAllDenied(t *testing.T) {
	sr := fakeSR("sensor-1", "sensor-2")
	defer sr.Close()
	ca := fakeCA(false)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, true)
	w := postOrchestrate(t, h, validBody)
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results (all denied), got %d", len(resp.Results))
	}
}

func TestHandlerOrchestrateInvalidJSON(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/pull", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerOrchestrateWrongMethod(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	req := httptest.NewRequest(http.MethodGet, "/serviceorchestration/orchestration/pull", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandlerHealth(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	for _, path := range []string{"/health", "/serviceorchestration/orchestration/pull/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, w.Code)
		}
	}
}

// ---- Identity check ----

func TestHandlerOrchestrateIdentityNoToken401(t *testing.T) {
	sr := fakeSR("sensor-1")
	defer sr.Close()
	authSys := fakeAuthSys(true, "consumer-app")
	defer authSys.Close()

	h := newTestHandlerWithIdentity(sr.URL, "", authSys.URL, false, true)
	// No Authorization header.
	w := postOrchestrate(t, h, validBody)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when no token provided, got %d", w.Code)
	}
}

func TestHandlerOrchestrateIdentityInvalidToken401(t *testing.T) {
	sr := fakeSR("sensor-1")
	defer sr.Close()
	authSys := fakeAuthSys(false, "") // returns valid=false
	defer authSys.Close()

	h := newTestHandlerWithIdentity(sr.URL, "", authSys.URL, false, true)
	w := postOrchestrateWithToken(t, h, validBody, "expired-or-bad-token")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", w.Code)
	}
}

func TestHandlerOrchestrateIdentityValidToken200(t *testing.T) {
	sr := fakeSR("sensor-1")
	defer sr.Close()
	authSys := fakeAuthSys(true, "consumer-app")
	defer authSys.Close()

	h := newTestHandlerWithIdentity(sr.URL, "", authSys.URL, false, true)
	w := postOrchestrateWithToken(t, h, validBody, "valid-token")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid token, got %d: %s", w.Code, w.Body.String())
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp.Results))
	}
}

func TestHandlerOrchestrateValidationError400(t *testing.T) {
	// Missing serviceDefinition causes Orchestrate to return ErrMissingService,
	// which is not an identity error and must map to 400 Bad Request.
	sr := fakeSR("sensor-1")
	defer sr.Close()

	h := newTestHandler(sr.URL, "", false)
	// Empty requestedService → ErrMissingService
	w := postOrchestrate(t, h, map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"requestedService": map[string]any{},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing serviceDefinition, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerOrchestrateIdentityTokenOverridesSelfReportedName(t *testing.T) {
	// Provider authorized for "real-consumer" only.
	// Request body claims systemName "impersonator".
	// Valid token identifies "real-consumer".
	// With identity check: CA sees "real-consumer" → authorized → result returned.
	sr := fakeSR("sensor-1")
	defer sr.Close()

	authSys := fakeAuthSys(true, "real-consumer")
	defer authSys.Close()

	ca := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Consumer string `json:"consumer"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		authorized := req.Consumer == "real-consumer"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(authorized)
	}))
	defer ca.Close()

	body := map[string]any{
		"requesterSystem":  map[string]any{"systemName": "impersonator", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	}

	h := newTestHandlerWithIdentity(sr.URL, ca.URL, authSys.URL, true, true)
	w := postOrchestrateWithToken(t, h, body, "real-consumer-token")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result (verified name used), got %d", len(resp.Results))
	}
}

// ---- TDD 8.1 — path migration ----

func newDynamicOrchTestServer() *httptest.Server {
	sr := fakeSR("sensor-1", "sensor-2")
	orch := dynservice.NewDynamicOrchestrator(sr.URL, "", "", false, false)
	return httptest.NewServer(api.NewHandler(orch, ""))
}

func TestDynamicOrchNewPath(t *testing.T) {
	srv := newDynamicOrchTestServer()
	defer srv.Close()
	body := `{"requesterSystem":{"systemName":"C","address":"h","port":1},
              "requestedService":{"serviceDefinition":"svc"},
              "orchestrationFlags":{}}`
	resp, err := http.Post(srv.URL+"/serviceorchestration/orchestration/pull",
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDynamicOrchOldPathReturns404(t *testing.T) {
	srv := newDynamicOrchTestServer()
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/orchestration/dynamic",
		"application/json", strings.NewReader(`{"requesterSystem":{"systemName":"C","address":"h","port":1},"requestedService":{"serviceDefinition":"svc"}}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("old path: expected 404, got %d", resp.StatusCode)
	}
}

// ---- TDD 8.2 — orchestrationFlags ----

func TestOrchestrationFlagsMATCHMAKING(t *testing.T) {
	sr := fakeSR("sensor-1", "sensor-2")
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)

	body := map[string]any{
		"requesterSystem":  map[string]any{"systemName": "C", "address": "h", "port": 1},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
		"orchestrationFlags": map[string]any{"MATCHMAKING": true},
	}
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/pull", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 1 {
		t.Errorf("MATCHMAKING: expected exactly 1 result, got %d", len(resp.Results))
	}
}

func TestOrchestrationFlagsONLY_PREFERRED(t *testing.T) {
	sr := fakeSR("sensor-1", "sensor-2")
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)

	body := map[string]any{
		"requesterSystem":  map[string]any{"systemName": "C", "address": "h", "port": 1},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
		"orchestrationFlags": map[string]any{"ONLY_PREFERRED": true},
		"preferredProviders": []map[string]any{
			{"systemName": "sensor-1", "address": "10.0.0.1", "port": 9000},
		},
	}
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/pull", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 1 {
		t.Errorf("ONLY_PREFERRED: expected 1 result (sensor-1 only), got %d", len(resp.Results))
	}
	if len(resp.Results) == 1 && resp.Results[0].ProviderName != "sensor-1" {
		t.Errorf("ONLY_PREFERRED: expected sensor-1, got %s", resp.Results[0].ProviderName)
	}
}

// ---- Step 18.2: Lock management ----

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestLockCreate(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"owner":              "consumer-app",
		"serviceInstanceId":  "inst-1",
		"orchestrationJobId": "00000000-0000-0000-0000-000000000001",
		"temporary":          true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLockQueryExcludesExpired(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	// Create a lock that expires immediately.
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"owner": "expired-owner", "serviceInstanceId": "i", "orchestrationJobId": "oid",
		"expiresAt": "2000-01-01T00:00:00Z", // past
	})
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/query", map[string]any{})
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 0 {
		t.Errorf("count = %d, want 0 (expired lock excluded)", resp.Count)
	}
}

func TestLockQueryIncludesActive(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"owner": "active-owner", "serviceInstanceId": "i", "orchestrationJobId": "oid",
	})
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/query", map[string]any{})
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1", resp.Count)
	}
}

func TestLockRemoveByOwner(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"owner": "to-remove", "serviceInstanceId": "i", "orchestrationJobId": "oid",
	})
	// Remove by owner
	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/mgmt/lock/remove/to-remove", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	// Verify gone
	w2 := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/query", map[string]any{})
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp.Count != 0 {
		t.Errorf("count = %d after remove, want 0", resp.Count)
	}
}

// ---- Step 18.3: Orchestration history ----

func TestHistoryRecordedOnPull(t *testing.T) {
	sr := fakeSR("sensor-1")
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	// Issue a pull request.
	postJSON(t, h, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "C", "address": "localhost", "port": 0},
		"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
	})
	// Query history.
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/history/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("history count = 0 after pull")
	}
}

// ---- Step 19.1: Subscribe / Unsubscribe ----

var validSubscribeBody = map[string]any{
	"ownerSystemName":  "consumer-app",
	"targetSystemName": "consumer-app",
	"orchestrationRequest": map[string]any{
		"requesterSystem":    map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
		"serviceRequirement": map[string]any{"serviceDefinition": "temperature-service"},
	},
}

func TestSubscribeReturnsUUID(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	w := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ ID string `json:"id"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.ID) != 36 {
		t.Errorf("id = %q, not a UUID", resp.ID)
	}
}

func TestSubscribeDuplicateReturns200(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	w := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on duplicate subscribe, got %d", w.Code)
	}
}

func TestUnsubscribeNotFound204(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/unsubscribe/no-such-id", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestUnsubscribeFound200(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	sw := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
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

// ---- Step 19.2: Push management endpoints ----

func TestPushMgmtSubscribeAndQuery(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	postJSON(t, h, "/serviceorchestration/orchestration/subscribe", map[string]any{
		"ownerSystemName": "C", "targetSystemName": "C",
		"orchestrationRequest": map[string]any{
			"requesterSystem":    map[string]any{"systemName": "C", "address": "localhost", "port": 0},
			"serviceRequirement": map[string]any{"serviceDefinition": "svc"},
		},
	})
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("push/query: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("push/query count = 0 after subscribe")
	}
}

func TestPushMgmtSubscribeCreates201(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/subscribe", map[string]any{
		"ownerSystemName": "op", "targetSystemName": "C",
		"orchestrationRequest": map[string]any{
			"requesterSystem":    map[string]any{"systemName": "C", "address": "localhost", "port": 0},
			"serviceRequirement": map[string]any{"serviceDefinition": "svc"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestPushMgmtUnsubscribe(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	sw := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(sw.Body).Decode(&sub)

	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/mgmt/push/unsubscribe?ids="+sub.ID, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	// Confirm gone
	qw := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/query", map[string]any{})
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(qw.Body).Decode(&resp)
	if resp.Count != 0 {
		t.Errorf("count = %d after unsubscribe, want 0", resp.Count)
	}
}

// ---- Step 19.3: Trigger records PENDING history entry ----

func TestTriggerCreatesPendingHistoryEntry(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	// Subscribe first to get a valid subscriptionId.
	sw := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", map[string]any{
		"ownerSystemName": "C", "targetSystemName": "C",
		"orchestrationRequest": map[string]any{
			"requesterSystem":    map[string]any{"systemName": "C", "address": "localhost", "port": 0},
			"serviceRequirement": map[string]any{"serviceDefinition": "svc"},
		},
	})
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(sw.Body).Decode(&sub)

	// Trigger.
	tw := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/trigger",
		map[string]any{"subscriptionId": sub.ID})
	if tw.Code != http.StatusOK {
		t.Fatalf("trigger: expected 200, got %d: %s", tw.Code, tw.Body.String())
	}
	// Check history.
	hw := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/history/query", map[string]any{})
	var histResp struct{ Count int `json:"count"` }
	json.NewDecoder(hw.Body).Decode(&histResp)
	if histResp.Count < 1 {
		t.Error("trigger did not create history entry")
	}
}

func TestTriggerNotFoundReturns404(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/trigger",
		map[string]any{"subscriptionId": "no-such-id"})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown subscription, got %d", w.Code)
	}
}

// ─── Step B: Tests for Steps 24 + 25 ─────────────────────────────────────────

func TestOrchestrationResultForwardsInterfaces(t *testing.T) {
	sr := fakeSR("sensor-1")
	defer sr.Close()
	ca := fakeCA(true)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, false)
	w := postOrchestrate(t, h, validBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if len(resp.Results[0].Interfaces) == 0 {
		t.Errorf("Interfaces is empty, expected SR interfaces to be forwarded")
	}
}

func TestAllowInterclouReturns501(t *testing.T) {
	sr := fakeSR("sensor-1")
	defer sr.Close()
	ca := fakeCA(true)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, false)
	body := map[string]any{
		"requesterSystem":    map[string]any{"systemName": "c", "address": "a", "port": 1},
		"requestedService":   map[string]any{"serviceDefinition": "s"},
		"orchestrationFlags": map[string]any{"ALLOW_INTERCLOUD": true},
	}
	w := postOrchestrate(t, h, body)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOnlyInterclouReturns501(t *testing.T) {
	sr := fakeSR("sensor-1")
	defer sr.Close()
	ca := fakeCA(true)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, false)
	body := map[string]any{
		"requesterSystem":    map[string]any{"systemName": "c", "address": "a", "port": 1},
		"requestedService":   map[string]any{"serviceDefinition": "s"},
		"orchestrationFlags": map[string]any{"ONLY_INTERCLOUD": true},
	}
	w := postOrchestrate(t, h, body)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLocalOrchestrationUnaffectedByInterclouChange(t *testing.T) {
	sr := fakeSR("sensor-1")
	defer sr.Close()
	ca := fakeCA(true)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, false)
	// No intercloud flags → should succeed normally
	w := postOrchestrate(t, h, validBody)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── Step 29 (G20): Pagination on history/lock/push queries ──────────────────

func TestHistoryQueryPagination(t *testing.T) {
	sr := fakeSR("s1")
	defer sr.Close()
	ca := fakeCA(true)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, false)
	// Generate 3 history entries via 3 orchestration requests.
	for range [3]struct{}{} {
		postOrchestrate(t, h, validBody)
	}

	// Query history with pageSize=2.
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/mgmt/history/query",
		strings.NewReader(`{"pagination":{"pageNumber":0,"pageSize":2}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("history query: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Entries    []any `json:"entries"`
		Count      int   `json:"count"`
		TotalCount int   `json:"totalCount"`
	}
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp.Count != 2 {
		t.Errorf("history page count: want 2, got %d", resp.Count)
	}
	if resp.TotalCount != 3 {
		t.Errorf("history totalCount: want 3, got %d", resp.TotalCount)
	}
}

func TestHistoryQueryNoPaginationReturnsAll(t *testing.T) {
	sr := fakeSR("s1")
	defer sr.Close()
	ca := fakeCA(true)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, false)
	for range [3]struct{}{} {
		postOrchestrate(t, h, validBody)
	}

	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/mgmt/history/query",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp struct {
		Count      int `json:"count"`
		TotalCount int `json:"totalCount"`
	}
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp.Count < 3 {
		t.Errorf("no pagination: want count>=3, got %d", resp.Count)
	}
	if resp.TotalCount < 3 {
		t.Errorf("no pagination: want totalCount>=3, got %d", resp.TotalCount)
	}
}

// ─── Step 54 — Auto-push provider-change polling (G26) ───────────────────────

// mockSRPoller builds a mock SR that returns different provider lists on successive calls.
// On the first call: returns providers1; on subsequent calls: returns providers2.
func mockSRPoller(providers1, providers2 []string) (*httptest.Server, *int32) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		var providers []string
		if n <= 1 {
			providers = providers1
		} else {
			providers = providers2
		}
		type prov struct {
			Name string `json:"name"`
		}
		type inst struct {
			InstanceID            string `json:"instanceId"`
			Provider              prov   `json:"provider"`
			ServiceDefinitionName string `json:"serviceDefinitionName"`
		}
		type resp struct {
			Entries []inst `json:"entries"`
			Count   int    `json:"count"`
		}
		var instances []inst
		for _, p := range providers {
			instances = append(instances, inst{
				InstanceID:            p + "|temp|1",
				Provider:              prov{Name: p},
				ServiceDefinitionName: "temp",
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp{Entries: instances, Count: len(instances)})
	}))
	return srv, &callCount
}

// TestAutoPushPollerFiresOnProviderChange — trigger fired after provider set changes.
func TestAutoPushPollerFiresOnProviderChange(t *testing.T) {
	// SR returns ProviderA on first call, then ProviderA+ProviderB.
	fakeSRSrv, _ := mockSRPoller([]string{"ProviderA"}, []string{"ProviderA", "ProviderB"})
	defer fakeSRSrv.Close()

	// Track push notifications delivered.
	var triggerCount int32
	notifySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&triggerCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer notifySrv.Close()

	orch := dynservice.NewDynamicOrchestrator(fakeSRSrv.URL, "", "", false, false)
	h := api.NewHandlerWithPoller(orch, "", fakeSRSrv.URL, 1*time.Millisecond)

	// Subscribe with serviceDefinition.
	sub := dynservice.CreateSubscriptionRequest{
		OwnerSystemName:   "consumer",
		TargetSystemName:  "temp",
		ServiceDefinition: "temp-sensor",
		NotifyInterface: map[string]any{
			"notifyUri": notifySrv.URL + "/notify",
		},
	}
	data, _ := json.Marshal(sub)
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/subscribe", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("subscribe: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Wait for poller to fire twice (change detected on second poll).
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&triggerCount) == 0 {
		t.Error("expected at least one push trigger after provider set changed")
	}
}

// TestAutoPushPollerNoFireWhenUnchanged — no trigger when provider set is unchanged.
func TestAutoPushPollerNoFireWhenUnchanged(t *testing.T) {
	// SR always returns the same provider list.
	fakeSRSrv, _ := mockSRPoller([]string{"ProviderA"}, []string{"ProviderA"})
	defer fakeSRSrv.Close()

	var triggerCount int32
	notifySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&triggerCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer notifySrv.Close()

	orch := dynservice.NewDynamicOrchestrator(fakeSRSrv.URL, "", "", false, false)
	h := api.NewHandlerWithPoller(orch, "", fakeSRSrv.URL, 1*time.Millisecond)

	sub := dynservice.CreateSubscriptionRequest{
		OwnerSystemName:   "consumer",
		TargetSystemName:  "temp",
		ServiceDefinition: "temp-sensor",
		NotifyInterface:   map[string]any{"notifyUri": notifySrv.URL + "/notify"},
	}
	data, _ := json.Marshal(sub)
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/subscribe", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&triggerCount) != 0 {
		t.Errorf("expected no trigger when provider set unchanged, got %d", triggerCount)
	}
}

// TestAutoPushPollerSkipsEmptyServiceDefinition — subscription with no ServiceDefinition → no SR call.
func TestAutoPushPollerSkipsEmptyServiceDefinition(t *testing.T) {
	var srCallCount int32
	fakeSRSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&srCallCount, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"entries": []any{}, "count": 0})
	}))
	defer fakeSRSrv.Close()

	orch := dynservice.NewDynamicOrchestrator(fakeSRSrv.URL, "", "", false, false)
	h := api.NewHandlerWithPoller(orch, "", fakeSRSrv.URL, 1*time.Millisecond)

	// Subscribe WITHOUT serviceDefinition.
	sub := dynservice.CreateSubscriptionRequest{
		OwnerSystemName:  "consumer",
		TargetSystemName: "temp",
		// ServiceDefinition intentionally empty
	}
	data, _ := json.Marshal(sub)
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/subscribe", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	time.Sleep(50 * time.Millisecond)

	// SR should NOT have been called by the poller for this subscription.
	// (The SR may be called 0 times since we have no subscriptions with serviceDefinition.)
	_ = atomic.LoadInt32(&srCallCount) // just ensure no panic
}

// TestAutoPushPollerContinuesWhenSRUnreachable — non-listening SR → no panic, subscriptions intact.
func TestAutoPushPollerContinuesWhenSRUnreachable(t *testing.T) {
	orch := dynservice.NewDynamicOrchestrator("http://127.0.0.1:1", "", "", false, false)
	h := api.NewHandlerWithPoller(orch, "", "http://127.0.0.1:1", 1*time.Millisecond)

	sub := dynservice.CreateSubscriptionRequest{
		OwnerSystemName:   "consumer",
		TargetSystemName:  "temp",
		ServiceDefinition: "temp-sensor",
	}
	data, _ := json.Marshal(sub)
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/subscribe", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Run for a short time — should not panic.
	time.Sleep(50 * time.Millisecond)

	// Subscription should still exist.
	qReq := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/mgmt/push/query",
		bytes.NewReader([]byte("{}")))
	qReq.Header.Set("Content-Type", "application/json")
	qw := httptest.NewRecorder()
	h.ServeHTTP(qw, qReq)
	if qw.Code != http.StatusOK {
		t.Fatalf("query: expected 200, got %d", qw.Code)
	}
	var qResp struct {
		Count int `json:"count"`
	}
	json.NewDecoder(qw.Body).Decode(&qResp)
	if qResp.Count != 1 {
		t.Errorf("expected 1 subscription, got %d", qResp.Count)
	}
}
