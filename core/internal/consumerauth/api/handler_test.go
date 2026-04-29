package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

var validGrantBody = map[string]string{
	"consumerSystemName": "consumer-app",
	"providerSystemName": "sensor-1",
	"serviceDefinition":  "temperature-service",
}

func grantAndGetID(t *testing.T, h http.Handler) int64 {
	t.Helper()
	w := postJSON(t, h, "/authorization/grant", validGrantBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("grant failed: %d %s", w.Code, w.Body.String())
	}
	var rule model.AuthRule
	json.NewDecoder(w.Body).Decode(&rule)
	return rule.ID
}

// ---- Grant ----

func TestHandlerGrantValid(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/authorization/grant", validGrantBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var rule model.AuthRule
	json.NewDecoder(w.Body).Decode(&rule)
	if rule.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestHandlerGrantValidation(t *testing.T) {
	tests := []struct {
		name string
		body map[string]string
	}{
		{"empty consumer", map[string]string{"consumerSystemName": "", "providerSystemName": "p", "serviceDefinition": "s"}},
		{"empty provider", map[string]string{"consumerSystemName": "c", "providerSystemName": "", "serviceDefinition": "s"}},
		{"empty service", map[string]string{"consumerSystemName": "c", "providerSystemName": "p", "serviceDefinition": ""}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(t, newTestHandler(), "/authorization/grant", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestHandlerGrantDuplicateReturns409(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/authorization/grant", validGrantBody)
	w := postJSON(t, h, "/authorization/grant", validGrantBody)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandlerGrantInvalidJSON(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/authorization/grant", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerGrantWrongMethod(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/authorization/grant", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---- Revoke ----

func TestHandlerRevokeValid(t *testing.T) {
	h := newTestHandler()
	id := grantAndGetID(t, h)
	w := deleteReq(t, h, "/authorization/revoke/"+itoa(id))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerRevokeNotFound(t *testing.T) {
	h := newTestHandler()
	w := deleteReq(t, h, "/authorization/revoke/999")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlerRevokeInvalidID(t *testing.T) {
	h := newTestHandler()
	w := deleteReq(t, h, "/authorization/revoke/notanumber")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerRevokeWrongMethod(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/authorization/revoke/1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---- Lookup ----

func TestHandlerLookupEmpty(t *testing.T) {
	h := newTestHandler()
	w := getReq(t, h, "/authorization/lookup")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.LookupResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Rules == nil {
		t.Error("expected non-nil rules slice")
	}
	if resp.Count != 0 {
		t.Errorf("expected 0, got %d", resp.Count)
	}
}

func TestHandlerLookupWithResults(t *testing.T) {
	h := newTestHandler()
	grantAndGetID(t, h)
	w := getReq(t, h, "/authorization/lookup")
	var resp model.LookupResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("expected 1, got %d", resp.Count)
	}
}

func TestHandlerLookupFilterByConsumer(t *testing.T) {
	h := newTestHandler()
	grantAndGetID(t, h)
	postJSON(t, h, "/authorization/grant", map[string]string{
		"consumerSystemName": "other-consumer",
		"providerSystemName": "sensor-1",
		"serviceDefinition":  "temperature-service",
	})

	w := getReq(t, h, "/authorization/lookup?consumer=consumer-app")
	var resp model.LookupResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 1 || resp.Rules[0].ConsumerSystemName != "consumer-app" {
		t.Errorf("unexpected lookup result: %+v", resp)
	}
}

// ---- Verify ----

func TestHandlerVerifyAuthorized(t *testing.T) {
	h := newTestHandler()
	grantAndGetID(t, h)
	w := postJSON(t, h, "/authorization/verify", map[string]string{
		"consumerSystemName": "consumer-app",
		"providerSystemName": "sensor-1",
		"serviceDefinition":  "temperature-service",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.VerifyResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Authorized {
		t.Error("expected authorized=true")
	}
}

func TestHandlerVerifyUnauthorized(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/authorization/verify", map[string]string{
		"consumerSystemName": "nobody",
		"providerSystemName": "sensor-1",
		"serviceDefinition":  "temperature-service",
	})
	var resp model.VerifyResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Authorized {
		t.Error("expected authorized=false")
	}
}

// ---- GenerateToken ----

func TestHandlerGenerateTokenAuthorized(t *testing.T) {
	h := newTestHandler()
	grantAndGetID(t, h)
	w := postJSON(t, h, "/authorization/token/generate", map[string]string{
		"consumerSystemName": "consumer-app",
		"providerSystemName": "sensor-1",
		"serviceDefinition":  "temperature-service",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.TokenResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestHandlerGenerateTokenUnauthorized(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/authorization/token/generate", map[string]string{
		"consumerSystemName": "stranger",
		"providerSystemName": "sensor-1",
		"serviceDefinition":  "temperature-service",
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// ---- Health ----

func TestHandlerHealth(t *testing.T) {
	h := newTestHandler()
	for _, path := range []string{"/health", "/authorization/health"} {
		w := getReq(t, h, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, w.Code)
		}
	}
}

func itoa(n int64) string {
	return fmt.Sprintf("%d", n)
}
