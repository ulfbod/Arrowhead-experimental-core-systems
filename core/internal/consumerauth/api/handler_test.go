package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"arrowhead/core/internal/consumerauth/api"
	"arrowhead/core/internal/consumerauth/model"
	"arrowhead/core/internal/consumerauth/repository"
	"arrowhead/core/internal/consumerauth/service"
)

func newTestHandler() http.Handler {
	svc := service.NewAuthService(repository.NewMemoryRepository())
	return api.NewHandler(svc)
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

func deleteReq(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
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

func grantAndGetInstanceID(t *testing.T, h http.Handler, provider, target string) string {
	t.Helper()
	w := postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType":    model.TargetServiceDef,
		"target":        target,
		"provider":      provider,
		"defaultPolicy": map[string]any{"policyType": model.PolicyAll},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("grant failed: %d %s", w.Code, w.Body.String())
	}
	var resp struct{ InstanceID string `json:"instanceId"` }
	json.NewDecoder(w.Body).Decode(&resp)
	return resp.InstanceID
}

// ---- ErrorResponse shape ----

func TestConsumerAuthGrantMissingFieldReturnsExceptionType(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]string{})
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

// ---- Cycle 14.2 — Grant and revoke with instanceId ----

func TestGrantCreatesInstanceID(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType":    "SERVICE_DEF",
		"target":        "temperatureService",
		"defaultPolicy": map[string]any{"policyType": "WHITELIST", "policyList": []string{"ConsumerApp"}},
		"provider":      "TemperatureProvider",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		InstanceID string `json:"instanceId"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.InstanceID == "" {
		t.Error("instanceId is empty")
	}
	want := "PR|LOCAL|TemperatureProvider|SERVICE_DEF|temperatureService"
	if resp.InstanceID != want {
		t.Errorf("instanceId = %q, want %q", resp.InstanceID, want)
	}
}

func TestRevokeByInstanceID(t *testing.T) {
	h := newTestHandler()
	instanceID := grantAndGetInstanceID(t, h, "P1", "svc")

	del := deleteReq(t, h, "/consumerauthorization/authorization/revoke/"+instanceID)
	if del.Code != http.StatusOK {
		t.Errorf("revoke: expected 200, got %d", del.Code)
	}
}

func TestRevokeURLEncodedInstanceID(t *testing.T) {
	h := newTestHandler()
	instanceID := grantAndGetInstanceID(t, h, "P1", "svc")
	encoded := model.EncodeInstanceID(instanceID)

	del := deleteReq(t, h, "/consumerauthorization/authorization/revoke/"+encoded)
	if del.Code != http.StatusOK {
		t.Errorf("revoke with encoded instanceId: expected 200, got %d", del.Code)
	}
}

func TestRevokeNotFound(t *testing.T) {
	h := newTestHandler()
	del := deleteReq(t, h, "/consumerauthorization/authorization/revoke/PR%7CLOCAL%7Cnobody%7CSERVICE_DEF%7Csvc")
	if del.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", del.Code)
	}
}

func TestGrantDuplicateReturns409(t *testing.T) {
	h := newTestHandler()
	grantAndGetInstanceID(t, h, "P1", "svc")
	w := postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType": model.TargetServiceDef, "target": "svc", "provider": "P1",
		"defaultPolicy": map[string]any{"policyType": model.PolicyAll},
	})
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestGrantValidation(t *testing.T) {
	tests := []struct {
		name string
		body map[string]any
	}{
		{"empty provider", map[string]any{"targetType": "SERVICE_DEF", "target": "svc", "provider": "", "defaultPolicy": map[string]any{"policyType": "ALL"}}},
		{"empty target", map[string]any{"targetType": "SERVICE_DEF", "target": "", "provider": "P", "defaultPolicy": map[string]any{"policyType": "ALL"}}},
		{"empty targetType", map[string]any{"targetType": "", "target": "svc", "provider": "P", "defaultPolicy": map[string]any{"policyType": "ALL"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(t, newTestHandler(), "/consumerauthorization/authorization/grant", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestGrantWrongMethod(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/consumerauthorization/authorization/grant", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---- Cycle 14.3 — Lookup with at-least-one-filter validation ----

func TestLookupRequiresAtLeastOneFilter(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization/lookup", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLookupByTargetName(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType": "SERVICE_DEF", "target": "svc-x", "provider": "ProvX",
		"defaultPolicy": map[string]any{"policyType": "ALL"},
	})
	w := postJSON(t, h, "/consumerauthorization/authorization/lookup", map[string]any{
		"targetNames": []string{"svc-x"},
		"targetType":  "SERVICE_DEF",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Policies []map[string]any `json:"policies"`
		Count    int              `json:"count"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("expected 1 policy, got %d", resp.Count)
	}
}

