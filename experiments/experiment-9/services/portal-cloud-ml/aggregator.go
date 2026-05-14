// aggregator.go — SSE consumer and telemetry buffer for portal-cloud-ml.
//
// ConnectSSE opens a GET /stream/{consumerName}?service={service} SSE connection
// to kafka-authz and feeds each data line into the Store.
package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Store holds the latest telemetry snapshot received from robot sites.
type Store struct {
	mu             sync.RWMutex
	latest         []byte
	msgCount       atomic.Int64
	lastReceivedAt sync.Mutex
	lastReceived   string
	deniedCount    atomic.Int64
}

// NewStore creates an empty Store.
func NewStore() *Store {
	return &Store{}
}

// Record stores a received telemetry payload.
func (s *Store) Record(payload []byte) {
	s.msgCount.Add(1)
	s.mu.Lock()
	s.latest = append([]byte(nil), payload...)
	s.mu.Unlock()

	s.lastReceivedAt.Lock()
	s.lastReceived = time.Now().UTC().Format(time.RFC3339)
	s.lastReceivedAt.Unlock()
}

// Latest returns a copy of the most recent payload, or nil if none.
func (s *Store) Latest() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.latest == nil {
		return nil
	}
	out := make([]byte, len(s.latest))
	copy(out, s.latest)
	return out
}

// Stats returns a snapshot of counters.
func (s *Store) Stats() map[string]interface{} {
	s.lastReceivedAt.Lock()
	ts := s.lastReceived
	s.lastReceivedAt.Unlock()

	return map[string]interface{}{
		"msgCount":       s.msgCount.Load(),
		"deniedCount":    s.deniedCount.Load(),
		"lastReceivedAt": ts,
		"transport":      "kafka-sse",
	}
}

// ParseSSELine extracts the payload from an SSE "data: ..." line.
// Returns ("", false) for comment, empty, or non-data lines.
func ParseSSELine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data: ") {
		return "", false
	}
	payload := strings.TrimPrefix(line, "data: ")
	if payload == "" {
		return "", false
	}
	return payload, true
}

// ConnectSSE opens an SSE stream to kafkaAuthzURL for consumerName/service
// and calls record for every data line. Reconnects on error with backoff.
// Exits only when ctx is cancelled via the caller closing the done channel.
func ConnectSSE(kafkaAuthzURL, consumerName, service string, store *Store, done <-chan struct{}) {
	url := fmt.Sprintf("%s/stream/%s?service=%s", kafkaAuthzURL, consumerName, service)
	backoff := 2 * time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-done:
			return
		default:
		}

		log.Printf("[portal-cloud-ml] SSE connecting to %s", url)
		if err := connectOnce(url, store, done); err != nil {
			log.Printf("[portal-cloud-ml] SSE disconnected: %v — reconnecting in %s", err, backoff)
		} else {
			log.Printf("[portal-cloud-ml] SSE stream ended — reconnecting in %s", backoff)
		}

		select {
		case <-done:
			return
		case <-time.After(backoff):
			if backoff < maxBackoff {
				backoff *= 2
			}
		}
	}
}

func connectOnce(url string, store *Store, done <-chan struct{}) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{} // plain HTTP to kafka-authz
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		store.deniedCount.Add(1)
		return fmt.Errorf("kafka-authz denied (403) — check ConsumerAuth grants")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}

	log.Printf("[portal-cloud-ml] SSE stream connected (200)")
	backoff := 2 * time.Second
	_ = backoff

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-done:
			return nil
		default:
		}
		line := scanner.Text()
		payload, ok := ParseSSELine(line)
		if !ok {
			continue
		}
		store.Record([]byte(payload))
		log.Printf("[portal-cloud-ml] received SSE payload (%d bytes)", len(payload))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}
	return nil
}
