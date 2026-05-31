package httputil_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"arrowhead/core/internal/httputil"
)

func TestErrorTypeForStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, "INVALID_PARAMETER"},
		{http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{http.StatusForbidden, "FORBIDDEN"},
		{http.StatusNotFound, "DATA_NOT_FOUND"},
		{http.StatusLocked, "LOCKED"},
		{http.StatusNotImplemented, "NOT_IMPLEMENTED"},
		{http.StatusInternalServerError, "ARROWHEAD_EXCEPTION"},
		{http.StatusConflict, "ARROWHEAD_EXCEPTION"},
		{http.StatusMethodNotAllowed, "ARROWHEAD_EXCEPTION"},
	}
	for _, tc := range tests {
		got := httputil.ErrorTypeForStatus(tc.status)
		if got != tc.want {
			t.Errorf("ErrorTypeForStatus(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	httputil.WriteError(w, http.StatusBadRequest, "test error", "myservice")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["errorMessage"] != "test error" {
		t.Errorf("errorMessage = %v", body["errorMessage"])
	}
	if int(body["errorCode"].(float64)) != http.StatusBadRequest {
		t.Errorf("errorCode = %v", body["errorCode"])
	}
	if body["exceptionType"] != "INVALID_PARAMETER" {
		t.Errorf("exceptionType = %v", body["exceptionType"])
	}
	if body["origin"] != "myservice" {
		t.Errorf("origin = %v", body["origin"])
	}
}

func TestWriteError_404(t *testing.T) {
	w := httptest.NewRecorder()
	httputil.WriteError(w, http.StatusNotFound, "not found", "svc")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body) //nolint:errcheck
	if body["exceptionType"] != "DATA_NOT_FOUND" {
		t.Errorf("exceptionType = %v", body["exceptionType"])
	}
}
