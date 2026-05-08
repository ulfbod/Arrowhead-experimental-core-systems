// data-provider-tls for experiment-7.
//
// Extends experiment-6's data-provider with TLS on both transports:
//   - Kafka consumer uses TLS (CA pool, no client cert required)
//   - HTTP server runs as HTTPS using own cert issued by the Arrowhead CA
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
//	CA_URL         Arrowhead CA base URL (default: http://ca:8086)
//	KAFKA_BROKERS  comma-separated Kafka broker addresses (default: kafka:9092)
//	KAFKA_TOPIC    topic to consume (default: arrowhead.telemetry)
//	PORT           HTTPS listen port (default: 9094)
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
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

// fetchCACert retrieves the CA certificate and returns a CertPool.
func fetchCACert(caURL string) (*x509.CertPool, error) {
	resp, err := http.Get(caURL + "/ca/info")
	if err != nil {
		return nil, fmt.Errorf("GET /ca/info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /ca/info returned %d", resp.StatusCode)
	}
	var info caInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode /ca/info: %w", err)
	}
	if info.Certificate == "" {
		return nil, fmt.Errorf("CA info: empty certificate")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(info.Certificate)) {
		return nil, fmt.Errorf("parse CA cert PEM")
	}
	return pool, nil
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

// ── State ─────────────────────────────────────────────────────────────────────

type telemetryStore struct {
	mu             gosync.RWMutex
	latest         []byte
	byRobot        map[string][]byte
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

func consumeKafka(brokers []string, topic string, store *telemetryStore, tlsCfg *tls.Config) {
	dialer := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
		TLS:       tlsCfg,
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   brokers,
		Topic:     topic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  1e6,
		MaxWait:   500 * time.Millisecond,
		StartOffset: kafka.LastOffset,
		Dialer:    dialer,
	})
	defer r.Close()

	log.Printf("[data-provider-tls] Kafka consumer started (TLS, brokers=%v topic=%s)", brokers, topic)

	for {
		m, err := r.ReadMessage(context.Background())
		if err != nil {
			log.Printf("[data-provider-tls] Kafka read error: %v — retrying in 3s", err)
			time.Sleep(3 * time.Second)
			continue
		}

		key := string(m.Key)
		robotID := ""
		if parts := strings.SplitN(key, ".", 2); len(parts) == 2 {
			robotID = parts[1]
		}

		store.record(robotID, m.Value)
		log.Printf("[data-provider-tls] received key=%s (%d bytes)", key, len(m.Value))
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
	caURL      := envOr("CA_URL", "http://ca:8086")
	brokersStr := envOr("KAFKA_BROKERS", "kafka:9092")
	topic      := envOr("KAFKA_TOPIC", "arrowhead.telemetry")
	port       := envOr("PORT", "9094")

	brokers := strings.Split(brokersStr, ",")

	// Fetch CA cert and issue own cert with retry.
	log.Printf("[data-provider-tls] fetching CA cert from %s", caURL)
	var caPool *x509.CertPool
	for attempt := 1; attempt <= 10; attempt++ {
		pool, err := fetchCACert(caURL)
		if err != nil {
			if attempt < 10 {
				log.Printf("[data-provider-tls] CA fetch attempt %d/10: %v — retrying in 3s", attempt, err)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[data-provider-tls] CA fetch failed after 10 attempts: %v", err)
		}
		caPool = pool
		break
	}

	log.Printf("[data-provider-tls] issuing own cert from %s", caURL)
	var ownCert tls.Certificate
	for attempt := 1; attempt <= 10; attempt++ {
		cert, err := issueCert(caURL, "data-provider-tls")
		if err != nil {
			if attempt < 10 {
				log.Printf("[data-provider-tls] cert issue attempt %d/10: %v — retrying in 3s", attempt, err)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[data-provider-tls] cert issue failed after 10 attempts: %v", err)
		}
		ownCert = cert
		break
	}

	// TLS config for Kafka reader: server-only TLS (no client cert).
	kafkaTLSCfg := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}

	store := newStore()
	go consumeKafka(brokers, topic, store, kafkaTLSCfg)

	// TLS config for HTTPS server: server-only TLS.
	serverTLSCfg := &tls.Config{
		Certificates: []tls.Certificate{ownCert},
		MinVersion:   tls.VersionTLS12,
	}

	log.Printf("[data-provider-tls] HTTPS server on :%s", port)
	ln, err := tls.Listen("tcp", ":"+port, serverTLSCfg)
	if err != nil {
		log.Fatalf("[data-provider-tls] listen HTTPS :%s: %v", port, err)
	}
	if err := http.Serve(ln, makeHandler(store)); err != nil {
		log.Fatal(err)
	}
}
