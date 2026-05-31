package tlsutil_test

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
	"os"
	"path/filepath"
	"testing"
	"time"

	"arrowhead/core/internal/tlsutil"
)

// ── test helpers ──────────────────────────────────────────────────────────────

// testCerts holds file paths to a self-signed CA + leaf cert/key pair.
type testCerts struct {
	caFile   string
	certFile string
	keyFile  string
}

// writeTestCerts generates a self-signed CA and issues a leaf cert, writing
// all three PEM files to a temporary directory.  t.Cleanup removes them.
func writeTestCerts(t *testing.T) testCerts {
	t.Helper()
	dir := t.TempDir()

	// CA key + cert
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
	caFile := filepath.Join(dir, "ca.crt")
	writePEMFile(t, caFile, "CERTIFICATE", caDER)

	// Leaf key + cert signed by CA
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-service"},
		DNSNames:     []string{"test-service"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf cert: %v", err)
	}
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatalf("marshal leaf key: %v", err)
	}

	certFile := filepath.Join(dir, "leaf.crt")
	keyFile := filepath.Join(dir, "leaf.key")
	writePEMFile(t, certFile, "CERTIFICATE", leafDER)
	writePEMFile(t, keyFile, "EC PRIVATE KEY", leafKeyDER)

	return testCerts{caFile: caFile, certFile: certFile, keyFile: keyFile}
}

func writePEMFile(t *testing.T, path, pemType string, der []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: pemType, Bytes: der}); err != nil {
		t.Fatalf("encode PEM to %s: %v", path, err)
	}
}

// ── LoadServerTLSConfig ───────────────────────────────────────────────────────

func TestLoadServerTLSConfig_Disabled(t *testing.T) {
	cfg, err := tlsutil.LoadServerTLSConfig("", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config when no cert/key provided")
	}
}

func TestLoadServerTLSConfig_ServerOnlyTLS(t *testing.T) {
	certs := writeTestCerts(t)
	cfg, err := tlsutil.LoadServerTLSConfig(certs.certFile, certs.keyFile, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(cfg.Certificates))
	}
	if cfg.ClientAuth != tls.NoClientCert {
		t.Errorf("expected NoClientCert without CA file, got %v", cfg.ClientAuth)
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion TLS12, got %d", cfg.MinVersion)
	}
}

func TestLoadServerTLSConfig_mTLS(t *testing.T) {
	certs := writeTestCerts(t)
	cfg, err := tlsutil.LoadServerTLSConfig(certs.certFile, certs.keyFile, certs.caFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("expected RequireAndVerifyClientCert, got %v", cfg.ClientAuth)
	}
	if cfg.ClientCAs == nil {
		t.Error("expected non-nil ClientCAs when CA file is set")
	}
}

func TestLoadServerTLSConfig_BadCertFile(t *testing.T) {
	_, err := tlsutil.LoadServerTLSConfig("/nonexistent.crt", "/nonexistent.key", "")
	if err == nil {
		t.Error("expected error for missing cert file")
	}
}

func TestLoadServerTLSConfig_BadCAFile(t *testing.T) {
	certs := writeTestCerts(t)
	_, err := tlsutil.LoadServerTLSConfig(certs.certFile, certs.keyFile, "/nonexistent-ca.crt")
	if err == nil {
		t.Error("expected error for missing CA file")
	}
}

// ── LoadClientTLSConfig ───────────────────────────────────────────────────────

func TestLoadClientTLSConfig_Disabled(t *testing.T) {
	cfg, err := tlsutil.LoadClientTLSConfig("", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config when no CA file provided")
	}
}

