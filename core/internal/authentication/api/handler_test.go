package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestHandlerVerifyValidToken(t *testing.T) {
	h := newTestHandler(time.Hour)
	token := loginAndGetToken(t, h, "sys")

	w := bearerRequest(t, h, http.MethodGet, "/authentication/identity/verify", token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.VerifyResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Valid {
		t.Error("expected valid=true")
	}
	if resp.SystemName != "sys" {
		t.Errorf("SystemName = %q, want sys", resp.SystemName)
	}
}

func TestHandlerVerifyNoToken(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := bearerRequest(t, h, http.MethodGet, "/authentication/identity/verify", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.VerifyResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Valid {
		t.Error("expected valid=false with no token")
	}
}

func TestHandlerVerifyInvalidToken(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := bearerRequest(t, h, http.MethodGet, "/authentication/identity/verify", "bogus")
	var resp model.VerifyResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Valid {
		t.Error("expected valid=false for unknown token")
	}
}

func TestHandlerVerifyExpiredToken(t *testing.T) {
	h := newTestHandler(-time.Second) // tokens immediately expired
	token := loginAndGetToken(t, h, "sys")

	w := bearerRequest(t, h, http.MethodGet, "/authentication/identity/verify", token)
	var resp model.VerifyResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Valid {
		t.Error("expected valid=false for expired token")
	}
}

func TestHandlerVerifyWrongMethod(t *testing.T) {
	h := newTestHandler(time.Hour)
	req := httptest.NewRequest(http.MethodPost, "/authentication/identity/verify", nil)
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

	w := bearerRequest(t, h, http.MethodDelete, "/authentication/identity/logout", token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Token must now be invalid.
	vw := bearerRequest(t, h, http.MethodGet, "/authentication/identity/verify", token)
	var resp model.VerifyResponse
	json.NewDecoder(vw.Body).Decode(&resp)
	if resp.Valid {
		t.Error("token should be invalid after logout")
	}
}

func TestHandlerLogoutMissingToken(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := bearerRequest(t, h, http.MethodDelete, "/authentication/identity/logout", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandlerLogoutUnknownToken(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := bearerRequest(t, h, http.MethodDelete, "/authentication/identity/logout", "ghost")
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
