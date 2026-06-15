// pip_handlers_test.go — TDD tests for CA-as-PIP HTTP endpoints.
//
// Experiment-15: CA-as-PIP — profile-ca serves PIP API endpoints directly from
// its cert store, removing the need for a separate pip service.
//
// Tested endpoints:
//
//	GET /pip/attributes/{cn}  — XACML-ready cert validity attributes
//	GET /pip/subjects         — list all cert records (including revoked)
//	GET /pip/subjects/{cn}    — single cert record by CN
//	GET /pip/status           — subjectCount
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── helpers ─────────────────────────────────────────────────────────────────

func newPIPTestCA(t *testing.T) *ProfileCA {
	t.Helper()
	ca, err := NewProfileCA(24*time.Hour, "")
	if err != nil {
		t.Fatalf("NewProfileCA: %v", err)
	}
	return ca
}

func pipRequest(t *testing.T, mux *http.ServeMux, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func pipMux(ca *ProfileCA) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/pip/attributes/", handlePIPAttributes(ca))
	mux.HandleFunc("/pip/subjects", handlePIPSubjects(ca))
	mux.HandleFunc("/pip/subjects/", handlePIPSubject(ca))
	mux.HandleFunc("/pip/status", handlePIPStatus(ca))
	return mux
}

// ── GET /pip/attributes/{cn} ─────────────────────────────────────────────────

// Test 1: known, non-revoked cert → valid=true with correct fields.
func TestPIPAttributes_KnownValidCert(t *testing.T) {
	ca := newPIPTestCA(t)
	ca.IssueInfraCert("portal-cloud-ml") //nolint:errcheck

	mux := pipMux(ca)
	w := pipRequest(t, mux, http.MethodGet, "/pip/attributes/portal-cloud-ml")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v — body: %s", err, w.Body.String())
	}

	if resp["systemName"] != "portal-cloud-ml" {
		t.Errorf("systemName: expected portal-cloud-ml, got %v", resp["systemName"])
	}
	if resp["certLevel"] != "sy" {
		t.Errorf("certLevel: expected sy, got %v", resp["certLevel"])
	}
	if resp["valid"] != true {
		t.Errorf("valid: expected true, got %v", resp["valid"])
	}
}

// Test 2: unknown CN → 404.
func TestPIPAttributes_UnknownCN(t *testing.T) {
	ca := newPIPTestCA(t)
	mux := pipMux(ca)

	w := pipRequest(t, mux, http.MethodGet, "/pip/attributes/ghost-system")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown CN, got %d: %s", w.Code, w.Body.String())
	}
}

