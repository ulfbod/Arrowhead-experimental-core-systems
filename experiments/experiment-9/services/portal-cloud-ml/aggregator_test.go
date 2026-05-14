package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── Store tests ───────────────────────────────────────────────────────────────

func TestStore_InitialState(t *testing.T) {
	s := NewStore()
	if got := s.Latest(); got != nil {
		t.Errorf("Latest() = %q, want nil", got)
	}
	stats := s.Stats()
	if stats["msgCount"].(int64) != 0 {
		t.Errorf("msgCount = %v, want 0", stats["msgCount"])
	}
	if stats["transport"] != "kafka-sse" {
		t.Errorf("transport = %q, want kafka-sse", stats["transport"])
	}
}

func TestStore_Record(t *testing.T) {
	s := NewStore()
	payload := []byte(`{"robot":"r1","val":42}`)
	s.Record(payload)

	got := s.Latest()
	if string(got) != string(payload) {
		t.Errorf("Latest() = %q, want %q", got, payload)
	}
	stats := s.Stats()
	if stats["msgCount"].(int64) != 1 {
		t.Errorf("msgCount = %v, want 1", stats["msgCount"])
	}
	if stats["lastReceivedAt"].(string) == "" {
		t.Errorf("lastReceivedAt is empty after Record")
	}
}

func TestStore_RecordMultiple(t *testing.T) {
	s := NewStore()
	for i := 0; i < 5; i++ {
		s.Record([]byte(fmt.Sprintf(`{"n":%d}`, i)))
	}
	stats := s.Stats()
	if stats["msgCount"].(int64) != 5 {
		t.Errorf("msgCount = %v, want 5", stats["msgCount"])
	}
	got := string(s.Latest())
	if got != `{"n":4}` {
		t.Errorf("Latest() after 5 records = %q, want %q", got, `{"n":4}`)
	}
}

func TestStore_LatestIsIsolated(t *testing.T) {
	s := NewStore()
	payload := []byte(`{"x":1}`)
	s.Record(payload)

	// Mutate the returned slice.
	got := s.Latest()
	got[0] = 'Z'

	// Original must be unchanged.
	got2 := s.Latest()
	if got2[0] == 'Z' {
		t.Errorf("Latest() not isolated — mutation visible in store")
	}
}

// ── ParseSSELine tests ────────────────────────────────────────────────────────

func TestParseSSELine(t *testing.T) {
	tests := []struct {
		line    string
		payload string
		ok      bool
	}{
		{"data: {\"x\":1}", `{"x":1}`, true},
		{"data: hello", "hello", true},
		{"data: ", "", false},
		{": comment", "", false},
		{"event: telemetry", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		p, ok := ParseSSELine(tc.line)
		if ok != tc.ok || p != tc.payload {
			t.Errorf("ParseSSELine(%q) = (%q, %v), want (%q, %v)",
				tc.line, p, ok, tc.payload, tc.ok)
		}
	}
}

// ── ConnectSSE integration test ───────────────────────────────────────────────

func TestConnectSSE_ReadsDataLines(t *testing.T) {
	// Serve a short SSE stream.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"site\":\"s1\"}\n\n")
		fmt.Fprint(w, "data: {\"site\":\"s2\"}\n\n")
		// Close immediately.
	}))
	defer srv.Close()

	store := NewStore()
	done := make(chan struct{})
	go ConnectSSE(srv.URL, "test-consumer", "telemetry", store, done)

	// Give goroutine time to consume the stream and reconnect once.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if store.msgCount.Load() >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	close(done)

	if store.msgCount.Load() < 2 {
		t.Errorf("msgCount = %d, want >= 2", store.msgCount.Load())
	}
}

func TestConnectSSE_Denied403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not authorized", http.StatusForbidden)
	}))
	defer srv.Close()

	store := NewStore()
	done := make(chan struct{})
	go ConnectSSE(srv.URL, "bad-consumer", "telemetry", store, done)

	// Wait briefly for a denied count.
	time.Sleep(200 * time.Millisecond)
	close(done)

	if store.deniedCount.Load() == 0 {
		t.Errorf("deniedCount = 0, want > 0 after 403 response")
	}
}

func TestConnectSSE_ClosedDoneBeforeConnect(t *testing.T) {
	// If done is already closed, ConnectSSE should return immediately.
	store := NewStore()
	done := make(chan struct{})
	close(done)

	// Should not block.
	returned := make(chan struct{})
	go func() {
		ConnectSSE("http://127.0.0.1:1", "c", "s", store, done)
		close(returned)
	}()

	select {
	case <-returned:
		// Expected.
	case <-time.After(3 * time.Second):
		t.Errorf("ConnectSSE did not return after done was closed")
	}
}

func TestConnectSSE_URLContainsConsumerAndService(t *testing.T) {
	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "text/event-stream")
		// Send one data line then close.
		fmt.Fprint(w, "data: {}\n\n")
	}))
	defer srv.Close()

	store := NewStore()
	done := make(chan struct{})
	go ConnectSSE(srv.URL, "my-consumer", "my-service", store, done)

	time.Sleep(200 * time.Millisecond)
	close(done)

	if !strings.Contains(capturedURL, "/stream/my-consumer") {
		t.Errorf("URL = %q, want /stream/my-consumer", capturedURL)
	}
	if !strings.Contains(capturedURL, "service=my-service") {
		t.Errorf("URL = %q, want service=my-service", capturedURL)
	}
}

func TestConnectSSE_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	store := NewStore()
	done := make(chan struct{})
	go ConnectSSE(srv.URL, "c", "s", store, done)
	time.Sleep(150 * time.Millisecond)
	close(done)
	// Should not panic; just reconnect.
}

func TestConnectSSE_MultipleMessages(t *testing.T) {
	// Server sends multiple data lines; all should be counted.
	const numMsg = 10
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < numMsg; i++ {
			fmt.Fprintf(w, "data: {\"n\":%d}\n\n", i)
		}
	}))
	defer srv.Close()

	store := NewStore()
	done := make(chan struct{})
	go ConnectSSE(srv.URL, "c", "s", store, done)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if store.msgCount.Load() >= numMsg {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	close(done)

	if store.msgCount.Load() < numMsg {
		t.Errorf("msgCount = %d, want >= %d", store.msgCount.Load(), numMsg)
	}
}
