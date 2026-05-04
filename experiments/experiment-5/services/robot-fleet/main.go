// robot-fleet for experiment-5.
//
// Extends experiment-4's robot-fleet by publishing telemetry to TWO transports:
//   1. RabbitMQ (AMQP) — for direct AMQP consumers (consumer-1/2/3)
//   2. Kafka — for analytics consumers via kafka-authz
//
// This dual-publish is the basis for the unified policy projection demo:
// both transports carry the same telemetry data, and access to both is
// governed by the same AuthzForce XACML policy derived from CA grants.
//
// Environment variables (existing):
//   AMQP_URL, FLEET_PORT, INITIAL_ROBOT_COUNT, PAYLOAD_TYPE, PAYLOAD_HZ,
//   NETWORK_PRESET, SR_URL, SYSTEM_NAME, SYSTEM_CREDENTIALS, AUTH_URL
//
// New for experiment-5:
//   KAFKA_BROKERS  Comma-separated Kafka broker addresses (default: kafka:9092)
//   KAFKA_TOPIC    Kafka topic to publish to (default: arrowhead.telemetry)
package main

import (
	"bytes"
	"context"
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

// registerService registers both the AMQP and Kafka-authz service endpoints
// in ServiceRegistry so consumers can discover which transport to use.
func registerServices(srURL, systemName, kafkaAuthzAddress string, kafkaAuthzPort int) error {
	reqs := []srRegisterRequest{
		{
			ServiceDefinition: "telemetry",
			ProviderSystem:    srSystem{SystemName: systemName, Address: "rabbitmq", Port: 5672},
			ServiceUri:        "arrowhead",
			Interfaces:        []string{"AMQP-INSECURE-JSON"},
			Version:           1,
			Metadata:          map[string]string{"exchangeType": "topic", "routingKeyPattern": "telemetry.*"},
		},
		{
			ServiceDefinition: "telemetry",
			ProviderSystem:    srSystem{SystemName: systemName, Address: kafkaAuthzAddress, Port: kafkaAuthzPort},
			ServiceUri:        "/stream",
			Interfaces:        []string{"KAFKA-INSECURE-JSON"},
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
		log.Printf("[robot-fleet] registered %v service in ServiceRegistry", req.Interfaces)
	}
	return nil
}

func unregisterServices(srURL, systemName string) {
	for _, iface := range []string{"AMQP-INSECURE-JSON", "KAFKA-INSECURE-JSON"} {
		addr := "rabbitmq"
		port := 5672
		if iface == "KAFKA-INSECURE-JSON" {
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
	log.Printf("[robot-fleet] unregistered services from ServiceRegistry")
}

func main() {
	amqpURL    := envOr("AMQP_URL", "amqp://guest:guest@localhost:5672/")
	port       := envOr("FLEET_PORT", "9003")
	countStr   := envOr("INITIAL_ROBOT_COUNT", "3")
	ptype      := envOr("PAYLOAD_TYPE", "imu")
	hzStr      := envOr("PAYLOAD_HZ", "10")
	preset     := envOr("NETWORK_PRESET", "5g_good")
	srURL      := envOr("SR_URL", "")
	systemName := envOr("SYSTEM_NAME", "robot-fleet")
	authURL    := envOr("AUTH_URL", "")
	sysCreds   := envOr("SYSTEM_CREDENTIALS", "fleet-secret")
	kafkaBrokers := strings.Split(envOr("KAFKA_BROKERS", "kafka:9092"), ",")
	kafkaTopic   := envOr("KAFKA_TOPIC", "arrowhead.telemetry")

	count, _ := strconv.Atoi(countStr)
	if count < 1 {
		count = 1
	}
	hz, _ := strconv.ParseFloat(hzStr, 64)
	if hz <= 0 {
		hz = 10
	}

	// Authenticate.
	if authURL != "" {
		if _, err := login(authURL, systemName, sysCreds); err != nil {
			log.Fatalf("[robot-fleet] authentication failed: %v", err)
		}
		log.Printf("[robot-fleet] authenticated as %q", systemName)
	}

	// Register in ServiceRegistry.
	if srURL != "" {
		for attempt := 1; attempt <= 10; attempt++ {
			if err := registerServices(srURL, systemName, "kafka-authz", 9091); err == nil {
				break
			} else if attempt == 10 {
				log.Fatalf("[robot-fleet] SR registration failed after 10 attempts: %v", err)
			} else {
				log.Printf("[robot-fleet] SR registration attempt %d failed, retrying: %v", attempt, err)
				time.Sleep(2 * time.Second)
			}
		}
	}

	// Connect to AMQP.
	var b *broker.Broker
	var err error
	for attempt := 1; attempt <= 15; attempt++ {
		b, err = broker.New(broker.Config{URL: amqpURL, Exchange: "arrowhead"})
		if err == nil {
			break
		}
		log.Printf("[robot-fleet] AMQP connect failed (attempt %d/15): %v — retrying in 2s", attempt, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("[robot-fleet] could not connect to AMQP after 15 attempts: %v", err)
	}
	defer b.Close()

	// Connect to Kafka (with retries — Kafka may not be immediately ready).
	var kw *kafka.Writer
	for attempt := 1; attempt <= 20; attempt++ {
		kw = &kafka.Writer{
			Addr:         kafka.TCP(kafkaBrokers...),
			Topic:        kafkaTopic,
			Balancer:     &kafka.LeastBytes{},
			BatchSize:    1,
			BatchTimeout: 10 * time.Millisecond,
		}
		// Test the connection by writing a probe message.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		testErr := kw.WriteMessages(ctx, kafka.Message{
			Key:   []byte("probe"),
			Value: []byte(`{"probe":true}`),
		})
		cancel()
		if testErr == nil {
			log.Printf("[robot-fleet] connected to Kafka topic=%s", kafkaTopic)
			break
		}
		kw.Close()
		kw = nil
		log.Printf("[robot-fleet] Kafka connect attempt %d/20: %v — retrying in 3s", attempt, testErr)
		time.Sleep(3 * time.Second)
	}
	if kw == nil {
		log.Printf("[robot-fleet] WARNING: could not connect to Kafka after 20 attempts — continuing AMQP-only")
	} else {
		defer kw.Close()
	}

	// Build fleet config.
	robots := make([]RobotConfig, count)
	for i := range robots {
		robots[i] = RobotConfig{
			ID:      fmt.Sprintf("robot-%d", i+1),
			Network: NetworkProfile{Preset: preset},
		}
	}
	cfg := FleetConfig{PayloadType: ptype, PayloadHz: hz, Robots: robots}

	// Dual-publish callback: sends each message to both AMQP and Kafka.
	publishFn := func(routingKey string, payload []byte) {
		if err := b.Publish(routingKey, payload); err != nil {
			log.Printf("[robot-fleet] AMQP publish error: %v", err)
		}
		if kw != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := kw.WriteMessages(ctx, kafka.Message{
				Key:   []byte(routingKey),
				Value: payload,
			}); err != nil {
				log.Printf("[robot-fleet] Kafka publish error: %v", err)
			}
			cancel()
		}
	}

	fleet = NewFleetWithPublisher(b, cfg, publishFn)
	log.Printf("[robot-fleet] started with %d robots, payload=%s, hz=%.1f, preset=%s (AMQP+Kafka)",
		count, ptype, hz, preset)

	// Graceful shutdown.
	if srURL != "" {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			unregisterServices(srURL, systemName)
			os.Exit(0)
		}()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/config", handleConfig)
	mux.HandleFunc("/stats", handleStats)

	log.Printf("[robot-fleet] HTTP server on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	fleetMu.RLock()
	n := 0
	if fleet != nil {
		n = fleet.RobotCount()
	}
	fleetMu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "system": "robot-fleet", "robotCount": n})
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
