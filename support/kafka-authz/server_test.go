package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	az "arrowhead/authzforce"
)

// mockAZPDP returns a PDP stub that permits the listed subjects.
func mockAZPDP(allowed map[string]bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
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

func makeServer(t *testing.T, allowed map[string]bool) (*authzServer, *httptest.Server) {
	t.Helper()
	pdp := mockAZPDP(allowed)
	cfg := serverConfig{azDomainID: "test-domain"}
	return newAuthzServer(cfg, az.New(pdp.URL)), pdp
}

// ── /health ───────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	srv, pdp := makeServer(t, nil)
	defer pdp.Close()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.handleHealth(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ── /auth/check ───────────────────────────────────────────────────────────────

func TestAuthCheck_permit(t *testing.T) {
	srv, pdp := makeServer(t, map[string]bool{"analytics-consumer": true})
	defer pdp.Close()

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
	srv, pdp := makeServer(t, map[string]bool{})
	defer pdp.Close()

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
	srv, pdp := makeServer(t, nil)
	defer pdp.Close()

	r := httptest.NewRequest(http.MethodPost, "/auth/check", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleAuthCheck(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAuthCheck_rejectsGet(t *testing.T) {
	srv, pdp := makeServer(t, nil)
	defer pdp.Close()
	r := httptest.NewRequest(http.MethodGet, "/auth/check", nil)
	w := httptest.NewRecorder()
	srv.handleAuthCheck(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// ── /stream/{consumer} ────────────────────────────────────────────────────────

func TestStream_deniedReturns403(t *testing.T) {
	srv, pdp := makeServer(t, map[string]bool{})
	defer pdp.Close()

	r := httptest.NewRequest(http.MethodGet, "/stream/analytics-consumer?service=telemetry", nil)
	w := httptest.NewRecorder()
	srv.handleStream(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unauthorized consumer, got %d", w.Code)
	}
}

func TestStream_missingConsumerName(t *testing.T) {
	srv, pdp := makeServer(t, nil)
	defer pdp.Close()
	r := httptest.NewRequest(http.MethodGet, "/stream/", nil)
	w := httptest.NewRecorder()
	srv.handleStream(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
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
	srv, pdp := makeServer(t, nil)
	defer pdp.Close()
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
