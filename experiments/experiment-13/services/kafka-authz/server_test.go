package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockPDP returns a PDP stub. It returns Permit if the XACML body contains a subject
// from the allowed map.
func mockPDP(allowed map[string]bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		body := string(buf[:n])
		w.Header().Set("Content-Type", "application/xml")
		for s := range allowed {
			if strings.Contains(body, ">"+s+"<") {
				w.Write([]byte(`<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"><Result><Decision>Permit</Decision></Result></Response>`))
				return
			}
		}
		w.Write([]byte(`<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"><Result><Decision>Deny</Decision></Result></Response>`))
	}))
}

// mockPIP returns a PIP stub that returns certLevel/valid for known subjects.
func mockPIP(attrs map[string]subjectAttrs) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/pip/attributes/")
		a, ok := attrs[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"systemName": name,
			"certLevel":  a.CertLevel,
			"valid":      a.CertValid,
		})
	}))
}

func makeServer(t *testing.T, allowed map[string]bool, pipAttrs map[string]subjectAttrs) (*authzServer, *httptest.Server, *httptest.Server) {
	t.Helper()
	pdp := mockPDP(allowed)
	pip := mockPIP(pipAttrs)
	cfg := serverConfig{azDomainID: "test-domain", azURL: pdp.URL, pipURL: pip.URL}
	return newAuthzServer(cfg), pdp, pip
}

// ── /health ───────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	srv, pdp, pip := makeServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.handleHealth(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ── /auth/check ───────────────────────────────────────────────────────────────

func TestAuthCheck_permit(t *testing.T) {
	pipAttrs := map[string]subjectAttrs{
		"analytics-consumer": {CertLevel: "sy", CertValid: true},
	}
	srv, pdp, pip := makeServer(t, map[string]bool{"analytics-consumer": true}, pipAttrs)
	defer pdp.Close()
	defer pip.Close()

	body := `{"consumer":"analytics-consumer","service":"telemetry"}`
	r := httptest.NewRequest(http.MethodPost, "/auth/check", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleAuthCheck(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["decision"] != "Permit" {
		t.Fatalf("expected Permit, got %v", resp["decision"])
	}
}

func TestAuthCheck_deny(t *testing.T) {
	srv, pdp, pip := makeServer(t, map[string]bool{}, nil)
	defer pdp.Close()
	defer pip.Close()

	body := `{"consumer":"analytics-consumer","service":"telemetry"}`
	r := httptest.NewRequest(http.MethodPost, "/auth/check", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleAuthCheck(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["decision"] != "Deny" {
		t.Fatalf("expected Deny, got %v", resp["decision"])
	}
}

func TestAuthCheck_missingFields(t *testing.T) {
	srv, pdp, pip := makeServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()

	r := httptest.NewRequest(http.MethodPost, "/auth/check", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleAuthCheck(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAuthCheck_rejectsGet(t *testing.T) {
	srv, pdp, pip := makeServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	r := httptest.NewRequest(http.MethodGet, "/auth/check", nil)
	w := httptest.NewRecorder()
	srv.handleAuthCheck(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// ── /stream/{consumer} ────────────────────────────────────────────────────────

func TestStream_deniedReturns403(t *testing.T) {
	srv, pdp, pip := makeServer(t, map[string]bool{}, nil)
	defer pdp.Close()
	defer pip.Close()

	r := httptest.NewRequest(http.MethodGet, "/stream/analytics-consumer?service=telemetry", nil)
	w := httptest.NewRecorder()
	srv.handleStream(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unauthorized consumer, got %d", w.Code)
	}
}

func TestStream_missingConsumerName(t *testing.T) {
	srv, pdp, pip := makeServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	r := httptest.NewRequest(http.MethodGet, "/stream/", nil)
	w := httptest.NewRecorder()
	srv.handleStream(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ── TestHandleStream_EnrichesXACMLWithCertLevel ───────────────────────────────
// Verifies that the PIP is queried before AuthzForce, and that the cert-level
// attributes are present in the XACML request body sent to AuthzForce.

func TestHandleStream_EnrichesXACMLWithCertLevel(t *testing.T) {
	var xacmlBody string
	pipQueried := false

	// PDP stub: captures the XACML body for inspection, returns Deny (we only care that PIP was queried)
	pdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		xacmlBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"><Result><Decision>Deny</Decision></Result></Response>`))
	}))
	defer pdp.Close()

	// PIP stub: records that it was called, returns cert level "sy"
	pip := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pipQueried = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"systemName": "analytics-consumer",
			"certLevel":  "sy",
			"valid":      true,
		})
	}))
	defer pip.Close()

	cfg := serverConfig{azDomainID: "test-domain", azURL: pdp.URL, pipURL: pip.URL}
	srv := newAuthzServer(cfg)

	r := httptest.NewRequest(http.MethodGet, "/stream/analytics-consumer?service=telemetry", nil)
	w := httptest.NewRecorder()
	srv.handleStream(w, r)

	// Expect 403 (Deny from PDP), but we verify PIP was queried first.
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (Deny), got %d", w.Code)
	}
	if !pipQueried {
		t.Error("expected PIP to be queried before AuthzForce, but it was not")
	}
	// Verify cert-level attributes appear in the XACML request.
	if !strings.Contains(xacmlBody, "urn:arrowhead:attribute:cert-level") {
		t.Error("expected cert-level attribute in XACML request, not found")
	}
	if !strings.Contains(xacmlBody, "urn:arrowhead:attribute:cert-valid") {
		t.Error("expected cert-valid attribute in XACML request, not found")
	}
	if !strings.Contains(xacmlBody, ">sy<") {
		t.Errorf("expected cert level value 'sy' in XACML request; body: %s", xacmlBody)
	}
}

// ── topicForService ───────────────────────────────────────────────────────────

func TestTopicForService(t *testing.T) {
	if got := topicForService("telemetry"); got != "arrowhead.telemetry" {
		t.Fatalf("got %q", got)
	}
}

// ── status ────────────────────────────────────────────────────────────────────

func TestStatus_empty(t *testing.T) {
	srv, pdp, pip := makeServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	r := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	srv.handleStatus(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["totalServed"] == nil {
		t.Fatal("expected totalServed in response")
	}
}
