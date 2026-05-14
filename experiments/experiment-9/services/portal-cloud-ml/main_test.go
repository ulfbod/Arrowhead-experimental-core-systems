package main

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// ── envOr ─────────────────────────────────────────────────────────────────────

func TestEnvOr_ReturnsEnv(t *testing.T) {
	os.Setenv("TEST_EXP9_KEY", "myvalue")
	defer os.Unsetenv("TEST_EXP9_KEY")
	if got := envOr("TEST_EXP9_KEY", "default"); got != "myvalue" {
		t.Errorf("envOr = %q, want myvalue", got)
	}
}

func TestEnvOr_ReturnsDefault(t *testing.T) {
	os.Unsetenv("TEST_EXP9_MISSING")
	if got := envOr("TEST_EXP9_MISSING", "fallback"); got != "fallback" {
		t.Errorf("envOr = %q, want fallback", got)
	}
}

func TestEnvOr_EmptyEnvUsesDefault(t *testing.T) {
	os.Setenv("TEST_EXP9_EMPTY", "")
	defer os.Unsetenv("TEST_EXP9_EMPTY")
	if got := envOr("TEST_EXP9_EMPTY", "default"); got != "default" {
		t.Errorf("envOr = %q, want default for empty env var", got)
	}
}

// ── portalConfigFromEnv ───────────────────────────────────────────────────────

func TestPortalConfigFromEnv_Defaults(t *testing.T) {
	// Clear relevant env vars.
	for _, k := range []string{"CA_URL", "CA_TLS_URL", "KAFKA_AUTHZ_URL",
		"CONSUMER_NAME", "SERVICE", "SYSTEM_NAME", "PORT", "TLS_PORT"} {
		os.Unsetenv(k)
	}

	cfg := portalConfigFromEnv()
	if cfg.caURL != "http://profile-ca:8187" {
		t.Errorf("caURL = %q, want http://profile-ca:8187", cfg.caURL)
	}
	if cfg.port != "9207" {
		t.Errorf("port = %q, want 9207", cfg.port)
	}
	if cfg.tlsPort != "9294" {
		t.Errorf("tlsPort = %q, want 9294", cfg.tlsPort)
	}
	if cfg.consumerName != "portal-cloud-ml" {
		t.Errorf("consumerName = %q, want portal-cloud-ml", cfg.consumerName)
	}
	if cfg.service != "telemetry" {
		t.Errorf("service = %q, want telemetry", cfg.service)
	}
}

func TestPortalConfigFromEnv_Overrides(t *testing.T) {
	os.Setenv("PORT", "9999")
	os.Setenv("CONSUMER_NAME", "my-portal")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("CONSUMER_NAME")
	}()

	cfg := portalConfigFromEnv()
	if cfg.port != "9999" {
		t.Errorf("port = %q, want 9999", cfg.port)
	}
	if cfg.consumerName != "my-portal" {
		t.Errorf("consumerName = %q, want my-portal", cfg.consumerName)
	}
}

// ── startPlainServer ──────────────────────────────────────────────────────────

func TestStartPlainServer_Listen(t *testing.T) {
	store := NewStore()
	// Port 0 lets the OS pick a free port.
	addr, err := startPlainServer(store, "0")
	if err != nil {
		t.Fatalf("startPlainServer: %v", err)
	}
	if addr == "" {
		t.Errorf("addr is empty")
	}

	// Give the goroutine a moment to start.
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestStartPlainServer_ServesStats(t *testing.T) {
	store := NewStore()
	store.Record([]byte(`{"v":1}`))
	addr, err := startPlainServer(store, "0")
	if err != nil {
		t.Fatalf("startPlainServer: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/stats")
	if err != nil {
		t.Fatalf("GET /stats: %v", err)
	}
	defer resp.Body.Close()

	var stats map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&stats)
	if stats["msgCount"].(float64) != 1 {
		t.Errorf("msgCount = %v, want 1", stats["msgCount"])
	}
}

// ── acquireWithRetry ──────────────────────────────────────────────────────────

func TestAcquireWithRetry_SuccessFirstAttempt(t *testing.T) {
	certPEM, keyPEM, _ := generateSelfSignedCert(t, "ca", "lo")
	onCertPEM, onKeyPEM, _ := generateSelfSignedCert(t, "on", "on")
	deCertPEM, deKeyPEM, _ := generateSelfSignedCert(t, "de", "de")
	syCertPEM, syKeyPEM, _ := generateSelfSignedCert(t, "sy", "sy")

	caTLSCert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatalf("parse CA TLS cert: %v", err)
	}
	serverTLSCfg := &tls.Config{
		Certificates: []tls.Certificate{caTLSCert},
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ca/info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "ca", Certificate: certPEM})
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

	httpSrv := httptest.NewServer(mux)
	defer httpSrv.Close()
	tlsSrv := httptest.NewUnstartedServer(mux)
	tlsSrv.TLS = serverTLSCfg
	tlsSrv.StartTLS()
	defer tlsSrv.Close()

	cert, err := acquireWithRetry(httpSrv.URL, tlsSrv.URL, "sys", 3)
	if err != nil {
		t.Fatalf("acquireWithRetry: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Errorf("returned cert has no data")
	}
}

func TestAcquireWithRetry_AllAttemptsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := acquireWithRetry(srv.URL, srv.URL, "x", 1)
	if err == nil {
		t.Errorf("expected error when all retries fail")
	}
}
