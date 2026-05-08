// consumer-direct-tls for experiment-7.
//
// Extends experiment-5's consumer-direct by using AMQPS (TLS) for the RabbitMQ
// connection.  The consumer fetches the CA certificate from the Arrowhead CA
// and uses it to verify the RabbitMQ TLS certificate.
//
// The authorization flow is unchanged from experiment-5: login → orchestration
// → ConsumerAuthorization token → AMQPS subscribe.
//
// Environment variables:
//
//	CONSUMER_NAME        system name (default: consumer-direct-tls)
//	SYSTEM_CREDENTIALS   credentials for Authentication login (default: consumer-secret)
//	CONSUMER_PASSWORD    AMQP password provisioned by topic-auth-xacml (default: consumer-secret)
//	AUTH_URL             Authentication system base URL (required)
//	ORCHESTRATION_URL    DynamicOrchestration base URL (required)
//	CONSUMERAUTH_URL     ConsumerAuthorization base URL (optional)
//	CA_URL               Arrowhead CA base URL (default: http://ca:8086)
//	HEALTH_PORT          HTTP port for /health and /stats (default: 9002)
package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
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

// requireHTTPS returns an error if url is non-empty and does not use https://.
// All inter-system calls to core services in experiment-7 must use mTLS (https://).
func requireHTTPS(envName, url string) error {
	if url == "" {
		return nil // optional env vars are allowed to be empty
	}
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("%s must use https:// scheme (got %q); experiment-7 requires mTLS for all core service calls", envName, url)
	}
	return nil
}

// ── CA helpers ────────────────────────────────────────────────────────────────

type caInfoResponse struct {
	CommonName  string `json:"commonName"`
	Certificate string `json:"certificate"`
}

type caIssueCertRequest struct {
	SystemName string `json:"systemName"`
}

