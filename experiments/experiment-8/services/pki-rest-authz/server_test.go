package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	az "arrowhead/authzforce"
)

// ── XACML mock helpers ────────────────────────────────────────────────────────

func xacmlPermit() string {
	return `<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">` +
		`<Result><Decision>Permit</Decision></Result></Response>`
}

func xacmlDeny() string {
	return `<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">` +
		`<Result><Decision>Deny</Decision></Result></Response>`
}

// mockPDP creates a stub AuthzForce PDP.
// It extracts the subject (first AttributeValue text content) from the XACML body
// and looks it up in the allowed map.
func mockPDP(allowed map[string]bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		// Find the first AttributeValue element and extract its text content.
		// XACML format: <AttributeValue DataType="...">subject-name</AttributeValue>
		const prefix = `<AttributeValue`
		start := strings.Index(s, prefix)
		subject := ""
		if start >= 0 {
			inner := s[start:]
			gt := strings.Index(inner, ">")   // position of closing > of opening tag
			if gt >= 0 {
				after := inner[gt+1:]              // content after the >
				lt := strings.Index(after, "<")    // position of < ending the text content
				if lt >= 0 {
					subject = after[:lt]
				}
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

// newTestCertAuthzServer wires a certAuthzServer backed by a mock PDP.
func newTestCertAuthzServer(t *testing.T, allowed map[string]bool, upstreamURL string) (*certAuthzServer, func()) {
	t.Helper()
	pdp := mockPDP(allowed)
	cfg := serverConfig{
		azDomainID:     "test-domain",
		upstreamURL:    upstreamURL,
		defaultService: "telemetry-rest",
		port:           "0",
		tlsPort:        "0",
	}
	client := az.New(pdp.URL)
	cache := newDecisionCache(0)
	srv := newCertAuthzServer(cfg, client, cache, http.DefaultClient)
	return srv, pdp.Close
}

// ── /health ───────────────────────────────────────────────────────────────────

func TestHealthHandler(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, nil, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerPlain(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`status: got %q, want "ok"`, body["status"])
	}
}

// ── /status ───────────────────────────────────────────────────────────────────

func TestStatusHandler(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, nil, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerPlain(mux)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	var m map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"requestsTotal", "permitted", "denied"} {
		if _, ok := m[key]; !ok {
			t.Errorf("status missing key %q", key)
		}
	}
}

// ── /auth/check ───────────────────────────────────────────────────────────────

func TestAuthCheckHandler_permit(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{"consumer-1": true}, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerPlain(mux)

	body := `{"consumer":"consumer-1","service":"telemetry"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	var m map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&m)
	if m["permit"] != true {
		t.Errorf("expected permit=true, got %v", m["permit"])
	}
	if m["decision"] != "Permit" {
		t.Errorf("expected decision=Permit, got %v", m["decision"])
	}
}

func TestAuthCheckHandler_deny(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{}, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerPlain(mux)

	body := `{"consumer":"bad-consumer","service":"telemetry"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	var m map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&m)
	if m["permit"] != false {
		t.Errorf("expected permit=false, got %v", m["permit"])
	}
}

// ── mTLS proxy handler tests ──────────────────────────────────────────────────

// makeSelfSignedCert creates an in-memory self-signed certificate with the given CN.
func makeSelfSignedCert(t *testing.T, cn string) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert, key
}

// makeTLSCert builds a tls.Certificate for a self-signed cert.
func makeTLSCert(t *testing.T, cn string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert DER: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	return tlsCert
}

// TestMTLSProxy_noCert verifies that a request without a client certificate → 401.
func TestMTLSProxy_noCert(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, nil, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerMTLS(mux)

	// Request has no TLS state at all.
	req := httptest.NewRequest(http.MethodGet, "/telemetry/latest", nil)
	req.TLS = nil
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rec.Code)
	}
}

// TestMTLSProxy_permit verifies that a request with a permitted CN → proxy response.
func TestMTLSProxy_permit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":"telemetry"}`))
	}))
	defer upstream.Close()

	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{"pki-consumer": true}, upstream.URL)
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerMTLS(mux)

	// Build a request with fake TLS state containing a peer cert with the permitted CN.
	clientCert, _ := makeSelfSignedCert(t, "pki-consumer")
	req := httptest.NewRequest(http.MethodGet, "/telemetry/latest", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{clientCert},
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "telemetry") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

