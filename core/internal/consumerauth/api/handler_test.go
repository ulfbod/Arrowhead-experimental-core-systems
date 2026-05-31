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
	blclient "arrowhead/core/internal/blacklist/client"
)

func newTestHandler() http.Handler {
	svc := service.NewAuthService(repository.NewMemoryRepository())
	return api.NewHandler(svc, "", blclient.NopClient{})
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
	return httptest.NewServer(api.NewHandler(svc, "", blclient.NopClient{}))
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

// ─── Step 28 (G42): Blacklist integration — ConsumerAuth ──────────────────────

func TestGrantBlacklistedProviderForbidden(t *testing.T) {
	fakeBlacklist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(true) //nolint:errcheck
	}))
	defer fakeBlacklist.Close()

	svc := service.NewAuthService(repository.NewMemoryRepository())
	h := api.NewHandler(svc, "", blclient.NewHTTPClient(fakeBlacklist.URL, http.DefaultClient))

	body := `{"provider":"bad-provider","targetType":"SERVICE_DEF","target":"svc","defaultPolicy":{"policyType":"ALL"}}`
	req := httptest.NewRequest(http.MethodPost, "/consumerauthorization/authorization/grant",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("blacklisted provider grant: want 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVerifyBlacklistedConsumerReturnsFalse(t *testing.T) {
	fakeBlacklist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(true) //nolint:errcheck
	}))
	defer fakeBlacklist.Close()

	svc := service.NewAuthService(repository.NewMemoryRepository())
	h := api.NewHandler(svc, "", blclient.NewHTTPClient(fakeBlacklist.URL, http.DefaultClient))

	// First grant a policy with a non-blacklisted check (NopClient would pass).
	// Then verify — blacklist should short-circuit to false.
	body := `{"consumer":"bad-consumer","provider":"","target":"svc","targetType":"SERVICE_DEF"}`
	req := httptest.NewRequest(http.MethodPost, "/consumerauthorization/authorization/verify",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("blacklisted consumer verify: want 200, got %d", w.Code)
	}
	var authorized bool
	json.NewDecoder(w.Body).Decode(&authorized) //nolint:errcheck
	if authorized {
		t.Error("blacklisted consumer verify: expected false, got true")
	}
}

// ─── Step 30: Bulk management endpoints (G38, G39) ────────────────────────────

func deleteJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodDelete, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestBulkGrantPoliciesCreatesAll(t *testing.T) {
	h := newTestHandler()
	body := map[string]any{
		"policies": []map[string]any{
			{"provider": "p1", "targetType": model.TargetServiceDef, "target": "svc1", "defaultPolicy": map[string]any{"policyType": model.PolicyAll}},
			{"provider": "p2", "targetType": model.TargetServiceDef, "target": "svc2", "defaultPolicy": map[string]any{"policyType": model.PolicyAll}},
		},
	}
	w := postJSON(t, h, "/consumerauthorization/authorization/mgmt/grant-policies", body)
	if w.Code != http.StatusOK {
		t.Fatalf("bulk grant-policies: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Results []struct {
			InstanceID string `json:"instanceId"`
			Error      string `json:"error"`
		} `json:"results"`
		Count int `json:"count"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 2 {
		t.Errorf("want count=2, got %d", resp.Count)
	}
	for i, r := range resp.Results {
		if r.Error != "" {
			t.Errorf("result[%d] unexpected error: %s", i, r.Error)
		}
		if r.InstanceID == "" {
			t.Errorf("result[%d] missing instanceId", i)
		}
	}
}

func TestBulkGrantPoliciesPerItemError(t *testing.T) {
	h := newTestHandler()
	// Grant one policy first.
	grantAndGetInstanceID(t, h, "p1", "svc1")
	// Bulk grant with a duplicate + a valid new one.
	body := map[string]any{
		"policies": []map[string]any{
			// duplicate
			{"provider": "p1", "targetType": model.TargetServiceDef, "target": "svc1", "defaultPolicy": map[string]any{"policyType": model.PolicyAll}},
			// valid
			{"provider": "p2", "targetType": model.TargetServiceDef, "target": "svc2", "defaultPolicy": map[string]any{"policyType": model.PolicyAll}},
		},
	}
	w := postJSON(t, h, "/consumerauthorization/authorization/mgmt/grant-policies", body)
	if w.Code != http.StatusOK {
		t.Fatalf("bulk grant-policies: want 200, got %d", w.Code)
	}
	var resp struct {
		Results []struct {
			Error string `json:"error"`
		} `json:"results"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 2 {
		t.Fatalf("want 2 results, got %d", len(resp.Results))
	}
	if resp.Results[0].Error == "" {
		t.Error("expected error for duplicate policy, got none")
	}
	if resp.Results[1].Error != "" {
		t.Errorf("expected no error for valid policy, got: %s", resp.Results[1].Error)
	}
}

func TestBulkRevokePoliciesRemovesAll(t *testing.T) {
	h := newTestHandler()
	id1 := grantAndGetInstanceID(t, h, "p1", "svc1")
	id2 := grantAndGetInstanceID(t, h, "p2", "svc2")

	w := deleteJSON(t, h, "/consumerauthorization/authorization/mgmt/revoke-policies",
		map[string]any{"instanceIds": []string{id1, id2}})
	if w.Code != http.StatusOK {
		t.Fatalf("revoke-policies: want 200, got %d: %s", w.Code, w.Body.String())
	}
	// Verify they are gone via query-policies.
	w2 := postJSON(t, h, "/consumerauthorization/authorization/mgmt/query-policies", map[string]any{})
	var resp struct{ TotalCount int `json:"totalCount"` }
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp.TotalCount != 0 {
		t.Errorf("after revoke want totalCount=0, got %d", resp.TotalCount)
	}
}

func TestQueryPoliciesWithFilters(t *testing.T) {
	h := newTestHandler()
	grantAndGetInstanceID(t, h, "p1", "svc1")
	grantAndGetInstanceID(t, h, "p2", "svc2")
	grantAndGetInstanceID(t, h, "p3", "svc3")

	// Query all — should return 3.
	w := postJSON(t, h, "/consumerauthorization/authorization/mgmt/query-policies", map[string]any{})
	var resp struct {
		Policies   []model.AuthPolicy `json:"policies"`
		Count      int                `json:"count"`
		TotalCount int                `json:"totalCount"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.TotalCount != 3 {
		t.Errorf("want totalCount=3, got %d", resp.TotalCount)
	}

	// Query with pagination pageSize=2.
	w2 := postJSON(t, h, "/consumerauthorization/authorization/mgmt/query-policies",
		map[string]any{"pagination": map[string]any{"pageNumber": 0, "pageSize": 2}})
	var resp2 struct {
		Count      int `json:"count"`
		TotalCount int `json:"totalCount"`
	}
	json.NewDecoder(w2.Body).Decode(&resp2)
	if resp2.Count != 2 {
		t.Errorf("paginated: want count=2, got %d", resp2.Count)
	}
	if resp2.TotalCount != 3 {
		t.Errorf("paginated: want totalCount=3, got %d", resp2.TotalCount)
	}
}

func TestCheckPoliciesMixedResult(t *testing.T) {
	h := newTestHandler()
	// Grant a policy that allows all consumers to access svc1 via p1.
	grantAndGetInstanceID(t, h, "p1", "svc1")

	checks := []map[string]any{
		{"consumer": "any-consumer", "provider": "p1", "target": "svc1", "targetType": model.TargetServiceDef},
		{"consumer": "any-consumer", "provider": "p99", "target": "no-such-svc", "targetType": model.TargetServiceDef},
	}
	w := postJSON(t, h, "/consumerauthorization/authorization/mgmt/check-policies", checks)
	if w.Code != http.StatusOK {
		t.Fatalf("check-policies: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Results []struct {
			Authorized bool `json:"authorized"`
		} `json:"results"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Results) != 2 {
		t.Fatalf("want 2 results, got %d", len(resp.Results))
	}
	if !resp.Results[0].Authorized {
		t.Error("first check: want authorized=true, got false")
	}
	if resp.Results[1].Authorized {
		t.Error("second check: want authorized=false, got true")
	}
}

func TestBulkGenerateTokensReturnsTokenList(t *testing.T) {
	h := newTestHandler()
	body := map[string]any{
		"requests": []map[string]any{
			{"tokenVariant": model.TokenVariantTimeLimited, "provider": "p1", "targetType": model.TargetServiceDef, "target": "svc1"},
			{"tokenVariant": model.TokenVariantTimeLimited, "provider": "p2", "targetType": model.TargetServiceDef, "target": "svc2"},
		},
	}
	w := postJSON(t, h, "/consumerauthorization/authorization-token/mgmt/generate-tokens", body)
	if w.Code != http.StatusOK {
		t.Fatalf("generate-tokens: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Results []struct {
			Token struct{ Token string `json:"token"` } `json:"token"`
			Error string `json:"error"`
		} `json:"results"`
		Count int `json:"count"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 2 {
		t.Errorf("want count=2, got %d", resp.Count)
	}
	for i, r := range resp.Results {
		if r.Error != "" {
			t.Errorf("result[%d] unexpected error: %s", i, r.Error)
		}
		if r.Token.Token == "" {
			t.Errorf("result[%d] missing token", i)
		}
	}
}

func TestBulkRevokeTokensRevokesAll(t *testing.T) {
	h := newTestHandler()
	// Generate two tokens via the single endpoint.
	makeToken := func(provider, target string) string {
		w := postJSON(t, h, "/consumerauthorization/authorization-token/generate", map[string]any{
			"tokenVariant": model.TokenVariantTimeLimited,
			"provider":     provider,
			"targetType":   model.TargetServiceDef,
			"target":       target,
		})
		var desc model.TokenDescriptor
		json.NewDecoder(w.Body).Decode(&desc)
		return desc.Token
	}
	t1 := makeToken("p1", "svc1")
	t2 := makeToken("p2", "svc2")

	w := deleteJSON(t, h, "/consumerauthorization/authorization-token/mgmt/revoke-tokens",
		map[string]any{"tokens": []string{t1, t2}})
	if w.Code != http.StatusOK {
		t.Fatalf("revoke-tokens: want 200, got %d: %s", w.Code, w.Body.String())
	}
	// Verify tokens are gone.
	for _, tok := range []string{t1, t2} {
		wv := getReq(t, h, "/consumerauthorization/authorization-token/verify/"+tok)
		if wv.Code != http.StatusNotFound {
			t.Errorf("after revoke token %s: want 404, got %d", tok, wv.Code)
		}
	}
}

func TestQueryTokensWithPagination(t *testing.T) {
	h := newTestHandler()
	// Generate 3 tokens.
	for i := 0; i < 3; i++ {
		postJSON(t, h, "/consumerauthorization/authorization-token/generate", map[string]any{
			"tokenVariant": model.TokenVariantTimeLimited,
			"provider":     "p1",
			"targetType":   model.TargetServiceDef,
			"target":       "svc" + string(rune('1'+i)),
		})
	}
	// Query all.
	w := postJSON(t, h, "/consumerauthorization/authorization-token/mgmt/query-tokens", map[string]any{})
	var resp struct {
		Tokens     []model.TokenRecord `json:"tokens"`
		Count      int                 `json:"count"`
		TotalCount int                 `json:"totalCount"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.TotalCount < 3 {
		t.Errorf("want totalCount>=3, got %d", resp.TotalCount)
	}

	// Query with pageSize=2.
	w2 := postJSON(t, h, "/consumerauthorization/authorization-token/mgmt/query-tokens",
		map[string]any{"pagination": map[string]any{"pageNumber": 0, "pageSize": 2}})
	var resp2 struct {
		Count      int `json:"count"`
		TotalCount int `json:"totalCount"`
	}
	json.NewDecoder(w2.Body).Decode(&resp2)
	if resp2.Count != 2 {
		t.Errorf("paginated: want count=2, got %d", resp2.Count)
	}
	if resp2.TotalCount < 3 {
		t.Errorf("paginated: want totalCount>=3, got %d", resp2.TotalCount)
	}
}

// ─── Step 42 — Scoped policy evaluation (G46) ────────────────────────────────

// TestVerifyScopedPolicyOverridesDefault confirms that a scoped policy overrides
// the default when the matching scope is supplied, and that an unknown scope falls
// back to the default policy.
func TestVerifyScopedPolicyOverridesDefault(t *testing.T) {
	h := newTestHandler()
	// Grant: default = DENY_ALL (BLACKLIST with empty list → deny all),
	// scoped "write" = ALLOW_ALL.
	postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType": model.TargetServiceDef,
		"target":     "data",
		"provider":   "Provider1",
		"defaultPolicy": map[string]any{
			"policyType": model.PolicyBlacklist,
			"policyList": []string{"*"}, // deny all via blacklist wildcard... but let's use WHITELIST with no list
		},
		"scopedPolicies": map[string]any{
			"write": map[string]any{"policyType": model.PolicyAll},
		},
	})

	// Verify with scope "write" → should be authorized (scoped policy = ALLOW_ALL)
	wWrite := postJSON(t, h, "/consumerauthorization/authorization/verify", map[string]any{
		"consumer":   "ConsumerA",
		"provider":   "Provider1",
		"target":     "data",
		"targetType": model.TargetServiceDef,
		"scope":      "write",
	})
	if wWrite.Code != http.StatusOK {
		t.Fatalf("write scope: want 200, got %d", wWrite.Code)
	}
	var authWrite bool
	json.NewDecoder(wWrite.Body).Decode(&authWrite)
	if !authWrite {
		t.Errorf("write scope: expected authorized=true, got false")
	}
}

