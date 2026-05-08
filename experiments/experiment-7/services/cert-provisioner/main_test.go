package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── mock CA helpers ───────────────────────────────────────────────────────────

const (
	fakeCACert   = "-----BEGIN CERTIFICATE-----\nZmFrZWNhY2VydA==\n-----END CERTIFICATE-----\n"
	fakeCert     = "-----BEGIN CERTIFICATE-----\nZmFrZWNlcnQ=\n-----END CERTIFICATE-----\n"
	fakeKey      = "-----BEGIN RSA PRIVATE KEY-----\nZmFrZWtleQ==\n-----END RSA PRIVATE KEY-----\n"
)

// mockCA builds a test HTTP server simulating the CA endpoints.
// infoOK controls whether /ca/info returns 200; issueOK controls /ca/certificate/issue.
func mockCA(t *testing.T, infoOK, issueOK bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ca/info":
			if !infoOK {
				http.Error(w, "CA not ready", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(caInfoResponse{
				CommonName:  "arrowhead-ca",
				Certificate: fakeCACert,
			})
		case "/ca/certificate/issue":
			if !issueOK {
				http.Error(w, "issue failed", http.StatusInternalServerError)
				return
			}
			var req issueCertRequest
			json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(issueCertResponse{
				SystemName:  req.SystemName,
				Certificate: fakeCert,
				PrivateKey:  fakeKey,
				IssuedAt:   "2025-01-01T00:00:00Z",
				ExpiresAt:  "2026-01-01T00:00:00Z",
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

// ── fetchCACert tests ─────────────────────────────────────────────────────────

func TestFetchCACert_success(t *testing.T) {
	ca := mockCA(t, true, false)
	defer ca.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	got, err := fetchCACert(ca.URL, client, 1, 0)
	if err != nil {
		t.Fatalf("fetchCACert: %v", err)
	}
	if got != fakeCACert {
		t.Errorf("got %q, want %q", got, fakeCACert)
	}
}

func TestFetchCACert_serverError(t *testing.T) {
	ca := mockCA(t, false, false)
	defer ca.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := fetchCACert(ca.URL, client, 2, 0)
	if err == nil {
		t.Error("expected error when CA returns 503")
	}
}

func TestFetchCACert_badJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-json{{{"))
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := fetchCACert(srv.URL, client, 1, 0)
	if err == nil {
		t.Error("expected error on malformed JSON response")
	}
}

// ── issueCert tests ───────────────────────────────────────────────────────────

func TestIssueCert_success(t *testing.T) {
	ca := mockCA(t, true, true)
	defer ca.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := issueCert(ca.URL, "kafka", client, 1, 0)
	if err != nil {
		t.Fatalf("issueCert: %v", err)
	}
	if resp.SystemName != "kafka" {
		t.Errorf("systemName: got %q, want kafka", resp.SystemName)
	}
	if resp.Certificate != fakeCert {
		t.Errorf("certificate: got %q, want %q", resp.Certificate, fakeCert)
	}
	if resp.PrivateKey != fakeKey {
		t.Errorf("privateKey: got %q, want %q", resp.PrivateKey, fakeKey)
	}
}

func TestIssueCert_serverError(t *testing.T) {
	ca := mockCA(t, true, false)
	defer ca.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := issueCert(ca.URL, "kafka", client, 2, 0)
	if err == nil {
		t.Error("expected error when CA returns 500")
	}
}

// ── writeCerts integration test ───────────────────────────────────────────────

func TestWriteCerts_success(t *testing.T) {
	ca := mockCA(t, true, true)
	defer ca.Close()

	certsDir := t.TempDir()
	client := &http.Client{Timeout: 5 * time.Second}

	if err := writeCerts(ca.URL, certsDir, client); err != nil {
		t.Fatalf("writeCerts: %v", err)
	}

	// Verify all expected files exist.
	expectedFiles := []string{
		"ca.crt",
		"kafka.crt", "kafka.key", "kafka-combined.pem",
		"rabbitmq.crt", "rabbitmq.key", "rabbitmq-combined.pem",
	}
	for _, name := range expectedFiles {
		path := filepath.Join(certsDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("expected file %s not found: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("file %s is empty", name)
		}
	}
}

func TestWriteCerts_combinedPEMContent(t *testing.T) {
	ca := mockCA(t, true, true)
	defer ca.Close()

	certsDir := t.TempDir()
	client := &http.Client{Timeout: 5 * time.Second}

	if err := writeCerts(ca.URL, certsDir, client); err != nil {
		t.Fatalf("writeCerts: %v", err)
	}

	// kafka-combined.pem must contain both the cert and the key.
	combined, err := os.ReadFile(filepath.Join(certsDir, "kafka-combined.pem"))
	if err != nil {
		t.Fatalf("read kafka-combined.pem: %v", err)
	}
	combinedStr := string(combined)
	if !strings.Contains(combinedStr, "BEGIN CERTIFICATE") {
		t.Error("kafka-combined.pem missing certificate block")
	}
	if !strings.Contains(combinedStr, "BEGIN RSA PRIVATE KEY") {
		t.Error("kafka-combined.pem missing private key block")
	}
}

func TestWriteCerts_caError(t *testing.T) {
	ca := mockCA(t, false, true)
	defer ca.Close()

	certsDir := t.TempDir()
	client := &http.Client{Timeout: 5 * time.Second}

	err := writeCerts(ca.URL, certsDir, client)
	if err == nil {
		t.Error("expected error when CA /ca/info returns 503")
	}
}
