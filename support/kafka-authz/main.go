// kafka-authz — Kafka SSE proxy with AuthzForce enforcement for experiment-5.
//
// Analytics consumers connect to GET /stream/{consumerName}?service=<service>.
// kafka-authz checks AuthzForce; if Permit it subscribes to the matching Kafka
// topic and streams messages as Server-Sent Events.  If Deny it returns 403.
//
// This is the Kafka enforcement adapter in the unified policy projection model:
// both RabbitMQ (via topic-auth-xacml) and Kafka (via kafka-authz) delegate
// authorization to the same AuthzForce PDP, which evaluates the same XACML
// policy derived from ConsumerAuthorization grants.
//
// Environment variables:
//   KAFKA_BROKERS       Comma-separated broker list (default: kafka:9092)
//   AUTHZFORCE_URL      AuthzForce base URL (default: http://authzforce:8080/authzforce-ce)
//   AUTHZFORCE_DOMAIN   AuthzForce domain externalId (default: arrowhead-exp5)
//   PORT                HTTP port (default: 9091)
//   CA_URL              Optional: Arrowhead CA base URL for TLS. When set,
//                       kafka-authz fetches the CA cert and uses TLS for all
//                       Kafka connections. When unset, plain TCP is used.
package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	az "arrowhead/authzforce"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ── CA helpers ────────────────────────────────────────────────────────────────

type caInfoResponse struct {
	CommonName  string `json:"commonName"`
	Certificate string `json:"certificate"`
}

type issueCertRequest struct {
	SystemName string `json:"systemName"`
}

type issueCertResponse struct {
	SystemName  string `json:"systemName"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
	IssuedAt   string `json:"issuedAt"`
	ExpiresAt  string `json:"expiresAt"`
}

// fetchCACertPool fetches the CA certificate and returns an x509.CertPool.
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

// issueCertAndKey issues a certificate for the given name and returns a tls.Certificate.
func issueCertAndKey(caURL, name string) (tls.Certificate, error) {
	body, _ := json.Marshal(issueCertRequest{SystemName: name})
	resp, err := http.Post(caURL+"/ca/certificate/issue", "application/json", bytes.NewReader(body))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("POST /ca/certificate/issue: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return tls.Certificate{}, fmt.Errorf("POST /ca/certificate/issue returned %d", resp.StatusCode)
	}
	var certResp issueCertResponse
	if err := json.NewDecoder(resp.Body).Decode(&certResp); err != nil {
		return tls.Certificate{}, fmt.Errorf("decode issue cert response: %w", err)
	}
	cert, err := tls.X509KeyPair([]byte(certResp.Certificate), []byte(certResp.PrivateKey))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse key pair: %w", err)
	}
	return cert, nil
}

func main() {
	azURL       := envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce")
	azDomainExt := envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp5")
	kafkaBrokers := strings.Split(envOr("KAFKA_BROKERS", "kafka:9092"), ",")
	port        := envOr("PORT", "9091")
	caURL       := os.Getenv("CA_URL") // optional: enables TLS when set

	// Optional TLS setup for Kafka.
	var kafkaTLSCfg *tls.Config
	if caURL != "" {
		log.Printf("[kafka-authz] CA_URL set (%s) — enabling Kafka TLS", caURL)
		var caPool *x509.CertPool
		for attempt := 1; attempt <= 10; attempt++ {
			pool, err := fetchCACertPool(caURL)
			if err != nil {
				if attempt < 10 {
					log.Printf("[kafka-authz] CA fetch attempt %d/10: %v — retrying in 3s", attempt, err)
					time.Sleep(3 * time.Second)
					continue
				}
				log.Fatalf("[kafka-authz] CA fetch failed after 10 attempts: %v", err)
			}
			caPool = pool
			break
		}

		ownCert, err := issueCertAndKey(caURL, "kafka-authz")
		if err != nil {
			log.Fatalf("[kafka-authz] cert issue failed: %v", err)
		}

		kafkaTLSCfg = &tls.Config{
			Certificates: []tls.Certificate{ownCert},
			RootCAs:      caPool,
			MinVersion:   tls.VersionTLS12,
		}
		log.Printf("[kafka-authz] Kafka TLS configured")
	}

	client := az.New(azURL)

	// Resolve the AuthzForce domain ID (set up by policy-sync).
	log.Printf("[kafka-authz] resolving AuthzForce domain %q at %s", azDomainExt, azURL)
	var domainID string
	for attempt := 1; ; attempt++ {
		id, err := client.EnsureDomain(azDomainExt)
		if err != nil || id == "" {
			if attempt == 1 || attempt%5 == 0 {
				log.Printf("[kafka-authz] domain lookup attempt %d: %v — retrying", attempt, err)
			}
			time.Sleep(3 * time.Second)
			continue
		}
		domainID = id
		break
	}
	log.Printf("[kafka-authz] using AuthzForce domain=%s kafka=%v", domainID, kafkaBrokers)

	cfg := serverConfig{
		kafkaBrokers: kafkaBrokers,
		azDomainID:   domainID,
		tlsConfig:    kafkaTLSCfg,
	}
	srv := newAuthzServer(cfg, client)

	mux := http.NewServeMux()
	srv.register(mux)

	log.Printf("[kafka-authz] listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
