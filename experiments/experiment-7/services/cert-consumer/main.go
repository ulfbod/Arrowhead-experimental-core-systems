// cert-consumer for experiment-7.
//
// Periodically polls the cert-rest-authz mTLS proxy for telemetry data,
// demonstrating certificate-based identity (mTLS) for REST authorization.
// The consumer authenticates with its own X.509 certificate; the consumer
// identity is taken from the certificate CN — no X-Consumer-Name header needed.
//
// Flow:
//  1. At startup: fetch CA cert (GET /ca/info), issue own cert (POST /ca/certificate/issue).
//  2. Build an HTTPS client using own cert + CA pool (mTLS client).
//  3. Poll {CERT_REST_AUTHZ_URL}/telemetry/latest using the mTLS client.
//  4. 200 OK → record message, update msgCount / lastReceivedAt.
//  5. 403 Forbidden → record denial, update lastDeniedAt.
//  6. Other error → log and retry after POLL_INTERVAL.
//
// Environment variables:
//
//	CONSUMER_NAME       system name / cert CN (default: cert-consumer)
//	CA_URL              Arrowhead CA base URL (default: http://ca:8086)
//	CERT_REST_AUTHZ_URL mTLS proxy base URL (default: https://cert-rest-authz:9098)
//	SERVICE             service name for X-Service-Name header (default: telemetry-rest)
//	POLL_INTERVAL       how often to poll (default: 2s)
//	HEALTH_PORT         HTTP port for /health and /stats (default: 9096)
package main

import (
	"bytes"
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

// ── CA client helpers ─────────────────────────────────────────────────────────

type caInfoResponse struct {
	CommonName  string `json:"commonName"`
	Certificate string `json:"certificate"`
}

type issueCertRequest struct {
	SystemName string `json:"systemName"`
}

type issueCertResponse struct {
	SystemName  string `json:"systemName"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
	IssuedAt   string `json:"issuedAt"`
	ExpiresAt  string `json:"expiresAt"`
}

// fetchCACert retrieves the CA certificate and returns a CertPool and the raw PEM.
func fetchCACert(caURL string) (*x509.CertPool, []byte, error) {
	resp, err := http.Get(caURL + "/ca/info")
	if err != nil {
		return nil, nil, fmt.Errorf("GET /ca/info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("GET /ca/info returned %d", resp.StatusCode)
	}
	var info caInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, nil, fmt.Errorf("decode /ca/info: %w", err)
	}
	if info.Certificate == "" {
		return nil, nil, fmt.Errorf("CA info: empty certificate")
	}
	pem := []byte(info.Certificate)
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, nil, fmt.Errorf("parse CA cert PEM")
	}
	return pool, pem, nil
}

// issueCert issues a certificate for systemName and returns a tls.Certificate.
func issueCert(caURL, systemName string) (tls.Certificate, error) {
	body, _ := json.Marshal(issueCertRequest{SystemName: systemName})
	resp, err := http.Post(caURL+"/ca/certificate/issue", "application/json", bytes.NewReader(body))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("POST /ca/certificate/issue: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return tls.Certificate{}, fmt.Errorf("POST /ca/certificate/issue returned %d", resp.StatusCode)
	}
	var certResp issueCertResponse
	if err := json.NewDecoder(resp.Body).Decode(&certResp); err != nil {
		return tls.Certificate{}, fmt.Errorf("decode issue cert response: %w", err)
	}
	cert, err := tls.X509KeyPair([]byte(certResp.Certificate), []byte(certResp.PrivateKey))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse key pair: %w", err)
	}
	return cert, nil
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
		"transport":      "rest-mtls",
		"msgCount":       st.msgCount.Load(),
		"deniedCount":    st.deniedCount.Load(),
		"lastReceivedAt": last,
		"lastDeniedAt":   denied,
	}
}

// ── Poll loop ─────────────────────────────────────────────────────────────────

// poll performs a single GET request against the cert-rest-authz proxy.
// The consumer identity is provided by the mTLS client certificate (CN).
func poll(client *http.Client, certRestAuthzURL, service string, st *statsTracker) error {
	url := fmt.Sprintf("%s/telemetry/latest", certRestAuthzURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	// No X-Consumer-Name header — identity comes from the client certificate CN.
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
		log.Printf("[cert-consumer] OK  %d bytes: %.120s", len(body), string(body))
	case http.StatusForbidden:
		st.recordDenied()
		log.Printf("[cert-consumer] 403 Forbidden (access denied by cert-rest-authz/AuthzForce)")
	default:
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

// newMux returns an http.ServeMux with /health and /stats registered.
func newMux(st *statsTracker, name string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st.snapshot(name))
	})
	return mux
}

func main() {
	name            := envOr("CONSUMER_NAME", "cert-consumer")
	caURL           := envOr("CA_URL", "http://ca:8086")
	certRestAuthzURL := envOr("CERT_REST_AUTHZ_URL", "https://cert-rest-authz:9098")
	service         := envOr("SERVICE", "telemetry-rest")
	pollIntervalStr := envOr("POLL_INTERVAL", "2s")
	healthPort      := envOr("HEALTH_PORT", "9096")

	pollInterval, err := time.ParseDuration(pollIntervalStr)
	if err != nil || pollInterval < time.Second {
		log.Fatalf("[cert-consumer] invalid POLL_INTERVAL %q: must be ≥1s", pollIntervalStr)
	}

	// Fetch CA cert and issue own cert with retry.
	log.Printf("[cert-consumer] fetching CA cert from %s", caURL)
	var caPool *x509.CertPool
	for attempt := 1; attempt <= 10; attempt++ {
		pool, _, fetchErr := fetchCACert(caURL)
		if fetchErr != nil {
			if attempt < 10 {
				log.Printf("[cert-consumer] CA fetch attempt %d/10: %v — retrying in 3s", attempt, fetchErr)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[cert-consumer] CA fetch failed after 10 attempts: %v", fetchErr)
		}
		caPool = pool
		break
	}

	log.Printf("[cert-consumer] issuing own cert from %s", caURL)
	var ownCert tls.Certificate
	for attempt := 1; attempt <= 10; attempt++ {
		cert, issueErr := issueCert(caURL, name)
		if issueErr != nil {
			if attempt < 10 {
				log.Printf("[cert-consumer] cert issue attempt %d/10: %v — retrying in 3s", attempt, issueErr)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[cert-consumer] cert issue failed after 10 attempts: %v", issueErr)
		}
		ownCert = cert
		break
	}

	// Build mTLS client: presents own cert, verifies server against CA pool.
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{ownCert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   5 * time.Second,
	}

	st := &statsTracker{}

	go func() {
		log.Printf("[cert-consumer] health server on :%s", healthPort)
		if err := http.ListenAndServe(":"+healthPort, newMux(st, name)); err != nil {
			log.Fatalf("[cert-consumer] health server: %v", err)
		}
	}()

	log.Printf("[cert-consumer] polling %s/telemetry/latest every %s (mTLS, CN=%s)",
		certRestAuthzURL, pollInterval, name)

	for {
		if err := poll(client, certRestAuthzURL, service, st); err != nil {
			log.Printf("[cert-consumer] poll error: %v", err)
		}
		time.Sleep(pollInterval)
	}
}
