// cert-provisioner — fetches TLS certificates from the Arrowhead CA and writes
// them to a shared Docker volume so that Kafka and RabbitMQ can use TLS.
//
// The provisioner is a one-shot init container: it exits 0 on success and 1 on
// any unrecoverable error.  It retries CA calls up to 10 times with a 3-second
// delay to tolerate slow CA startup.
//
// Files written to CERTS_DIR:
//
//	ca.crt              — CA certificate (PEM)
//	kafka.crt           — Kafka server certificate (PEM)
//	kafka.key           — Kafka server private key (PEM)
//	kafka-combined.pem  — kafka.crt + kafka.key concatenated
//	rabbitmq.crt        — RabbitMQ server certificate (PEM)
//	rabbitmq.key        — RabbitMQ server private key (PEM)
//	rabbitmq-combined.pem — rabbitmq.crt + rabbitmq.key concatenated
//
// Environment variables:
//
//	CA_URL    Base URL of the Arrowhead CA (default: http://ca:8086)
//	CERTS_DIR Directory to write certificate files (default: /certs)
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// caInfoResponse is the JSON body returned by GET /ca/info.
type caInfoResponse struct {
	CommonName  string `json:"commonName"`
	Certificate string `json:"certificate"`
}

// issueCertRequest is the JSON body sent to POST /ca/certificate/issue.
type issueCertRequest struct {
	SystemName string `json:"systemName"`
}

// issueCertResponse is the JSON body returned by POST /ca/certificate/issue.
type issueCertResponse struct {
	SystemName  string `json:"systemName"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
	IssuedAt   string `json:"issuedAt"`
	ExpiresAt  string `json:"expiresAt"`
}

// fetchCACert retrieves the CA certificate PEM from GET /ca/info.
// It retries up to maxAttempts times with retryDelay between attempts.
func fetchCACert(caURL string, client *http.Client, maxAttempts int, retryDelay time.Duration) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := client.Get(caURL + "/ca/info")
		if err != nil {
			lastErr = fmt.Errorf("GET /ca/info: %w", err)
			log.Printf("[cert-provisioner] CA info attempt %d/%d failed: %v — retrying in %s",
				attempt, maxAttempts, err, retryDelay)
			time.Sleep(retryDelay)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("GET /ca/info returned %d", resp.StatusCode)
			log.Printf("[cert-provisioner] CA info attempt %d/%d: %v — retrying in %s",
				attempt, maxAttempts, lastErr, retryDelay)
			resp.Body.Close()
			time.Sleep(retryDelay)
			continue
		}
		var info caInfoResponse
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return "", fmt.Errorf("decode /ca/info response: %w", err)
		}
		if info.Certificate == "" {
			return "", fmt.Errorf("CA info: empty certificate field")
		}
		log.Printf("[cert-provisioner] fetched CA cert (CN=%s)", info.CommonName)
		return info.Certificate, nil
	}
	return "", fmt.Errorf("CA info after %d attempts: %w", maxAttempts, lastErr)
}

// issueCert requests a certificate for the given systemName from POST /ca/certificate/issue.
// It retries up to maxAttempts times with retryDelay between attempts.
func issueCert(caURL, systemName string, client *http.Client, maxAttempts int, retryDelay time.Duration) (issueCertResponse, error) {
	body, err := json.Marshal(issueCertRequest{SystemName: systemName})
	if err != nil {
		return issueCertResponse{}, fmt.Errorf("marshal issue request: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := client.Post(caURL+"/ca/certificate/issue", "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = fmt.Errorf("POST /ca/certificate/issue: %w", err)
			log.Printf("[cert-provisioner] issue cert %q attempt %d/%d failed: %v — retrying in %s",
				systemName, attempt, maxAttempts, err, retryDelay)
			time.Sleep(retryDelay)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			lastErr = fmt.Errorf("POST /ca/certificate/issue returned %d", resp.StatusCode)
			log.Printf("[cert-provisioner] issue cert %q attempt %d/%d: %v — retrying in %s",
				systemName, attempt, maxAttempts, lastErr, retryDelay)
			resp.Body.Close()
			time.Sleep(retryDelay)
			continue
		}
		var certResp issueCertResponse
		if err := json.NewDecoder(resp.Body).Decode(&certResp); err != nil {
			return issueCertResponse{}, fmt.Errorf("decode issue cert response for %q: %w", systemName, err)
		}
		log.Printf("[cert-provisioner] issued cert for %q (expires %s)", systemName, certResp.ExpiresAt)
		return certResp, nil
	}
	return issueCertResponse{}, fmt.Errorf("issue cert %q after %d attempts: %w", systemName, maxAttempts, lastErr)
}

// writeCerts fetches the CA cert and issues certs for kafka and rabbitmq,
// then writes all certificate files to certsDir.
func writeCerts(caURL, certsDir string, client *http.Client) error {
	const maxAttempts = 10
	const retryDelay = 3 * time.Second

	// Fetch CA certificate.
	caCert, err := fetchCACert(caURL, client, maxAttempts, retryDelay)
	if err != nil {
		return fmt.Errorf("fetch CA cert: %w", err)
	}
	if err := writeFile(filepath.Join(certsDir, "ca.crt"), caCert); err != nil {
		return err
	}

	// Issue and write kafka cert.
	kafkaCert, err := issueCert(caURL, "kafka", client, maxAttempts, retryDelay)
	if err != nil {
		return fmt.Errorf("issue kafka cert: %w", err)
	}
	if err := writeFile(filepath.Join(certsDir, "kafka.crt"), kafkaCert.Certificate); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(certsDir, "kafka.key"), kafkaCert.PrivateKey); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(certsDir, "kafka-combined.pem"), kafkaCert.Certificate+kafkaCert.PrivateKey); err != nil {
		return err
	}

	// Issue and write rabbitmq cert.
	rabbitCert, err := issueCert(caURL, "rabbitmq", client, maxAttempts, retryDelay)
	if err != nil {
		return fmt.Errorf("issue rabbitmq cert: %w", err)
	}
	if err := writeFile(filepath.Join(certsDir, "rabbitmq.crt"), rabbitCert.Certificate); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(certsDir, "rabbitmq.key"), rabbitCert.PrivateKey); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(certsDir, "rabbitmq-combined.pem"), rabbitCert.Certificate+rabbitCert.PrivateKey); err != nil {
		return err
	}

	return nil
}

// writeFile writes content to path, creating or truncating the file.
func writeFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	log.Printf("[cert-provisioner] wrote %s", path)
	return nil
}

func main() {
	caURL    := envOr("CA_URL", "http://ca:8086")
	certsDir := envOr("CERTS_DIR", "/certs")

	log.Printf("[cert-provisioner] starting (CA=%s CERTS_DIR=%s)", caURL, certsDir)

	if err := os.MkdirAll(certsDir, 0o755); err != nil {
		log.Fatalf("[cert-provisioner] create certs dir: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	if err := writeCerts(caURL, certsDir, client); err != nil {
		log.Fatalf("[cert-provisioner] %v", err)
	}

	log.Printf("[cert-provisioner] all certificates written to %s — done", certsDir)
}
