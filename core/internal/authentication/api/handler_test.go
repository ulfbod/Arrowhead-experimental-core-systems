package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"arrowhead/core/internal/authentication/api"
	"arrowhead/core/internal/authentication/model"
	"arrowhead/core/internal/authentication/repository"
	"arrowhead/core/internal/authentication/service"
)

func newTestHandler(dur time.Duration) http.Handler {
	svc := service.NewAuthService(repository.NewMemoryRepository(), dur)
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

func bearerRequest(t *testing.T, h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func loginAndGetToken(t *testing.T, h http.Handler, systemName string) string {
	t.Helper()
	w := postJSON(t, h, "/authentication/identity/login", map[string]string{"systemName": systemName})
	if w.Code != http.StatusCreated {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	var resp model.LoginResponse
	json.NewDecoder(w.Body).Decode(&resp)
	return resp.Token
}

// ---- ErrorResponse shape ----

func TestAuthLoginMissingFieldReturnsExceptionType(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := postJSON(t, h, "/authentication/identity/login", map[string]string{})
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

// ---- Login ----

func TestHandlerLoginValid(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := postJSON(t, h, "/authentication/identity/login",
		map[string]string{"systemName": "sensor-1", "credentials": "secret"})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.LoginResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.SystemName != "sensor-1" {
		t.Errorf("SystemName = %q", resp.SystemName)
	}
}

func TestHandlerLoginMissingSystemName(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := postJSON(t, h, "/authentication/identity/login", map[string]string{"credentials": "x"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerLoginInvalidJSON(t *testing.T) {
	h := newTestHandler(time.Hour)
	req := httptest.NewRequest(http.MethodPost, "/authentication/identity/login",
		bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerLoginWrongMethod(t *testing.T) {
	h := newTestHandler(time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/authentication/identity/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---- Verify ----

func getReq(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestHandlerVerifyValidToken(t *testing.T) {
	h := newTestHandler(time.Hour)
	token := loginAndGetToken(t, h, "sys")

	w := getReq(t, h, "/authentication/identity/verify/"+token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.VerifyResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Verified {
		t.Error("expected verified=true")
	}
	if resp.SystemName != "sys" {
		t.Errorf("SystemName = %q, want sys", resp.SystemName)
	}
}

func TestHandlerVerifyNoToken(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := getReq(t, h, "/authentication/identity/verify/")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.VerifyResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Verified {
		t.Error("expected verified=false with no token")
	}
}

func TestHandlerVerifyInvalidToken(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := getReq(t, h, "/authentication/identity/verify/bogus")
	var resp model.VerifyResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Verified {
		t.Error("expected verified=false for unknown token")
	}
}

func TestHandlerVerifyExpiredToken(t *testing.T) {
	h := newTestHandler(-time.Second) // tokens immediately expired
	token := loginAndGetToken(t, h, "sys")

	w := getReq(t, h, "/authentication/identity/verify/"+token)
	var resp model.VerifyResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Verified {
		t.Error("expected verified=false for expired token")
	}
}

func TestHandlerVerifyWrongMethod(t *testing.T) {
	h := newTestHandler(time.Hour)
	req := httptest.NewRequest(http.MethodPost, "/authentication/identity/verify/sometoken", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---- Logout ----

func TestHandlerLogoutValid(t *testing.T) {
	h := newTestHandler(time.Hour)
	token := loginAndGetToken(t, h, "sys")

	w := bearerRequest(t, h, http.MethodPost, "/authentication/identity/logout", token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Token must now be invalid.
	vw := getReq(t, h, "/authentication/identity/verify/"+token)
	var resp model.VerifyResponse
	json.NewDecoder(vw.Body).Decode(&resp)
	if resp.Verified {
		t.Error("token should be invalid after logout")
	}
}

func TestHandlerLogoutMissingToken(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := bearerRequest(t, h, http.MethodPost, "/authentication/identity/logout", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandlerLogoutUnknownToken(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := bearerRequest(t, h, http.MethodPost, "/authentication/identity/logout", "ghost")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandlerLogoutWrongMethod(t *testing.T) {
	h := newTestHandler(time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/authentication/identity/logout", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---- Health ----

func TestHandlerHealth(t *testing.T) {
	h := newTestHandler(time.Hour)
	for _, path := range []string{"/health", "/authentication/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, w.Code)
		}
	}
}

// ---- TDD 7.1 — Login response field names ----

func newAuthTestServer() *httptest.Server {
	svc := service.NewAuthService(repository.NewMemoryRepository(), time.Hour)
	return httptest.NewServer(api.NewHandler(svc))
}

func keys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func TestLoginResponseHasExpirationTime(t *testing.T) {
	srv := newAuthTestServer()
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/authentication/identity/login",
		"application/json", strings.NewReader(`{"systemName":"Sys1","credentials":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&m)
	resp.Body.Close()
	if _, ok := m["expirationTime"]; !ok {
		t.Errorf("response missing expirationTime field; got keys: %v", keys(m))
	}
	if _, ok := m["expiresAt"]; ok {
		t.Error("response still has old expiresAt field")
	}
	if _, ok := m["sysop"]; !ok {
		t.Errorf("response missing sysop field; got keys: %v", keys(m))
	}
}

// ---- TDD 7.2 — Logout uses POST ----

func TestLogoutRequiresPOST(t *testing.T) {
	srv := newAuthTestServer()
	defer srv.Close()

	// Login first to get a token.
	loginResp, _ := http.Post(srv.URL+"/authentication/identity/login",
		"application/json",
		strings.NewReader(`{"systemName":"S","credentials":"x"}`))
	var lr map[string]interface{}
	json.NewDecoder(loginResp.Body).Decode(&lr)
	loginResp.Body.Close()
	token := lr["token"].(string)

	// DELETE should return 405.
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/authentication/identity/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("DELETE logout: expected 405, got %d", resp.StatusCode)
	}

	// POST should return 200 or 204.
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/authentication/identity/logout", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	resp2, _ := http.DefaultClient.Do(req2)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK && resp2.StatusCode != http.StatusNoContent {
		t.Errorf("POST logout: expected 200/204, got %d", resp2.StatusCode)
	}
}

// ---- TDD 7.3 — Verify uses path parameter ----

func TestVerifyAcceptsPathParam(t *testing.T) {
	srv := newAuthTestServer()
	defer srv.Close()

	loginResp, _ := http.Post(srv.URL+"/authentication/identity/login",
		"application/json",
		strings.NewReader(`{"systemName":"S","credentials":"x"}`))
	var lr map[string]interface{}
	json.NewDecoder(loginResp.Body).Decode(&lr)
	loginResp.Body.Close()
	token := lr["token"].(string)

	resp, err := http.Get(srv.URL + "/authentication/identity/verify/" +
		url.PathEscape(token))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var vr map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&vr)
	resp.Body.Close()
	if vr["verified"] != true {
		t.Errorf("expected verified=true, got %v", vr["verified"])
	}
	if _, ok := vr["loginTime"]; !ok {
		t.Errorf("response missing loginTime field; got keys: %v", keys(vr))
	}
}

// ---- Cycle 12.1 — Verify response includes expirationTime and sysop ----

func TestVerifyResponseIncludesExpirationTime(t *testing.T) {
	h := newTestHandler(time.Hour)
	token := loginAndGetToken(t, h, "sys-a")
	w := bearerRequest(t, h, http.MethodGet, "/authentication/identity/verify/"+token, "")
	if w.Code != http.StatusOK {
		t.Fatalf("verify failed: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		ExpirationTime string `json:"expirationTime"`
		Sysop          *bool  `json:"sysop"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ExpirationTime == "" {
		t.Error("expirationTime is empty")
	}
	if resp.Sysop == nil {
		t.Error("sysop field is absent")
	}
	if *resp.Sysop != false {
		t.Errorf("sysop = %v, want false", *resp.Sysop)
	}
}

// ---- Cycle 12.3 — Change endpoint ----

func TestChangeCredentials200(t *testing.T) {
	h := newTestHandler(time.Hour)
	loginAndGetToken(t, h, "sys-b")
	w := postJSON(t, h, "/authentication/identity/change", map[string]any{
		"systemName":     "sys-b",
		"credentials":    map[string]string{"password": "old"},
		"newCredentials": map[string]string{"password": "new"},
	})
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChangeCredentials401NoSession(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := postJSON(t, h, "/authentication/identity/change", map[string]any{
		"systemName":     "nobody",
		"credentials":    map[string]string{"password": "x"},
		"newCredentials": map[string]string{"password": "y"},
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ---- Cycle 13.2 — Management endpoints ----

func newTestHandlerFull(dur time.Duration) http.Handler {
	tokenRepo := repository.NewMemoryRepository()
	identityRepo := repository.NewMemoryIdentityRepository()
	svc := service.NewAuthServiceFull(tokenRepo, identityRepo, dur)
	return api.NewHandler(svc)
}

func TestMgmtCreateIdentities201(t *testing.T) {
	h := newTestHandlerFull(time.Hour)
	w := postJSON(t, h, "/authentication/mgmt/identities", map[string]any{
		"authenticationMethod": "PASSWORD",
		"identities": []map[string]any{
			{"systemName": "robot-1", "credentials": map[string]string{"password": "secret"}, "sysop": false},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Identities []map[string]any `json:"identities"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Identities) != 1 {
		t.Errorf("identities len = %d, want 1", len(resp.Identities))
	}
}

func TestMgmtQueryIdentities200(t *testing.T) {
	h := newTestHandlerFull(time.Hour)
	postJSON(t, h, "/authentication/mgmt/identities", map[string]any{
		"authenticationMethod": "PASSWORD",
		"identities": []map[string]any{
			{"systemName": "q-sys", "credentials": map[string]string{"password": "pw"}},
		},
	})
	w := postJSON(t, h, "/authentication/mgmt/identities/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Identities []map[string]any `json:"identities"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Identities) < 1 {
		t.Errorf("no identities in query response")
	}
}

// ---- Cycle 13.3 — Login with credential verification ----

func TestLoginWrongPassword401(t *testing.T) {
	h := newTestHandlerFull(time.Hour)
	postJSON(t, h, "/authentication/mgmt/identities", map[string]any{
		"authenticationMethod": "PASSWORD",
		"identities": []map[string]any{
			{"systemName": "guarded", "credentials": map[string]string{"password": "correct"}},
		},
	})
	w := postJSON(t, h, "/authentication/identity/login", map[string]any{
		"systemName":  "guarded",
		"credentials": map[string]string{"password": "wrong"},
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLoginUnknownSystem401(t *testing.T) {
	h := newTestHandlerFull(time.Hour)
	w := postJSON(t, h, "/authentication/identity/login", map[string]any{
		"systemName":  "ghost",
		"credentials": map[string]string{"password": "x"},
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBootstrapSysopIdentity(t *testing.T) {
	h := newTestHandlerFull(time.Hour)
	w := postJSON(t, h, "/authentication/identity/login", map[string]any{
		"systemName":  "Sysop",
		"credentials": map[string]string{"password": "arrowhead"},
	})
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 for Sysop login, got %d: %s", w.Code, w.Body.String())
	}
}

// ---- Cycle 13.4 — Sysop flag in verify response ----

func TestVerifySysopTrue(t *testing.T) {
	h := newTestHandlerFull(time.Hour)
	w := postJSON(t, h, "/authentication/identity/login", map[string]any{
		"systemName":  "Sysop",
		"credentials": map[string]string{"password": "arrowhead"},
	})
	var loginResp struct{ Token string `json:"token"` }
	json.NewDecoder(w.Body).Decode(&loginResp)

	w2 := bearerRequest(t, h, http.MethodGet, "/authentication/identity/verify/"+loginResp.Token, "")
	var verifyResp struct{ Sysop bool `json:"sysop"` }
	json.NewDecoder(w2.Body).Decode(&verifyResp)
	if !verifyResp.Sysop {
		t.Error("sysop = false for Sysop identity, want true")
	}
}
