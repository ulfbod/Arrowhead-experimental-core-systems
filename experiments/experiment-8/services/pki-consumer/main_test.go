package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── statsTracker ───────────────────────────────────────────────────────────────

func TestStatsTracker_initial(t *testing.T) {
	st := &statsTracker{}
	snap := st.snapshot("test-consumer")
	if snap["name"] != "test-consumer" {
		t.Errorf("expected name=test-consumer, got %v", snap["name"])
	}
	if snap["transport"] != "rest-mtls-pki" {
		t.Errorf("expected transport=rest-mtls-pki, got %v", snap["transport"])
	}
	if snap["msgCount"] != int64(0) {
		t.Errorf("expected msgCount=0, got %v", snap["msgCount"])
	}
	if snap["deniedCount"] != int64(0) {
		t.Errorf("expected deniedCount=0, got %v", snap["deniedCount"])
	}
}

func TestStatsTracker_recordMsg(t *testing.T) {
	st := &statsTracker{}
	st.recordMsg()
	st.recordMsg()
	if st.msgCount.Load() != 2 {
		t.Errorf("expected msgCount=2, got %d", st.msgCount.Load())
	}
	snap := st.snapshot("x")
	if snap["msgCount"] != int64(2) {
		t.Errorf("snapshot msgCount mismatch: %v", snap["msgCount"])
	}
	if snap["lastReceivedAt"] == "" {
		t.Error("expected non-empty lastReceivedAt")
	}
}

func TestStatsTracker_recordDenied(t *testing.T) {
	st := &statsTracker{}
	st.recordDenied()
	if st.deniedCount.Load() != 1 {
		t.Errorf("expected deniedCount=1, got %d", st.deniedCount.Load())
	}
	snap := st.snapshot("x")
	if snap["lastDeniedAt"] == "" {
		t.Error("expected non-empty lastDeniedAt")
	}
}

// ── poll ──────────────────────────────────────────────────────────────────────

func TestPoll_200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"robotId":"r1"}`)
	}))
	defer srv.Close()

	st := &statsTracker{}
	if err := poll(http.DefaultClient, srv.URL, "telemetry-rest", st); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if st.msgCount.Load() != 1 {
		t.Errorf("expected msgCount=1, got %d", st.msgCount.Load())
	}
}

func TestPoll_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"not authorized"}`)
	}))
	defer srv.Close()

	st := &statsTracker{}
	if err := poll(http.DefaultClient, srv.URL, "telemetry-rest", st); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if st.deniedCount.Load() != 1 {
		t.Errorf("expected deniedCount=1, got %d", st.deniedCount.Load())
	}
}

func TestPoll_unexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"crash"}`)
	}))
	defer srv.Close()

	st := &statsTracker{}
	err := poll(http.DefaultClient, srv.URL, "telemetry-rest", st)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestPoll_ServiceNameHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Name") != "my-service" {
			t.Errorf("expected X-Service-Name=my-service, got %s", r.Header.Get("X-Service-Name"))
		}
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	st := &statsTracker{}
	poll(http.DefaultClient, srv.URL, "my-service", st) //nolint:errcheck
}

// ── /health and /stats handlers ───────────────────────────────────────────────

func TestNewMux_health(t *testing.T) {
	st := &statsTracker{}
	mux := newMux(st, "test-consumer")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Errorf("expected ok in body, got %s", w.Body.String())
	}
}

func TestNewMux_stats(t *testing.T) {
	st := &statsTracker{}
	st.recordMsg()
	mux := newMux(st, "test-consumer")
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var snap map[string]interface{}
	json.NewDecoder(w.Body).Decode(&snap)
	if snap["name"] != "test-consumer" {
		t.Errorf("expected name=test-consumer, got %v", snap["name"])
	}
	if snap["msgCount"] != float64(1) {
		t.Errorf("expected msgCount=1, got %v", snap["msgCount"])
	}
}

func TestEnvOr_default(t *testing.T) {
	val := envOr("__NONEXISTENT_ENV_VAR__", "default-value")
	if val != "default-value" {
		t.Errorf("expected default-value, got %s", val)
	}
}
