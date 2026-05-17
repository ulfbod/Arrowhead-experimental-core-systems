// pip_test.go — unit tests for the PIP HTTP client.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetAttributes_200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"systemName": "test-system",
			"certLevel":  "sy",
			"valid":      true,
		})
	}))
	defer srv.Close()

	c := newPIPClient(srv.URL)
	attrs, err := c.GetAttributes("test-system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attrs.CertLevel != "sy" {
		t.Errorf("CertLevel: got %q, want %q", attrs.CertLevel, "sy")
	}
	if !attrs.CertValid {
		t.Error("CertValid: got false, want true")
	}
}

func TestGetAttributes_404_failClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newPIPClient(srv.URL)
	attrs, err := c.GetAttributes("unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attrs.CertValid {
		t.Error("expected certValid=false for 404")
	}
	if attrs.CertLevel != "" {
		t.Errorf("expected empty certLevel for 404, got %q", attrs.CertLevel)
	}
}

func TestGetAttributes_networkError_failClosed(t *testing.T) {
	c := newPIPClient("http://127.0.0.1:1") // unreachable
	attrs, err := c.GetAttributes("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attrs.CertValid {
		t.Error("expected certValid=false on network error")
	}
}
