// server.go — HTTP handlers for the kafka-authz service.
//
// kafka-authz is the Kafka enforcement adapter for experiment-5.  It exposes:
//
//   GET  /health                  — liveness probe
//   GET  /status                  — current connected streams
//   GET  /stream/{consumerName}   — SSE stream of Kafka messages (authorized only)
//   POST /auth/check              — explicit AuthzForce decision (for the dashboard)
//
// When a client connects to /stream/{consumerName}?service=telemetry, the
// handler queries AuthzForce.  If the decision is Permit, it subscribes to the
// corresponding Kafka topic and forwards messages as SSE events until the
// client disconnects or the context is cancelled.  If the decision is Deny,
// it returns 403 immediately.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	az "arrowhead/authzforce"
)

type serverConfig struct {
	kafkaBrokers []string
	azDomainID   string
	adminUser    string
	adminPass    string
}

type authzServer struct {
	cfg     serverConfig
	client  *az.Client
	mu      sync.RWMutex
	streams map[string]int64 // consumerName → active stream count
	total   atomic.Int64
}

func newAuthzServer(cfg serverConfig, client *az.Client) *authzServer {
	return &authzServer{
		cfg:     cfg,
		client:  client,
		streams: make(map[string]int64),
	}
}

func (s *authzServer) register(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/stream/", s.handleStream)
	mux.HandleFunc("/auth/check", s.handleAuthCheck)
}

func (s *authzServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}

func (s *authzServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	var active int64
	for _, v := range s.streams {
		active += v
	}
	s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"activeStreams": active,
		"totalServed":  s.total.Load(),
	})
}

// handleAuthCheck accepts POST /auth/check with JSON body
// {"consumer":"...", "service":"..."} and returns the AuthzForce decision.
func (s *authzServer) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Consumer string `json:"consumer"`
		Service  string `json:"service"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Consumer == "" || req.Service == "" {
		http.Error(w, "consumer and service required", http.StatusBadRequest)
		return
	}
	permit, err := s.client.Decide(s.cfg.azDomainID, req.Consumer, req.Service, "subscribe")
	if err != nil {
		log.Printf("[kafka-authz] AuthzForce error check consumer=%q service=%q: %v", req.Consumer, req.Service, err)
		http.Error(w, "PDP unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"consumer": req.Consumer,
		"service":  req.Service,
		"permit":   permit,
		"decision": map[bool]string{true: "Permit", false: "Deny"}[permit],
	})
}

// handleStream serves GET /stream/{consumerName}?service=<service> as SSE.
func (s *authzServer) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	// Extract consumer name from path: /stream/consumer-1
	consumerName := strings.TrimPrefix(r.URL.Path, "/stream/")
	if consumerName == "" {
		http.Error(w, "consumer name required", http.StatusBadRequest)
		return
	}

	service := r.URL.Query().Get("service")
	if service == "" {
		service = "telemetry"
	}

	// AuthzForce decision.
	permit, err := s.client.Decide(s.cfg.azDomainID, consumerName, service, "subscribe")
	if err != nil {
		log.Printf("[kafka-authz] AuthzForce error stream consumer=%q service=%q: %v", consumerName, service, err)
		http.Error(w, "PDP unavailable", http.StatusServiceUnavailable)
		return
	}
	if !permit {
		log.Printf("[kafka-authz] DENY stream consumer=%q service=%q", consumerName, service)
		http.Error(w, `{"error":"not authorized"}`, http.StatusForbidden)
		return
	}

	log.Printf("[kafka-authz] PERMIT stream consumer=%q service=%q", consumerName, service)

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("[kafka-authz] ResponseWriter does not support flushing")
		return
	}
	// Flush SSE headers immediately so the client's http.Get() returns right
	// away, even before the first Kafka message arrives.  Without this, the
	// client blocks waiting for response headers while we block waiting for
	// Kafka — a silent deadlock that lasts until the first message is produced.
	flusher.Flush()

	// Track active stream.
	s.mu.Lock()
	s.streams[consumerName]++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.streams[consumerName]--
		if s.streams[consumerName] == 0 {
			delete(s.streams, consumerName)
		}
		s.mu.Unlock()
	}()

	// Subscribe to Kafka topic.
	topic := topicForService(service)
	reader := newKafkaReader(s.cfg.kafkaBrokers, topic)
	defer reader.Close()

	ctx := r.Context()

	// Kafka reader goroutine — runs with the request context so it exits when
	// the client disconnects.  Buffered channels prevent goroutine leaks when
	// the handler returns before the goroutine does.
	type kafkaResult struct {
		msg []byte
		err error
	}
	resultCh := make(chan kafkaResult, 1)
	go func() {
		for {
			msg, err := reader.ReadMessage(ctx)
			select {
			case resultCh <- kafkaResult{msg, err}:
				if err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Keepalive ticker — sends an SSE comment every 5 s so the connection
	// stays alive even when Kafka has no new messages.
	keepalive := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()

	msgN := 0
	for {
		select {
		case <-ctx.Done():
			return

		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()

		case res := <-resultCh:
			if res.err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("[kafka-authz] Kafka read error consumer=%q: %v", consumerName, res.err)
				fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%v\"}\n\n", res.err)
				flusher.Flush()
				return
			}

			// Re-check AuthzForce every 100 messages to detect revocation.
			msgN++
			if msgN%100 == 0 {
				ok, err := s.client.Decide(s.cfg.azDomainID, consumerName, service, "subscribe")
				if err != nil {
					log.Printf("[kafka-authz] AuthzForce re-check error: %v", err)
				} else if !ok {
					log.Printf("[kafka-authz] revoked mid-stream consumer=%q", consumerName)
					fmt.Fprintf(w, "event: revoked\ndata: {\"reason\":\"grant revoked\"}\n\n")
					flusher.Flush()
					return
				}
			}

			fmt.Fprintf(w, "data: %s\n\n", res.msg)
			flusher.Flush()
			s.total.Add(1)
		}
	}
}

// decideForTesting wraps Decide for use in tests.
func (s *authzServer) decideForTesting(ctx context.Context, consumer, service string) (bool, error) {
	return s.client.Decide(s.cfg.azDomainID, consumer, service, "subscribe")
}
