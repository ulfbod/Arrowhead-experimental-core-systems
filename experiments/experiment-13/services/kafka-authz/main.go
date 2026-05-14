// kafka-authz — Kafka SSE proxy with AuthzForce + PIP cert-level enforcement for experiment-13.
//
// Extends the experiment-5/6 kafka-authz with PIP cert-level attribute enrichment.
// Before calling AuthzForce, the PEP queries PIP (GET /pip/attributes/{name}) to
// get the consumer's certLevel and valid attributes, which are included as additional
// subject attributes in the XACML request.
//
// Environment variables:
//
//	KAFKA_BROKERS       Comma-separated broker list (default: kafka:9092)
//	AUTHZFORCE_URL      AuthzForce base URL (default: http://authzforce:8080/authzforce-ce)
//	AUTHZFORCE_DOMAIN   AuthzForce domain externalId (default: arrowhead-exp13)
//	PIP_URL             PIP base URL (default: http://pip:9506)
//	PORT                HTTP port (default: 9091)
//	CA_URL              Optional: Arrowhead CA base URL for TLS.
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
	"time"
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
	IssuedAt    string `json:"issuedAt"`
	ExpiresAt   string `json:"expiresAt"`
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
	azURL        := envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce")
	azDomainExt  := envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp13")
	kafkaBrokers := strings.Split(envOr("KAFKA_BROKERS", "kafka:9092"), ",")
	pipURL        := envOr("PIP_URL", "http://pip:9506")
	port         := envOr("PORT", "9091")
	caURL        := os.Getenv("CA_URL") // optional: enables TLS when set

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

	// Resolve the AuthzForce domain ID by direct HTTP query.
	// (No shared library dependency — domain ID is derived from EnsureDomain-like logic.)
	log.Printf("[kafka-authz] resolving AuthzForce domain %q at %s", azDomainExt, azURL)
	domainID, err := ensureDomain(azURL, azDomainExt)
	if err != nil {
		log.Fatalf("[kafka-authz] domain resolve failed: %v", err)
	}
	log.Printf("[kafka-authz] using AuthzForce domain=%s pip=%s kafka=%v", domainID, pipURL, kafkaBrokers)

	cfg := serverConfig{
		kafkaBrokers: kafkaBrokers,
		azDomainID:   domainID,
		azURL:        azURL,
		pipURL:       pipURL,
		tlsConfig:    kafkaTLSCfg,
	}
	srv := newAuthzServer(cfg)

	mux := http.NewServeMux()
	srv.register(mux)

	log.Printf("[kafka-authz] listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

// ensureDomain resolves or creates the AuthzForce domain with the given externalID.
// Retries until the domain is available (blocks until domain is set up by policy-sync).
func ensureDomain(azURL, externalID string) (string, error) {
	for attempt := 1; ; attempt++ {
		id, err := findDomain(azURL, externalID)
		if err == nil && id != "" {
			return id, nil
		}
		if attempt == 1 || attempt%5 == 0 {
			log.Printf("[kafka-authz] domain lookup attempt %d: %v — retrying", attempt, err)
		}
		time.Sleep(3 * time.Second)
	}
}

// findDomain looks up an AuthzForce domain by externalID and returns its ID.
func findDomain(azURL, externalID string) (string, error) {
	url := fmt.Sprintf("%s/domains?externalId=%s", azURL, externalID)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("list domains returned %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	return extractHrefID(string(body)), nil
}

// extractHrefID finds the last path segment of the first href attribute in s.
func extractHrefID(s string) string {
	for _, prefix := range []string{`href="`, `href='`} {
		idx := strings.Index(s, prefix)
		if idx < 0 {
			continue
		}
		rest := s[idx+len(prefix):]
		end := strings.IndexAny(rest, `"'`)
		if end < 0 {
			continue
		}
		href := rest[:end]
		href = strings.TrimRight(href, "/")
		i := strings.LastIndex(href, "/")
		if i >= 0 {
			return href[i+1:]
		}
		return href
	}
	return ""
}
