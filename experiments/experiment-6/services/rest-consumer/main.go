// rest-consumer for experiment-6.
//
// Periodically polls the rest-authz reverse proxy for telemetry data,
// demonstrating REST authorization via the unified XACML policy.  The consumer
// identifies itself with the X-Consumer-Name request header; rest-authz
// forwards this identity to AuthzForce for an (consumer, service, "invoke")
// decision.
//
// Flow:
//  1. GET {REST_AUTHZ_URL}/telemetry/latest
//     with header X-Consumer-Name: {CONSUMER_NAME}
//  2. 200 OK → record message, update msgCount / lastReceivedAt
//  3. 403 Forbidden → record denial, update lastDeniedAt
//  4. Other error → log and retry after POLL_INTERVAL
//
// This mirrors the AHC flow for AMQP/Kafka consumers but for REST.  The
// rest-authz PEP enforces the same AuthzForce policy as topic-auth-xacml
// (AMQP) and kafka-authz (Kafka), achieving unified policy projection across
// all three transports.
//
// Sync-delay caveat: if a grant is revoked, the first 403 response occurs
// after the next policy-sync cycle has uploaded the new PolicySet to
// AuthzForce.  This means a REST consumer may receive 200 OK for up to
// SYNC_INTERVAL after revocation.
//
// Environment variables:
//
//	CONSUMER_NAME   system name sent in X-Consumer-Name header (default: rest-consumer)
//	REST_AUTHZ_URL  base URL of rest-authz (default: http://rest-authz:9093)
//	SERVICE         service name for X-Service-Name header (default: telemetry-rest)
//	POLL_INTERVAL   how often to poll (default: 2s)
//	HEALTH_PORT     HTTP port for /health and /stats (default: 9097)
package main

import (
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
	msgCount     atomic.Int64
	deniedCount  atomic.Int64
	mu           gosync.Mutex
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
		"transport":      "rest",
		"msgCount":       st.msgCount.Load(),
		"deniedCount":    st.deniedCount.Load(),
		"lastReceivedAt": last,
		"lastDeniedAt":   denied,
	}
}

// ── Poll loop ─────────────────────────────────────────────────────────────────

func poll(client *http.Client, restAuthzURL, consumer, service string, st *statsTracker) error {
	url := fmt.Sprintf("%s/telemetry/latest", restAuthzURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("X-Consumer-Name", consumer)
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
		log.Printf("[%s] OK  %d bytes: %.120s", consumer, len(body), string(body))
	case http.StatusForbidden:
		st.recordDenied()
		log.Printf("[%s] 403 Forbidden (access denied by rest-authz/AuthzForce)", consumer)
	default:
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func main() {
	name         := envOr("CONSUMER_NAME", "rest-consumer")
	restAuthzURL := envOr("REST_AUTHZ_URL", "http://rest-authz:9093")
	service      := envOr("SERVICE", "telemetry-rest")
	pollIntervalStr := envOr("POLL_INTERVAL", "2s")
	healthPort   := envOr("HEALTH_PORT", "9097")

	pollInterval, err := time.ParseDuration(pollIntervalStr)
	if err != nil || pollInterval < time.Second {
		log.Fatalf("invalid POLL_INTERVAL %q: must be ≥1s", pollIntervalStr)
	}

	st := &statsTracker{}
	client := &http.Client{Timeout: 5 * time.Second}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st.snapshot(name))
	})

	go func() {
		log.Printf("[%s] health server on :%s", name, healthPort)
		if err := http.ListenAndServe(":"+healthPort, nil); err != nil {
			log.Fatalf("health server: %v", err)
		}
	}()

	log.Printf("[%s] polling %s/telemetry/latest every %s", name, restAuthzURL, pollInterval)

	for {
		if err := poll(client, restAuthzURL, name, service, st); err != nil {
			log.Printf("[%s] poll error: %v", name, err)
		}
		time.Sleep(pollInterval)
	}
}
