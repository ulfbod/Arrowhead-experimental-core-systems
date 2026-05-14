// server.go — HTTP handlers for the kafka-authz service (experiment-13).
//
// This is a local copy of support/kafka-authz/server.go enriched with PIP
// cert-level attribute queries. Before every AuthzForce decision, the PEP
// queries PIP for the consumer's cert-level attributes, then passes them as
// additional subject attributes in the XACML request.
//
// Decision D1: no PEP-side caching of PIP responses.
// Decision D2: cert-valid is forwarded to AuthzForce; the PEP does not pre-gate on it.
// Fail-closed: PIP 404 or unreachable → certLevel="", certValid=false → AuthzForce likely DENY.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type serverConfig struct {
	kafkaBrokers []string
	azDomainID   string
	azURL        string // AuthzForce base URL (e.g. http://authzforce:8080/authzforce-ce)
	pipURL       string // PIP base URL (e.g. http://pip:9506)
	adminUser    string
	adminPass    string
	// tlsConfig is optional. When non-nil, Kafka connections use TLS.
	tlsConfig *tls.Config
}

type authzServer struct {
	cfg     serverConfig
	pip     *pipClient
	mu      sync.RWMutex
	streams map[string]int64 // consumerName → active stream count
	total   atomic.Int64
}

func newAuthzServer(cfg serverConfig) *authzServer {
	return &authzServer{
		cfg:     cfg,
		pip:     newPIPClient(cfg.pipURL),
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
// The PIP is queried first to enrich the XACML request with cert-level attrs.
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

	attrs, _ := s.pip.GetAttributes(req.Consumer)
	permit, err := decideWithCertLevel(s.cfg.azURL, s.cfg.azDomainID, req.Consumer, req.Service, "subscribe", attrs.CertLevel, attrs.CertValid)
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
// PIP is queried before AuthzForce to enrich the XACML request.
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

	// Query PIP for cert-level attributes (D1: no caching, query on every decision).
	attrs, _ := s.pip.GetAttributes(consumerName)

	// AuthzForce decision with cert-level enrichment.
	permit, err := decideWithCertLevel(s.cfg.azURL, s.cfg.azDomainID, consumerName, service, "subscribe", attrs.CertLevel, attrs.CertValid)
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

	log.Printf("[kafka-authz] PERMIT stream consumer=%q service=%q certLevel=%q", consumerName, service, attrs.CertLevel)

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
	reader := newKafkaReader(s.cfg.kafkaBrokers, topic, s.cfg.tlsConfig)
	defer reader.Close()

	ctx := r.Context()

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
			// PIP is re-queried each time (D1: no caching).
			msgN++
			if msgN%100 == 0 {
				reAttrs, _ := s.pip.GetAttributes(consumerName)
				ok, err := decideWithCertLevel(s.cfg.azURL, s.cfg.azDomainID, consumerName, service, "subscribe", reAttrs.CertLevel, reAttrs.CertValid)
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

// decideForTesting wraps decideWithCertLevel for use in tests.
func (s *authzServer) decideForTesting(ctx context.Context, consumer, service string) (bool, error) {
	attrs, _ := s.pip.GetAttributes(consumer)
	return decideWithCertLevel(s.cfg.azURL, s.cfg.azDomainID, consumer, service, "subscribe", attrs.CertLevel, attrs.CertValid)
}
