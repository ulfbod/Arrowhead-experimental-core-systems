// consumer-direct for experiment-5.
//
// Identical in behaviour to experiment-4's consumer-direct: connects via
// RabbitMQ AMQP using the full AHC integration flow (login → orchestration →
// ConsumerAuthorization token → subscribe).
//
// Authorization is now enforced by topic-auth-xacml (backed by AuthzForce)
// instead of topic-auth-http, but the consumer's wire protocol is unchanged.
//
// Environment variables:
//
//	CONSUMER_NAME        system name (default: consumer-direct)
//	SYSTEM_CREDENTIALS   credentials for Authentication login (default: consumer-secret)
//	CONSUMER_PASSWORD    AMQP password provisioned by topic-auth-xacml (default: consumer-secret)
//	AUTH_URL             Authentication system base URL (required)
//	ORCHESTRATION_URL    DynamicOrchestration base URL (required)
//	CONSUMERAUTH_URL     ConsumerAuthorization base URL (required for Phase 4)
//	HEALTH_PORT          HTTP port for /health and /stats (default: 9002)
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	gosync "sync"
	"sync/atomic"
	"time"

	broker "arrowhead/message-broker"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ── Stats tracker ──────────────────────────────────────────────────────────────

type statsTracker struct {
	msgCount       atomic.Int64
	mu             gosync.Mutex
	lastReceivedAt string
}

func (st *statsTracker) record() {
	st.msgCount.Add(1)
	st.mu.Lock()
	st.lastReceivedAt = time.Now().UTC().Format(time.RFC3339)
	st.mu.Unlock()
}

func (st *statsTracker) snapshot(name string) map[string]interface{} {
	st.mu.Lock()
	last := st.lastReceivedAt
	st.mu.Unlock()
	return map[string]interface{}{
		"name":           name,
		"msgCount":       st.msgCount.Load(),
		"lastReceivedAt": last,
	}
}

// ── AHC wire types ─────────────────────────────────────────────────────────────

type authLoginRequest struct {
	SystemName  string `json:"systemName"`
	Credentials string `json:"credentials"`
}

type authLoginResponse struct {
	Token      string    `json:"token"`
	SystemName string    `json:"systemName"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type orchSystem struct {
	SystemName string `json:"systemName"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
}

type orchServiceFilter struct {
	ServiceDefinition string   `json:"serviceDefinition"`
	Interfaces        []string `json:"interfaces,omitempty"`
}

type orchRequest struct {
	RequesterSystem  orchSystem        `json:"requesterSystem"`
	RequestedService orchServiceFilter `json:"requestedService"`
}

