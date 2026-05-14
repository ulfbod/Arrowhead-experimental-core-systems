package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

// ── envOr ─────────────────────────────────────────────────────────────────────

func TestEnvOr_ReturnsEnv(t *testing.T) {
	os.Setenv("TEST_SP_KEY", "spvalue")
	defer os.Unsetenv("TEST_SP_KEY")
	if got := envOr("TEST_SP_KEY", "default"); got != "spvalue" {
		t.Errorf("envOr = %q, want spvalue", got)
	}
}

func TestEnvOr_ReturnsDefault(t *testing.T) {
	os.Unsetenv("TEST_SP_MISSING")
	if got := envOr("TEST_SP_MISSING", "fallback"); got != "fallback" {
		t.Errorf("envOr = %q, want fallback", got)
	}
}

func TestEnvOr_EmptyEnvUsesDefault(t *testing.T) {
	os.Setenv("TEST_SP_EMPTY", "")
	defer os.Unsetenv("TEST_SP_EMPTY")
	if got := envOr("TEST_SP_EMPTY", "def"); got != "def" {
		t.Errorf("envOr = %q, want def for empty env", got)
	}
}

// ── spConfigFromEnv ───────────────────────────────────────────────────────────

func TestSpConfigFromEnv_Defaults(t *testing.T) {
	for _, k := range []string{"PARTNER_NAME", "CA_URL", "CA_TLS_URL", "PKI_REST_AUTHZ_URL",
		"SERVICE", "POLL_INTERVAL", "HEALTH_PORT"} {
		os.Unsetenv(k)
	}

	cfg, err := spConfigFromEnv()
	if err != nil {
		t.Fatalf("spConfigFromEnv: %v", err)
	}
	if cfg.partnerName != "service-partner-1" {
		t.Errorf("partnerName = %q, want service-partner-1", cfg.partnerName)
	}
	if cfg.caURL != "http://profile-ca:8187" {
		t.Errorf("caURL = %q, want http://profile-ca:8187", cfg.caURL)
	}
	if cfg.service != "telemetry-rest" {
		t.Errorf("service = %q, want telemetry-rest", cfg.service)
	}
	if cfg.healthPort != "9211" {
		t.Errorf("healthPort = %q, want 9211", cfg.healthPort)
	}
	if cfg.interval != 5*time.Second {
		t.Errorf("interval = %v, want 5s", cfg.interval)
	}
}

func TestSpConfigFromEnv_Overrides(t *testing.T) {
	os.Setenv("PARTNER_NAME", "sp-test")
	os.Setenv("POLL_INTERVAL", "2s")
	os.Setenv("HEALTH_PORT", "9999")
	defer func() {
		os.Unsetenv("PARTNER_NAME")
		os.Unsetenv("POLL_INTERVAL")
		os.Unsetenv("HEALTH_PORT")
	}()

	cfg, err := spConfigFromEnv()
	if err != nil {
		t.Fatalf("spConfigFromEnv: %v", err)
	}
	if cfg.partnerName != "sp-test" {
		t.Errorf("partnerName = %q, want sp-test", cfg.partnerName)
	}
	if cfg.interval != 2*time.Second {
		t.Errorf("interval = %v, want 2s", cfg.interval)
	}
	if cfg.healthPort != "9999" {
		t.Errorf("healthPort = %q, want 9999", cfg.healthPort)
	}
}

func TestSpConfigFromEnv_InvalidInterval(t *testing.T) {
	os.Setenv("POLL_INTERVAL", "not-a-duration")
	defer os.Unsetenv("POLL_INTERVAL")

	_, err := spConfigFromEnv()
	if err == nil {
		t.Errorf("expected error for invalid POLL_INTERVAL")
	}
}

// ── startHealthServer ─────────────────────────────────────────────────────────

func TestStartHealthServer_Listen(t *testing.T) {
	stats := NewStats("rest-mtls-pki")
	// Use port 0 to let OS pick a free port — but startHealthServer doesn't
	// return an address, so we pick a free-ish high port.
	// Instead, we use a hand-built server that mirrors what startHealthServer does.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", HandleHealth("test-partner"))
	mux.HandleFunc("/stats", HandleStats(stats))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("health status = %d, want 200", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["partner"] != "test-partner" {
		t.Errorf("partner = %q, want test-partner", resp["partner"])
	}
}

// ── PollClient.Stats ──────────────────────────────────────────────────────────

func TestPollClient_Stats(t *testing.T) {
	c := NewPollClient(tls.Certificate{}, nil, "https://example.com", "svc")
	if c.Stats() == nil {
		t.Errorf("Stats() returned nil")
	}
}

// ── startHealthServer ─────────────────────────────────────────────────────────

func TestStartHealthServer_Responds(t *testing.T) {
	// Find a free port.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()

	stats := NewStats("rest-mtls-pki")
	startHealthServer("test-partner", port, stats)

	// Wait for the goroutine to start.
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/health", port))
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// ── acquireWithRetry ──────────────────────────────────────────────────────────

func TestAcquireWithRetry_AllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := acquireWithRetry(srv.URL, srv.URL, "x", 1)
	if err == nil {
		t.Errorf("expected error when all retries fail")
	}
}

func TestAcquireWithRetry_SuccessFirstAttempt(t *testing.T) {
	caPEM, caKey := generateSelfSignedCert(t, "ca", "lo")
	onPEM, onKey := generateSelfSignedCert(t, "on", "on")
	dePEM, deKey := generateSelfSignedCert(t, "de", "de")
	syPEM, syKey := generateSelfSignedCert(t, "sy", "sy")

	caTLSCert, err := tls.X509KeyPair([]byte(caPEM), []byte(caKey))
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{caTLSCert},
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ca/info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caInfoResp{CommonName: "ca", Certificate: caPEM})
	})
	mux.HandleFunc("/bootstrap/onboarding-cert", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: onPEM, PrivateKey: onKey})
	})
	mux.HandleFunc("/ca/device-cert", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: dePEM, PrivateKey: deKey})
	})
	mux.HandleFunc("/ca/system-cert", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(certResp{Certificate: syPEM, PrivateKey: syKey})
	})

	httpSrv := httptest.NewServer(mux)
	defer httpSrv.Close()

	tlsSrv := httptest.NewUnstartedServer(mux)
	tlsSrv.TLS = serverTLS
	tlsSrv.StartTLS()
	defer tlsSrv.Close()

	pc, err := acquireWithRetry(httpSrv.URL, tlsSrv.URL, "test-sys", 3)
	if err != nil {
		t.Fatalf("acquireWithRetry: %v", err)
	}
	if pc == nil {
		t.Errorf("expected non-nil PollClient")
	}
}
