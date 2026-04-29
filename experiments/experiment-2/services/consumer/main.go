// consumer demonstrates late-binding via Arrowhead DynamicOrchestration.
//
// On startup it asks the orchestrator for the "telemetry" service, then polls
// the discovered endpoint every 5 seconds and logs the result.
//
// Environment variables:
//
//	ORCH_URL        (default http://localhost:8083)
//	CONSUMER_NAME   (default demo-consumer)
//	POLL_INTERVAL   (default 5s — parsed by time.ParseDuration)
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
)

func main() {
	orchURL      := envOr("ORCH_URL", "http://localhost:8083")
	consumerName := envOr("CONSUMER_NAME", "demo-consumer")
	pollStr      := envOr("POLL_INTERVAL", "5s")

	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		log.Fatalf("[consumer] invalid POLL_INTERVAL %q: %v", pollStr, err)
	}

	log.Printf("[consumer] starting as %s — orchestrating via %s", consumerName, orchURL)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var telemetryURL string

	for ; ; <-ticker.C {
		// Re-orchestrate each cycle to demonstrate late binding.
		url, err := orchestrate(orchURL, consumerName)
		if err != nil {
			log.Printf("[consumer] orchestration failed: %v", err)
			continue
		}
		if url != telemetryURL {
			log.Printf("[consumer] orchestrated endpoint: %s", url)
			telemetryURL = url
		}

		// Fetch latest telemetry.
		data, err := fetchLatest(telemetryURL)
		if err != nil {
			log.Printf("[consumer] fetch failed: %v", err)
			continue
		}
		log.Printf("[consumer] telemetry: %s", data)
	}
}

// orchestrate calls DynamicOrchestration and returns the URL to the telemetry
// endpoint.
func orchestrate(orchURL, consumerName string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"requesterSystem": map[string]any{
			"systemName": consumerName,
			"address":    "localhost",
			"port":       9002,
		},
		"requestedService": map[string]any{
			"serviceDefinition": "telemetry",
			"interfaces":        []string{"HTTP-INSECURE-JSON"},
		},
	})

	resp, err := http.Post(orchURL+"/orchestration/dynamic", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("POST orchestration: %w", err)
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("orchestrator returned %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Response []struct {
			Provider struct {
				Address string `json:"address"`
				Port    int    `json:"port"`
			} `json:"provider"`
			ServiceURI string `json:"serviceUri"`
		} `json:"response"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(result.Response) == 0 {
		return "", fmt.Errorf("no providers found for telemetry service")
	}

	r := result.Response[0]
	return fmt.Sprintf("http://%s:%d%s", r.Provider.Address, r.Provider.Port, r.ServiceURI), nil
}

func fetchLatest(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return "(no data yet)", nil
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
