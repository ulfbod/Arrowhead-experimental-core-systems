// portal-cloud-ml — UC3 Portal & Cloud ML service for experiment-9.
//
// Subscribes to robot telemetry via kafka-authz SSE and exposes an
// HTTPS REST API for service partners (via pki-rest-authz mTLS proxy).
//
// Uses the Arrowhead 5.2 PKI lifecycle (on → de → sy) to acquire a system
// certificate for its HTTPS server.
//
// Environment variables:
//
//	CA_URL          profile-ca plain HTTP URL (default: http://profile-ca:8187)
//	CA_TLS_URL      profile-ca mTLS URL      (default: https://profile-ca:8188)
//	KAFKA_AUTHZ_URL kafka-authz base URL     (default: http://kafka-authz:9201)
//	CONSUMER_NAME   kafka consumer identity  (default: portal-cloud-ml)
//	SERVICE         service name to consume  (default: telemetry)
//	SYSTEM_NAME     PKI system name / CN     (default: portal-cloud-ml)
//	PORT            plain HTTP health port   (default: 9207)
//	TLS_PORT        HTTPS REST port          (default: 9294)
package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
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

// portalConfig holds configuration resolved from environment variables.
type portalConfig struct {
	caURL         string
	caTLSURL      string
	kafkaAuthzURL string
	consumerName  string
	service       string
	systemName    string
	port          string
	tlsPort       string
}

// portalConfigFromEnv reads configuration from environment variables.
func portalConfigFromEnv() portalConfig {
	return portalConfig{
		caURL:         envOr("CA_URL", "http://profile-ca:8187"),
		caTLSURL:      envOr("CA_TLS_URL", "https://profile-ca:8188"),
		kafkaAuthzURL: envOr("KAFKA_AUTHZ_URL", "http://kafka-authz:9201"),
		consumerName:  envOr("CONSUMER_NAME", "portal-cloud-ml"),
		service:       envOr("SERVICE", "telemetry"),
		systemName:    envOr("SYSTEM_NAME", "portal-cloud-ml"),
		port:          envOr("PORT", "9207"),
		tlsPort:       envOr("TLS_PORT", "9294"),
	}
}

// acquireWithRetry retries AcquireSystemCert up to maxRetries times with 3 s delay.
func acquireWithRetry(caURL, caTLSURL, systemName string, maxRetries int) (tls.Certificate, error) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		cert, _, lcErr := AcquireSystemCert(caURL, caTLSURL, systemName)
		if lcErr == nil {
			return cert, nil
		}
		if attempt < maxRetries {
			log.Printf("[portal-cloud-ml] lifecycle attempt %d/%d: %v — retrying in 3s", attempt, maxRetries, lcErr)
			time.Sleep(3 * time.Second)
		} else {
			return tls.Certificate{}, fmt.Errorf("lifecycle failed after %d attempts: %w", maxRetries, lcErr)
		}
	}
	return tls.Certificate{}, fmt.Errorf("lifecycle: no attempts made")
}

// startPlainServer starts the plain HTTP health/stats server on the given port.
// It returns the listening address (host:port) and any error.
func startPlainServer(store *Store, port string) (string, error) {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return "", fmt.Errorf("listen plain HTTP :%s: %w", port, err)
	}
	addr := ln.Addr().String()
	go func() {
		log.Printf("[portal-cloud-ml] plain HTTP listening on %s", addr)
		if serveErr := http.Serve(ln, makeHTTPHandler(store)); serveErr != nil {
			log.Printf("[portal-cloud-ml] plain HTTP server error: %v", serveErr)
		}
	}()
	return addr, nil
}

// startTLSServer starts the HTTPS server using the given TLS certificate.
// It blocks until the server terminates.
func startTLSServer(store *Store, cert tls.Certificate, port string) error {
	serverTLSCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	tlsLn, err := tls.Listen("tcp", ":"+port, serverTLSCfg)
	if err != nil {
		return fmt.Errorf("listen HTTPS :%s: %w", port, err)
	}
	log.Printf("[portal-cloud-ml] HTTPS REST listening on :%s", port)
	return http.Serve(tlsLn, makeHTTPSHandler(store))
}

func main() {
	cfg := portalConfigFromEnv()

	// Start plain HTTP health server immediately.
	store := NewStore()
	if _, err := startPlainServer(store, cfg.port); err != nil {
		log.Fatalf("[portal-cloud-ml] %v", err)
	}

	// Acquire Arrowhead 5.2 system cert via onboarding lifecycle.
	log.Printf("[portal-cloud-ml] starting PKI lifecycle: on → de → sy for CN=%s", cfg.systemName)
	ownCert, err := acquireWithRetry(cfg.caURL, cfg.caTLSURL, cfg.systemName, 10)
	if err != nil {
		log.Fatalf("[portal-cloud-ml] %v", err)
	}
	log.Printf("[portal-cloud-ml] system cert acquired (OU=sy, CN=%s)", cfg.systemName)

	// Start SSE consumer in background.
	done := make(chan struct{})
	go ConnectSSE(cfg.kafkaAuthzURL, cfg.consumerName, cfg.service, store, done)

	// Start HTTPS server for service partners.
	if err := startTLSServer(store, ownCert, cfg.tlsPort); err != nil {
		log.Fatal(err)
	}
}
