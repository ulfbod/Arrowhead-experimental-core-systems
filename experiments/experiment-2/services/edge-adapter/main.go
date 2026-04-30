// edge-adapter bridges the AMQP data plane with the Arrowhead core systems.
//
// On startup it:
//  1. Obtains a certificate from the CA (port 8086) for the system "edge-adapter".
//  2. Registers the "telemetry" service with the ServiceRegistry (port 8080).
//  3. Subscribes to "telemetry.#" on the AMQP exchange and stores the latest
//     payload per robot.
//  4. Serves HTTP endpoints:
//     GET /telemetry/latest — backward-compat: most recently received (any robot)
//     GET /telemetry/all   — latest payload per robot (map[robotId]entry)
//     GET /telemetry/stats — per-robot latency/rate statistics
//     GET /health
//
// Environment variables:
//
//	AMQP_URL   (default amqp://guest:guest@localhost:5672/)
//	CA_URL     (default http://localhost:8086)
//	SR_URL     (default http://localhost:8080)
//	PORT       (default 9001)
//	EDGE_HOST  (default edge-adapter)
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	broker "arrowhead/message-broker"
)

var state = newState()

func main() {
	amqpURL := envOr("AMQP_URL", "amqp://guest:guest@localhost:5672/")
	caURL   := envOr("CA_URL", "http://localhost:8086")
	srURL   := envOr("SR_URL", "http://localhost:8080")
	port    := envOr("PORT", "9001")

	// 1. Get certificate from CA.
	certPEM, err := issueCert(caURL, "edge-adapter")
	if err != nil {
		log.Printf("[edge-adapter] WARNING: could not obtain CA cert: %v (continuing without TLS)", err)
	} else {
		log.Printf("[edge-adapter] CA certificate obtained (%d bytes)", len(certPEM))
	}

	// 2. Register with ServiceRegistry.
	host := envOr("EDGE_HOST", "edge-adapter")
	addr := fmt.Sprintf("%s:%s", host, port)
	if err := registerService(srURL, host, port); err != nil {
		log.Printf("[edge-adapter] WARNING: ServiceRegistry registration failed: %v", err)
	} else {
		log.Printf("[edge-adapter] registered telemetry service at %s", addr)
	}

	// 3. Subscribe to AMQP telemetry (retry to survive slow RabbitMQ startup).
	var b *broker.Broker
	for attempts := 0; attempts < 15; attempts++ {
		b, err = broker.New(broker.Config{URL: amqpURL, Exchange: "arrowhead"})
		if err == nil {
			break
		}
		log.Printf("[edge-adapter] AMQP connect failed (attempt %d/15): %v — retrying in 2s", attempts+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("[edge-adapter] could not connect to AMQP after 15 attempts: %v", err)
	}
	defer b.Close()

	if err := b.Subscribe("edge-adapter-queue", "telemetry.#", func(payload []byte) {
		state.Update(json.RawMessage(payload))
		log.Printf("[edge-adapter] received telemetry (%d bytes)", len(payload))
	}); err != nil {
		log.Fatalf("[edge-adapter] AMQP subscribe: %v", err)
	}

	log.Printf("[edge-adapter] listening on :%s", port)

	// 4. Serve HTTP.
	mux := http.NewServeMux()
	mux.HandleFunc("/telemetry/latest", handleLatest)
	mux.HandleFunc("/telemetry/all", handleAll)
	mux.HandleFunc("/telemetry/stats", handleStats)
	mux.HandleFunc("/health", handleHealth)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

func handleLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	msg, ts, ok := state.Latest()
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	resp := map[string]any{
		"receivedAt": ts.UTC().Format(time.RFC3339),
		"payload":    msg,
	}
	json.NewEncoder(w).Encode(resp)
}

func handleAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state.All())
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state.Stats())
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "system": "edge-adapter"})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func issueCert(caURL, systemName string) (string, error) {
	body, _ := json.Marshal(map[string]string{"systemName": systemName})
	resp, err := http.Post(caURL+"/ca/certificate/issue", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("CA returned %d: %s", resp.StatusCode, b)
	}
	var result struct {
		Certificate string `json:"certificate"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Certificate, nil
}

func registerService(srURL, address, port string) error {
	portNum := 9001
	fmt.Sscanf(port, "%d", &portNum)

	body, _ := json.Marshal(map[string]any{
		"serviceDefinition": "telemetry",
		"providerSystem": map[string]any{
			"systemName": "edge-adapter",
			"address":    address,
			"port":       portNum,
		},
		"serviceUri": "/telemetry/latest",
		"interfaces": []string{"HTTP-INSECURE-JSON"},
		"version":    1,
	})
	resp, err := http.Post(srURL+"/serviceregistry/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SR returned %d: %s", resp.StatusCode, b)
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
