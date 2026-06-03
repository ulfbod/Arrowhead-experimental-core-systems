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
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ── Self-signed TLS helpers ───────────────────────────────────────────────────

// generateSelfSignedCert creates a self-signed x509 certificate and returns
// PEM-encoded cert+key strings plus the parsed *x509.Certificate.
func generateSelfSignedCert(t *testing.T, cn, ou string) (certPEM, keyPEM string, cert *x509.Certificate) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn, OrganizationalUnit: []string{ou}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	parsed, _ := x509.ParseCertificate(certDER)

	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	keyBytes, _ := x509.MarshalECPrivateKey(priv)
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}))
	return certPEM, keyPEM, parsed
}

// ── fetchCAPool tests ─────────────────────────────────────────────────────────

func TestFetchCAPool_OK(t *testing.T) {
	certPEM, _, _ := generateSelfSignedCert(t, "test-ca", "lo")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ca/info" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "test-ca", Certificate: certPEM})
	}))
	defer srv.Close()

	pool, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("fetchCAPool: %v", err)
	}
	if pool == nil {
		t.Errorf("fetchCAPool returned nil pool")
	}
}

func TestFetchCAPool_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Errorf("expected error for 404 response")
	}
}

func TestFetchCAPool_EmptyCert(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "ca", Certificate: ""})
	}))
	defer srv.Close()

	_, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Errorf("expected error for empty certificate")
	}
}

func TestFetchCAPool_InvalidPEM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "ca", Certificate: "not-a-pem"})
	}))
	defer srv.Close()

	_, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Errorf("expected error for invalid PEM")
	}
}

// ── requestCert tests ─────────────────────────────────────────────────────────

func TestRequestCert_OK(t *testing.T) {
	certPEM, keyPEM, _ := generateSelfSignedCert(t, "test-sys", "sy")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{
			SystemName:  "test-sys",
			Certificate: certPEM,
			PrivateKey:  keyPEM,
			Profile:     "sy",
		})
	}))
	defer srv.Close()

	gotCert, gotKey, err := requestCert(srv.URL, "test-sys", &http.Client{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("requestCert: %v", err)
	}
	if gotCert != certPEM {
		t.Errorf("cert mismatch")
	}
	if gotKey != keyPEM {
		t.Errorf("key mismatch")
	}
}

func TestRequestCert_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, _, err := requestCert(srv.URL, "x", &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Errorf("expected error for 500 response")
	}
}

func TestRequestCert_EmptyFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(certResp{})
	}))
	defer srv.Close()

	_, _, err := requestCert(srv.URL, "x", &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Errorf("expected error for empty cert/key fields")
	}
}

// ── buildProfileClient test ───────────────────────────────────────────────────

func TestBuildProfileClient_NotNil(t *testing.T) {
	cert := tls.Certificate{}
	pool := x509.NewCertPool()
	c := buildProfileClient(cert, pool)
	if c == nil {
		t.Errorf("buildProfileClient returned nil")
	}
}

// ── AcquireSystemCert tests ───────────────────────────────────────────────────

func TestAcquireSystemCert_Step1Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, _, err := AcquireSystemCert(srv.URL, srv.URL, "test-sys")
	if err == nil {
		t.Errorf("expected error when CA is unavailable")
	}
}

func TestAcquireSystemCert_Step2Failure(t *testing.T) {
	certPEM, _, _ := generateSelfSignedCert(t, "test-ca", "lo")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ca/info":
			json.NewEncoder(w).Encode(caInfoResp{CommonName: "ca", Certificate: certPEM})
		case "/bootstrap/onboarding-cert":
			w.WriteHeader(http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, _, err := AcquireSystemCert(srv.URL, srv.URL, "test-sys")
	if err == nil {
		t.Errorf("expected error when onboarding cert fails")
	}
}

// TestAcquireSystemCert_Step3Failure — device cert endpoint returns bad JSON.
func TestAcquireSystemCert_Step3Failure(t *testing.T) {
	caCertPEM, caKeyPEM, _ := generateSelfSignedCert(t, "ca", "lo")
	onCertPEM, onKeyPEM, _ := generateSelfSignedCert(t, "on", "on")

	caTLSCert, err := tls.X509KeyPair([]byte(caCertPEM), []byte(caKeyPEM))
	if err != nil {
		t.Fatalf("parse CA TLS cert: %v", err)
	}
	serverTLSCfg := &tls.Config{
		Certificates: []tls.Certificate{caTLSCert},
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/ca/info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "ca", Certificate: caCertPEM})
	})
	httpMux.HandleFunc("/bootstrap/onboarding-cert", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: onCertPEM, PrivateKey: onKeyPEM, Profile: "on"})
	})

	tlsMux := http.NewServeMux()
	tlsMux.HandleFunc("/ca/device-cert", func(w http.ResponseWriter, r *http.Request) {
		// Return empty cert — triggers "empty certificate or key" error.
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{})
	})

	httpSrv := httptest.NewServer(httpMux)
	defer httpSrv.Close()

	tlsSrv := httptest.NewUnstartedServer(tlsMux)
	tlsSrv.TLS = serverTLSCfg
	tlsSrv.StartTLS()
	defer tlsSrv.Close()

	_, _, err = AcquireSystemCert(httpSrv.URL, tlsSrv.URL, "sys")
	if err == nil {
		t.Errorf("expected error when device cert is empty")
	}
}

