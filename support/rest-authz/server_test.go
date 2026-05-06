package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	az "arrowhead/authzforce"
)

// xacmlPermit / xacmlDeny are the XML responses the authzforce client parses.
func xacmlPermit() string {
	return `<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">` +
		`<Result><Decision>Permit</Decision></Result></Response>`
}

func xacmlDeny() string {
	return `<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">` +
		`<Result><Decision>Deny</Decision></Result></Response>`
}

// mockPDP creates a stub AuthzForce PDP.
// allowed maps consumer names to permit/deny decisions.
// The mock inspects the raw XACML request body for the consumer name.
func mockPDP(allowed map[string]bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		// Find the first AttributeValue (subject).
		const prefix = `<AttributeValue`
		start := strings.Index(s, prefix)
		subject := ""
		if start >= 0 {
			inner := s[start:]
			gt := strings.Index(inner, ">")
			lt := strings.Index(inner[gt:], "<")
			if gt >= 0 && lt >= 0 {
				subject = inner[gt+1 : gt+1+lt]
			}
		}
		w.Header().Set("Content-Type", "application/xml")
		if allowed[subject] {
			w.Write([]byte(xacmlPermit()))
		} else {
			w.Write([]byte(xacmlDeny()))
		}
	}))
}

// newTestServer wires a authzServer to a httptest.Server.
func newTestServer(t *testing.T, allowed map[string]bool, upstreamURL string) (*authzServer, *httptest.Server, func()) {
	t.Helper()
	pdp := mockPDP(allowed)

	cfg := config{
		azDomainID:     "test-domain",
		upstreamURL:    upstreamURL,
		defaultService: "telemetry-rest",
		port:           "0",
	}
	client := az.New(pdp.URL)
	cache := newDecisionCache(0) // no caching
	srv := newAuthzServer(cfg, client, cache)

	mux := http.NewServeMux()
	srv.register(mux)
	ts := httptest.NewServer(mux)

	return srv, ts, func() {
		ts.Close()
		pdp.Close()
	}
}

// ── /health ───────────────────────────────────────────────────────────────────

func TestHealth_200(t *testing.T) {
	_, ts, cleanup := newTestServer(t, nil, "http://unused")
	defer cleanup()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}
}

// ── /status ───────────────────────────────────────────────────────────────────

func TestStatus_counters(t *testing.T) {
	_, ts, cleanup := newTestServer(t, map[string]bool{"consumer-a": true}, "http://unused")
	defer cleanup()

	resp, err := http.Get(ts.URL + "/status")
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}

	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"requestsTotal", "permitted", "denied"} {
		if _, ok := m[key]; !ok {
			t.Errorf("status missing key %q", key)
		}
	}
}

// ── /auth/check ───────────────────────────────────────────────────────────────

func TestAuthCheck_permit(t *testing.T) {
	_, ts, cleanup := newTestServer(t, map[string]bool{"consumer-1": true}, "http://unused")
	defer cleanup()

	body := `{"consumer":"consumer-1","service":"telemetry"}`
	resp, err := http.Post(ts.URL+"/auth/check", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /auth/check: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}

	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m["permit"] != true {
		t.Errorf("expected permit=true, got %v", m["permit"])
	}
	if m["decision"] != "Permit" {
		t.Errorf("expected decision=Permit, got %v", m["decision"])
	}
}

func TestAuthCheck_deny(t *testing.T) {
	_, ts, cleanup := newTestServer(t, map[string]bool{}, "http://unused")
	defer cleanup()

	body := `{"consumer":"bad-consumer","service":"telemetry"}`
	resp, err := http.Post(ts.URL+"/auth/check", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /auth/check: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}

	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m["permit"] != false {
		t.Errorf("expected permit=false, got %v", m["permit"])
	}
}