// Test 3: revoked cert → valid=false.
func TestPIPAttributes_RevokedCert(t *testing.T) {
	ca := newPIPTestCA(t)
	ca.IssueInfraCert("revoke-me") //nolint:errcheck
	if err := ca.Revoke("revoke-me"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	mux := pipMux(ca)
	w := pipRequest(t, mux, http.MethodGet, "/pip/attributes/revoke-me")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for revoked cert, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["valid"] != false {
		t.Errorf("valid: expected false for revoked cert, got %v", resp["valid"])
	}
	if resp["systemName"] != "revoke-me" {
		t.Errorf("systemName: expected revoke-me, got %v", resp["systemName"])
	}
}

// Test 4: expired cert → valid=false.
// We use a CA with 1ns cert duration so certs expire immediately.
func TestPIPAttributes_ExpiredCert(t *testing.T) {
	ca, err := NewProfileCA(1, "") // 1 nanosecond cert lifetime
	if err != nil {
		t.Fatalf("NewProfileCA: %v", err)
	}
	ca.IssueInfraCert("expired-sys") //nolint:errcheck

	// Wait a tiny bit to ensure expiry has passed
	time.Sleep(5 * time.Millisecond)

	mux := pipMux(ca)
	w := pipRequest(t, mux, http.MethodGet, "/pip/attributes/expired-sys")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for expired cert, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["valid"] != false {
		t.Errorf("valid: expected false for expired cert, got %v", resp["valid"])
	}
}

// ── GET /pip/subjects ────────────────────────────────────────────────────────

// Test 5: GET /pip/subjects → list of all subjects including revoked.
func TestPIPSubjects_ListAll(t *testing.T) {
	ca := newPIPTestCA(t)
	ca.IssueInfraCert("system-a")  //nolint:errcheck
	ca.IssueInfraCert("system-b")  //nolint:errcheck
	ca.Revoke("system-b")          //nolint:errcheck

	mux := pipMux(ca)
	w := pipRequest(t, mux, http.MethodGet, "/pip/subjects")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "system-a") {
		t.Errorf("subjects list should contain system-a, got: %s", body)
	}
	if !strings.Contains(body, "system-b") {
		t.Errorf("subjects list should contain revoked system-b (GetAllRecords includes revoked), got: %s", body)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := resp["subjects"]; !ok {
		t.Error("response missing 'subjects' field")
	}
}

// ── GET /pip/subjects/{cn} ───────────────────────────────────────────────────

// Test 6: known CN → subject details.
func TestPIPSubject_KnownCN(t *testing.T) {
	ca := newPIPTestCA(t)
	ca.IssueInfraCert("known-system") //nolint:errcheck

	mux := pipMux(ca)
	w := pipRequest(t, mux, http.MethodGet, "/pip/subjects/known-system")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["cn"] != "known-system" {
		t.Errorf("cn: expected known-system, got %v", resp["cn"])
	}
}

// Test 7: unknown CN → 404.
func TestPIPSubject_UnknownCN(t *testing.T) {
	ca := newPIPTestCA(t)
	mux := pipMux(ca)

	w := pipRequest(t, mux, http.MethodGet, "/pip/subjects/no-such-system")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown CN, got %d: %s", w.Code, w.Body.String())
	}
}

// ── GET /pip/status ──────────────────────────────────────────────────────────

// Test 8: GET /pip/status → {"subjectCount":N}.
func TestPIPStatus(t *testing.T) {
	ca := newPIPTestCA(t)
	ca.IssueInfraCert("s1") //nolint:errcheck
	ca.IssueInfraCert("s2") //nolint:errcheck
	ca.IssueInfraCert("s3") //nolint:errcheck

	mux := pipMux(ca)
	w := pipRequest(t, mux, http.MethodGet, "/pip/status")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	countVal, ok := resp["subjectCount"]
	if !ok {
		t.Fatalf("response missing 'subjectCount' field: %s", w.Body.String())
	}
	// JSON numbers decode as float64
	count, ok := countVal.(float64)
	if !ok {
		t.Fatalf("subjectCount is not a number: %T %v", countVal, countVal)
	}
	if int(count) != 3 {
		t.Errorf("subjectCount: expected 3, got %d", int(count))
	}
}

// TestPIPStatus_IncludesRevoked verifies status count includes revoked records.
func TestPIPStatus_IncludesRevoked(t *testing.T) {
	ca := newPIPTestCA(t)
	ca.IssueInfraCert("active") //nolint:errcheck
	ca.IssueInfraCert("revoked-one") //nolint:errcheck
	ca.Revoke("revoked-one") //nolint:errcheck

	mux := pipMux(ca)
	w := pipRequest(t, mux, http.MethodGet, "/pip/status")

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	count := int(resp["subjectCount"].(float64))
	if count != 2 {
		t.Errorf("subjectCount: expected 2 (including revoked), got %d", count)
	}
}

// TestPIPAttributes_ContentType verifies the response has application/json content type.
func TestPIPAttributes_ContentType(t *testing.T) {
	ca := newPIPTestCA(t)
	ca.IssueInfraCert("ct-test") //nolint:errcheck

	mux := pipMux(ca)
	w := pipRequest(t, mux, http.MethodGet, "/pip/attributes/ct-test")

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: expected application/json, got %s", ct)
	}
}
