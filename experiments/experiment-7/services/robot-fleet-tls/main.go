// robot-fleet-tls for experiment-7.
//
// Extends experiment-5's robot-fleet by adding TLS to both transports:
//   - RabbitMQ: AMQPS (TLS AMQP) using CA pool
//   - Kafka: TLS transport using CA pool
//
// Environment variables (same as experiment-5 except AMQP_URL uses amqps://):
//
//	AMQP_URL              AMQPS connection string (default: amqps://guest:guest@rabbitmq:5671/)
//	FLEET_PORT            HTTP port for /health, /config, /stats (default: 9003)
//	INITIAL_ROBOT_COUNT   number of robots (default: 3)
//	PAYLOAD_TYPE          "basic" | "imu" (default: imu)
//	PAYLOAD_HZ            publish rate in Hz (default: 10)
//	NETWORK_PRESET        network profile preset (default: 5g_good)
//	SR_URL                ServiceRegistry base URL (optional)
//	SYSTEM_NAME           system name for SR registration (default: robot-fleet-tls)
//	AUTH_URL              Authentication system URL (optional)
//	SYSTEM_CREDENTIALS    credentials for auth login (default: fleet-secret)
//	KAFKA_BROKERS         comma-separated Kafka broker addresses (default: kafka:9092)
//	KAFKA_TOPIC           Kafka topic (default: arrowhead.telemetry)
//	CA_URL                Arrowhead CA base URL (default: http://ca:8086)
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	broker "arrowhead/message-broker"
	kafka "github.com/segmentio/kafka-go"
)

var (
	fleetMu sync.RWMutex
	fleet   *Fleet
)

