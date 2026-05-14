// service-partner — UC3 Service Partner (SP1/SP2) for experiment-9.
//
// Polls portal-cloud-ml REST API via pki-rest-authz mTLS proxy.
// Uses the Arrowhead 5.2 PKI lifecycle (on → de → sy) to acquire a system
// certificate for mTLS authentication.
//
// Environment variables:
//
//	PARTNER_NAME        system name / cert CN          (default: service-partner-1)
//	CA_URL              profile-ca plain HTTP URL       (default: http://profile-ca:8187)
//	CA_TLS_URL          profile-ca mTLS URL            (default: https://profile-ca:8188)
//	PKI_REST_AUTHZ_URL  mTLS proxy HTTPS URL           (default: https://pki-rest-authz:9208)
//	SERVICE             X-Service-Name header value    (default: telemetry-rest)
//	POLL_INTERVAL       polling interval               (default: 5s)
//	HEALTH_PORT         plain HTTP health/stats port   (default: 9211)
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// spConfig holds configuration for service-partner.
type spConfig struct {
	partnerName string
	caURL       string
	caTLSURL    string
	authzURL    string
	service     string
	healthPort  string
	interval    time.Duration
}

// spConfigFromEnv reads configuration from environment variables.
func spConfigFromEnv() (spConfig, error) {
	intervalStr := envOr("POLL_INTERVAL", "5s")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		return spConfig{}, fmt.Errorf("invalid POLL_INTERVAL %q: %w", intervalStr, err)
	}
	return spConfig{
		partnerName: envOr("PARTNER_NAME", "service-partner-1"),
		caURL:       envOr("CA_URL", "http://profile-ca:8187"),
		caTLSURL:    envOr("CA_TLS_URL", "https://profile-ca:8188"),
		authzURL:    envOr("PKI_REST_AUTHZ_URL", "https://pki-rest-authz:9208"),
		service:     envOr("SERVICE", "telemetry-rest"),
		healthPort:  envOr("HEALTH_PORT", "9211"),
		interval:    interval,
	}, nil
}

// acquireWithRetry retries AcquireSystemCert up to maxRetries times with 3 s delay.
func acquireWithRetry(caURL, caTLSURL, systemName string, maxRetries int) (*PollClient, error) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		cert, caPool, lcErr := AcquireSystemCert(caURL, caTLSURL, systemName)
		if lcErr == nil {
			return NewPollClient(cert, caPool, systemName, systemName), nil
		}
		if attempt < maxRetries {
			log.Printf("[%s] lifecycle attempt %d/%d: %v — retrying in 3s", systemName, attempt, maxRetries, lcErr)
			time.Sleep(3 * time.Second)
		} else {
			return nil, fmt.Errorf("lifecycle failed after %d attempts: %w", maxRetries, lcErr)
		}
	}
	return nil, fmt.Errorf("lifecycle: no attempts made")
}

// startHealthServer starts the plain HTTP health/stats server and returns.
func startHealthServer(partnerName, port string, stats *Stats) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", HandleHealth(partnerName))
	mux.HandleFunc("/stats", HandleStats(stats))
	go func() {
		log.Printf("[%s] health server on :%s", partnerName, port)
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Fatalf("[%s] health server: %v", partnerName, err)
		}
	}()
}

func main() {
	cfg, err := spConfigFromEnv()
	if err != nil {
		log.Fatalf("[service-partner] config error: %v", err)
	}

	// Acquire PKI system cert.
	log.Printf("[%s] starting PKI lifecycle: on → de → sy", cfg.partnerName)
	pollClient, err := acquireWithRetry(cfg.caURL, cfg.caTLSURL, cfg.partnerName, 10)
	if err != nil {
		log.Fatalf("[%s] %v", cfg.partnerName, err)
	}
	// Override the target URL and service name from config.
	pollClient.targetURL = cfg.authzURL
	pollClient.serviceName = cfg.service
	log.Printf("[%s] system cert acquired, starting poll loop", cfg.partnerName)

	startHealthServer(cfg.partnerName, cfg.healthPort, pollClient.Stats())

	done := make(chan struct{})
	pollClient.RunPoll(cfg.interval, done)
}
