package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// mockPDP serves stub AuthzForce PDP responses.
func mockPDP(allowedSubjects map[string]bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		reqXML := string(buf[:n])
		w.Header().Set("Content-Type", "application/xml")
		for s := range allowedSubjects {
			if strings.Contains(reqXML, ">"+s+"<") {
				w.Write([]byte(permitXML()))
				return
			}
		}
		w.Write([]byte(denyXML()))
	}))
}

func permitXML() string {
	return `<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"><Result><Decision>Permit</Decision></Result></Response>`
}
func denyXML() string {
	return `<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"><Result><Decision>Deny</Decision></Result></Response>`
}

// mockPIP creates a stub PIP server.
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

func testServer(t *testing.T, allowedSubjects map[string]bool, pipAttrs map[string]subjectAttrs) (*authServer, *httptest.Server, *httptest.Server) {
	t.Helper()
	pdp := mockPDP(allowedSubjects)
	pip := mockPIP(pipAttrs)
	cfg := config{
		rmqAdminUser:     "admin",
		rmqAdminPass:     "admin-secret",
		publisherUser:    "robot-fleet",
		publisherPass:    "fleet-secret",
		consumerPassword: "consumer-secret",
		azDomainID:       "test-domain",
		azURL:            pdp.URL,
		pipURL:           pip.URL,
	}
	return newAuthServer(cfg, newDecisionCache(0)), pdp, pip
}

func postForm(handler http.HandlerFunc, values url.Values) *httptest.ResponseRecorder {
	body := strings.NewReader(values.Encode())
	r := httptest.NewRequest(http.MethodPost, "/", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}

func bodyStr(w *httptest.ResponseRecorder) string {
	return strings.TrimSpace(w.Body.String())
}

// ── /auth/user ───────────────────────────────────────────────────────────────

func TestHandleUser_adminCorrectPassword(t *testing.T) {
	srv, pdp, pip := testServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleUser, url.Values{"username": {"admin"}, "password": {"admin-secret"}})
	if bodyStr(w) != "allow administrator management" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

func TestHandleUser_adminWrongPassword(t *testing.T) {
	srv, pdp, pip := testServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleUser, url.Values{"username": {"admin"}, "password": {"wrong"}})
	if bodyStr(w) != "deny" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

func TestHandleUser_publisherCorrect(t *testing.T) {
	srv, pdp, pip := testServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleUser, url.Values{"username": {"robot-fleet"}, "password": {"fleet-secret"}})
	if bodyStr(w) != "allow" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

func TestHandleUser_consumerWithGrant(t *testing.T) {
	pipAttrs := map[string]subjectAttrs{"consumer-1": {CertLevel: "sy", CertValid: true}}
	srv, pdp, pip := testServer(t, map[string]bool{"consumer-1": true}, pipAttrs)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleUser, url.Values{"username": {"consumer-1"}, "password": {"consumer-secret"}})
	if bodyStr(w) != "allow" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

func TestHandleUser_consumerNoGrant_denied(t *testing.T) {
	srv, pdp, pip := testServer(t, map[string]bool{}, nil)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleUser, url.Values{"username": {"consumer-X"}, "password": {"consumer-secret"}})
	if bodyStr(w) != "deny" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

func TestHandleUser_consumerWrongPassword(t *testing.T) {
	pipAttrs := map[string]subjectAttrs{"consumer-1": {CertLevel: "sy", CertValid: true}}
	srv, pdp, pip := testServer(t, map[string]bool{"consumer-1": true}, pipAttrs)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleUser, url.Values{"username": {"consumer-1"}, "password": {"wrong"}})
	if bodyStr(w) != "deny" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

// ── /auth/vhost ──────────────────────────────────────────────────────────────

func TestHandleVhost_adminAlwaysAllowed(t *testing.T) {
	srv, pdp, pip := testServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleVhost, url.Values{"username": {"admin"}, "vhost": {"/"}})
	if bodyStr(w) != "allow" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

func TestHandleVhost_consumerWithGrant(t *testing.T) {
	pipAttrs := map[string]subjectAttrs{"consumer-1": {CertLevel: "sy", CertValid: true}}
	srv, pdp, pip := testServer(t, map[string]bool{"consumer-1": true}, pipAttrs)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleVhost, url.Values{"username": {"consumer-1"}, "vhost": {"/"}})
	if bodyStr(w) != "allow" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

func TestHandleVhost_consumerRevoked(t *testing.T) {
	srv, pdp, pip := testServer(t, map[string]bool{}, nil)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleVhost, url.Values{"username": {"consumer-1"}, "vhost": {"/"}})
	if bodyStr(w) != "deny" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

// ── /auth/resource ───────────────────────────────────────────────────────────

func TestHandleResource_alwaysAllow(t *testing.T) {
	srv, pdp, pip := testServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleResource, url.Values{"username": {"any"}, "resource": {"exchange"}})
	if bodyStr(w) != "allow" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

// ── /auth/topic ──────────────────────────────────────────────────────────────

func TestHandleTopic_publisherWrite(t *testing.T) {
	srv, pdp, pip := testServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleTopic, url.Values{
		"username": {"robot-fleet"}, "permission": {"write"}, "routing_key": {"telemetry.robot-1"},
	})
	if bodyStr(w) != "allow" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

func TestHandleTopic_publisherReadDenied(t *testing.T) {
	srv, pdp, pip := testServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleTopic, url.Values{
		"username": {"robot-fleet"}, "permission": {"read"}, "routing_key": {"telemetry.#"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

func TestHandleTopic_consumerReadWithGrant(t *testing.T) {
	pipAttrs := map[string]subjectAttrs{"consumer-1": {CertLevel: "sy", CertValid: true}}
	srv, pdp, pip := testServer(t, map[string]bool{"consumer-1": true}, pipAttrs)
	defer pdp.Close()
	defer pip.Close()
	for _, rk := range []string{"telemetry.robot-1", "telemetry.*", "telemetry.#"} {
		w := postForm(srv.handleTopic, url.Values{
			"username": {"consumer-1"}, "permission": {"read"}, "routing_key": {rk},
		})
		if bodyStr(w) != "allow" {
			t.Fatalf("routing_key=%q: got %q", rk, bodyStr(w))
		}
	}
}

func TestHandleTopic_consumerRevoked(t *testing.T) {
	srv, pdp, pip := testServer(t, map[string]bool{}, nil)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleTopic, url.Values{
		"username": {"consumer-1"}, "permission": {"read"}, "routing_key": {"telemetry.#"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

func TestHandleTopic_consumerWriteDenied(t *testing.T) {
	pipAttrs := map[string]subjectAttrs{"consumer-1": {CertLevel: "sy", CertValid: true}}
	srv, pdp, pip := testServer(t, map[string]bool{"consumer-1": true}, pipAttrs)
	defer pdp.Close()
	defer pip.Close()
	w := postForm(srv.handleTopic, url.Values{
		"username": {"consumer-1"}, "permission": {"write"}, "routing_key": {"telemetry.robot-1"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("got %q", bodyStr(w))
	}
}

// ── TestHandleTopic_EnrichesXACMLWithCertLevel ───────────────────────────────
// Verifies that the PIP is queried before AuthzForce and cert-level attrs are
// present in the XACML request body sent to AuthzForce.

func TestHandleTopic_EnrichesXACMLWithCertLevel(t *testing.T) {
	var xacmlBody string
	pipQueried := false

	pdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		xacmlBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(denyXML()))
	}))
	defer pdp.Close()

	pip := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pipQueried = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"systemName": "consumer-1",
			"certLevel":  "sy",
			"valid":      true,
		})
	}))
	defer pip.Close()

	cfg := config{
		rmqAdminUser:     "admin",
		rmqAdminPass:     "admin-secret",
		publisherUser:    "robot-fleet",
		publisherPass:    "fleet-secret",
		consumerPassword: "consumer-secret",
		azDomainID:       "test-domain",
		azURL:            pdp.URL,
		pipURL:           pip.URL,
	}
	srv := newAuthServer(cfg, newDecisionCache(0))

	w := postForm(srv.handleTopic, url.Values{
		"username": {"consumer-1"}, "permission": {"read"}, "routing_key": {"telemetry.#"},
	})

	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny, got %q", bodyStr(w))
	}
	if !pipQueried {
		t.Error("expected PIP to be queried before AuthzForce, but it was not")
	}
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

