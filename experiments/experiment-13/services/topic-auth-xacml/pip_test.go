package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPIPClient_GetAttributes_Success verifies that a 200 response is parsed correctly.
func TestPIPClient_GetAttributes_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pip/attributes/consumer-1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"systemName": "consumer-1",
			"certLevel":  "sy",
			"valid":      true,
		})
	}))
	defer srv.Close()

	pip := &pipClient{baseURL: srv.URL}
	attrs, err := pip.GetAttributes("consumer-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attrs.CertLevel != "sy" {
		t.Errorf("CertLevel: got %q, want %q", attrs.CertLevel, "sy")
	}
	if !attrs.CertValid {
		t.Error("CertValid: expected true")
	}
}

// TestPIPClient_GetAttributes_NotFound verifies that a 404 returns empty/false attrs (fail-closed).
func TestPIPClient_GetAttributes_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	pip := &pipClient{baseURL: srv.URL}
	attrs, err := pip.GetAttributes("unknown-system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attrs.CertLevel != "" {
		t.Errorf("CertLevel: expected empty string, got %q", attrs.CertLevel)
	}
	if attrs.CertValid {
		t.Error("CertValid: expected false for 404")
	}
}

// TestPIPClient_GetAttributes_Unreachable verifies that an unreachable PIP returns fail-closed attrs.
func TestPIPClient_GetAttributes_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	pip := &pipClient{baseURL: srv.URL}
	attrs, err := pip.GetAttributes("consumer-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attrs.CertLevel != "" {
		t.Errorf("CertLevel: expected empty string for unreachable PIP, got %q", attrs.CertLevel)
	}
	if attrs.CertValid {
		t.Error("CertValid: expected false for unreachable PIP")
	}
}
