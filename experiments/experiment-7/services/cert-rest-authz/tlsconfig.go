// tlsconfig.go — CA-fetching and TLS configuration helpers for cert-rest-authz.
package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// caInfoResp mirrors GET /ca/info response.
type caInfoResp struct {
	CommonName  string `json:"commonName"`
	Certificate string `json:"certificate"`
}

// issueCertReq is sent to POST /ca/certificate/issue.
type issueCertReq struct {
	SystemName string `json:"systemName"`
}

// issueCertResp mirrors POST /ca/certificate/issue response.
type issueCertResp struct {
	SystemName  string `json:"systemName"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
	IssuedAt   string `json:"issuedAt"`
	ExpiresAt  string `json:"expiresAt"`
}

// fetchCACert fetches the CA certificate from GET /ca/info.
// Returns the populated CertPool and the raw PEM bytes.
func fetchCACert(caURL string) (*x509.CertPool, []byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(caURL + "/ca/info")
	if err != nil {
		return nil, nil, fmt.Errorf("GET /ca/info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("GET /ca/info returned %d", resp.StatusCode)
	}
	var info caInfoResp
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, nil, fmt.Errorf("decode /ca/info: %w", err)
	}
	if info.Certificate == "" {
		return nil, nil, fmt.Errorf("CA info: empty certificate field")
	}
	pem := []byte(info.Certificate)
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, nil, fmt.Errorf("failed to parse CA certificate PEM")
	}
	return pool, pem, nil
}

// issueCert issues a certificate for systemName from POST /ca/certificate/issue.
// Returns the parsed tls.Certificate.
func issueCert(caURL, systemName string) (tls.Certificate, error) {
	body, _ := json.Marshal(issueCertReq{SystemName: systemName})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(caURL+"/ca/certificate/issue", "application/json", bytes.NewReader(body))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("POST /ca/certificate/issue: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return tls.Certificate{}, fmt.Errorf("POST /ca/certificate/issue returned %d", resp.StatusCode)
	}
	var certResp issueCertResp
	if err := json.NewDecoder(resp.Body).Decode(&certResp); err != nil {
		return tls.Certificate{}, fmt.Errorf("decode issue cert response: %w", err)
	}
	cert, err := tls.X509KeyPair([]byte(certResp.Certificate), []byte(certResp.PrivateKey))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse issued key pair for %q: %w", systemName, err)
	}
	return cert, nil
}

// buildServerTLSConfig creates a TLS config for the mTLS server.
// It presents cert as the server certificate and requires client certificates
// verified against caPool.
func buildServerTLSConfig(cert tls.Certificate, caPool *x509.CertPool) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS12,
	}
}

// buildClientTLSConfig creates a TLS config for outgoing HTTPS clients.
// It presents cert as the client certificate and verifies servers against caPool.
func buildClientTLSConfig(cert tls.Certificate, caPool *x509.CertPool) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}
}
