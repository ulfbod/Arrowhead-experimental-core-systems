// edge-adapter bridges the AMQP data plane with the Arrowhead core systems.
//
// On startup it:
//  1. Obtains a certificate from the CA (port 8086) for the system "edge-adapter".
//  2. Registers the "telemetry" service with the ServiceRegistry (port 8080).
//  3. Subscribes to "telemetry.#" on the AMQP exchange and stores the latest payload.
//  4. Serves GET /telemetry/latest — used by orchestrated consumers.
//
// Environment variables:
//
//	AMQP_URL  (default amqp://guest:guest@localhost:5672/)
//	CA_URL    (default http://localhost:8086)
//	SR_URL    (default http://localhost:8080)
//	PORT      (default 9001)
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	broker "arrowhead/message-broker"
)

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

var (
	mu          sync.RWMutex
	latestMsg   json.RawMessage
	latestTime  time.Time
)

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

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
	addr := fmt.Sprintf("localhost:%s", port)
	if err := registerService(srURL, addr, port); err != nil {
		log.Printf("[edge-adapter] WARNING: ServiceRegistry registration failed: %v", err)
	} else {
		log.Printf("[edge-adapter] registered telemetry service at %s", addr)
	}

	// 3. Subscribe to AMQP telemetry.
	b, err := broker.New(broker.Config{URL: amqpURL, Exchange: "arrowhead"})
	if err != nil {
		log.Fatalf("[edge-adapter] AMQP connect: %v", err)
	}
	defer b.Close()

	if err := b.Subscribe("edge-adapter-queue", "telemetry.#", func(payload []byte) {
		mu.Lock()
		latestMsg = json.RawMessage(payload)
		latestTime = time.Now()
		mu.Unlock()
		log.Printf("[edge-adapter] received telemetry (%d bytes)", len(payload))
	}); err != nil {
		log.Fatalf("[edge-adapter] AMQP subscribe: %v", err)
	}

	log.Printf("[edge-adapter] listening on :%s", port)

	// 4. Serve HTTP.
	mux := http.NewServeMux()
	mux.HandleFunc("/telemetry/latest", handleLatest)
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
	mu.RLock()
	msg := latestMsg
	ts  := latestTime
	mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if msg == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	resp := map[string]any{
		"receivedAt": ts.UTC().Format(time.RFC3339),
		"payload":    msg,
	}
	json.NewEncoder(w).Encode(resp)
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
