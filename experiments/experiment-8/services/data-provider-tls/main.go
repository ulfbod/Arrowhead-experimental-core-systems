// data-provider-tls for experiment-8.
//
// Extends experiment-7's data-provider-tls to use the Arrowhead 5.2 onboarding
// lifecycle (on → de → sy) instead of calling /ca/certificate/issue directly.
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
//	CA_URL         profile-ca plain HTTP base URL (default: http://profile-ca:8087)
//	CA_TLS_URL     profile-ca mTLS base URL (default: https://profile-ca:8088)
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

// ── Lifecycle helpers ─────────────────────────────────────────────────────────

type caInfoResponse struct {
	CommonName  string `json:"commonName"`
	Certificate string `json:"certificate"`
}

type certResponse struct {
	SystemName  string `json:"systemName"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
	Profile     string `json:"profile"`
}

// fetchCAPool fetches the CA cert and builds a CertPool.
func fetchCAPool(caURL string) (*x509.CertPool, error) {
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

// requestCert sends POST to url with {"systemName": name}, presenting the given client cert.
func requestCert(url, name string, client *http.Client) (certPEM, keyPEM string, err error) {
	body, _ := json.Marshal(map[string]string{"systemName": name})
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("POST %s returned %d", url, resp.StatusCode)
	}
	var cr certResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", "", fmt.Errorf("decode cert response: %w", err)
	}
	if cr.Certificate == "" || cr.PrivateKey == "" {
		return "", "", fmt.Errorf("empty certificate or key in response")
	}
	return cr.Certificate, cr.PrivateKey, nil
}

// acquireSystemCert performs the full Arrowhead 5.2 lifecycle: on → de → sy.
func acquireSystemCert(caHTTPURL, caTLSURL, systemName string) (tls.Certificate, *x509.CertPool, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}

	caPool, err := fetchCAPool(caHTTPURL)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 1 (CA cert): %w", err)
	}

	onCertPEM, onKeyPEM, err := requestCert(caHTTPURL+"/bootstrap/onboarding-cert", systemName, httpClient)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 2 (onboarding cert): %w", err)
	}
	onTLSCert, err := tls.X509KeyPair([]byte(onCertPEM), []byte(onKeyPEM))
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 2 (parse onboarding cert): %w", err)
	}

	onClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{onTLSCert},
			RootCAs:      caPool,
			MinVersion:   tls.VersionTLS12,
		}},
		Timeout: 10 * time.Second,
	}
	deCertPEM, deKeyPEM, err := requestCert(caTLSURL+"/ca/device-cert", systemName, onClient)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 3 (device cert): %w", err)
	}
	deTLSCert, err := tls.X509KeyPair([]byte(deCertPEM), []byte(deKeyPEM))
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 3 (parse device cert): %w", err)
	}

	deClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{deTLSCert},
			RootCAs:      caPool,
			MinVersion:   tls.VersionTLS12,
		}},
		Timeout: 10 * time.Second,
	}
	syCertPEM, syKeyPEM, err := requestCert(caTLSURL+"/ca/system-cert", systemName, deClient)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 4 (system cert): %w", err)
	}
	syTLSCert, err := tls.X509KeyPair([]byte(syCertPEM), []byte(syKeyPEM))
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 4 (parse system cert): %w", err)
	}

	return syTLSCert, caPool, nil
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
	caURL      := envOr("CA_URL", "http://profile-ca:8087")
	caTLSURL   := envOr("CA_TLS_URL", "https://profile-ca:8088")
	brokersStr := envOr("KAFKA_BROKERS", "kafka:9092")
	topic      := envOr("KAFKA_TOPIC", "arrowhead.telemetry")
	port       := envOr("PORT", "9094")

	brokers := strings.Split(brokersStr, ",")

	// Perform the full Arrowhead 5.2 onboarding lifecycle to acquire a System cert.
	log.Printf("[data-provider-tls] starting PKI lifecycle: on → de → sy for CN=data-provider-tls")
	var ownCert tls.Certificate
	var caPool *x509.CertPool
	for attempt := 1; attempt <= 10; attempt++ {
		cert, pool, lcErr := acquireSystemCert(caURL, caTLSURL, "data-provider-tls")
		if lcErr != nil {
			if attempt < 10 {
				log.Printf("[data-provider-tls] lifecycle attempt %d/10: %v — retrying in 3s", attempt, lcErr)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[data-provider-tls] lifecycle failed after 10 attempts: %v", lcErr)
		}
		ownCert = cert
		caPool = pool
		break
	}
	log.Printf("[data-provider-tls] system cert acquired (OU=sy, CN=data-provider-tls)")

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
