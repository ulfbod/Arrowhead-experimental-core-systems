// robot-fleet manages N simulated robots with per-robot 5G network emulation.
//
// Each robot publishes JSON telemetry to the AMQP exchange "arrowhead" on
// routing key "telemetry.{robotId}".  Network effects (latency, jitter,
// packet-loss, bandwidth cap) are emulated at the application layer.
//
// Environment variables:
//
//	AMQP_URL            (default amqp://guest:guest@localhost:5672/)
//	FLEET_PORT          (default 9003)
//	INITIAL_ROBOT_COUNT (default 3)
//	PAYLOAD_TYPE        (default imu)   — "basic" | "imu"
//	PAYLOAD_HZ          (default 10)
//	NETWORK_PRESET      (default 5g_good)
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	broker "arrowhead/message-broker"
)

var (
	fleetMu sync.RWMutex
	fleet   *Fleet
)

func main() {
	amqpURL  := envOr("AMQP_URL", "amqp://guest:guest@localhost:5672/")
	port     := envOr("FLEET_PORT", "9003")
	countStr := envOr("INITIAL_ROBOT_COUNT", "3")
	ptype    := envOr("PAYLOAD_TYPE", "imu")
	hzStr    := envOr("PAYLOAD_HZ", "10")
	preset   := envOr("NETWORK_PRESET", "5g_good")

	count, _ := strconv.Atoi(countStr)
	if count < 1 {
		count = 1
	}
	hz, _ := strconv.ParseFloat(hzStr, 64)
	if hz <= 0 {
		hz = 10
	}

	// Build initial config.
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
		"status":      "ok",
		"system":      "robot-fleet",
		"robotCount":  n,
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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
