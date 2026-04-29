// example-client demonstrates how experiments interact with the Arrowhead Core
// Service Registry exclusively via its HTTP API.
//
// Usage:
//
//	go run main.go                          # uses http://localhost:8080
//	CORE_URL=http://localhost:9000 go run main.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// ---- Types mirroring the core API contract --------------------------------
// These are local copies. This client MUST NOT import core/internal/model.

type System struct {
	SystemName         string `json:"systemName"`
	Address            string `json:"address"`
	Port               int    `json:"port"`
	AuthenticationInfo string `json:"authenticationInfo,omitempty"`
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

type RegisterRequest struct {
	ServiceDefinition string            `json:"serviceDefinition"`
	ProviderSystem    System            `json:"providerSystem"`
	ServiceUri        string            `json:"serviceUri"`
	Interfaces        []string          `json:"interfaces"`
	Version           int               `json:"version,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Secure            string            `json:"secure,omitempty"`
}

type QueryRequest struct {
	ServiceDefinition  string            `json:"serviceDefinition,omitempty"`
	Interfaces         []string          `json:"interfaces,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	VersionRequirement int               `json:"versionRequirement,omitempty"`
}

type QueryResponse struct {
	ServiceQueryData []ServiceInstance `json:"serviceQueryData"`
	UnfilteredHits   int               `json:"unfilteredHits"`
}

// ---- HTTP helpers ---------------------------------------------------------

var client = &http.Client{Timeout: 5 * time.Second}

func postJSON(url string, body any, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var errBody map[string]string
		json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errBody["error"])
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

// ---- Main flow ------------------------------------------------------------

func main() {
	coreURL := os.Getenv("CORE_URL")
	if coreURL == "" {
		coreURL = "http://localhost:8080"
	}
	log.Printf("Using core at %s\n", coreURL)

	// Step 1: Register two services.
	services := []RegisterRequest{
		{
			ServiceDefinition: "temperature-service",
			ProviderSystem:    System{SystemName: "sensor-eu-1", Address: "10.0.1.10", Port: 9001},
			ServiceUri:        "/temperature",
			Interfaces:        []string{"HTTP-SECURE-JSON"},
			Version:           1,
			Metadata:          map[string]string{"region": "eu", "unit": "celsius"},
			Secure:            "NOT_SECURE",
		},
		{
			ServiceDefinition: "temperature-service",
			ProviderSystem:    System{SystemName: "sensor-us-1", Address: "10.0.2.10", Port: 9001},
			ServiceUri:        "/temperature",
			Interfaces:        []string{"HTTP-SECURE-JSON", "HTTP-INSECURE-JSON"},
			Version:           1,
			Metadata:          map[string]string{"region": "us", "unit": "fahrenheit"},
		},
	}

	fmt.Println("=== Registering services ===")
	for _, req := range services {
		var registered ServiceInstance
		if err := postJSON(coreURL+"/serviceregistry/register", req, &registered); err != nil {
			log.Fatalf("register %q: %v", req.ProviderSystem.SystemName, err)
		}
		fmt.Printf("  Registered: id=%d  %s @ %s:%d\n",
			registered.ID,
			registered.ServiceDefinition,
			registered.ProviderSystem.SystemName,
			registered.ProviderSystem.Port,
		)
	}

	// Step 2: Query — all temperature services.
	fmt.Println("\n=== Query: all temperature-service ===")
	runQuery(coreURL, QueryRequest{ServiceDefinition: "temperature-service"})

	// Step 3: Query — EU region only.
	fmt.Println("\n=== Query: region=eu ===")
	runQuery(coreURL, QueryRequest{
		ServiceDefinition: "temperature-service",
		Metadata:          map[string]string{"region": "eu"},
	})

	// Step 4: Query — services providing HTTP-INSECURE-JSON.
	fmt.Println("\n=== Query: interface HTTP-INSECURE-JSON ===")
	runQuery(coreURL, QueryRequest{
		Interfaces: []string{"HTTP-INSECURE-JSON"},
	})

	// Step 5: Query — non-existent service.
	fmt.Println("\n=== Query: unknown-service (expect empty) ===")
	runQuery(coreURL, QueryRequest{ServiceDefinition: "unknown-service"})
}

func runQuery(coreURL string, req QueryRequest) {
	var resp QueryResponse
	if err := postJSON(coreURL+"/serviceregistry/query", req, &resp); err != nil {
		log.Fatalf("query: %v", err)
	}
	fmt.Printf("  unfilteredHits=%d  matches=%d\n", resp.UnfilteredHits, len(resp.ServiceQueryData))
	for _, s := range resp.ServiceQueryData {
		fmt.Printf("    [id=%d] %s @ %s  interfaces=%v  metadata=%v\n",
			s.ID, s.ServiceDefinition, s.ProviderSystem.SystemName, s.Interfaces, s.Metadata)
	}
}