func TestLookupWrongMethod(t *testing.T) {
	h := newTestHandler()
	w := getReq(t, h, "/consumerauthorization/authorization/lookup")
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---- Cycle 14.4 — Verify returns plain JSON Boolean ----

func TestVerifyReturnsTruePlainBoolean(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType": "SERVICE_DEF", "target": "svc", "provider": "P",
		"defaultPolicy": map[string]any{"policyType": "WHITELIST", "policyList": []string{"Consumer1"}},
	})
	w := postJSON(t, h, "/consumerauthorization/authorization/verify", map[string]any{
		"consumer": "Consumer1", "target": "svc", "targetType": "SERVICE_DEF",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := strings.TrimSpace(w.Body.String())
	if body != "true" {
		t.Errorf("body = %q, want plain JSON true", body)
	}
}

func TestVerifyReturnsFalsePlainBoolean(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization/verify", map[string]any{
		"consumer": "Nobody", "target": "svc", "targetType": "SERVICE_DEF",
	})
	body := strings.TrimSpace(w.Body.String())
	if body != "false" {
		t.Errorf("body = %q, want plain JSON false", body)
	}
}

func TestVerifyWrongMethod(t *testing.T) {
	h := newTestHandler()
	w := getReq(t, h, "/consumerauthorization/authorization/verify")
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---- Cycle 14.5 — Management endpoints ----

func TestMgmtGrantAndQuery(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization/mgmt/grant", map[string]any{
		"targetType": "SERVICE_DEF", "target": "mgmt-svc", "provider": "MgmtProv",
		"defaultPolicy": map[string]any{"policyType": "ALL"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("mgmt/grant: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	qw := postJSON(t, h, "/consumerauthorization/authorization/mgmt/query", map[string]any{})
	if qw.Code != http.StatusOK {
		t.Fatalf("mgmt/query: expected 200, got %d", qw.Code)
	}
	var resp struct {
		Policies []map[string]any `json:"policies"`
	}
	json.NewDecoder(qw.Body).Decode(&resp)
	if len(resp.Policies) < 1 {
		t.Error("expected at least 1 policy in query response")
	}
}

func TestMgmtRevokeBulk(t *testing.T) {
	h := newTestHandler()
	id := grantAndGetInstanceID(t, h, "BulkP", "bulk-svc")
	encoded := model.EncodeInstanceID(id)

	req := httptest.NewRequest(http.MethodDelete,
		"/consumerauthorization/authorization/mgmt/revoke?instanceIds="+encoded, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("mgmt/revoke: expected 200, got %d", w.Code)
	}
}

func TestMgmtCheck(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType": "SERVICE_DEF", "target": "svc", "provider": "P",
		"defaultPolicy": map[string]any{"policyType": "ALL"},
	})
	w := postJSON(t, h, "/consumerauthorization/authorization/mgmt/check", []map[string]any{
		{"consumer": "anyone", "target": "svc", "targetType": "SERVICE_DEF"},
		{"consumer": "anyone", "target": "nonexistent", "targetType": "SERVICE_DEF"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("mgmt/check: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var results []bool
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0] {
		t.Error("expected results[0]=true")
	}
	if results[1] {
		t.Error("expected results[1]=false")
	}
}

// ---- Health ----

func TestHandlerHealth(t *testing.T) {
	h := newTestHandler()
	for _, path := range []string{"/health", "/consumerauthorization/authorization/health"} {
		w := getReq(t, h, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, w.Code)
		}
	}
}

// ---- Old path migration ----

func newConsumerAuthTestServer() *httptest.Server {
	svc := service.NewAuthService(repository.NewMemoryRepository())
	return httptest.NewServer(api.NewHandler(svc))
}

func TestConsumerAuthOldPathReturns404(t *testing.T) {
	srv := newConsumerAuthTestServer()
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/authorization/grant", "application/json",
		bytes.NewBufferString(`{"provider":"P","targetType":"SERVICE_DEF","target":"svc","defaultPolicy":{"policyType":"ALL"}}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("old path: expected 404, got %d", resp.StatusCode)
	}
}

func TestConsumerAuthNewPathAcceptsGrant(t *testing.T) {
	srv := newConsumerAuthTestServer()
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/consumerauthorization/authorization/grant", "application/json",
		bytes.NewBufferString(`{"provider":"P","targetType":"SERVICE_DEF","target":"svc","defaultPolicy":{"policyType":"ALL"}}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("new path: expected 201, got %d", resp.StatusCode)
	}
}

// ---- Cycle 15 — authorization-token sub-service ----

func TestGenerateTimeLimitedToken201(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization-token/generate", map[string]any{
		"tokenVariant": "TIME_LIMITED_TOKEN",
		"provider":     "SensorProvider",
		"targetType":   "SERVICE_DEF",
		"target":       "temperatureService",
		"consumer":     "ConsumerApp",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var desc model.TokenDescriptor
	json.NewDecoder(w.Body).Decode(&desc)
	if desc.Token == "" {
		t.Error("token is empty")
	}
	if desc.TokenType != "TIME_LIMITED_TOKEN" {
		t.Errorf("tokenType = %q, want TIME_LIMITED_TOKEN", desc.TokenType)
	}
	if desc.ExpiresAt == "" {
		t.Error("expiresAt is empty")
	}
}

func TestGenerateUnsupportedVariant501(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization-token/generate", map[string]any{
		"tokenVariant": "CERTIFICATE_TOKEN",
		"provider":     "P",
		"targetType":   "SERVICE_DEF",
		"target":       "svc",
	})
	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
}

func TestVerifyValidAuthToken(t *testing.T) {
	h := newTestHandler()
	// Generate a token
	gw := postJSON(t, h, "/consumerauthorization/authorization-token/generate", map[string]any{
		"tokenVariant": "TIME_LIMITED_TOKEN",
		"provider":     "P",
		"targetType":   "SERVICE_DEF",
		"target":       "svc",
		"consumer":     "App",
	})
	if gw.Code != http.StatusCreated {
		t.Fatalf("generate: expected 201, got %d", gw.Code)
	}
	var desc model.TokenDescriptor
	json.NewDecoder(gw.Body).Decode(&desc)

	// Verify the token
	vw := getReq(t, h, "/consumerauthorization/authorization-token/verify/"+desc.Token)
	if vw.Code != http.StatusOK {
		t.Fatalf("verify: expected 200, got %d: %s", vw.Code, vw.Body.String())
	}
	var resp model.TokenVerifyResponse
	json.NewDecoder(vw.Body).Decode(&resp)
	if !resp.Verified {
		t.Error("expected verified=true")
	}
	if resp.Consumer != "App" {
		t.Errorf("consumer = %q, want App", resp.Consumer)
	}
	if resp.Target != "svc" {
		t.Errorf("target = %q, want svc", resp.Target)
	}
}

func TestVerifyUnknownAuthToken404(t *testing.T) {
	h := newTestHandler()
	w := getReq(t, h, "/consumerauthorization/authorization-token/verify/nonexistent-token-xyz")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRegisterEncryptionKey201(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization-token/encryption-key", map[string]any{
		"systemName": "SensorProvider",
		"algorithm":  "RSA",
		"key":        "base64encodedkey==",
	})
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRemoveEncryptionKey200(t *testing.T) {
	h := newTestHandler()
	// Register first
	postJSON(t, h, "/consumerauthorization/authorization-token/encryption-key", map[string]any{
		"systemName": "SensorProvider",
		"algorithm":  "RSA",
		"key":        "key==",
	})
	// Delete
	req := httptest.NewRequest(http.MethodDelete,
		"/consumerauthorization/authorization-token/encryption-key?systemName=SensorProvider", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
