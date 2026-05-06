package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPoll_200OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify identity headers are present.
		if r.Header.Get("X-Consumer-Name") == "" {
			t.Error("X-Consumer-Name header missing")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"robotId":"robot-1","temp":22.0}`))
	}))
	defer srv.Close()

	st := &statsTracker{}
	client := &http.Client{}

	err := poll(client, srv.URL, "rest-consumer", "telemetry-rest", st)
	if err != nil {
		t.Fatalf("poll: unexpected error: %v", err)
	}
	if st.msgCount.Load() != 1 {
		t.Errorf("msgCount: got %d, want 1", st.msgCount.Load())
	}
	if st.deniedCount.Load() != 0 {
		t.Errorf("deniedCount: got %d, want 0", st.deniedCount.Load())
	}
}

func TestPoll_403Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"not authorized"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	st := &statsTracker{}
	client := &http.Client{}

	err := poll(client, srv.URL, "bad-consumer", "telemetry-rest", st)
	if err != nil {
		t.Fatalf("poll: unexpected error on 403: %v", err)
	}
	if st.deniedCount.Load() != 1 {
		t.Errorf("deniedCount: got %d, want 1", st.deniedCount.Load())
	}
	if st.msgCount.Load() != 0 {
		t.Errorf("msgCount: got %d, want 0", st.msgCount.Load())
	}
}

func TestPoll_unexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	st := &statsTracker{}
	client := &http.Client{}

	err := poll(client, srv.URL, "consumer", "service", st)
	if err == nil {
		t.Error("expected error on 500")
	}
}

func TestPoll_serviceNameHeader(t *testing.T) {
	var gotService string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotService = r.Header.Get("X-Service-Name")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	st := &statsTracker{}
	poll(&http.Client{}, srv.URL, "consumer", "my-service", st)

	if gotService != "my-service" {
		t.Errorf("X-Service-Name: got %q, want my-service", gotService)
	}
}

func TestPoll_unreachableServer(t *testing.T) {
	st := &statsTracker{}
	err := poll(&http.Client{}, "http://127.0.0.1:1", "c", "s", st)
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}
