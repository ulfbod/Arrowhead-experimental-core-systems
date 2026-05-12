package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func setupCA(t *testing.T) *ProfileCA {
	t.Helper()
	ca, err := NewProfileCA(24 * time.Hour)
	if err != nil {
		t.Fatalf("NewProfileCA: %v", err)
	}
	return ca
}

func TestHandleInfo(t *testing.T) {
	ca := setupCA(t)
	req := httptest.NewRequest(http.MethodGet, "/ca/info", nil)
	w := httptest.NewRecorder()
	handleInfo(ca)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["certificate"] == "" {
		t.Error("expected certificate in response")
	}
	if resp["commonName"] == "" {
		t.Error("expected commonName in response")
	}
}

func TestHandleInfo_MethodNotAllowed(t *testing.T) {
	ca := setupCA(t)
	req := httptest.NewRequest(http.MethodPost, "/ca/info", nil)
	w := httptest.NewRecorder()
	handleInfo(ca)(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleBootstrapOnboarding(t *testing.T) {
	ca := setupCA(t)
	body := `{"systemName":"test-device"}`
	req := httptest.NewRequest(http.MethodPost, "/bootstrap/onboarding-cert", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handleBootstrapOnboarding(ca)(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp certResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Certificate == "" {
		t.Error("expected certificate")
	}
	if resp.Profile != "on" {
		t.Errorf("expected profile=on, got %s", resp.Profile)
	}
}

func TestHandleBootstrapOnboarding_BadJSON(t *testing.T) {
	ca := setupCA(t)
	req := httptest.NewRequest(http.MethodPost, "/bootstrap/onboarding-cert", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	handleBootstrapOnboarding(ca)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleIssueInfra(t *testing.T) {
	ca := setupCA(t)
	body := `{"systemName":"kafka"}`
	req := httptest.NewRequest(http.MethodPost, "/ca/certificate/issue", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	handleIssueInfra(ca)(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	var resp certResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Profile != "sy" {
		t.Errorf("infra cert should have profile=sy, got %s", resp.Profile)
	}
}

func buildMTLSRequest(t *testing.T, method, path string, body string, clientCert tls.Certificate) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	// Simulate TLS with peer certificate
	leafCert, err := x509.ParseCertificate(clientCert.Certificate[0])
	if err != nil {
		t.Fatalf("parse client cert: %v", err)
	}
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{leafCert},
	}
	return req
}

func TestHandleDeviceCert_WithOnboardingCert(t *testing.T) {
	ca := setupCA(t)
	onCertPEM, onKeyPEM, _ := ca.IssueOnboardingCert("my-device")
	onTLSCert, _ := tls.X509KeyPair([]byte(onCertPEM), []byte(onKeyPEM))

	req := buildMTLSRequest(t, http.MethodPost, "/ca/device-cert", `{"systemName":"my-device"}`, onTLSCert)
	w := httptest.NewRecorder()
	handleDeviceCert(ca)(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp certResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Profile != "de" {
		t.Errorf("expected profile=de, got %s", resp.Profile)
	}
}

func TestHandleDeviceCert_WithoutClientCert(t *testing.T) {
	ca := setupCA(t)
	req := httptest.NewRequest(http.MethodPost, "/ca/device-cert", bytes.NewBufferString(`{"systemName":"x"}`))
	// No TLS state set → no client cert
	w := httptest.NewRecorder()
	handleDeviceCert(ca)(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleDeviceCert_WrongProfile(t *testing.T) {
	ca := setupCA(t)
	// Use a system cert instead of onboarding cert
	syCertPEM, syKeyPEM, _ := ca.IssueInfraCert("wrong")
	syTLSCert, _ := tls.X509KeyPair([]byte(syCertPEM), []byte(syKeyPEM))

	req := buildMTLSRequest(t, http.MethodPost, "/ca/device-cert", `{"systemName":"wrong"}`, syTLSCert)
	w := httptest.NewRecorder()
	handleDeviceCert(ca)(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandleSystemCert_WithDeviceCert(t *testing.T) {
	ca := setupCA(t)
	onCertPEM, onKeyPEM, _ := ca.IssueOnboardingCert("my-system")
	onTLSCert, _ := tls.X509KeyPair([]byte(onCertPEM), []byte(onKeyPEM))
	onLeaf, _ := x509.ParseCertificate(onTLSCert.Certificate[0])
	deCertPEM, deKeyPEM, _ := ca.IssueDeviceCert("my-system", onLeaf)
	deTLSCert, _ := tls.X509KeyPair([]byte(deCertPEM), []byte(deKeyPEM))

	req := buildMTLSRequest(t, http.MethodPost, "/ca/system-cert", `{"systemName":"my-system"}`, deTLSCert)
	w := httptest.NewRecorder()
	handleSystemCert(ca)(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp certResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Profile != "sy" {
		t.Errorf("expected profile=sy, got %s", resp.Profile)
	}
}

func TestHandleSystemCert_WrongProfile(t *testing.T) {
	ca := setupCA(t)
	// Try to use an onboarding cert directly to get a system cert (skip device step)
	onCertPEM, onKeyPEM, _ := ca.IssueOnboardingCert("shortcut")
	onTLSCert, _ := tls.X509KeyPair([]byte(onCertPEM), []byte(onKeyPEM))

	req := buildMTLSRequest(t, http.MethodPost, "/ca/system-cert", `{"systemName":"shortcut"}`, onTLSCert)
	w := httptest.NewRecorder()
	handleSystemCert(ca)(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandleIssueInfra_MethodNotAllowed(t *testing.T) {
	ca := setupCA(t)
	req := httptest.NewRequest(http.MethodGet, "/ca/certificate/issue", nil)
	w := httptest.NewRecorder()
	handleIssueInfra(ca)(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleIssueInfra_BadJSON(t *testing.T) {
	ca := setupCA(t)
	req := httptest.NewRequest(http.MethodPost, "/ca/certificate/issue", bytes.NewBufferString("bad"))
	w := httptest.NewRecorder()
	handleIssueInfra(ca)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeviceCert_MethodNotAllowed(t *testing.T) {
	ca := setupCA(t)
	req := httptest.NewRequest(http.MethodGet, "/ca/device-cert", nil)
	w := httptest.NewRecorder()
	handleDeviceCert(ca)(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleDeviceCert_BadJSON(t *testing.T) {
	ca := setupCA(t)
	onCertPEM, onKeyPEM, _ := ca.IssueOnboardingCert("test")
	onTLSCert, _ := tls.X509KeyPair([]byte(onCertPEM), []byte(onKeyPEM))
	req := buildMTLSRequest(t, http.MethodPost, "/ca/device-cert", "not-json", onTLSCert)
	w := httptest.NewRecorder()
	handleDeviceCert(ca)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSystemCert_MethodNotAllowed(t *testing.T) {
	ca := setupCA(t)
	req := httptest.NewRequest(http.MethodGet, "/ca/system-cert", nil)
	w := httptest.NewRecorder()
	handleSystemCert(ca)(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleSystemCert_WithoutClientCert(t *testing.T) {
	ca := setupCA(t)
	req := httptest.NewRequest(http.MethodPost, "/ca/system-cert", bytes.NewBufferString(`{"systemName":"x"}`))
	w := httptest.NewRecorder()
	handleSystemCert(ca)(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleSystemCert_BadJSON(t *testing.T) {
	ca := setupCA(t)
	onCertPEM, onKeyPEM, _ := ca.IssueOnboardingCert("test")
	onLeaf, _ := x509.ParseCertificate(mustParseTLSCert(t, onCertPEM, onKeyPEM).Certificate[0])
	deCertPEM, deKeyPEM, _ := ca.IssueDeviceCert("test", onLeaf)
	deTLSCert, _ := tls.X509KeyPair([]byte(deCertPEM), []byte(deKeyPEM))
	req := buildMTLSRequest(t, http.MethodPost, "/ca/system-cert", "not-json", deTLSCert)
	w := httptest.NewRecorder()
	handleSystemCert(ca)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleBootstrapOnboarding_EmptySystemName(t *testing.T) {
	ca := setupCA(t)
	req := httptest.NewRequest(http.MethodPost, "/bootstrap/onboarding-cert", bytes.NewBufferString(`{"systemName":""}`))
	w := httptest.NewRecorder()
	handleBootstrapOnboarding(ca)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleBootstrapOnboarding_MethodNotAllowed(t *testing.T) {
	ca := setupCA(t)
	req := httptest.NewRequest(http.MethodGet, "/bootstrap/onboarding-cert", nil)
	w := httptest.NewRecorder()
	handleBootstrapOnboarding(ca)(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func mustParseTLSCert(t *testing.T, certPEM, keyPEM string) tls.Certificate {
	t.Helper()
	c, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	return c
}

func TestEnvOrDefault(t *testing.T) {
	val := envOr("__NONEXISTENT_ENV_VAR_PROFILE_CA__", "mydefault")
	if val != "mydefault" {
		t.Errorf("expected mydefault, got %s", val)
	}
}