type caIssueCertResponse struct {
	SystemName  string `json:"systemName"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
}

// fetchCACertPool retrieves the CA certificate and returns an x509.CertPool.
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

// buildMTLSClient issues a certificate from the CA for systemName and returns
// an *http.Client that presents the cert in TLS handshakes.  The client verifies
// server certificates against caPool.
func buildMTLSClient(caURL, systemName string, caPool *x509.CertPool) (*http.Client, error) {
	body, _ := json.Marshal(caIssueCertRequest{SystemName: systemName})
	resp, err := http.Post(caURL+"/ca/certificate/issue", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("POST /ca/certificate/issue: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("POST /ca/certificate/issue returned %d", resp.StatusCode)
	}
	var issued caIssueCertResponse
	if err := json.NewDecoder(resp.Body).Decode(&issued); err != nil {
		return nil, fmt.Errorf("decode issue cert response: %w", err)
	}
	cert, err := tls.X509KeyPair([]byte(issued.Certificate), []byte(issued.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("parse issued cert/key: %w", err)
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}, nil
}

// ── Stats tracker ─────────────────────────────────────────────────────────────

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

// ── AHC wire types ────────────────────────────────────────────────────────────

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

// ── AHC calls ─────────────────────────────────────────────────────────────────

func authLogin(client *http.Client, authURL, systemName, credentials string) (string, error) {
	body, _ := json.Marshal(authLoginRequest{SystemName: systemName, Credentials: credentials})
	resp, err := client.Post(authURL+"/authentication/identity/login", "application/json", bytes.NewReader(body))
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

func orchestrate(client *http.Client, orchURL, systemName, token string) (orchResult, error) {
	req := orchRequest{
		RequesterSystem: orchSystem{SystemName: systemName, Address: systemName, Port: 0},
		RequestedService: orchServiceFilter{
			ServiceDefinition: "telemetry",
			Interfaces:        []string{"AMQP-SECURE-JSON"},
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
	resp, err := client.Do(r)
	if err != nil {
		return orchResult{}, fmt.Errorf("orchestration request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return orchResult{}, fmt.Errorf("orchestration denied (401)")
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

// ── Main flow ─────────────────────────────────────────────────────────────────

func run(name, sysCreds, consumerPass, authURL, orchURL, consumerAuthURL string, caPool *x509.CertPool, coreClient *http.Client, st *statsTracker) error {
	token, err := authLogin(coreClient, authURL, name, sysCreds)
	if err != nil {
		return fmt.Errorf("authentication: %w", err)
	}
	log.Printf("[%s] authenticated", name)

	result, err := orchestrate(coreClient, orchURL, name, token)
	if err != nil {
		return fmt.Errorf("orchestration: %w", err)
	}
	// Use amqps:// for TLS AMQP.
	amqpURL := fmt.Sprintf("amqps://%s:%s@%s:%d/",
		name, consumerPass, result.Provider.Address, result.Provider.Port)
	exchange := result.Service.ServiceUri
	bindingKey := routingKeyPattern(result.Service.Metadata)
	log.Printf("[%s] orchestration result: amqps://%s:%d  exchange=%s  key=%s",
		name, result.Provider.Address, result.Provider.Port, exchange, bindingKey)

	generateToken(consumerAuthURL, name, result.Provider.SystemName, result.Service.ServiceDefinition)

	// Build TLS config for AMQPS: verify server cert against CA pool.
	amqpTLSCfg := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}

	b, err := broker.New(broker.Config{
		URL:       amqpURL,
		Exchange:  exchange,
		TLSConfig: amqpTLSCfg,
	})
	if err != nil {
		return fmt.Errorf("AMQPS connect: %w", err)
	}
	defer b.Close()

	queue := name + "-queue"
	if err := b.Subscribe(queue, bindingKey, func(payload []byte) {
		st.record()
		log.Printf("[%s] received: %s", name, payload)
	}); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	log.Printf("[%s] subscribed with binding key %q (AMQPS/TLS) — waiting for messages", name, bindingKey)
	<-b.Done()
	return fmt.Errorf("connection closed")
}

func main() {
	name         := envOr("CONSUMER_NAME", "consumer-direct-tls")
	sysCreds     := envOr("SYSTEM_CREDENTIALS", "consumer-secret")
	consumerPass := envOr("CONSUMER_PASSWORD", "consumer-secret")
	authURL      := os.Getenv("AUTH_URL")
	orchURL      := os.Getenv("ORCHESTRATION_URL")
	consumerAuthURL := envOr("CONSUMERAUTH_URL", "")
	caURL        := envOr("CA_URL", "http://ca:8086")
	healthPort   := envOr("HEALTH_PORT", "9002")

	if authURL == "" {
		log.Fatal("AUTH_URL is required")
	}
	if orchURL == "" {
		log.Fatal("ORCHESTRATION_URL is required")
	}
	if err := requireHTTPS("AUTH_URL", authURL); err != nil {
		log.Fatal(err)
	}
	if err := requireHTTPS("ORCHESTRATION_URL", orchURL); err != nil {
		log.Fatal(err)
	}
	if err := requireHTTPS("CONSUMERAUTH_URL", consumerAuthURL); err != nil {
		log.Fatal(err)
	}

	// Fetch CA cert with retry.
	log.Printf("[%s] fetching CA cert from %s", name, caURL)
	var caPool *x509.CertPool
	for attempt := 1; attempt <= 10; attempt++ {
		pool, err := fetchCACertPool(caURL)
		if err != nil {
			if attempt < 10 {
				log.Printf("[%s] CA fetch attempt %d/10: %v — retrying in 3s", name, attempt, err)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[%s] CA fetch failed after 10 attempts: %v", name, err)
		}
		caPool = pool
		break
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

	// Build mTLS client for calls to core services (auth, orchestration).
	// Issue own cert from CA so the server can verify our identity.
	log.Printf("[%s] issuing own certificate from CA", name)
	var coreClient *http.Client
	for attempt := 1; attempt <= 10; attempt++ {
		c, err := buildMTLSClient(caURL, name, caPool)
		if err != nil {
			if attempt < 10 {
				log.Printf("[%s] cert issuance attempt %d/10: %v — retrying in 3s", name, attempt, err)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[%s] cert issuance failed after 10 attempts: %v", name, err)
		}
		coreClient = c
		log.Printf("[%s] issued own certificate — mTLS client ready", name)
		break
	}

	for {
		if err := run(name, sysCreds, consumerPass, authURL, orchURL, consumerAuthURL, caPool, coreClient, st); err != nil {
			log.Printf("[%s] error: %v — retrying in 5s", name, err)
			time.Sleep(5 * time.Second)
		}
	}
}
