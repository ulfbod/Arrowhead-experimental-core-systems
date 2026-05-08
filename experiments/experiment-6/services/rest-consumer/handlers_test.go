package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// statsResp mirrors the JSON shape of /stats responses.
// Field names must match the keys in statsTracker.snapshot().
type statsResp struct {
	Name           string `json:"name"`
	Transport      string `json:"transport"`
	MsgCount       int64  `json:"msgCount"`
	DeniedCount    int64  `json:"deniedCount"`
	LastReceivedAt string `json:"lastReceivedAt"`
	LastDeniedAt   string `json:"lastDeniedAt"`
}

func decodeStats(t *testing.T, body *httptest.ResponseRecorder) statsResp {
	t.Helper()
	var s statsResp
	if err := json.NewDecoder(body.Body).Decode(&s); err != nil {
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

// TestStatsHandler_shape verifies GET /stats returns all required JSON fields
// with the correct static values (name, transport) before any polls occur.
func TestStatsHandler_shape(t *testing.T) {
	mux := newMux(&statsTracker{}, "my-consumer")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stats", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	s := decodeStats(t, w)

	if s.Name != "my-consumer" {
		t.Errorf("name: got %q, want %q", s.Name, "my-consumer")
	}
	if s.Transport != "rest" {
		t.Errorf("transport: got %q, want %q", s.Transport, "rest")
	}
	if s.MsgCount != 0 {
		t.Errorf("msgCount: got %d, want 0", s.MsgCount)
	}
	if s.DeniedCount != 0 {
		t.Errorf("deniedCount: got %d, want 0", s.DeniedCount)
	}
}

// TestStatsHandler_afterSuccessfulPoll verifies that after poll() returns 200,
// the /stats endpoint reflects msgCount=1 and a non-empty lastReceivedAt.
// This tests the full path: poll → recordMsg → snapshot → JSON encoding → HTTP response.
func TestStatsHandler_afterSuccessfulPoll(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"robotId":"r1","temp":20.5}`))
	}))
	defer upstream.Close()

	st := &statsTracker{}
	if err := poll(&http.Client{}, upstream.URL, "my-consumer", "svc", st); err != nil {
		t.Fatalf("poll: %v", err)
	}

	mux := newMux(st, "my-consumer")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stats", nil))

	s := decodeStats(t, w)
	if s.MsgCount != 1 {
		t.Errorf("msgCount: got %d, want 1", s.MsgCount)
	}
	if s.DeniedCount != 0 {
		t.Errorf("deniedCount: got %d, want 0", s.DeniedCount)
	}
	if s.LastReceivedAt == "" {
		t.Error("lastReceivedAt should be set after a successful poll")
	}
	if s.LastDeniedAt != "" {
		t.Errorf("lastDeniedAt should remain empty after successful poll, got %q", s.LastDeniedAt)
	}
}

// TestStatsHandler_afterDeniedPoll verifies that after poll() receives a 403,
// the /stats endpoint reflects deniedCount=1, a non-empty lastDeniedAt, and
// msgCount=0.  This is the primary observable signal for the dashboard when
// rest-authz denies access.
func TestStatsHandler_afterDeniedPoll(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"denied"}`, http.StatusForbidden)
	}))
	defer upstream.Close()

	st := &statsTracker{}
	poll(&http.Client{}, upstream.URL, "bad-consumer", "svc", st)

	mux := newMux(st, "bad-consumer")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stats", nil))

	s := decodeStats(t, w)
	if s.MsgCount != 0 {
		t.Errorf("msgCount: got %d, want 0", s.MsgCount)
	}
	if s.DeniedCount != 1 {
		t.Errorf("deniedCount: got %d, want 1", s.DeniedCount)
	}
	if s.LastDeniedAt == "" {
		t.Error("lastDeniedAt should be set after a denied poll")
	}
	if s.LastReceivedAt != "" {
		t.Errorf("lastReceivedAt should remain empty after denied poll, got %q", s.LastReceivedAt)
	}
}

// TestStatsHandler_accumulatesAcrossPolls verifies that repeated polls accumulate
// correctly — the counter is additive, not replaced on each call.
func TestStatsHandler_accumulatesAcrossPolls(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	st := &statsTracker{}
	for range 3 {
		poll(&http.Client{}, upstream.URL, "c", "s", st)
	}

	mux := newMux(st, "c")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stats", nil))

	s := decodeStats(t, w)
	if s.MsgCount != 3 {
		t.Errorf("msgCount: got %d, want 3", s.MsgCount)
	}
}