func TestAuthCheck_missingFields(t *testing.T) {
	_, ts, cleanup := newTestServer(t, nil, "http://unused")
	defer cleanup()

	resp, err := http.Post(ts.URL+"/auth/check", "application/json", strings.NewReader(`{"consumer":"only"}`))
	if err != nil {
		t.Fatalf("POST /auth/check: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("got %d, want 400", resp.StatusCode)
	}
}

func TestAuthCheck_rejectGET(t *testing.T) {
	_, ts, cleanup := newTestServer(t, nil, "http://unused")
	defer cleanup()

	resp, err := http.Get(ts.URL + "/auth/check")
	if err != nil {
		t.Fatalf("GET /auth/check: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("got %d, want 405", resp.StatusCode)
	}
}

// ── proxy (handleProxy) ───────────────────────────────────────────────────────

func TestProxy_permitted(t *testing.T) {
	// Create a mock upstream that returns 200 with a known body.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify identity headers are stripped.
		if r.Header.Get("X-Consumer-Name") != "" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("X-Consumer-Name leaked upstream"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":"telemetry"}`))
	}))
	defer upstream.Close()

	_, ts, cleanup := newTestServer(t, map[string]bool{"consumer-allowed": true}, upstream.URL)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/telemetry/latest", nil)
	req.Header.Set("X-Consumer-Name", "consumer-allowed")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "telemetry") {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestProxy_denied(t *testing.T) {
	_, ts, cleanup := newTestServer(t, map[string]bool{}, "http://unused")
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/telemetry/latest", nil)
	req.Header.Set("X-Consumer-Name", "bad-consumer")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("got %d, want 403", resp.StatusCode)
	}
}

func TestProxy_missingConsumer(t *testing.T) {
	_, ts, cleanup := newTestServer(t, nil, "http://unused")
	defer cleanup()

	resp, err := http.Get(ts.URL + "/telemetry/latest")
	if err != nil {
		t.Fatalf("GET without consumer: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", resp.StatusCode)
	}
}

func TestProxy_consumerViaQueryParam(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// consumer param should be stripped
		if r.URL.Query().Get("consumer") != "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	_, ts, cleanup := newTestServer(t, map[string]bool{"qconsumer": true}, upstream.URL)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/telemetry/latest?consumer=qconsumer")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}
}

// ── stripParam ────────────────────────────────────────────────────────────────

func TestStripParam(t *testing.T) {
	cases := []struct {
		raw  string
		key  string
		want string
	}{
		{"consumer=alice&format=json", "consumer", "format=json"},
		{"format=json&consumer=alice", "consumer", "format=json"},
		{"consumer=alice", "consumer", ""},
		{"format=json", "consumer", "format=json"},
		{"", "consumer", ""},
		{"a=1&b=2&c=3", "b", "a=1&c=3"},
	}
	for _, c := range cases {
		got := stripParam(c.raw, c.key)
		if got != c.want {
			t.Errorf("stripParam(%q, %q) = %q, want %q", c.raw, c.key, got, c.want)
		}
	}
}

// ── stats counter increments ──────────────────────────────────────────────────

func TestProxyStatsCounters(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	srv, ts, cleanup := newTestServer(t, map[string]bool{"good": true}, upstream.URL)
	defer cleanup()

	// One permitted request.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/path", nil)
	req.Header.Set("X-Consumer-Name", "good")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	// One denied request.
	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/path", nil)
	req2.Header.Set("X-Consumer-Name", "bad")
	resp2, _ := http.DefaultClient.Do(req2)
	resp2.Body.Close()

	if srv.stats.total.Load() != 2 {
		t.Errorf("total: got %d, want 2", srv.stats.total.Load())
	}
	if srv.stats.permitted.Load() != 1 {
		t.Errorf("permitted: got %d, want 1", srv.stats.permitted.Load())
	}
	if srv.stats.denied.Load() != 1 {
		t.Errorf("denied: got %d, want 1", srv.stats.denied.Load())
	}
}