// TestMTLSProxy_deny verifies that a request with a denied CN → 403.
func TestMTLSProxy_deny(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{}, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerMTLS(mux)

	clientCert, _ := makeSelfSignedCert(t, "bad-consumer")
	req := httptest.NewRequest(http.MethodGet, "/telemetry/latest", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{clientCert},
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rec.Code)
	}
}

// ── proxyRequest tests ────────────────────────────────────────────────────────

func TestProxyRequest_upstreamError(t *testing.T) {
	// Use a closed server to trigger upstream error
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	upstream.Close() // close immediately

	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{"x": true}, upstream.URL)
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerMTLS(mux)

	clientCert, _ := makeSelfSignedCert(t, "x")
	req := httptest.NewRequest(http.MethodGet, "/telemetry/latest", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{clientCert}}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("got %d, want 502", rec.Code)
	}
}

func TestMTLSProxy_emptyCN(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, nil, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerMTLS(mux)

	// Cert with empty CN
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(99),
		Subject:      pkix.Name{CommonName: ""},
		NotBefore:    timeNow().Add(-time.Hour),
		NotAfter:     timeNow().Add(time.Hour),
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	emptyCNCert, _ := x509.ParseCertificate(der)

	req := httptest.NewRequest(http.MethodGet, "/telemetry/latest", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{emptyCNCert}}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rec.Code)
	}
}

// ── decision cache ─────────────────────────────────────────────────────────────

func TestDecisionCache_noTTL(t *testing.T) {
	c := newDecisionCache(0)
	c.set("s", "r", "a", true)
	_, ok := c.get("s", "r", "a")
	if ok {
		t.Error("cache with TTL=0 should never hit")
	}
}

func TestDecisionCache_withTTL(t *testing.T) {
	c := newDecisionCache(time.Hour)
	c.set("s", "r", "a", true)
	permit, ok := c.get("s", "r", "a")
	if !ok {
		t.Error("expected cache hit")
	}
	if !permit {
		t.Error("expected permit=true")
	}
}

func TestDecisionCache_miss(t *testing.T) {
	c := newDecisionCache(time.Hour)
	_, ok := c.get("unknown", "r", "a")
	if ok {
		t.Error("expected cache miss")
	}
}

// ── /auth/check error paths ────────────────────────────────────────────────────

func TestAuthCheckHandler_missingFields(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, nil, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerPlain(mux)

	// Missing consumer
	body := `{"service":"telemetry"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/check", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing consumer, got %d", rec.Code)
	}
}

func TestAuthCheckHandler_methodNotAllowed(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, nil, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerPlain(mux)

	req := httptest.NewRequest(http.MethodGet, "/auth/check", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestAuthCheckHandler_badJSON(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, nil, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerPlain(mux)

	req := httptest.NewRequest(http.MethodPost, "/auth/check", strings.NewReader("not-json"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ── proxyRequest with query string ────────────────────────────────────────────

func TestMTLSProxy_queryString(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "robot=r1" {
			t.Errorf("expected query robot=r1, got %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{"pki-consumer": true}, upstream.URL)
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerMTLS(mux)

	clientCert, _ := makeSelfSignedCert(t, "pki-consumer")
	req := httptest.NewRequest(http.MethodGet, "/telemetry/latest?robot=r1", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{clientCert}}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

// ── decide with cache ────────────────────────────────────────────────────────

func TestDecide_cachedPermit(t *testing.T) {
	pdp := mockPDP(map[string]bool{"cached-consumer": true})
	defer pdp.Close()

	cfg := serverConfig{
		azDomainID:     "test-domain",
		upstreamURL:    "http://unused",
		defaultService: "telemetry",
	}
	client := az.New(pdp.URL)
	cache := newDecisionCache(time.Hour) // TTL > 0 to enable caching
	srv := newCertAuthzServer(cfg, client, cache, http.DefaultClient)

	// First call: populate cache
	permit, err := srv.decide(nil, "cached-consumer", "telemetry", "invoke")
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if !permit {
		t.Error("expected permit=true")
	}

	// Second call: should hit cache (PDP would still say Permit but cache is checked first)
	permit2, err := srv.decide(nil, "cached-consumer", "telemetry", "invoke")
	if err != nil {
		t.Fatalf("decide (cached): %v", err)
	}
	if !permit2 {
		t.Error("expected cached permit=true")
	}
}

// timeNow is an alias used in tests only.
var timeNow = time.Now

// Keep rand import used by helpers.
var _ = rand.Reader
