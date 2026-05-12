// pki-consumer for experiment-8.
//
// Periodically polls the pki-rest-authz mTLS proxy for telemetry data.
// Uses the full Arrowhead 5.2 onboarding lifecycle to obtain its System certificate:
//
//  1. GET /ca/info → CA cert pool (plain HTTP)
//  2. POST /bootstrap/onboarding-cert → onboarding cert OU=on (plain HTTP)
//  3. POST /ca/device-cert → device cert OU=de (TLS, presenting onboarding cert)
//  4. POST /ca/system-cert → system cert OU=sy (TLS, presenting device cert)
//  5. Poll pki-rest-authz using system cert for mTLS
//
// Environment variables:
//
//	CONSUMER_NAME        system name / cert CN (default: pki-consumer)
//	CA_URL               profile-ca plain HTTP base URL (default: http://profile-ca:8087)
//	CA_TLS_URL           profile-ca mTLS base URL (default: https://profile-ca:8088)
//	PKI_REST_AUTHZ_URL   mTLS proxy base URL (default: https://pki-rest-authz:9108)
//	SERVICE              service name for X-Service-Name header (default: telemetry-rest)
//	POLL_INTERVAL        how often to poll (default: 2s)
//	HEALTH_PORT          HTTP port for /health and /stats (default: 9107)
package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	gosync "sync"
	"sync/atomic"
	"time"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ── Stats ─────────────────────────────────────────────────────────────────────

type statsTracker struct {
	msgCount    atomic.Int64
	deniedCount atomic.Int64
	mu          gosync.Mutex
	lastReceivedAt string
	lastDeniedAt   string
}

func (st *statsTracker) recordMsg() {
	st.msgCount.Add(1)
	st.mu.Lock()
	st.lastReceivedAt = time.Now().UTC().Format(time.RFC3339)
	st.mu.Unlock()
}

func (st *statsTracker) recordDenied() {
	st.deniedCount.Add(1)
	st.mu.Lock()
	st.lastDeniedAt = time.Now().UTC().Format(time.RFC3339)
	st.mu.Unlock()
}

func (st *statsTracker) snapshot(name string) map[string]interface{} {
	st.mu.Lock()
	last := st.lastReceivedAt
	denied := st.lastDeniedAt
	st.mu.Unlock()
	return map[string]interface{}{
		"name":           name,
		"transport":      "rest-mtls-pki",
		"msgCount":       st.msgCount.Load(),
		"deniedCount":    st.deniedCount.Load(),
		"lastReceivedAt": last,
		"lastDeniedAt":   denied,
	}
}

// ── Poll loop ─────────────────────────────────────────────────────────────────

func poll(client *http.Client, pkiRestAuthzURL, service string, st *statsTracker) error {
	url := fmt.Sprintf("%s/telemetry/latest", pkiRestAuthzURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("X-Service-Name", service)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	switch resp.StatusCode {
	case http.StatusOK:
		st.recordMsg()
		log.Printf("[pki-consumer] OK  %d bytes: %.120s", len(body), string(body))
	case http.StatusForbidden:
		st.recordDenied()
		log.Printf("[pki-consumer] 403 Forbidden (access denied by pki-rest-authz/AuthzForce)")
	default:
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

func newMux(st *statsTracker, name string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(st.snapshot(name))
	})
	return mux
}

func main() {
	name            := envOr("CONSUMER_NAME", "pki-consumer")
	caURL           := envOr("CA_URL", "http://profile-ca:8087")
	caTLSURL        := envOr("CA_TLS_URL", "https://profile-ca:8088")
	pkiRestAuthzURL := envOr("PKI_REST_AUTHZ_URL", "https://pki-rest-authz:9108")
	service         := envOr("SERVICE", "telemetry-rest")
	pollIntervalStr := envOr("POLL_INTERVAL", "2s")
	healthPort      := envOr("HEALTH_PORT", "9107")

	pollInterval, err := time.ParseDuration(pollIntervalStr)
	if err != nil || pollInterval < time.Second {
		log.Fatalf("[pki-consumer] invalid POLL_INTERVAL %q: must be ≥1s", pollIntervalStr)
	}

	// Perform the full Arrowhead 5.2 onboarding lifecycle to acquire a System cert.
	log.Printf("[pki-consumer] starting PKI lifecycle: on → de → sy for CN=%s", name)
	var syTLSCert tls.Certificate
	var caPool *x509.CertPool
	for attempt := 1; attempt <= 10; attempt++ {
		cert, pool, lcErr := AcquireSystemCert(caURL, caTLSURL, name)
		if lcErr != nil {
			if attempt < 10 {
				log.Printf("[pki-consumer] lifecycle attempt %d/10: %v — retrying in 3s", attempt, lcErr)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[pki-consumer] lifecycle failed after 10 attempts: %v", lcErr)
		}
		syTLSCert = cert
		caPool = pool
		break
	}
	log.Printf("[pki-consumer] system cert acquired (OU=sy, CN=%s)", name)

	// Build mTLS client using system cert.
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{syTLSCert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   5 * time.Second,
	}

	st := &statsTracker{}

	go func() {
		log.Printf("[pki-consumer] health server on :%s", healthPort)
		if serveErr := http.ListenAndServe(":"+healthPort, newMux(st, name)); serveErr != nil {
			log.Fatalf("[pki-consumer] health server: %v", serveErr)
		}
	}()

	log.Printf("[pki-consumer] polling %s/telemetry/latest every %s (mTLS, CN=%s, OU=sy)",
		pkiRestAuthzURL, pollInterval, name)

	for {
		if pollErr := poll(client, pkiRestAuthzURL, service, st); pollErr != nil {
			log.Printf("[pki-consumer] poll error: %v", pollErr)
		}
		time.Sleep(pollInterval)
	}
}
