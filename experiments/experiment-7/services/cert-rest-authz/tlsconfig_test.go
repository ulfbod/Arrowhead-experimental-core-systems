package main

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── helper: generate real self-signed cert PEM for CA mock ───────────────────

// selfSignedPEM generates a self-signed ECDSA P-256 certificate and returns
// the PEM-encoded certificate and private key.
func selfSignedPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	tlsCert := makeTLSCert(t, "test-ca")

	// Extract certificate bytes from the tls.Certificate.
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsCert.Certificate[0]})

	// Extract and re-encode the private key.
	privKey, ok := tlsCert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatal("expected *ecdsa.PrivateKey")
	}
	der, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		t.Fatalf("marshal EC private key: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	return certPEM, keyPEM
}

// ── mock CA for TLS config tests ──────────────────────────────────────────────

// mockCAForTLSConfig creates a test CA server that returns real PEM certs.
func mockCAForTLSConfig(t *testing.T) *httptest.Server {
	t.Helper()
	certPEM, keyPEM := selfSignedPEM(t)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ca/info":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(caInfoResp{
				CommonName:  "test-ca",
				Certificate: string(certPEM),
			})
		case "/ca/certificate/issue":
			var req issueCertReq
			json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(issueCertResp{
				SystemName:  req.SystemName,
				Certificate: string(certPEM),
				PrivateKey:  string(keyPEM),
				IssuedAt:   "2025-01-01T00:00:00Z",
				ExpiresAt:  "2026-01-01T00:00:00Z",
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestFetchCACert verifies that fetchCACert returns a populated pool and PEM.
func TestFetchCACert(t *testing.T) {
	ca := mockCAForTLSConfig(t)
	defer ca.Close()

	pool, rawPEM, err := fetchCACert(ca.URL)
	if err != nil {
		t.Fatalf("fetchCACert: %v", err)
	}
	if pool == nil {
		t.Error("returned pool is nil")
	}
	if len(rawPEM) == 0 {
		t.Error("returned PEM is empty")
	}
}

// TestIssueCert verifies that issueCert returns a valid tls.Certificate.
func TestIssueCert(t *testing.T) {
	ca := mockCAForTLSConfig(t)
	defer ca.Close()

	cert, err := issueCert(ca.URL, "test-system")
	if err != nil {
		t.Fatalf("issueCert: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Error("returned certificate has no certificate chain")
	}
}

// TestBuildServerTLSConfig verifies that the config requires client certificates.
func TestBuildServerTLSConfig(t *testing.T) {
	ca := mockCAForTLSConfig(t)
	defer ca.Close()

	pool, _, err := fetchCACert(ca.URL)
	if err != nil {
		t.Fatalf("fetchCACert: %v", err)
	}
	cert, err := issueCert(ca.URL, "test-server")
	if err != nil {
		t.Fatalf("issueCert: %v", err)
	}

	cfg := buildServerTLSConfig(cert, pool)
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth: got %v, want RequireAndVerifyClientCert", cfg.ClientAuth)
	}
	if len(cfg.Certificates) == 0 {
		t.Error("no server certificate configured")
	}
}

// TestBuildClientTLSConfig verifies that the client TLS config is set up correctly.
func TestBuildClientTLSConfig(t *testing.T) {
	ca := mockCAForTLSConfig(t)
	defer ca.Close()

	pool, _, err := fetchCACert(ca.URL)
	if err != nil {
		t.Fatalf("fetchCACert: %v", err)
	}
	cert, err := issueCert(ca.URL, "test-client")
	if err != nil {
		t.Fatalf("issueCert: %v", err)
	}

	cfg := buildClientTLSConfig(cert, pool)
	if len(cfg.Certificates) == 0 {
		t.Error("no client certificate in config")
	}
	if cfg.RootCAs == nil {
		t.Error("RootCAs is nil")
	}
}

// TestFetchCACert_badJSON verifies that malformed JSON returns an error.
func TestFetchCACert_badJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-json{{{"))
	}))
	defer srv.Close()

	_, _, err := fetchCACert(srv.URL)
	if err == nil {
		t.Error("expected error on malformed JSON response")
	}
}

// TestFetchCACert_nonOKStatus verifies error is returned on non-200 response.
func TestFetchCACert_nonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, _, err := fetchCACert(srv.URL)
	if err == nil {
		t.Error("expected error on 503 response")
	}
}

// Keep rand import used by helpers.
var _ = rand.Reader
