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

// ── helpers ───────────────────────────────────────────────────────────────────

func generateSelfSignedCert(t *testing.T, cn, ou string) (certPEM, keyPEM string) {
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
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	keyBytes, _ := x509.MarshalECPrivateKey(priv)
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}))
	return
}

// ── fetchCAPool tests ─────────────────────────────────────────────────────────

func TestFetchCAPool_OK(t *testing.T) {
	certPEM, _ := generateSelfSignedCert(t, "ca", "lo")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "ca", Certificate: certPEM})
	}))
	defer srv.Close()

	pool, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("fetchCAPool: %v", err)
	}
	if pool == nil {
		t.Errorf("nil pool returned")
	}
}

func TestFetchCAPool_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Errorf("expected error for 500 response")
	}
}

func TestFetchCAPool_EmptyCert(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "ca", Certificate: ""})
	}))
	defer srv.Close()
	_, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Errorf("expected error for empty cert")
	}
}

func TestFetchCAPool_InvalidPEM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "ca", Certificate: "garbage"})
	}))
	defer srv.Close()
	_, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Errorf("expected error for non-PEM cert")
	}
}

// ── requestCert tests ─────────────────────────────────────────────────────────

func TestRequestCert_OK(t *testing.T) {
	certPEM, keyPEM := generateSelfSignedCert(t, "sys", "sy")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: certPEM, PrivateKey: keyPEM})
	}))
	defer srv.Close()

	c, k, err := requestCert(srv.URL, "sys", &http.Client{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("requestCert: %v", err)
	}
	if c != certPEM || k != keyPEM {
		t.Errorf("cert/key mismatch")
	}
}

func TestRequestCert_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	_, _, err := requestCert(srv.URL, "sys", &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Errorf("expected error for 403")
	}
}

func TestRequestCert_EmptyFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(certResp{})
	}))
	defer srv.Close()
	_, _, err := requestCert(srv.URL, "sys", &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Errorf("expected error for empty cert/key")
	}
}

// ── buildProfileClient ────────────────────────────────────────────────────────

func TestBuildProfileClient_NotNil(t *testing.T) {
	c := buildProfileClient(tls.Certificate{}, x509.NewCertPool())
	if c == nil {
		t.Errorf("buildProfileClient returned nil")
	}
}

// ── AcquireSystemCert error paths ─────────────────────────────────────────────

func TestAcquireSystemCert_CAUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, _, err := AcquireSystemCert(srv.URL, srv.URL, "x")
	if err == nil {
		t.Errorf("expected error when CA unavailable")
	}
}

func TestAcquireSystemCert_OnboardingFails(t *testing.T) {
	certPEM, _ := generateSelfSignedCert(t, "ca", "lo")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ca/info":
			json.NewEncoder(w).Encode(caInfoResp{CommonName: "ca", Certificate: certPEM})
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer srv.Close()

	_, _, err := AcquireSystemCert(srv.URL, srv.URL, "x")
	if err == nil {
		t.Errorf("expected error when onboarding fails")
	}
}
