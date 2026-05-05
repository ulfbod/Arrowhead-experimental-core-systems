// data-provider for experiment-6.
//
// Consumes telemetry messages from the Kafka topic arrowhead.telemetry and
// exposes them via a REST API.  rest-authz sits in front of this service and
// enforces XACML authorization on every request, so data-provider itself does
// not perform any authorization — it is a plain read-only data server.
//
// REST endpoints:
//
//	GET /health              — liveness probe
//	GET /stats               — message count and last-received timestamp
//	GET /telemetry/latest    — most recently received message across all robots
//	GET /telemetry/{robotId} — most recently received message for robotId
//
// Environment variables:
//
//	KAFKA_BROKERS  comma-separated Kafka broker addresses (default: kafka:9092)
//	KAFKA_TOPIC    topic to consume (default: arrowhead.telemetry)
//	PORT           HTTP listen port (default: 9094)
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	gosync "sync"
	"sync/atomic"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ── State ─────────────────────────────────────────────────────────────────────

type telemetryStore struct {
	mu             gosync.RWMutex
	latest         []byte            // raw JSON of most recent message
	byRobot        map[string][]byte // robotId → raw JSON
	msgCount       atomic.Int64
	lastReceivedAt gosync.Mutex
	lastReceived   string
}

func newStore() *telemetryStore {
	return &telemetryStore{byRobot: make(map[string][]byte)}
}

func (s *telemetryStore) record(robotID string, raw []byte) {
	s.msgCount.Add(1)
	s.mu.Lock()
	s.latest = raw
	if robotID != "" {
		s.byRobot[robotID] = raw
	}
	s.mu.Unlock()

	s.lastReceivedAt.Lock()
	s.lastReceived = time.Now().UTC().Format(time.RFC3339)
	s.lastReceivedAt.Unlock()
}

func (s *telemetryStore) getLatest() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.latest == nil {
		return []byte(`null`)
	}
	return s.latest
}

func (s *telemetryStore) getByRobot(robotID string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msg, ok := s.byRobot[robotID]
	return msg, ok
}

func (s *telemetryStore) stats() map[string]interface{} {
	s.lastReceivedAt.Lock()
	ts := s.lastReceived
	s.lastReceivedAt.Unlock()

	s.mu.RLock()
	robots := len(s.byRobot)
	s.mu.RUnlock()

	return map[string]interface{}{
		"msgCount":       s.msgCount.Load(),
		"robotCount":     robots,
		"lastReceivedAt": ts,
	}
}

// ── Kafka consumer ────────────────────────────────────────────────────────────

func consumeKafka(brokers []string, topic string, store *telemetryStore) {
	// Use a partition-level reader (no consumer group) for the same reason as
	// kafka-authz: consumer group coordination can fail silently when the topic
	// does not yet exist at startup time (data-provider starts before
	// robot-fleet creates the topic).  A partition reader is simpler, has no
	// rebalancing overhead, and recovers correctly when the topic is created
	// after the reader starts.
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		Partition:   0,
		MinBytes:    1,
		MaxBytes:    1e6,
		MaxWait:     500 * time.Millisecond,
		StartOffset: kafka.LastOffset,
	})
	defer r.Close()

	log.Printf("[data-provider] Kafka consumer started (brokers=%v topic=%s)", brokers, topic)

	for {
		m, err := r.ReadMessage(context.Background())
		if err != nil {
			log.Printf("[data-provider] Kafka read error: %v — retrying in 3s", err)
			time.Sleep(3 * time.Second)
			continue
		}

		// Key is e.g. "telemetry.robot-1"; extract robotId.
		key := string(m.Key)
		robotID := ""
		if parts := strings.SplitN(key, ".", 2); len(parts) == 2 {
			robotID = parts[1]
		}

		store.record(robotID, m.Value)
		log.Printf("[data-provider] received key=%s (%d bytes)", key, len(m.Value))
	}
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

func makeHandler(store *telemetryStore) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(store.stats())
	})

	// GET /telemetry/latest  or  GET /telemetry/{robotId}
	mux.HandleFunc("/telemetry/", func(w http.ResponseWriter, r *http.Request) {
		suffix := strings.TrimPrefix(r.URL.Path, "/telemetry/")
		w.Header().Set("Content-Type", "application/json")

		if suffix == "latest" || suffix == "" {
			_, _ = w.Write(store.getLatest())
			return
		}

		msg, ok := store.getByRobot(suffix)
		if !ok {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write(msg)
	})

	return mux
}

func main() {
	brokersStr := envOr("KAFKA_BROKERS", "kafka:9092")
	topic      := envOr("KAFKA_TOPIC", "arrowhead.telemetry")
	port       := envOr("PORT", "9094")

	brokers := strings.Split(brokersStr, ",")

	store := newStore()

	go consumeKafka(brokers, topic, store)

	log.Printf("[data-provider] HTTP server on :%s", port)
	if err := http.ListenAndServe(":"+port, makeHandler(store)); err != nil {
		log.Fatal(err)
	}
}
