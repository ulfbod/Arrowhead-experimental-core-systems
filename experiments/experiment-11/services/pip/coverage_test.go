package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPIPHealth_MethodNotAllowed(t *testing.T) {
	srv := newPIPServer(NewGrantStore())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/health", strings.NewReader("")))
	if w.Code != http.StatusNotFound {
		t.Errorf("health POST: code = %d, want 404", w.Code)
	}
}

func TestPIPStatus_MethodNotAllowed(t *testing.T) {
	srv := newPIPServer(NewGrantStore())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/status", strings.NewReader("")))
	if w.Code != http.StatusNotFound {
		t.Errorf("status POST: code = %d, want 404", w.Code)
	}
}

func TestPIPGrants_MethodNotAllowed(t *testing.T) {
	srv := newPIPServer(NewGrantStore())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/grants", strings.NewReader("")))
	if w.Code != http.StatusNotFound {
		t.Errorf("grants POST: code = %d, want 404", w.Code)
	}
}

func TestPIPStatus_HasLastSyncAt(t *testing.T) {
	store := NewGrantStore()
	store.Update([]Grant{{ConsumerSystemName: "sp1", ServiceDefinition: "svc"}})
	srv := newPIPServer(store)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/status", nil))
	body := w.Body.String()
	if !strings.Contains(body, "lastSyncAt") {
		t.Error("status: missing lastSyncAt field")
	}
}

func TestGrantsEqual_OrderInsensitive(t *testing.T) {
	a := []Grant{{ConsumerSystemName: "sp1", ServiceDefinition: "svc-a"}, {ConsumerSystemName: "sp2", ServiceDefinition: "svc-b"}}
	b := []Grant{{ConsumerSystemName: "sp2", ServiceDefinition: "svc-b"}, {ConsumerSystemName: "sp1", ServiceDefinition: "svc-a"}}
	if !grantsEqual(a, b) {
		t.Error("grantsEqual: order-insensitive check failed")
	}
}

func TestGrantsEqual_DifferentContent(t *testing.T) {
	a := []Grant{{ConsumerSystemName: "sp1", ServiceDefinition: "svc-a"}}
	b := []Grant{{ConsumerSystemName: "sp1", ServiceDefinition: "svc-b"}}
	if grantsEqual(a, b) {
		t.Error("grantsEqual: different content should return false")
	}
}