// TestAcquireSystemCert_Step4Failure — system cert endpoint returns bad JSON.
func TestAcquireSystemCert_Step4Failure(t *testing.T) {
	caCertPEM, caKeyPEM, _ := generateSelfSignedCert(t, "ca", "lo")
	onCertPEM, onKeyPEM, _ := generateSelfSignedCert(t, "on", "on")
	deCertPEM, deKeyPEM, _ := generateSelfSignedCert(t, "de", "de")

	caTLSCert, err := tls.X509KeyPair([]byte(caCertPEM), []byte(caKeyPEM))
	if err != nil {
		t.Fatalf("parse CA TLS cert: %v", err)
	}
	serverTLSCfg := &tls.Config{
		Certificates: []tls.Certificate{caTLSCert},
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/ca/info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "ca", Certificate: caCertPEM})
	})
	httpMux.HandleFunc("/bootstrap/onboarding-cert", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: onCertPEM, PrivateKey: onKeyPEM, Profile: "on"})
	})

	tlsMux := http.NewServeMux()
	tlsMux.HandleFunc("/ca/device-cert", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: deCertPEM, PrivateKey: deKeyPEM, Profile: "de"})
	})
	tlsMux.HandleFunc("/ca/system-cert", func(w http.ResponseWriter, r *http.Request) {
		// Return empty cert — triggers "empty certificate or key" error.
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{})
	})

	httpSrv := httptest.NewServer(httpMux)
	defer httpSrv.Close()

	tlsSrv := httptest.NewUnstartedServer(tlsMux)
	tlsSrv.TLS = serverTLSCfg
	tlsSrv.StartTLS()
	defer tlsSrv.Close()

	_, _, err = AcquireSystemCert(httpSrv.URL, tlsSrv.URL, "sys")
	if err == nil {
		t.Errorf("expected error when system cert is empty")
	}
}

// TestAcquireSystemCert_FullPath exercises all four lifecycle steps against
// a mock profile-ca. The TLS server uses the same self-signed cert that is
// also advertised via /ca/info, so the client trusts the server.
func TestAcquireSystemCert_FullPath(t *testing.T) {
	// Generate a CA/server cert with 127.0.0.1 SAN (used by both the HTTP
	// server and the TLS server, and advertised via /ca/info).
	caCertPEM, caKeyPEM, _ := generateSelfSignedCert(t, "test-ca", "lo")
	onCertPEM, onKeyPEM, _ := generateSelfSignedCert(t, "test-on", "on")
	deCertPEM, deKeyPEM, _ := generateSelfSignedCert(t, "test-de", "de")
	syCertPEM, syKeyPEM, _ := generateSelfSignedCert(t, "test-sy", "sy")

	// Build a TLS config for the mock TLS server using the CA cert.
	caTLSCert, err := tls.X509KeyPair([]byte(caCertPEM), []byte(caKeyPEM))
	if err != nil {
		t.Fatalf("parse CA TLS cert: %v", err)
	}
	serverTLSCfg := &tls.Config{
		Certificates: []tls.Certificate{caTLSCert},
		ClientAuth:   tls.NoClientCert, // skip client cert validation for simplicity
		MinVersion:   tls.VersionTLS12,
	}

	// Build a combined handler for both plain HTTP and TLS paths.
	mux := http.NewServeMux()
	mux.HandleFunc("/ca/info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "test-ca", Certificate: caCertPEM})
	})
	mux.HandleFunc("/bootstrap/onboarding-cert", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: onCertPEM, PrivateKey: onKeyPEM, Profile: "on"})
	})
	mux.HandleFunc("/ca/device-cert", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: deCertPEM, PrivateKey: deKeyPEM, Profile: "de"})
	})
	mux.HandleFunc("/ca/system-cert", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: syCertPEM, PrivateKey: syKeyPEM, Profile: "sy"})
	})

	// Plain HTTP server for steps 1 & 2.
	httpSrv := httptest.NewServer(mux)
	defer httpSrv.Close()

	// TLS server for steps 3 & 4.
	tlsSrv := httptest.NewUnstartedServer(mux)
	tlsSrv.TLS = serverTLSCfg
	tlsSrv.StartTLS()
	defer tlsSrv.Close()

	// We need the caPool to trust the TLS server's cert.
	// The TLS server uses caCertPEM which is advertised via /ca/info.
	// However, httptest.NewUnstartedServer generates its own cert; our serverTLSCfg
	// overrides it, so the TLS server presents caCertPEM.
	// The AcquireSystemCert will fetch caCertPEM from /ca/info (step 1) and use
	// it as the RootCAs pool, which trusts the TLS server.

	_, _, lcErr := AcquireSystemCert(httpSrv.URL, tlsSrv.URL, "test-sys")
	if lcErr != nil {
		t.Fatalf("AcquireSystemCert full path: %v", lcErr)
	}
}
