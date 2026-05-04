// analytics-consumer for experiment-5.
//
// Connects to kafka-authz's SSE endpoint to receive telemetry over the Kafka
// transport path.  Authorization is enforced by kafka-authz (backed by the
// same AuthzForce PDP as topic-auth-xacml), demonstrating unified policy
// projection across two transports.
//
// Flow:
//  1. GET /stream/{consumerName}?service=telemetry  (SSE, via kafka-authz)
//  2. If 403 → log denial and retry after back-off
//  3. On "event: revoked" SSE event → disconnect and retry
//
// Environment variables:
//
//	CONSUMER_NAME     system name (default: analytics-consumer)
//	KAFKA_AUTHZ_URL   base URL of kafka-authz (default: http://kafka-authz:9091)
//	SERVICE           service name to subscribe to (default: telemetry)
//	HEALTH_PORT       HTTP port for /health and /stats (default: 9004)
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	gosync "sync"
	"sync/atomic"
	"strings"
	"time"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ── Stats ──────────────────────────────────────────────────────────────────────

type statsTracker struct {
	msgCount       atomic.Int64
	mu             gosync.Mutex
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
		"transport":      "kafka-sse",
		"msgCount":       st.msgCount.Load(),
		"lastReceivedAt": last,
		"lastDeniedAt":   denied,
	}
}

// ── SSE stream ─────────────────────────────────────────────────────────────────

// subscribe opens the SSE stream and reads until revoked, closed, or error.
// Returns nil on normal close, an error on unexpected failure, or
// errRevoked when the server sends an "event: revoked" line.
var errRevoked = fmt.Errorf("access revoked by policy")

func subscribe(kafkaAuthzURL, consumer, service string, st *statsTracker) error {
	url := fmt.Sprintf("%s/stream/%s?service=%s", kafkaAuthzURL, consumer, service)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		st.recordDenied()
		return fmt.Errorf("access denied (403) — no grant for %s on %s", consumer, service)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	log.Printf("[%s] SSE stream open (kafka-authz → topic arrowhead.%s)", consumer, service)

	scanner := bufio.NewScanner(resp.Body)
	var eventType string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if eventType == "revoked" {
				log.Printf("[%s] policy revoked — disconnecting", consumer)
				st.recordDenied()
				return errRevoked
			}
			st.recordMsg()
			log.Printf("[%s] received: %s", consumer, data)
			eventType = ""
		case line == "":
			eventType = ""
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream read: %w", err)
	}
	return fmt.Errorf("stream closed by server")
}

func main() {
	name         := envOr("CONSUMER_NAME", "analytics-consumer")
	kafkaAuthzURL := envOr("KAFKA_AUTHZ_URL", "http://kafka-authz:9091")
	service      := envOr("SERVICE", "telemetry")
	healthPort   := envOr("HEALTH_PORT", "9004")

	st := &statsTracker{}

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

	// Retry loop with exponential back-off.
	backoff := 5 * time.Second
	for {
		err := subscribe(kafkaAuthzURL, name, service, st)
		if err == errRevoked {
			log.Printf("[%s] revoked — retrying in %s", name, backoff)
		} else if err != nil {
			log.Printf("[%s] error: %v — retrying in %s", name, err, backoff)
		}
		time.Sleep(backoff)
		if backoff < 60*time.Second {
			backoff *= 2
		}
	}
}