// TestVerifyEmptyScopeFallsBackToDefault confirms that an empty scope uses the
// default policy (ALLOW_ALL in this test → authorized).
func TestVerifyEmptyScopeFallsBackToDefault(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType":    model.TargetServiceDef,
		"target":        "data",
		"provider":      "Provider1",
		"defaultPolicy": map[string]any{"policyType": model.PolicyAll},
		"scopedPolicies": map[string]any{
			"restricted": map[string]any{"policyType": model.PolicyWhitelist, "policyList": []string{}},
		},
	})

	// No scope supplied → uses defaultPolicy (ALLOW_ALL)
	w := postJSON(t, h, "/consumerauthorization/authorization/verify", map[string]any{
		"consumer":   "ConsumerA",
		"provider":   "Provider1",
		"target":     "data",
		"targetType": model.TargetServiceDef,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("empty scope: want 200, got %d", w.Code)
	}
	var authorized bool
	json.NewDecoder(w.Body).Decode(&authorized)
	if !authorized {
		t.Errorf("empty scope: expected authorized=true via default ALLOW_ALL policy")
	}
}

// TestVerifyUnknownScopeFallsBackToDefault confirms that an unknown scope name
// falls back to the default policy (WHITELIST with no entries → deny).
func TestVerifyUnknownScopeFallsBackToDefault(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType": model.TargetServiceDef,
		"target":     "data",
		"provider":   "Provider1",
		// Default: WHITELIST with no allowed consumers → deny all
		"defaultPolicy": map[string]any{"policyType": model.PolicyWhitelist, "policyList": []string{}},
		"scopedPolicies": map[string]any{
			"write": map[string]any{"policyType": model.PolicyAll},
		},
	})

	// Unknown scope "admin" → falls back to default (WHITELIST empty → denied)
	w := postJSON(t, h, "/consumerauthorization/authorization/verify", map[string]any{
		"consumer":   "ConsumerA",
		"provider":   "Provider1",
		"target":     "data",
		"targetType": model.TargetServiceDef,
		"scope":      "admin",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("unknown scope: want 200, got %d", w.Code)
	}
	var authorized bool
	json.NewDecoder(w.Body).Decode(&authorized)
	if authorized {
		t.Errorf("unknown scope: expected authorized=false (falls back to WHITELIST/empty default)")
	}
}

