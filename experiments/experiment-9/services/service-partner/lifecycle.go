// lifecycle.go — Arrowhead 5.2 onboarding lifecycle for experiment-9 service-partner.
//
// AcquireSystemCert performs the full four-step process:
//  1. GET /ca/info          → CA cert pool (plain HTTP)
//  2. POST /bootstrap/onboarding-cert → onboarding cert (OU=on, plain HTTP)
//  3. POST /ca/device-cert  → device cert (OU=de, TLS with onboarding cert)
//  4. POST /ca/system-cert  → system cert (OU=sy, TLS with device cert)
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

type certResp struct {
	SystemName  string `json:"systemName"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
	Profile     string `json:"profile"`
}

type caInfoResp struct {
	CommonName  string `json:"commonName"`
	Certificate string `json:"certificate"`
}

// AcquireSystemCert performs the full Arrowhead 5.2 onboarding lifecycle.
func AcquireSystemCert(caHTTPURL, caTLSURL, systemName string) (tls.Certificate, *x509.CertPool, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}

	caPool, err := fetchCAPool(caHTTPURL, httpClient)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 1 (CA cert): %w", err)
	}

	onCertPEM, onKeyPEM, err := requestCert(caHTTPURL+"/bootstrap/onboarding-cert", systemName, httpClient)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 2 (onboarding cert): %w", err)
	}
	onTLSCert, err := tls.X509KeyPair([]byte(onCertPEM), []byte(onKeyPEM))
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 2 (parse onboarding cert): %w", err)
	}

	onClient := buildProfileClient(onTLSCert, caPool)
	deCertPEM, deKeyPEM, err := requestCert(caTLSURL+"/ca/device-cert", systemName, onClient)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 3 (device cert): %w", err)
	}
	deTLSCert, err := tls.X509KeyPair([]byte(deCertPEM), []byte(deKeyPEM))
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 3 (parse device cert): %w", err)
	}

	deClient := buildProfileClient(deTLSCert, caPool)
	syCertPEM, syKeyPEM, err := requestCert(caTLSURL+"/ca/system-cert", systemName, deClient)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 4 (system cert): %w", err)
	}
	syTLSCert, err := tls.X509KeyPair([]byte(syCertPEM), []byte(syKeyPEM))
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("step 4 (parse system cert): %w", err)
	}

	return syTLSCert, caPool, nil
}

func fetchCAPool(caHTTPURL string, client *http.Client) (*x509.CertPool, error) {
	resp, err := client.Get(caHTTPURL + "/ca/info")
	if err != nil {
		return nil, fmt.Errorf("GET /ca/info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /ca/info returned %d", resp.StatusCode)
	}
	var info caInfoResp
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode /ca/info: %w", err)
	}
	if info.Certificate == "" {
		return nil, fmt.Errorf("empty certificate in /ca/info")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(info.Certificate)) {
		return nil, fmt.Errorf("parse CA cert PEM")
	}
	return pool, nil
}

func requestCert(url, name string, client *http.Client) (certPEM, keyPEM string, err error) {
	body, _ := json.Marshal(map[string]string{"systemName": name})
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("POST %s returned %d", url, resp.StatusCode)
	}
	var cr certResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", "", fmt.Errorf("decode cert response: %w", err)
	}
	if cr.Certificate == "" || cr.PrivateKey == "" {
		return "", "", fmt.Errorf("empty certificate or key in response")
	}
	return cr.Certificate, cr.PrivateKey, nil
}

func buildProfileClient(cert tls.Certificate, caPool *x509.CertPool) *http.Client {
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   10 * time.Second,
	}
}