// ── cache ────────────────────────────────────────────────────────────────────

func TestDecisionCache_hitWithinTTL(t *testing.T) {
	c := newDecisionCache(5 * time.Second)
	c.set("consumer-1", "telemetry", "subscribe", true)
	permit, ok := c.get("consumer-1", "telemetry", "subscribe")
	if !ok || !permit {
		t.Fatal("expected cache hit with Permit")
	}
}

func TestDecisionCache_zeroTTL_alwaysMiss(t *testing.T) {
	c := newDecisionCache(0)
	c.set("consumer-1", "telemetry", "subscribe", true)
	_, ok := c.get("consumer-1", "telemetry", "subscribe")
	if ok {
		t.Fatal("expected cache miss with TTL=0")
	}
}

// ── serviceFromRoutingKey ────────────────────────────────────────────────────

func TestServiceFromRoutingKey(t *testing.T) {
	cases := []struct{ key, want string }{
		{"telemetry.#", "telemetry"},
		{"telemetry.*", "telemetry"},
		{"telemetry.robot-001", "telemetry"},
		{"sensors.sensor-1", "sensors"},
		{"bare", "bare"},
	}
	for _, c := range cases {
		got := serviceFromRoutingKey(c.key)
		if got != c.want {
			t.Errorf("serviceFromRoutingKey(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}

// ── HTTP method guard ────────────────────────────────────────────────────────

func TestHandlers_rejectGet(t *testing.T) {
	srv, pdp, pip := testServer(t, nil, nil)
	defer pdp.Close()
	defer pip.Close()
	for _, h := range []http.HandlerFunc{srv.handleUser, srv.handleVhost, srv.handleTopic} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		srv.requirePOST(h)(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for GET, got %d", w.Code)
		}
	}
}
