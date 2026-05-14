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
func mockPDP(allowed map[string]bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		const prefix = `<AttributeValue`
		start := strings.Index(s, prefix)
		subject := ""
		if start >= 0 {
			inner := s[start:]
			gt := strings.Index(inner, ">")
			if gt >= 0 {
				after := inner[gt+1:]
				lt := strings.Index(after, "<")
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

// mockPIP creates a stub PIP server.
func mockPIP(attrs map[string]subjectAttrs) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/pip/attributes/")
		a, ok := attrs[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"systemName": name,
			"certLevel":  a.CertLevel,
			"valid":      a.CertValid,
		})
	}))
}

// newTestCertAuthzServer wires a certAuthzServer backed by mock PDP and PIP.
func newTestCertAuthzServer(t *testing.T, allowed map[string]bool, pipAttrs map[string]subjectAttrs, upstreamURL string) (*certAuthzServer, func()) {
	t.Helper()
	pdp := mockPDP(allowed)
	pip := mockPIP(pipAttrs)
	cfg := serverConfig{
		azDomainID:     "test-domain",
		azURL:          pdp.URL,
		pipURL:         pip.URL,
		upstreamURL:    upstreamURL,
		defaultService: "telemetry-rest",
		port:           "0",
		tlsPort:        "0",
	}
	cache := newDecisionCache(0)
	srv := newCertAuthzServer(cfg, cache, http.DefaultClient)
	return srv, func() {
		pdp.Close()
		pip.Close()
	}
}

// ── /health ───────────────────────────────────────────────────────────────────

func TestHealthHandler(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, nil, nil, "http://unused")
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
	srv, cleanup := newTestCertAuthzServer(t, nil, nil, "http://unused")
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
	pipAttrs := map[string]subjectAttrs{"consumer-1": {CertLevel: "sy", CertValid: true}}
	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{"consumer-1": true}, pipAttrs, "http://unused")
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
	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{}, nil, "http://unused")
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
	srv, cleanup := newTestCertAuthzServer(t, nil, nil, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerMTLS(mux)

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

	pipAttrs := map[string]subjectAttrs{"pki-consumer": {CertLevel: "sy", CertValid: true}}
	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{"pki-consumer": true}, pipAttrs, upstream.URL)
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerMTLS(mux)

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
	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{}, nil, "http://unused")
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

// TestMTLSProxy_EnrichesXACMLWithCertLevel verifies PIP is queried before AuthzForce
// and cert-level attributes appear in the XACML request.
func TestMTLSProxy_EnrichesXACMLWithCertLevel(t *testing.T) {
	var xacmlBody string
	pipQueried := false

	pdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		xacmlBody = string(body)
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xacmlDeny()))
	}))
	defer pdp.Close()

	pip := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pipQueried = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"systemName": "pki-consumer",
			"certLevel":  "sy",
			"valid":      true,
		})
	}))
	defer pip.Close()

	cfg := serverConfig{
		azDomainID:     "test-domain",
		azURL:          pdp.URL,
		pipURL:         pip.URL,
		upstreamURL:    "http://unused",
		defaultService: "telemetry-rest",
	}
	srv := newCertAuthzServer(cfg, newDecisionCache(0), http.DefaultClient)

	mux := http.NewServeMux()
	srv.registerMTLS(mux)

	clientCert, _ := makeSelfSignedCert(t, "pki-consumer")
	req := httptest.NewRequest(http.MethodGet, "/telemetry/latest", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{clientCert}}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (Deny), got %d", rec.Code)
	}
	if !pipQueried {
		t.Error("expected PIP to be queried before AuthzForce, but it was not")
	}
	if !strings.Contains(xacmlBody, "urn:arrowhead:attribute:cert-level") {
		t.Error("expected cert-level attribute in XACML request, not found")
	}
	if !strings.Contains(xacmlBody, "urn:arrowhead:attribute:cert-valid") {
		t.Error("expected cert-valid attribute in XACML request, not found")
	}
	if !strings.Contains(xacmlBody, ">sy<") {
		t.Errorf("expected cert level value 'sy' in XACML request; body: %s", xacmlBody)
	}
}

// ── proxyRequest tests ────────────────────────────────────────────────────────

func TestProxyRequest_upstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	upstream.Close()

	pipAttrs := map[string]subjectAttrs{"x": {CertLevel: "sy", CertValid: true}}
	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{"x": true}, pipAttrs, upstream.URL)
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
	srv, cleanup := newTestCertAuthzServer(t, nil, nil, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerMTLS(mux)

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
	srv, cleanup := newTestCertAuthzServer(t, nil, nil, "http://unused")
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerPlain(mux)

	body := `{"service":"telemetry"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/check", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing consumer, got %d", rec.Code)
	}
}

func TestAuthCheckHandler_methodNotAllowed(t *testing.T) {
	srv, cleanup := newTestCertAuthzServer(t, nil, nil, "http://unused")
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
	srv, cleanup := newTestCertAuthzServer(t, nil, nil, "http://unused")
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

	pipAttrs := map[string]subjectAttrs{"pki-consumer": {CertLevel: "sy", CertValid: true}}
	srv, cleanup := newTestCertAuthzServer(t, map[string]bool{"pki-consumer": true}, pipAttrs, upstream.URL)
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

// timeNow is an alias used in tests only.
var timeNow = time.Now

// Keep rand import used by helpers.
var _ = rand.Reader
