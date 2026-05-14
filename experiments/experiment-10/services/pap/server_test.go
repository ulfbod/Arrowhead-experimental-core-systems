package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// noopPusher is used in handler tests so we don't need a real AuthzForce.
type noopPusher struct{ pushCount int }

func (n *noopPusher) Push(_ []*Policy, _ int) error { n.pushCount++; return nil }

func newTestServer() (http.Handler, *PolicyStore, *noopPusher) {
	store := NewPolicyStore()
	pusher := &noopPusher{}
	srv := NewServer(store, pusher, "arrowhead-exp10")
	return srv, store, pusher
}

// ── /health ──────────────────────────────────────────────────────────────────

func TestHealth_OK(t *testing.T) {
	srv, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Errorf("health: status = %d, want 200", w.Code)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("health: status field = %q, want ok", body["status"])
	}
}

// ── /status ──────────────────────────────────────────────────────────────────

func TestStatus_Fields(t *testing.T) {
	srv, store, _ := newTestServer()
	store.Add("sp1", "svc", "consume", "Permit")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/status", nil))
	if w.Code != http.StatusOK {
		t.Errorf("status: code = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if _, ok := body["policies"]; !ok {
		t.Error("status: missing 'policies' field")
	}
	if _, ok := body["version"]; !ok {
		t.Error("status: missing 'version' field")
	}
	if body["domainExternalId"] != "arrowhead-exp10" {
		t.Errorf("status: domainExternalId = %v, want arrowhead-exp10", body["domainExternalId"])
	}
}

// ── POST /policies ────────────────────────────────────────────────────────────

func TestCreatePolicy_201(t *testing.T) {
	srv, _, pusher := newTestServer()
	body := `{"subject":"sp1","resource":"telemetry-rest","action":"consume"}`
	req := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("create: code = %d, want 201", w.Code)
	}
	var p Policy
	json.NewDecoder(w.Body).Decode(&p)
	if p.ID == "" {
		t.Error("create: response has empty ID")
	}
	if pusher.pushCount != 1 {
		t.Errorf("create: pusher.pushCount = %d, want 1", pusher.pushCount)
	}
}

func TestCreatePolicy_DefaultsEffect(t *testing.T) {
	srv, _, _ := newTestServer()
	body := `{"subject":"sp1","resource":"svc","action":"consume"}`
	req := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var p Policy
	json.NewDecoder(w.Body).Decode(&p)
	if p.Effect != "Permit" {
		t.Errorf("create default effect: effect = %q, want Permit", p.Effect)
	}
}

func TestCreatePolicy_400_EmptySubject(t *testing.T) {
	srv, _, _ := newTestServer()
	body := `{"subject":"","resource":"svc","action":"consume"}`
	req := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("create empty subject: code = %d, want 400", w.Code)
	}
}

func TestCreatePolicy_400_EmptyResource(t *testing.T) {
	srv, _, _ := newTestServer()
	body := `{"subject":"sp1","resource":"","action":"consume"}`
	req := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("create empty resource: code = %d, want 400", w.Code)
	}
}

func TestCreatePolicy_400_BadJSON(t *testing.T) {
	srv, _, _ := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewBufferString(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("create bad JSON: code = %d, want 400", w.Code)
	}
}

// ── GET /policies ─────────────────────────────────────────────────────────────

func TestListPolicies_Empty(t *testing.T) {
	srv, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/policies", nil))
	if w.Code != http.StatusOK {
		t.Errorf("list empty: code = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["count"].(float64) != 0 {
		t.Errorf("list empty: count = %v, want 0", body["count"])
	}
}

func TestListPolicies_WithEntries(t *testing.T) {
	srv, store, _ := newTestServer()
	store.Add("sp1", "svc", "consume", "Permit")
	store.Add("sp2", "svc", "consume", "Permit")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/policies", nil))
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["count"].(float64) != 2 {
		t.Errorf("list: count = %v, want 2", body["count"])
	}
}

// ── DELETE /policies/{id} ─────────────────────────────────────────────────────

func TestDeletePolicy_204(t *testing.T) {
	srv, store, pusher := newTestServer()
	p, _ := store.Add("sp1", "svc", "consume", "Permit")
	pusher.pushCount = 0 // reset counter
	req := httptest.NewRequest(http.MethodDelete, "/policies/"+p.ID, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("delete: code = %d, want 204", w.Code)
	}
	if pusher.pushCount != 1 {
		t.Errorf("delete: pusher.pushCount = %d, want 1", pusher.pushCount)
	}
}

func TestDeletePolicy_404(t *testing.T) {
	srv, _, _ := newTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/policies/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("delete nonexistent: code = %d, want 404", w.Code)
	}
}

// ── GET /policies/{id} ────────────────────────────────────────────────────────

func TestGetPolicy_200(t *testing.T) {
	srv, store, _ := newTestServer()
	p, _ := store.Add("sp1", "svc", "consume", "Permit")
	req := httptest.NewRequest(http.MethodGet, "/policies/"+p.ID, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("get policy: code = %d, want 200", w.Code)
	}
}

func TestGetPolicy_404(t *testing.T) {
	srv, _, _ := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/policies/missing", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("get missing: code = %d, want 404", w.Code)
	}
}

// ── method not allowed ────────────────────────────────────────────────────────

func TestUnknownRoute_404(t *testing.T) {
	srv, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown route: code = %d, want 404", w.Code)
	}
}