func TestLoadClientTLSConfig_ServerVerify(t *testing.T) {
	certs := writeTestCerts(t)
	cfg, err := tlsutil.LoadClientTLSConfig("", "", certs.caFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.RootCAs == nil {
		t.Error("expected non-nil RootCAs")
	}
	if len(cfg.Certificates) != 0 {
		t.Errorf("expected no client cert without certFile, got %d", len(cfg.Certificates))
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion TLS12, got %d", cfg.MinVersion)
	}
}

func TestLoadClientTLSConfig_mTLS(t *testing.T) {
	certs := writeTestCerts(t)
	cfg, err := tlsutil.LoadClientTLSConfig(certs.certFile, certs.keyFile, certs.caFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.RootCAs == nil {
		t.Error("expected non-nil RootCAs")
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("expected 1 client certificate, got %d", len(cfg.Certificates))
	}
}

func TestLoadClientTLSConfig_BadCAFile(t *testing.T) {
	_, err := tlsutil.LoadClientTLSConfig("", "", "/nonexistent-ca.crt")
	if err == nil {
		t.Error("expected error for missing CA file")
	}
}

// ── NewHTTPClient ─────────────────────────────────────────────────────────────

func TestNewHTTPClient_Nil(t *testing.T) {
	client := tlsutil.NewHTTPClient(nil)
	if client == nil {
		t.Error("expected non-nil http.Client")
	}
	if client.Timeout == 0 {
		t.Error("expected non-zero timeout")
	}
	if client.Transport != nil {
		t.Error("expected nil transport for plain HTTP client")
	}
}

func TestNewHTTPClient_WithTLS(t *testing.T) {
	certs := writeTestCerts(t)
	cfg, _ := tlsutil.LoadClientTLSConfig("", "", certs.caFile)
	client := tlsutil.NewHTTPClient(cfg)
	if client == nil {
		t.Error("expected non-nil http.Client")
	}
	if client.Transport == nil {
		t.Error("expected non-nil transport for TLS client")
	}
}

// ── ServeHTTPS ────────────────────────────────────────────────────────────────

// TestServeHTTPSFalseStartsPlainHTTP — httpsOnly=false, tlsCfg=nil → plain HTTP.
// The function should invoke the plain HTTP path (we verify it doesn't panic).
func TestServeHTTPSFalseStartsPlainHTTP(t *testing.T) {
	// We cannot actually block on ListenAndServe in a unit test, so we verify
	// the function signature and basic argument routing by passing a bad addr
	// that fails immediately — the important thing is no panic on nil tlsCfg.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	err := tlsutil.ServeHTTPS("bad-addr-will-fail:0", "", handler, nil, false)
	// An error is expected (bad addr), but no panic.
	if err == nil {
		t.Error("expected error from bad plain addr")
	}
}

// TestServeHTTPSOnlyWithoutTLSFallsBack — httpsOnly=true, tlsCfg=nil → falls back to plain HTTP.
func TestServeHTTPSOnlyWithoutTLSFallsBack(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// httpsOnly=true but tlsCfg=nil → fail-safe: use plain HTTP, which fails on bad addr.
	err := tlsutil.ServeHTTPS("bad-addr-will-fail:0", "", handler, nil, true)
	if err == nil {
		t.Error("expected error from bad plain addr (fail-safe)")
	}
}

// TestServeHTTPSOnlyHealthOnlyOnPlainPort — with TLS config, plain port serves /health only.
func TestServeHTTPSOnlyHealthOnlyOnPlainPort(t *testing.T) {
	certs := writeTestCerts(t)
	serverTLS, err := tlsutil.LoadServerTLSConfig(certs.certFile, certs.keyFile, "")
	if err != nil {
		t.Fatalf("LoadServerTLSConfig: %v", err)
	}

	// Start a real HTTP listener on a random port (plain port).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	plainAddr := ln.Addr().String()
	ln.Close() // Release port; ServeHTTPS will rebind it

	mainHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("main-handler"))
	})

	// Run ServeHTTPS in background — it will start the health-only plain listener
	// and attempt TLS on a bad port (will fail, but plain port should be up).
	done := make(chan error, 1)
	go func() {
		done <- tlsutil.ServeHTTPS(plainAddr, "127.0.0.1:0", mainHandler, serverTLS, true)
	}()

	// Give the goroutine a moment to start.
	time.Sleep(50 * time.Millisecond)

	// /health on plain port → 200
	resp, err := http.Get("http://" + plainAddr + "/health")
	if err != nil {
		t.Fatalf("/health request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/health: want 200, got %d", resp.StatusCode)
	}

	// /other on plain port → 451
	resp2, err := http.Get("http://" + plainAddr + "/other")
	if err != nil {
		t.Fatalf("/other request: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnavailableForLegalReasons {
		t.Errorf("/other: want 451, got %d", resp2.StatusCode)
	}
}
