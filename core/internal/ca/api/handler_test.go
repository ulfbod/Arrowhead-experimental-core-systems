package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"arrowhead/core/internal/ca/api"
	"arrowhead/core/internal/ca/model"
	"arrowhead/core/internal/ca/service"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	svc, err := service.NewCAService(24 * time.Hour)
	if err != nil {
		t.Fatalf("NewCAService: %v", err)
	}
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

func getReq(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// ---- Issue ----

func TestHandlerIssueValid(t *testing.T) {
	h := newTestHandler(t)
	w := postJSON(t, h, "/ca/certificate/issue", map[string]string{"systemName": "sensor-1"})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.IssuedCert
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.SystemName != "sensor-1" {
		t.Errorf("SystemName = %q, want sensor-1", resp.SystemName)
	}
	if resp.Certificate == "" {
		t.Error("Certificate is empty")
	}
	if resp.PrivateKey == "" {
		t.Error("PrivateKey is empty")
	}
}

func TestHandlerIssueMissingSystemName(t *testing.T) {
	h := newTestHandler(t)
	w := postJSON(t, h, "/ca/certificate/issue", map[string]string{"systemName": ""})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerIssueInvalidJSON(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/ca/certificate/issue", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerIssueWrongMethod(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/ca/certificate/issue", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---- Verify ----

func TestHandlerVerifyValid(t *testing.T) {
	h := newTestHandler(t)

	// Issue first.
	issueW := postJSON(t, h, "/ca/certificate/issue", map[string]string{"systemName": "trusted"})
	var issued model.IssuedCert
	json.NewDecoder(issueW.Body).Decode(&issued)

	// Then verify.
	w := postJSON(t, h, "/ca/certificate/verify", map[string]string{"certificate": issued.Certificate})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["valid"] != true {
		t.Errorf("expected valid=true, got %v", resp["valid"])
	}
	if resp["systemName"] != "trusted" {
		t.Errorf("systemName = %v, want trusted", resp["systemName"])
	}
}

func TestHandlerVerifyInvalidCert(t *testing.T) {
	h := newTestHandler(t)
	w := postJSON(t, h, "/ca/certificate/verify", map[string]string{"certificate": "not-a-cert"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["valid"] != false {
		t.Errorf("expected valid=false for garbage cert")
	}
}

// ---- Info ----

func TestHandlerInfo(t *testing.T) {
	h := newTestHandler(t)
	w := getReq(t, h, "/ca/info")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var info model.CAInfo
	json.NewDecoder(w.Body).Decode(&info)
	if info.CommonName == "" {
		t.Error("expected non-empty CommonName")
	}
	if info.Certificate == "" {
		t.Error("expected non-empty Certificate")
	}
}

// ---- Health ----

func TestHandlerHealth(t *testing.T) {
	h := newTestHandler(t)
	for _, path := range []string{"/health", "/ca/health"} {
		w := getReq(t, h, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, w.Code)
		}
	}
}
