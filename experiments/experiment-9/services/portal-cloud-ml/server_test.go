package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── makeHTTPHandler tests ─────────────────────────────────────────────────────

func TestHTTPHandler_Health(t *testing.T) {
	store := NewStore()
	h := makeHTTPHandler(store)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := rec.Body.String()
	if body == "" {
		t.Errorf("empty body")
	}
}

func TestHTTPHandler_Stats(t *testing.T) {
	store := NewStore()
	store.Record([]byte(`{"x":1}`))
	h := makeHTTPHandler(store)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stats", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var stats map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	if _, ok := stats["msgCount"]; !ok {
		t.Errorf("stats missing msgCount: %v", stats)
	}
}

// ── makeHTTPSHandler tests ────────────────────────────────────────────────────

func TestHTTPSHandler_TelemetryLatest_NoData(t *testing.T) {
	store := NewStore()
	h := makeHTTPSHandler(store)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/telemetry/latest", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	// Should return a valid JSON response even with no data.
	if body == "" {
		t.Errorf("empty body for /telemetry/latest with no data")
	}
}

func TestHTTPSHandler_TelemetryLatest_WithData(t *testing.T) {
	store := NewStore()
	store.Record([]byte(`{"robot":"r1","val":99}`))
	h := makeHTTPSHandler(store)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/telemetry/latest", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if body != `{"robot":"r1","val":99}` {
		t.Errorf("body = %q, want {\"robot\":\"r1\",\"val\":99}", body)
	}
}

func TestHTTPSHandler_Health(t *testing.T) {
	store := NewStore()
	h := makeHTTPSHandler(store)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestHTTPSHandler_StatsReflectsStore(t *testing.T) {
	store := NewStore()
	for i := 0; i < 3; i++ {
		store.Record([]byte(`{"n":1}`))
	}
	h := makeHTTPSHandler(store)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stats", nil))

	var stats map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &stats)
	// JSON numbers decode as float64.
	if stats["msgCount"].(float64) != 3 {
		t.Errorf("msgCount = %v, want 3", stats["msgCount"])
	}
}
