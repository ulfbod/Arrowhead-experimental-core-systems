package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── /health ───────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	s := newStore()
	h := makeHandler(s)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ok") {
		t.Errorf("body missing 'ok': %s", rec.Body.String())
	}
}

// ── /stats ────────────────────────────────────────────────────────────────────

func TestStats_empty(t *testing.T) {
	s := newStore()
	h := makeHandler(s)
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}

	var m map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"msgCount", "robotCount", "lastReceivedAt"} {
		if _, ok := m[key]; !ok {
			t.Errorf("stats missing key %q", key)
		}
	}
}

func TestStats_afterRecord(t *testing.T) {
	s := newStore()
	s.record("robot-1", []byte(`{"data":1}`))
	s.record("robot-2", []byte(`{"data":2}`))

	h := makeHandler(s)
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var m map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&m)

	if m["msgCount"].(float64) != 2 {
		t.Errorf("msgCount: got %v, want 2", m["msgCount"])
	}
	if m["robotCount"].(float64) != 2 {
		t.Errorf("robotCount: got %v, want 2", m["robotCount"])
	}
}

// ── /telemetry/latest ─────────────────────────────────────────────────────────

func TestTelemetryLatest_noData(t *testing.T) {
	s := newStore()
	h := makeHandler(s)
	req := httptest.NewRequest(http.MethodGet, "/telemetry/latest", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if rec.Body.String() != "null" {
		t.Errorf("empty store: got %q, want null", rec.Body.String())
	}
}

func TestTelemetryLatest_withData(t *testing.T) {
	s := newStore()
	s.record("robot-1", []byte(`{"robotId":"robot-1"}`))
	h := makeHandler(s)

	req := httptest.NewRequest(http.MethodGet, "/telemetry/latest", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "robot-1") {
		t.Errorf("expected robot-1 in body: %s", rec.Body.String())
	}
}

// ── /telemetry/{robotId} ──────────────────────────────────────────────────────

func TestTelemetryByRobot_found(t *testing.T) {
	s := newStore()
	s.record("robot-42", []byte(`{"robotId":"robot-42","temp":21.0}`))
	h := makeHandler(s)

	req := httptest.NewRequest(http.MethodGet, "/telemetry/robot-42", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "robot-42") {
		t.Errorf("expected robot-42 in body: %s", rec.Body.String())
	}
}

func TestTelemetryByRobot_notFound(t *testing.T) {
	s := newStore()
	h := makeHandler(s)

	req := httptest.NewRequest(http.MethodGet, "/telemetry/unknown-robot", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestTelemetryByRobot_doesNotReturnOtherRobot(t *testing.T) {
	s := newStore()
	s.record("robot-A", []byte(`{"id":"A"}`))
	s.record("robot-B", []byte(`{"id":"B"}`))
	h := makeHandler(s)

	req := httptest.NewRequest(http.MethodGet, "/telemetry/robot-A", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, `"id":"B"`) {
		t.Errorf("robot-A response should not contain robot-B data: %s", body)
	}
}
