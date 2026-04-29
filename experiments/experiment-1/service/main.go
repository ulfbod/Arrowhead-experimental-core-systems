// example-service demonstrates an Arrowhead participant:
//   - registers itself with the Service Registry on startup
//   - exposes GET /hello (its own capability)
//   - exposes POST /query (proxy to core with CORS headers so the browser
//     frontend can call it without requiring CORS on the core)
//
// Usage:
//
//	go run main.go                          # core=localhost:8080, self=localhost:9090
//	CORE_URL=http://localhost:9000 go run main.go
//	PORT=9091 go run main.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

// ── Local type copies ────────────────────────────────────────────────────────
// This service must NOT import core/internal packages.
// These mirror the types defined in core/internal/model/types.go.

type System struct {
	SystemName         string `json:"systemName"`
	Address            string `json:"address"`
	Port               int    `json:"port"`
	AuthenticationInfo string `json:"authenticationInfo,omitempty"`
}

type RegisterRequest struct {
	ServiceDefinition string            `json:"serviceDefinition"`
	ProviderSystem    System            `json:"providerSystem"`
	ServiceUri        string            `json:"serviceUri"`
	Interfaces        []string          `json:"interfaces"`
	Version           int               `json:"version"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Secure            string            `json:"secure,omitempty"`
}

type ServiceInstance struct {
	ID                int64             `json:"id"`
	ServiceDefinition string            `json:"serviceDefinition"`
	ProviderSystem    System            `json:"providerSystem"`
	ServiceUri        string            `json:"serviceUri"`
	Interfaces        []string          `json:"interfaces"`
	Version           int               `json:"version"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Secure            string            `json:"secure,omitempty"`
}

// ── Startup ──────────────────────────────────────────────────────────────────

func main() {
	coreURL := os.Getenv("CORE_URL")
	if coreURL == "" {
		coreURL = "http://localhost:8080"
	}
	portStr := os.Getenv("PORT")
	if portStr == "" {
		portStr = "9090"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("invalid PORT %q: %v", portStr, err)
	}

	if err := registerSelf(coreURL, port); err != nil {
		log.Fatalf("[example-service] Failed to register with core: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/hello", handleHello)
	mux.HandleFunc("/query", makeQueryProxy(coreURL))

	log.Printf("[example-service] Listening on :%d  (core=%s)", port, coreURL)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), mux))
}

// registerSelf sends a POST /serviceregistry/register to the core.
// The payload matches the RegisterRequest format defined in SPEC.md exactly.
func registerSelf(coreURL string, port int) error {
	req := RegisterRequest{
		ServiceDefinition: "example-service",
		ProviderSystem: System{
			SystemName: "example-system",
			Address:    "localhost",
			Port:       port,
		},
		ServiceUri: "/hello",
		Interfaces: []string{"HTTP-SECURE-JSON"},
		Version:    1,
		Metadata:   map[string]string{"purpose": "demo", "language": "go"},
		Secure:     "NOT_SECURE",
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(coreURL+"/serviceregistry/register", "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("could not reach core at %s: %w", coreURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("core returned HTTP %d: %s", resp.StatusCode, body)
	}

	var registered ServiceInstance
	if err := json.NewDecoder(resp.Body).Decode(&registered); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	log.Printf("[example-service] Registered — id=%d  serviceDefinition=%s",
		registered.ID, registered.ServiceDefinition)
	return nil
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// GET /hello — the service's own capability.
func handleHello(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Hello from example-service!",
		"service": "example-service",
	})
}

// POST /query — proxies to core's POST /serviceregistry/query.
//
// The browser frontend cannot call the core directly because the core does not
// serve CORS headers (see CORS note below). This endpoint forwards the request
// body unchanged and adds the CORS headers the browser requires.
//
// CORS note: to let a browser call the core directly, the core would need to
// respond with:
//
//	Access-Control-Allow-Origin: *
//	Access-Control-Allow-Headers: Content-Type
//	Access-Control-Allow-Methods: POST, OPTIONS
//
// Since the core must not be modified for experiments, this proxy is the
// correct approach for browser-based clients.
func makeQueryProxy(coreURL string) http.HandlerFunc {
	client := &http.Client{Timeout: 5 * time.Second}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}

		resp, err := client.Post(coreURL+"/serviceregistry/query", "application/json", r.Body)
		if err != nil {
			http.Error(w, "core unreachable: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}
