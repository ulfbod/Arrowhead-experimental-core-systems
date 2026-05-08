package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// statsResp mirrors the JSON shape of /stats responses.
type statsResp struct {
	Name           string `json:"name"`
	Transport      string `json:"transport"`
	MsgCount       int64  `json:"msgCount"`
	DeniedCount    int64  `json:"deniedCount"`
	LastReceivedAt string `json:"lastReceivedAt"`
	LastDeniedAt   string `json:"lastDeniedAt"`
}

func decodeStats(t *testing.T, rec *httptest.ResponseRecorder) statsResp {
	t.Helper()
	var s statsResp
	if err := json.NewDecoder(rec.Body).Decode(&s); err != nil {
		t.Fatalf("decode /stats: %v", err)
	}
	return s
}

// TestHealthHandler verifies GET /health returns 200 {"status":"ok"}.
func TestHealthHandler(t *testing.T) {
	mux := newMux(&statsTracker{}, "test-consumer")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`status field: got %q, want "ok"`, body["status"])
	}
}

// TestStatsHandler_shape verifies GET /stats returns all required JSON fields.
func TestStatsHandler_shape(t *testing.T) {
	mux := newMux(&statsTracker{}, "my-cert-consumer")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stats", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	s := decodeStats(t, w)

	if s.Name != "my-cert-consumer" {
		t.Errorf("name: got %q, want %q", s.Name, "my-cert-consumer")
	}
	if s.Transport != "rest-mtls" {
		t.Errorf("transport: got %q, want %q", s.Transport, "rest-mtls")
	}
	if s.MsgCount != 0 {
		t.Errorf("msgCount: got %d, want 0", s.MsgCount)
	}
	if s.DeniedCount != 0 {
		t.Errorf("deniedCount: got %d, want 0", s.DeniedCount)
	}
}

// TestStatsHandler_afterRecordMsg verifies msgCount increments after recordMsg.
func TestStatsHandler_afterRecordMsg(t *testing.T) {
	st := &statsTracker{}
	st.recordMsg()
	st.recordMsg()

	mux := newMux(st, "consumer")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stats", nil))

	s := decodeStats(t, w)
	if s.MsgCount != 2 {
		t.Errorf("msgCount: got %d, want 2", s.MsgCount)
	}
	if s.LastReceivedAt == "" {
		t.Error("lastReceivedAt should be set")
	}
}

// TestStatsHandler_afterRecordDenied verifies deniedCount increments after recordDenied.
func TestStatsHandler_afterRecordDenied(t *testing.T) {
	st := &statsTracker{}
	st.recordDenied()

	mux := newMux(st, "consumer")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stats", nil))

	s := decodeStats(t, w)
	if s.DeniedCount != 1 {
		t.Errorf("deniedCount: got %d, want 1", s.DeniedCount)
	}
	if s.LastDeniedAt == "" {
		t.Error("lastDeniedAt should be set")
	}
}
