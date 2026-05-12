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

// testCAForLifecycle is a minimal CA helper for lifecycle tests.
type testCAForLifecycle struct {
	key     *ecdsa.PrivateKey
	cert    *x509.Certificate
	certPEM []byte
}

func newTestCAForLifecycle(t *testing.T) *testCAForLifecycle {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Test CA", OrganizationalUnit: []string{"lo"}},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(der)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return &testCAForLifecycle{key: key, cert: cert, certPEM: certPEM}
}

func (tc *testCAForLifecycle) issueLeaf(t *testing.T, systemName, profile string) (string, string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:         systemName,
			OrganizationalUnit: []string{profile},
		},
		DNSNames:    []string{systemName},
		NotBefore:   time.Now().Add(-time.Minute),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tc.cert, &key.PublicKey, tc.key)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return string(certPEM), string(keyPEM)
}

func (tc *testCAForLifecycle) tlsCert(t *testing.T) tls.Certificate {
	t.Helper()
	keyBytes, _ := x509.MarshalECPrivateKey(tc.key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	c, _ := tls.X509KeyPair(tc.certPEM, keyPEM)
	return c
}

func TestAcquireSystemCert_FullLifecycle(t *testing.T) {
	tc := newTestCAForLifecycle(t)

	// HTTP server: /ca/info + /bootstrap/onboarding-cert
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/ca/info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "test-ca", Certificate: string(tc.certPEM)})
	})
	httpMux.HandleFunc("/bootstrap/onboarding-cert", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ SystemName string `json:"systemName"` }
		json.NewDecoder(r.Body).Decode(&req)
		certPEM, keyPEM := tc.issueLeaf(t, req.SystemName, "on")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: certPEM, PrivateKey: keyPEM, Profile: "on"})
	})
	httpSrv := httptest.NewServer(httpMux)
	defer httpSrv.Close()

	// TLS server: /ca/device-cert + /ca/system-cert
	caPool := x509.NewCertPool()
	caPool.AddCert(tc.cert)
	tlsMux := http.NewServeMux()
	tlsMux.HandleFunc("/ca/device-cert", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "no cert", 401); return
		}
		var req struct{ SystemName string `json:"systemName"` }
		json.NewDecoder(r.Body).Decode(&req)
		certPEM, keyPEM := tc.issueLeaf(t, req.SystemName, "de")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: certPEM, PrivateKey: keyPEM, Profile: "de"})
	})
	tlsMux.HandleFunc("/ca/system-cert", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "no cert", 401); return
		}
		var req struct{ SystemName string `json:"systemName"` }
		json.NewDecoder(r.Body).Decode(&req)
		certPEM, keyPEM := tc.issueLeaf(t, req.SystemName, "sy")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: certPEM, PrivateKey: keyPEM, Profile: "sy"})
	})
	tlsSrv := httptest.NewUnstartedServer(tlsMux)
	tlsSrv.TLS = &tls.Config{
		Certificates: []tls.Certificate{tc.tlsCert(t)},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	tlsSrv.StartTLS()
	defer tlsSrv.Close()

	cert, pool, err := AcquireSystemCert(httpSrv.URL, tlsSrv.URL, "test-system")
	if err != nil {
		t.Fatalf("AcquireSystemCert: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Error("expected system cert")
	}
	if pool == nil {
		t.Error("expected CA pool")
	}
	leaf, _ := x509.ParseCertificate(cert.Certificate[0])
	if leaf.Subject.CommonName != "test-system" {
		t.Errorf("expected CN=test-system, got %s", leaf.Subject.CommonName)
	}
}

func TestFetchCAPool_Success(t *testing.T) {
	tc := newTestCAForLifecycle(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "test", Certificate: string(tc.certPEM)})
	}))
	defer srv.Close()

	pool, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("fetchCAPool: %v", err)
	}
	if pool == nil {
		t.Error("expected non-nil pool")
	}
}

func TestFetchCAPool_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	_, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Error("expected error for non-200 response")
	}
}

func TestFetchCAPool_EmptyCert(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "test", Certificate: ""})
	}))
	defer srv.Close()

	_, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Error("expected error for empty certificate")
	}
}

func TestFetchCAPool_InvalidPEM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "test", Certificate: "not-a-pem"})
	}))
	defer srv.Close()

	_, err := fetchCAPool(srv.URL, &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

func TestRequestCert_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, _, err := requestCert(srv.URL+"/bootstrap/onboarding-cert", "test", &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Error("expected error for server error")
	}
}

func TestRequestCert_EmptyCertInResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: "", PrivateKey: ""})
	}))
	defer srv.Close()

	_, _, err := requestCert(srv.URL+"/bootstrap/onboarding-cert", "test", &http.Client{Timeout: 5 * time.Second})
	if err == nil {
		t.Error("expected error for empty certificate")
	}
}
