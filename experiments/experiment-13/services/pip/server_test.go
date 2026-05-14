package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newPIPTestServer() http.Handler {
	store := NewSubjectStore()
	return NewServer(store)
}

// ── /health ───────────────────────────────────────────────────────────────────

func TestPIPHealth_OK(t *testing.T) {
	srv := newPIPTestServer()
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

func TestPIPStatus_Fields(t *testing.T) {
	store := NewSubjectStore()
	store.Register("sp1", "sy", true)
	srv := NewServer(store)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/status", nil))
	if w.Code != http.StatusOK {
		t.Errorf("status: code = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if _, ok := body["subjects"]; !ok {
		t.Error("status: missing 'subjects' field")
	}
}

// ── POST /subjects ────────────────────────────────────────────────────────────

func TestRegisterSubject_201(t *testing.T) {
	srv := newPIPTestServer()
	body := `{"name":"sp1","certLevel":"sy","valid":true}`
	req := httptest.NewRequest(http.MethodPost, "/subjects", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("register: code = %d, want 201", w.Code)
	}
	var sub Subject
	json.NewDecoder(w.Body).Decode(&sub)
	if sub.Name != "sp1" {
		t.Errorf("register: name = %q, want sp1", sub.Name)
	}
	if sub.CertLevel != "sy" {
		t.Errorf("register: certLevel = %q, want sy", sub.CertLevel)
	}
}

func TestRegisterSubject_400_BadJSON(t *testing.T) {
	srv := newPIPTestServer()
	req := httptest.NewRequest(http.MethodPost, "/subjects", bytes.NewBufferString(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("register bad JSON: code = %d, want 400", w.Code)
	}
}

func TestRegisterSubject_400_InvalidLevel(t *testing.T) {
	srv := newPIPTestServer()
	body := `{"name":"sp1","certLevel":"invalid","valid":true}`
	req := httptest.NewRequest(http.MethodPost, "/subjects", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("register invalid level: code = %d, want 400", w.Code)
	}
}

// ── GET /subjects ─────────────────────────────────────────────────────────────

func TestListSubjects_Empty(t *testing.T) {
	srv := newPIPTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/subjects", nil))
	if w.Code != http.StatusOK {
		t.Errorf("list empty: code = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["count"].(float64) != 0 {
		t.Errorf("list empty: count = %v, want 0", body["count"])
	}
}

func TestListSubjects_WithEntries(t *testing.T) {
	store := NewSubjectStore()
	store.Register("sp1", "sy", true)
	store.Register("sp2", "on", false)
	srv := NewServer(store)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/subjects", nil))
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["count"].(float64) != 2 {
		t.Errorf("list: count = %v, want 2", body["count"])
	}
}

// ── GET /subjects/{name} ──────────────────────────────────────────────────────

func TestGetSubject_200(t *testing.T) {
	store := NewSubjectStore()
	store.Register("sp1", "sy", true)
	srv := NewServer(store)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/subjects/sp1", nil))
	if w.Code != http.StatusOK {
		t.Errorf("get subject: code = %d, want 200", w.Code)
	}
}

func TestGetSubject_404(t *testing.T) {
	srv := newPIPTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/subjects/missing", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("get missing: code = %d, want 404", w.Code)
	}
}

// ── DELETE /subjects/{name} ───────────────────────────────────────────────────

func TestDeleteSubject_204(t *testing.T) {
	store := NewSubjectStore()
	store.Register("sp1", "sy", true)
	srv := NewServer(store)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/subjects/sp1", nil))
	if w.Code != http.StatusNoContent {
		t.Errorf("delete: code = %d, want 204", w.Code)
	}
}

func TestDeleteSubject_404(t *testing.T) {
	srv := newPIPTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/subjects/nonexistent", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("delete nonexistent: code = %d, want 404", w.Code)
	}
}

// ── GET /attributes/{name} ───────────────────────────────────────────────────

func TestGetAttributes_200(t *testing.T) {
	store := NewSubjectStore()
	store.Register("sp1", "sy", true)
	srv := NewServer(store)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/attributes/sp1", nil))
	if w.Code != http.StatusOK {
		t.Errorf("attributes: code = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["certLevel"] != "sy" {
		t.Errorf("attributes: certLevel = %v, want sy", body["certLevel"])
	}
	if body["valid"] != true {
		t.Errorf("attributes: valid = %v, want true", body["valid"])
	}
}

func TestGetAttributes_404(t *testing.T) {
	srv := newPIPTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/attributes/unknown", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("attributes missing: code = %d, want 404", w.Code)
	}
}

// ── unknown route ─────────────────────────────────────────────────────────────

func TestPIPUnknownRoute_404(t *testing.T) {
	srv := newPIPTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown route: code = %d, want 404", w.Code)
	}
}

// ── method not allowed ────────────────────────────────────────────────────────

func TestPIPHealth_MethodNotAllowed(t *testing.T) {
	srv := newPIPTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/health", strings.NewReader("")))
	if w.Code != http.StatusNotFound {
		t.Errorf("health POST: code = %d, want 404", w.Code)
	}
}

func TestPIPSubjects_MethodNotAllowed(t *testing.T) {
	srv := newPIPTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/subjects", strings.NewReader("")))
	if w.Code != http.StatusNotFound {
		t.Errorf("subjects PUT: code = %d, want 404", w.Code)
	}
}