type srSystem struct {
	SystemName string `json:"systemName"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
}

type srRegisterRequest struct {
	ServiceDefinition string            `json:"serviceDefinition"`
	ProviderSystem    srSystem          `json:"providerSystem"`
	ServiceUri        string            `json:"serviceUri"`
	Interfaces        []string          `json:"interfaces"`
	Version           int               `json:"version"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type srUnregisterRequest struct {
	ServiceDefinition string   `json:"serviceDefinition"`
	ProviderSystem    srSystem `json:"providerSystem"`
	Version           int      `json:"version"`
}

type authLoginRequest struct {
	SystemName  string `json:"systemName"`
	Credentials string `json:"credentials"`
}

type authLoginResponse struct {
	Token      string    `json:"token"`
	SystemName string    `json:"systemName"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ── CA helpers ────────────────────────────────────────────────────────────────

type caInfoResponse struct {
	CommonName  string `json:"commonName"`
	Certificate string `json:"certificate"`
}

// fetchCACertPool fetches the CA certificate and returns an x509.CertPool.
func fetchCACertPool(caURL string) (*x509.CertPool, error) {
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

func login(authURL, systemName, credentials string) (string, error) {
	if authURL == "" {
		return "", nil
	}
	body, _ := json.Marshal(authLoginRequest{SystemName: systemName, Credentials: credentials})
	resp, err := http.Post(authURL+"/authentication/identity/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("auth login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("auth login returned %d: %s", resp.StatusCode, string(b))
	}
	var lr authLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return "", fmt.Errorf("auth login decode: %w", err)
	}
	return lr.Token, nil
}

func registerServices(srURL, systemName, kafkaAuthzAddress string, kafkaAuthzPort int) error {
	reqs := []srRegisterRequest{
		{
			ServiceDefinition: "telemetry",
			ProviderSystem:    srSystem{SystemName: systemName, Address: "rabbitmq", Port: 5671},
			ServiceUri:        "arrowhead",
			Interfaces:        []string{"AMQP-SECURE-JSON"},
			Version:           1,
			Metadata:          map[string]string{"exchangeType": "topic", "routingKeyPattern": "telemetry.*"},
		},
		{
			ServiceDefinition: "telemetry",
			ProviderSystem:    srSystem{SystemName: systemName, Address: kafkaAuthzAddress, Port: kafkaAuthzPort},
			ServiceUri:        "/stream",
			Interfaces:        []string{"KAFKA-SECURE-JSON"},
			Version:           1,
			Metadata:          map[string]string{"transport": "kafka-sse"},
		},
	}
	for _, req := range reqs {
		body, _ := json.Marshal(req)
		resp, err := http.Post(srURL+"/serviceregistry/register", "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("SR register %v: %w", req.Interfaces, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("SR register %v returned %d", req.Interfaces, resp.StatusCode)
		}
		log.Printf("[robot-fleet-tls] registered %v service in ServiceRegistry", req.Interfaces)
	}
	return nil
}

func unregisterServices(srURL, systemName string) {
	for _, iface := range []string{"AMQP-SECURE-JSON", "KAFKA-SECURE-JSON"} {
		addr := "rabbitmq"
		port := 5671
		if iface == "KAFKA-SECURE-JSON" {
			addr = "kafka-authz"
			port = 9091
		}
		req := srUnregisterRequest{
			ServiceDefinition: "telemetry",
			ProviderSystem:    srSystem{SystemName: systemName, Address: addr, Port: port},
			Version:           1,
		}
		body, _ := json.Marshal(req)
		r, _ := http.NewRequest(http.MethodDelete, srURL+"/serviceregistry/unregister", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		http.DefaultClient.Do(r)
	}
	log.Printf("[robot-fleet-tls] unregistered services from ServiceRegistry")
}

func main() {
	amqpURL      := envOr("AMQP_URL", "amqps://guest:guest@rabbitmq:5671/")
	port         := envOr("FLEET_PORT", "9003")
	countStr     := envOr("INITIAL_ROBOT_COUNT", "3")
	ptype        := envOr("PAYLOAD_TYPE", "imu")
	hzStr        := envOr("PAYLOAD_HZ", "10")
	preset       := envOr("NETWORK_PRESET", "5g_good")
	srURL        := envOr("SR_URL", "")
	systemName   := envOr("SYSTEM_NAME", "robot-fleet-tls")
	authURL      := envOr("AUTH_URL", "")
	sysCreds     := envOr("SYSTEM_CREDENTIALS", "fleet-secret")
	kafkaBrokers := strings.Split(envOr("KAFKA_BROKERS", "kafka:9092"), ",")
	kafkaTopic   := envOr("KAFKA_TOPIC", "arrowhead.telemetry")
	caURL        := envOr("CA_URL", "http://ca:8086")

	count, _ := strconv.Atoi(countStr)
	if count < 1 {
		count = 1
	}
	hz, _ := strconv.ParseFloat(hzStr, 64)
	if hz <= 0 {
		hz = 10
	}

	// Start health server immediately.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/config", handleConfig)
	mux.HandleFunc("/stats", handleStats)
	go func() {
		log.Printf("[robot-fleet-tls] HTTP server on :%s", port)
		log.Fatal(http.ListenAndServe(":"+port, mux))
	}()

	// Fetch CA cert with retry.
	log.Printf("[robot-fleet-tls] fetching CA cert from %s", caURL)
	var caPool *x509.CertPool
	for attempt := 1; attempt <= 10; attempt++ {
		pool, err := fetchCACertPool(caURL)
		if err != nil {
			if attempt < 10 {
				log.Printf("[robot-fleet-tls] CA fetch attempt %d/10: %v — retrying in 3s", attempt, err)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[robot-fleet-tls] CA fetch failed after 10 attempts: %v", err)
		}
		caPool = pool
		break
	}

	// Authenticate.
	if authURL != "" {
		if _, err := login(authURL, systemName, sysCreds); err != nil {
			log.Fatalf("[robot-fleet-tls] authentication failed: %v", err)
		}
		log.Printf("[robot-fleet-tls] authenticated as %q", systemName)
	}

	// Register in ServiceRegistry.
	if srURL != "" {
		for attempt := 1; attempt <= 10; attempt++ {
			if err := registerServices(srURL, systemName, "kafka-authz", 9091); err == nil {
				break
			} else if attempt == 10 {
				log.Fatalf("[robot-fleet-tls] SR registration failed after 10 attempts: %v", err)
			} else {
				log.Printf("[robot-fleet-tls] SR registration attempt %d failed, retrying: %v", attempt, err)
				time.Sleep(2 * time.Second)
			}
		}
	}

	// TLS config for AMQPS: server-side TLS using CA pool.
	amqpTLSCfg := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}

	// Connect to AMQPS with TLS.
	var b *broker.Broker
	var err error
	for attempt := 1; attempt <= 15; attempt++ {
		b, err = broker.New(broker.Config{
			URL:       amqpURL,
			Exchange:  "arrowhead",
			TLSConfig: amqpTLSCfg,
		})
		if err == nil {
			break
		}
		log.Printf("[robot-fleet-tls] AMQPS connect failed (attempt %d/15): %v — retrying in 2s", attempt, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("[robot-fleet-tls] could not connect to AMQPS after 15 attempts: %v", err)
	}
	defer b.Close()

	// TLS config for Kafka: server-side TLS using CA pool.
	kafkaTLSCfg := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}

	// Create Kafka writer with TLS transport.
	kw := &kafka.Writer{
		Addr:         kafka.TCP(kafkaBrokers...),
		Topic:        kafkaTopic,
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    1,
		BatchTimeout: 10 * time.Millisecond,
		Transport:    &kafka.Transport{TLS: kafkaTLSCfg},
	}
	defer kw.Close()
	log.Printf("[robot-fleet-tls] Kafka writer created (TLS) topic=%s brokers=%v", kafkaTopic, kafkaBrokers)

	// Build fleet config.
	robots := make([]RobotConfig, count)
	for i := range robots {
		robots[i] = RobotConfig{
			ID:      fmt.Sprintf("robot-%d", i+1),
			Network: NetworkProfile{Preset: preset},
		}
	}
	cfg := FleetConfig{PayloadType: ptype, PayloadHz: hz, Robots: robots}

	// Dual-publish: AMQPS + Kafka (both TLS).
	publishFn := func(routingKey string, payload []byte) {
		if err := b.Publish(routingKey, payload); err != nil {
			log.Printf("[robot-fleet-tls] AMQPS publish error: %v", err)
		}
		if kw != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := kw.WriteMessages(ctx, kafka.Message{
				Key:   []byte(routingKey),
				Value: payload,
			}); err != nil {
				log.Printf("[robot-fleet-tls] Kafka publish error: %v", err)
			}
			cancel()
		}
	}

	fleet = NewFleetWithPublisher(b, cfg, publishFn)
	log.Printf("[robot-fleet-tls] started with %d robots, payload=%s, hz=%.1f, preset=%s (AMQPS+Kafka/TLS)",
		count, ptype, hz, preset)

	if srURL != "" {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			unregisterServices(srURL, systemName)
			os.Exit(0)
		}()
	}

	select {}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	fleetMu.RLock()
	n := 0
	if fleet != nil {
		n = fleet.RobotCount()
	}
	fleetMu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "system": "robot-fleet-tls", "robotCount": n})
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		fleetMu.RLock()
		cfg := fleet.Config()
		fleetMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		var cfg FleetConfig
		if err := json.Unmarshal(body, &cfg); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(cfg.Robots) == 0 {
			http.Error(w, "robots array must not be empty", http.StatusBadRequest)
			return
		}
		fleetMu.Lock()
		fleet.UpdateConfig(cfg)
		fleetMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "GET or POST required", http.StatusMethodNotAllowed)
	}
}

func handleStats(w http.ResponseWriter, _ *http.Request) {
	fleetMu.RLock()
	stats := fleet.Stats()
	fleetMu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