type orchServiceInfo struct {
	ServiceDefinition string            `json:"serviceDefinition"`
	ServiceUri        string            `json:"serviceUri"`
	Interfaces        []string          `json:"interfaces"`
	Version           int               `json:"version"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type orchResult struct {
	Provider orchSystem      `json:"provider"`
	Service  orchServiceInfo `json:"service"`
}

type orchResponse struct {
	Response []orchResult `json:"response"`
}

type tokenRequest struct {
	ConsumerSystemName string `json:"consumerSystemName"`
	ProviderSystemName string `json:"providerSystemName"`
	ServiceDefinition  string `json:"serviceDefinition"`
}

type tokenResponse struct {
	Token              string `json:"token"`
	ConsumerSystemName string `json:"consumerSystemName"`
	ServiceDefinition  string `json:"serviceDefinition"`
}

// ── AHC calls ──────────────────────────────────────────────────────────────────

func authLogin(authURL, systemName, credentials string) (string, error) {
	body, _ := json.Marshal(authLoginRequest{SystemName: systemName, Credentials: credentials})
	resp, err := http.Post(authURL+"/authentication/identity/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("auth login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("auth login %d: %s", resp.StatusCode, b)
	}
	var lr authLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return "", err
	}
	return lr.Token, nil
}

func orchestrate(orchURL, systemName, token string) (orchResult, error) {
	req := orchRequest{
		RequesterSystem: orchSystem{SystemName: systemName, Address: systemName, Port: 0},
		RequestedService: orchServiceFilter{
			ServiceDefinition: "telemetry",
			Interfaces:        []string{"AMQP-INSECURE-JSON"},
		},
	}
	body, _ := json.Marshal(req)
	r, err := http.NewRequest(http.MethodPost, orchURL+"/orchestration/dynamic", bytes.NewReader(body))
	if err != nil {
		return orchResult{}, err
	}
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return orchResult{}, fmt.Errorf("orchestration request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return orchResult{}, fmt.Errorf("orchestration denied (401): token may have expired")
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return orchResult{}, fmt.Errorf("orchestration %d: %s", resp.StatusCode, b)
	}
	var or orchResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return orchResult{}, err
	}
	if len(or.Response) == 0 {
		return orchResult{}, fmt.Errorf("no authorized providers — grant may be missing")
	}
	return or.Response[0], nil
}

func generateToken(caURL, consumer, provider, service string) {
	if caURL == "" {
		return
	}
	body, _ := json.Marshal(tokenRequest{
		ConsumerSystemName: consumer,
		ProviderSystemName: provider,
		ServiceDefinition:  service,
	})
	resp, err := http.Post(caURL+"/authorization/token/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[consumer] authorization token request failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated {
		var tr tokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&tr); err == nil {
			log.Printf("[consumer] authorization token obtained for %s→%s/%s", consumer, provider, service)
			return
		}
	}
	b, _ := io.ReadAll(resp.Body)
	log.Printf("[consumer] authorization token returned %d: %s", resp.StatusCode, b)
}

func routingKeyPattern(metadata map[string]string) string {
	if p, ok := metadata["routingKeyPattern"]; ok && p != "" {
		if len(p) > 2 && p[len(p)-2:] == ".*" {
			return p[:len(p)-1] + "#"
		}
		return p
	}
	return "telemetry.#"
}

// ── Main flow ──────────────────────────────────────────────────────────────────

func run(name, sysCreds, consumerPass, authURL, orchURL, caURL string, st *statsTracker) error {
	token, err := authLogin(authURL, name, sysCreds)
	if err != nil {
		return fmt.Errorf("authentication: %w", err)
	}
	log.Printf("[%s] authenticated", name)

	result, err := orchestrate(orchURL, name, token)
	if err != nil {
		return fmt.Errorf("orchestration: %w", err)
	}
	amqpURL := fmt.Sprintf("amqp://%s:%s@%s:%d/",
		name, consumerPass, result.Provider.Address, result.Provider.Port)
	exchange := result.Service.ServiceUri
	bindingKey := routingKeyPattern(result.Service.Metadata)
	log.Printf("[%s] orchestration result: amqp://%s:%d  exchange=%s  key=%s",
		name, result.Provider.Address, result.Provider.Port, exchange, bindingKey)

	generateToken(caURL, name, result.Provider.SystemName, result.Service.ServiceDefinition)

	b, err := broker.New(broker.Config{URL: amqpURL, Exchange: exchange})
	if err != nil {
		return fmt.Errorf("AMQP connect: %w", err)
	}
	defer b.Close()

	queue := name + "-queue"
	if err := b.Subscribe(queue, bindingKey, func(payload []byte) {
		st.record()
		log.Printf("[%s] received: %s", name, payload)
	}); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	log.Printf("[%s] subscribed with binding key %q — waiting for messages", name, bindingKey)
	<-b.Done()
	return fmt.Errorf("connection closed")
}

func main() {
	name         := envOr("CONSUMER_NAME", "consumer-direct")
	sysCreds     := envOr("SYSTEM_CREDENTIALS", "consumer-secret")
	consumerPass := envOr("CONSUMER_PASSWORD", "consumer-secret")
	authURL      := os.Getenv("AUTH_URL")
	orchURL      := os.Getenv("ORCHESTRATION_URL")
	caURL        := envOr("CONSUMERAUTH_URL", "")
	healthPort   := envOr("HEALTH_PORT", "9002")

	if authURL == "" {
		log.Fatal("AUTH_URL is required")
	}
	if orchURL == "" {
		log.Fatal("ORCHESTRATION_URL is required")
	}

	st := &statsTracker{}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st.snapshot(name))
	})

	go func() {
		log.Printf("[%s] health server on :%s", name, healthPort)
		if err := http.ListenAndServe(":"+healthPort, nil); err != nil {
			log.Fatalf("health server: %v", err)
		}
	}()

	for {
		if err := run(name, sysCreds, consumerPass, authURL, orchURL, caURL, st); err != nil {
			log.Printf("[%s] error: %v — retrying in 5s", name, err)
			time.Sleep(5 * time.Second)
		}
	}
}
