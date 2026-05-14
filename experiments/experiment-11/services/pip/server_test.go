package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newPIPServer(store *GrantStore) http.Handler {
	return NewServer(store, "http://consumerauth:8082")
}

// ── /health ───────────────────────────────────────────────────────────────────

func TestPIPHealth_OK(t *testing.T) {
	srv := newPIPServer(NewGrantStore())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Errorf("health: code = %d, want 200", w.Code)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("health: status = %q, want ok", body["status"])
	}
}

// ── /status ───────────────────────────────────────────────────────────────────

func TestPIPStatus_BeforeSync(t *testing.T) {
	srv := newPIPServer(NewGrantStore())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/status", nil))
	if w.Code != http.StatusOK {
		t.Errorf("status: code = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	for _, field := range []string{"synced", "version", "grants", "consumerAuthURL"} {
		if _, ok := body[field]; !ok {
			t.Errorf("status: missing field %q", field)
		}
	}
	if body["synced"] != false {
		t.Errorf("status synced: got %v, want false", body["synced"])
	}
}

func TestPIPStatus_AfterSync(t *testing.T) {
	store := NewGrantStore()
	store.Update([]Grant{{ConsumerSystemName: "sp1", ServiceDefinition: "svc"}})
	srv := newPIPServer(store)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/status", nil))
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["synced"] != true {
		t.Errorf("status synced: got %v, want true", body["synced"])
	}
	if body["grants"].(float64) != 1 {
		t.Errorf("status grants: got %v, want 1", body["grants"])
	}
}

// ── GET /grants ───────────────────────────────────────────────────────────────

func TestGetGrants_Empty(t *testing.T) {
	srv := newPIPServer(NewGrantStore())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/grants", nil))
	if w.Code != http.StatusOK {
		t.Errorf("grants empty: code = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["count"].(float64) != 0 {
		t.Errorf("grants count: got %v, want 0", body["count"])
	}
}

func TestGetGrants_WithEntries(t *testing.T) {
	store := NewGrantStore()
	store.Update([]Grant{
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc"},
		{ConsumerSystemName: "sp2", ServiceDefinition: "svc"},
	})
	srv := newPIPServer(store)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/grants", nil))
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["count"].(float64) != 2 {
		t.Errorf("grants count: got %v, want 2", body["count"])
	}
}

func TestGetGrants_FilterBySubject(t *testing.T) {
	store := NewGrantStore()
	store.Update([]Grant{
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc-a"},
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc-b"},
		{ConsumerSystemName: "sp2", ServiceDefinition: "svc-a"},
	})
	srv := newPIPServer(store)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/grants?subject=sp1", nil))
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["count"].(float64) != 2 {
		t.Errorf("grants filter sp1: got %v, want 2", body["count"])
	}
}

func TestGetGrants_FilterBySubjectResource(t *testing.T) {
	store := NewGrantStore()
	store.Update([]Grant{
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc-a"},
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc-b"},
	})
	srv := newPIPServer(store)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/grants?subject=sp1&resource=svc-a", nil))
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["count"].(float64) != 1 {
		t.Errorf("grants filter sp1/svc-a: got %v, want 1", body["count"])
	}
	if body["granted"] != true {
		t.Errorf("grants filter sp1/svc-a: granted = %v, want true", body["granted"])
	}
}

func TestGetGrants_FilterNotGranted(t *testing.T) {
	store := NewGrantStore()
	store.Update([]Grant{{ConsumerSystemName: "sp1", ServiceDefinition: "svc-a"}})
	srv := newPIPServer(store)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/grants?subject=sp2&resource=svc-a", nil))
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["granted"] != false {
		t.Errorf("grants filter sp2/svc-a: granted = %v, want false", body["granted"])
	}
}

// ── unknown route ─────────────────────────────────────────────────────────────

func TestPIPUnknownRoute_404(t *testing.T) {
	srv := newPIPServer(NewGrantStore())
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown route: code = %d, want 404", w.Code)
	}
}
