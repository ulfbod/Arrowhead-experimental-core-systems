// robot-fleet for experiment-4.
//
// Adds ServiceRegistry registration (Phase 1) and optional Authentication
// login (Phase 2) on top of the experiment-2 robot-fleet.
//
// New environment variables:
//   SR_URL              ServiceRegistry base URL (required)
//   SYSTEM_NAME         system name for registration (default: robot-fleet)
//   SYSTEM_CREDENTIALS  credentials for Authentication login
//   AUTH_URL            Authentication system base URL (optional; skip if empty)
//
// Existing variables (unchanged from experiment-2):
//   AMQP_URL, FLEET_PORT, INITIAL_ROBOT_COUNT, PAYLOAD_TYPE, PAYLOAD_HZ, NETWORK_PRESET
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	broker "arrowhead/message-broker"
)

var (
	fleetMu sync.RWMutex
	fleet   *Fleet
)

// srSystem mirrors the ServiceRegistry System type.
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

// login calls the Authentication system and returns a Bearer token.
// Returns ("", nil) when authURL is empty (Phase 2 not configured).
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

// registerService registers the AMQP telemetry service in ServiceRegistry.
func registerService(srURL, systemName string) error {
	req := srRegisterRequest{
		ServiceDefinition: "telemetry",
		ProviderSystem: srSystem{
			SystemName: systemName,
			Address:    "rabbitmq",
			Port:       5672,
		},
		ServiceUri: "arrowhead",
		Interfaces: []string{"AMQP-INSECURE-JSON"},
		Version:    1,
		Metadata: map[string]string{
			"exchangeType":      "topic",
			"routingKeyPattern": "telemetry.*",
		},
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(srURL+"/serviceregistry/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("SR register: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		log.Printf("[robot-fleet] registered telemetry service in ServiceRegistry")
		return nil
	}
	b, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("SR register returned %d: %s", resp.StatusCode, string(b))
}

// unregisterService removes the service from ServiceRegistry on graceful shutdown.
func unregisterService(srURL, systemName string) {
	req := srUnregisterRequest{
		ServiceDefinition: "telemetry",
		ProviderSystem: srSystem{
			SystemName: systemName,
			Address:    "rabbitmq",
			Port:       5672,
		},
		Version: 1,
	}
	body, _ := json.Marshal(req)
	r, err := http.NewRequest(http.MethodDelete, srURL+"/serviceregistry/unregister", bytes.NewReader(body))
	if err != nil {
		log.Printf("[robot-fleet] unregister request build failed: %v", err)
		return
	}
	r.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		log.Printf("[robot-fleet] unregister failed: %v", err)
		return
	}
	resp.Body.Close()
	log.Printf("[robot-fleet] unregistered from ServiceRegistry (status %d)", resp.StatusCode)
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

	count, _ := strconv.Atoi(countStr)
	if count < 1 {
		count = 1
	}
	hz, _ := strconv.ParseFloat(hzStr, 64)
	if hz <= 0 {
		hz = 10
	}

	// Phase 2: Authenticate to get identity token.
	if authURL != "" {
		token, err := login(authURL, systemName, sysCreds)
		if err != nil {
			log.Fatalf("[robot-fleet] authentication failed: %v", err)
		}
		if token != "" {
			log.Printf("[robot-fleet] authenticated as %q", systemName)
		}
	}

	// Phase 1: Register service in ServiceRegistry (with retries).
	if srURL != "" {
		for attempt := 1; attempt <= 10; attempt++ {
			if err := registerService(srURL, systemName); err == nil {
				break
			} else if attempt == 10 {
				log.Fatalf("[robot-fleet] SR registration failed after 10 attempts: %v", err)
			} else {
				log.Printf("[robot-fleet] SR registration attempt %d failed, retrying: %v", attempt, err)
				time.Sleep(2 * time.Second)
			}
		}
	}

	// Build initial robot config.
	robots := make([]RobotConfig, count)
	for i := range robots {
		robots[i] = RobotConfig{
			ID:      fmt.Sprintf("robot-%d", i+1),
			Network: NetworkProfile{Preset: preset},
		}
	}
	cfg := FleetConfig{
		PayloadType: ptype,
		PayloadHz:   hz,
		Robots:      robots,
	}

	// Connect to AMQP with retries.
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

	fleet = NewFleet(b, cfg)
	log.Printf("[robot-fleet] started with %d robots, payload=%s, hz=%.1f, preset=%s",
		count, ptype, hz, preset)

	// Graceful shutdown: unregister from SR on SIGINT/SIGTERM.
	if srURL != "" {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			unregisterService(srURL, systemName)
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
	json.NewEncoder(w).Encode(map[string]any{
		"status":     "ok",
		"system":     "robot-fleet",
		"robotCount": n,
	})
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
		log.Printf("[robot-fleet] config updated: %d robots, payload=%s, hz=%.1f",
			len(cfg.Robots), cfg.PayloadType, cfg.PayloadHz)
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
