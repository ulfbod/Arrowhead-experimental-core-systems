package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"arrowhead/core/internal/api"
)

func TestWriteErrorResponseShape(t *testing.T) {
	w := httptest.NewRecorder()
	api.WriteErrorResponse(w, http.StatusBadRequest, "field missing", "INVALID_PARAMETER", "serviceregistry.service-discovery.register")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body struct {
		ErrorMessage  string `json:"errorMessage"`
		ErrorCode     int    `json:"errorCode"`
		ExceptionType string `json:"exceptionType"`
		Origin        string `json:"origin"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ExceptionType != "INVALID_PARAMETER" {
		t.Errorf("exceptionType = %q, want INVALID_PARAMETER", body.ExceptionType)
	}
	if body.ErrorCode != 400 {
		t.Errorf("errorCode = %d, want 400", body.ErrorCode)
	}
	if body.Origin != "serviceregistry.service-discovery.register" {
		t.Errorf("origin = %q", body.Origin)
	}
}

func TestWriteErrorResponse404(t *testing.T) {
	w := httptest.NewRecorder()
	api.WriteErrorResponse(w, http.StatusNotFound, "not found", "DATA_NOT_FOUND", "serviceregistry.service-discovery.lookup")
	var body struct {
		ExceptionType string `json:"exceptionType"`
	}
	json.NewDecoder(w.Body).Decode(&body)
	if body.ExceptionType != "DATA_NOT_FOUND" {
		t.Errorf("exceptionType = %q, want DATA_NOT_FOUND", body.ExceptionType)
	}
}
