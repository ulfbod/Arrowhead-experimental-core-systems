package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── mock HTTPS server helpers ─────────────────────────────────────────────────

// generateTestCert creates a self-signed TLS certificate for test servers.
// The certificate includes 127.0.0.1 as an IP SAN so that connections to
// the httptest server (which binds on 127.0.0.1) can be verified.
// Returns the tls.Certificate and a CertPool that trusts it.
func generateTestCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "test-server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
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

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)

	return tlsCert, pool
}

// newTLSTestServer creates an httptest.Server with TLS, using the given handler.
// The returned client is pre-configured to trust the server's self-signed cert.
func newTLSTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	tlsCert, pool := generateTestCert(t)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}
	srv.StartTLS()

	// Client that trusts the test server's self-signed cert.
	// (No client cert needed for these tests since we're testing poll() directly.)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: pool,
			},
		},
		Timeout: 5 * time.Second,
	}
	return srv, client
}

// ── poll() tests ──────────────────────────────────────────────────────────────

// TestPoll_200OK verifies that a 200 response calls recordMsg.
func TestPoll_200OK(t *testing.T) {
	srv, client := newTLSTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify no X-Consumer-Name header is sent (identity from cert, not header).
		if r.Header.Get("X-Consumer-Name") != "" {
			t.Errorf("poll should not set X-Consumer-Name header")
		}
		w.Write([]byte(`{"robotId":"robot-1"}`))
	})
	defer srv.Close()

	st := &statsTracker{}
	err := poll(client, srv.URL, "telemetry-rest", st)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if st.msgCount.Load() != 1 {
		t.Errorf("msgCount: got %d, want 1", st.msgCount.Load())
	}
	if st.deniedCount.Load() != 0 {
		t.Errorf("deniedCount: got %d, want 0", st.deniedCount.Load())
	}
	st.mu.Lock()
	last := st.lastReceivedAt
	st.mu.Unlock()
	if last == "" {
		t.Error("lastReceivedAt should be set after 200 OK")
	}
}

// TestPoll_403Forbidden verifies that a 403 response calls recordDenied.
func TestPoll_403Forbidden(t *testing.T) {
	srv, client := newTLSTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"not authorized"}`, http.StatusForbidden)
	})
	defer srv.Close()

	st := &statsTracker{}
	err := poll(client, srv.URL, "telemetry-rest", st)
	if err != nil {
		t.Fatalf("poll returned unexpected error on 403: %v", err)
	}
	if st.msgCount.Load() != 0 {
		t.Errorf("msgCount: got %d, want 0", st.msgCount.Load())
	}
	if st.deniedCount.Load() != 1 {
		t.Errorf("deniedCount: got %d, want 1", st.deniedCount.Load())
	}
	st.mu.Lock()
	last := st.lastDeniedAt
	st.mu.Unlock()
	if last == "" {
		t.Error("lastDeniedAt should be set after 403")
	}
}

// TestPoll_connectionError verifies that a connection error is returned as an error.
func TestPoll_connectionError(t *testing.T) {
	// Use a URL that will fail to connect immediately.
	client := &http.Client{Timeout: 100 * time.Millisecond}
	st := &statsTracker{}

	err := poll(client, "https://127.0.0.1:1", "telemetry-rest", st)
	if err == nil {
		t.Error("expected error on connection failure")
	}
	if !strings.Contains(err.Error(), "HTTP") {
		t.Errorf("error should mention HTTP: %v", err)
	}
}

// TestPoll_unexpectedStatus verifies that unexpected status codes return an error.
func TestPoll_unexpectedStatus(t *testing.T) {
	srv, client := newTLSTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	defer srv.Close()

	st := &statsTracker{}
	err := poll(client, srv.URL, "telemetry-rest", st)
	if err == nil {
		t.Error("expected error on 500 status")
	}
	if st.msgCount.Load() != 0 || st.deniedCount.Load() != 0 {
		t.Error("no counters should be incremented on unexpected status")
	}
}
