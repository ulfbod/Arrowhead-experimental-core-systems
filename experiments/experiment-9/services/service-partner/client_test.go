package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ── Stats tests ───────────────────────────────────────────────────────────────

func TestStats_Initial(t *testing.T) {
	s := NewStats("rest-mtls-pki")
	snap := s.Snapshot()
	if snap["msgCount"].(int64) != 0 {
		t.Errorf("msgCount = %v, want 0", snap["msgCount"])
	}
	if snap["transport"] != "rest-mtls-pki" {
		t.Errorf("transport = %q, want rest-mtls-pki", snap["transport"])
	}
	if snap["lastDeniedAt"] != "" {
		t.Errorf("lastDeniedAt = %q, want empty", snap["lastDeniedAt"])
	}
}

func TestStats_RecordSuccess(t *testing.T) {
	s := NewStats("t")
	s.recordSuccess()
	s.recordSuccess()
	snap := s.Snapshot()
	if snap["msgCount"].(int64) != 2 {
		t.Errorf("msgCount = %v, want 2", snap["msgCount"])
	}
	if snap["lastReceived"].(string) == "" {
		t.Errorf("lastReceived empty after recordSuccess")
	}
}

func TestStats_RecordDenied(t *testing.T) {
	s := NewStats("t")
	s.recordDenied()
	snap := s.Snapshot()
	if snap["deniedCount"].(int64) != 1 {
		t.Errorf("deniedCount = %v, want 1", snap["deniedCount"])
	}
	if snap["lastDeniedAt"].(string) == "" {
		t.Errorf("lastDeniedAt empty after recordDenied")
	}
}

// ── PollClient tests (using httptest) ─────────────────────────────────────────

// makePollClient creates a PollClient pointing at the given server URL.
// Uses empty cert pool (server cert verification will be skipped via httptest).
func makePollClient(t *testing.T, srv *httptest.Server, serviceName string) *PollClient {
	t.Helper()
	// For plain httptest.Server (HTTP), use a client with no TLS.
	return &PollClient{
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec — test-only server
				},
			},
			Timeout: 5 * time.Second,
		},
		targetURL:   srv.URL,
		serviceName: serviceName,
		stats:       NewStats("rest-mtls-pki"),
	}
}

func TestPollClient_PollOnce_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/telemetry/latest" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"site":"s1","val":7}`)
	}))
	defer srv.Close()

	c := makePollClient(t, srv, "telemetry-rest")
	body, err := c.PollOnce()
	if err != nil {
		t.Fatalf("PollOnce() error: %v", err)
	}
	if string(body) != `{"site":"s1","val":7}` {
		t.Errorf("body = %q", string(body))
	}
	if c.stats.msgCount.Load() != 1 {
		t.Errorf("msgCount = %d, want 1", c.stats.msgCount.Load())
	}
}

func TestPollClient_PollOnce_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not authorized", http.StatusForbidden)
	}))
	defer srv.Close()

	c := makePollClient(t, srv, "telemetry-rest")
	_, err := c.PollOnce()
	if err == nil {
		t.Errorf("PollOnce() expected error on 403, got nil")
	}
	if c.stats.deniedCount.Load() != 1 {
		t.Errorf("deniedCount = %d, want 1", c.stats.deniedCount.Load())
	}
}

func TestPollClient_PollOnce_SendsXServiceName(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Service-Name")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c := makePollClient(t, srv, "my-service")
	c.PollOnce()
	if gotHeader != "my-service" {
		t.Errorf("X-Service-Name = %q, want my-service", gotHeader)
	}
}

func TestPollClient_PollOnce_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := makePollClient(t, srv, "")
	_, err := c.PollOnce()
	if err == nil {
		t.Errorf("PollOnce() expected error on 503, got nil")
	}
}

func TestPollClient_RunPoll(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c := makePollClient(t, srv, "")
	done := make(chan struct{})
	go c.RunPoll(50*time.Millisecond, done)

	time.Sleep(200 * time.Millisecond)
	close(done)

	if calls < 2 {
		t.Errorf("RunPoll made %d calls, want >= 2", calls)
	}
}

// ── Handler tests ─────────────────────────────────────────────────────────────

func TestHandleHealth(t *testing.T) {
	h := HandleHealth("sp1")
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["partner"] != "sp1" {
		t.Errorf("partner = %q, want sp1", resp["partner"])
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}
}

func TestHandleStats(t *testing.T) {
	s := NewStats("rest-mtls-pki")
	s.recordSuccess()
	h := HandleStats(s)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/stats", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var snap map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if snap["msgCount"].(float64) != 1 {
		t.Errorf("msgCount = %v, want 1", snap["msgCount"])
	}
}

// ── NewPollClient smoke test ──────────────────────────────────────────────────

func TestNewPollClient_SetsFields(t *testing.T) {
	cert := tls.Certificate{}
	pool := x509.NewCertPool()
	c := NewPollClient(cert, pool, "https://example.com", "my-svc")

	if c.targetURL != "https://example.com" {
		t.Errorf("targetURL = %q, want https://example.com", c.targetURL)
	}
	if c.serviceName != "my-svc" {
		t.Errorf("serviceName = %q, want my-svc", c.serviceName)
	}
	if c.client == nil {
		t.Errorf("client is nil")
	}
	if c.stats == nil {
		t.Errorf("stats is nil")
	}
}
