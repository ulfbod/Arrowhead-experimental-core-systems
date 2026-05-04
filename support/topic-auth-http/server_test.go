package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fetchRulesFunc returns a fetchRules closure backed by a fixed rule set.
func fetchRulesFunc(rules []AuthRule) func(context.Context) ([]AuthRule, error) {
	return func(_ context.Context) ([]AuthRule, error) {
		return rules, nil
	}
}

// fetchRulesError returns a fetchRules closure that always errors.
func fetchRulesError() func(context.Context) ([]AuthRule, error) {
	return func(_ context.Context) ([]AuthRule, error) {
		return nil, errors.New("CA unavailable")
	}
}

func testServer(rules []AuthRule) *authServer {
	return newAuthServer(config{
		rmqAdminUser:     "admin",
		rmqAdminPass:     "admin-secret",
		consumerPassword: "consumer-secret",
		publisherUser:    "robot-fleet",
		publisherPass:    "fleet-secret",
	}, fetchRulesFunc(rules))
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
	srv := testServer(nil)
	w := postForm(srv.handleUser, url.Values{
		"username": {"admin"},
		"password": {"admin-secret"},
	})
	got := bodyStr(w)
	if got != "allow administrator management" {
		t.Fatalf("expected management tags for admin, got %q", got)
	}
}

func TestHandleUser_adminWrongPassword(t *testing.T) {
	srv := testServer(nil)
	w := postForm(srv.handleUser, url.Values{
		"username": {"admin"},
		"password": {"wrong"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny for wrong admin password, got %q", bodyStr(w))
	}
}

func TestHandleUser_publisherCorrectPassword(t *testing.T) {
	srv := testServer(nil)
	w := postForm(srv.handleUser, url.Values{
		"username": {"robot-fleet"},
		"password": {"fleet-secret"},
	})
	if bodyStr(w) != "allow" {
		t.Fatalf("expected allow, got %q", bodyStr(w))
	}
}

func TestHandleUser_publisherWrongPassword(t *testing.T) {
	srv := testServer(nil)
	w := postForm(srv.handleUser, url.Values{
		"username": {"robot-fleet"},
		"password": {"wrong"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny, got %q", bodyStr(w))
	}
}

func TestHandleUser_consumerWithGrant(t *testing.T) {
	rules := []AuthRule{{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"}}
	srv := testServer(rules)
	w := postForm(srv.handleUser, url.Values{
		"username": {"consumer-1"},
		"password": {"consumer-secret"},
	})
	if bodyStr(w) != "allow" {
		t.Fatalf("expected allow, got %q", bodyStr(w))
	}
}

func TestHandleUser_consumerWithoutGrant(t *testing.T) {
	srv := testServer(nil) // no rules
	w := postForm(srv.handleUser, url.Values{
		"username": {"consumer-1"},
		"password": {"consumer-secret"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny, got %q", bodyStr(w))
	}
}

func TestHandleUser_consumerWrongPassword(t *testing.T) {
	rules := []AuthRule{{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"}}
	srv := testServer(rules)
	w := postForm(srv.handleUser, url.Values{
		"username": {"consumer-1"},
		"password": {"wrong"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny, got %q", bodyStr(w))
	}
}

func TestHandleUser_caError_denies(t *testing.T) {
	as := newAuthServer(config{
		rmqAdminUser:     "admin",
		rmqAdminPass:     "admin-secret",
		consumerPassword: "consumer-secret",
		publisherUser:    "robot-fleet",
		publisherPass:    "fleet-secret",
	}, fetchRulesError())
	w := postForm(as.handleUser, url.Values{
		"username": {"consumer-1"},
		"password": {"consumer-secret"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny on CA error, got %q", bodyStr(w))
	}
}

// ── /auth/vhost ──────────────────────────────────────────────────────────────

func TestHandleVhost_adminAlwaysAllowed(t *testing.T) {
	srv := testServer(nil)
	w := postForm(srv.handleVhost, url.Values{
		"username": {"admin"},
		"vhost":    {"/"},
	})
	if bodyStr(w) != "allow" {
		t.Fatalf("expected allow for admin, got %q", bodyStr(w))
	}
}

func TestHandleVhost_publisherAlwaysAllowed(t *testing.T) {
	srv := testServer(nil)
	w := postForm(srv.handleVhost, url.Values{
		"username": {"robot-fleet"},
		"vhost":    {"/"},
	})
	if bodyStr(w) != "allow" {
		t.Fatalf("expected allow for publisher, got %q", bodyStr(w))
	}
}

func TestHandleVhost_consumerWithGrant(t *testing.T) {
	rules := []AuthRule{{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"}}
	srv := testServer(rules)
	w := postForm(srv.handleVhost, url.Values{
		"username": {"consumer-1"},
		"vhost":    {"/"},
	})
	if bodyStr(w) != "allow" {
		t.Fatalf("expected allow, got %q", bodyStr(w))
	}
}

func TestHandleVhost_consumerRevokedGrant(t *testing.T) {
	// consumer-1 has no grant → simulates revocation
	srv := testServer(nil)
	w := postForm(srv.handleVhost, url.Values{
		"username": {"consumer-1"},
		"vhost":    {"/"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny for revoked consumer, got %q", bodyStr(w))
	}
}

func TestHandleVhost_caError_denies(t *testing.T) {
	as := newAuthServer(config{rmqAdminUser: "admin", rmqAdminPass: "admin-secret", publisherUser: "robot-fleet"}, fetchRulesError())
	w := postForm(as.handleVhost, url.Values{
		"username": {"consumer-1"},
		"vhost":    {"/"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny on CA error, got %q", bodyStr(w))
	}
}

// ── /auth/resource ───────────────────────────────────────────────────────────

func TestHandleResource_alwaysAllow(t *testing.T) {
	srv := testServer(nil)
	for _, perm := range []string{"configure", "write", "read"} {
		w := postForm(srv.handleResource, url.Values{
			"username":   {"any-user"},
			"vhost":      {"/"},
			"resource":   {"exchange"},
			"name":       {"arrowhead"},
			"permission": {perm},
		})
		if bodyStr(w) != "allow" {
			t.Fatalf("expected allow for resource perm=%q, got %q", perm, bodyStr(w))
		}
	}
}

// ── /auth/topic ──────────────────────────────────────────────────────────────

func TestHandleTopic_publisherWriteAllowed(t *testing.T) {
	srv := testServer(nil)
	w := postForm(srv.handleTopic, url.Values{
		"username":    {"robot-fleet"},
		"permission":  {"write"},
		"routing_key": {"telemetry.robot-001"},
	})
	if bodyStr(w) != "allow" {
		t.Fatalf("expected allow for publisher write, got %q", bodyStr(w))
	}
}

func TestHandleTopic_publisherReadDenied(t *testing.T) {
	// Publisher should not subscribe — only write is permitted.
	srv := testServer(nil)
	w := postForm(srv.handleTopic, url.Values{
		"username":    {"robot-fleet"},
		"permission":  {"read"},
		"routing_key": {"telemetry.#"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny for publisher read, got %q", bodyStr(w))
	}
}

func TestHandleTopic_adminAllowed(t *testing.T) {
	srv := testServer(nil)
	w := postForm(srv.handleTopic, url.Values{
		"username":    {"admin"},
		"permission":  {"write"},
		"routing_key": {"any.key"},
	})
	if bodyStr(w) != "allow" {
		t.Fatalf("expected allow for admin, got %q", bodyStr(w))
	}
}

func TestHandleTopic_consumerReadMatchingKey(t *testing.T) {
	rules := []AuthRule{{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"}}
	srv := testServer(rules)
	for _, rk := range []string{"telemetry.robot-001", "telemetry.*", "telemetry.#"} {
		w := postForm(srv.handleTopic, url.Values{
			"username":    {"consumer-1"},
			"permission":  {"read"},
			"routing_key": {rk},
		})
		if bodyStr(w) != "allow" {
			t.Fatalf("routing key %q should be allowed, got %q", rk, bodyStr(w))
		}
	}
}

func TestHandleTopic_consumerReadWrongService(t *testing.T) {
	rules := []AuthRule{{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"}}
	srv := testServer(rules)
	w := postForm(srv.handleTopic, url.Values{
		"username":    {"consumer-1"},
		"permission":  {"read"},
		"routing_key": {"sensors.sensor-1"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny for wrong service, got %q", bodyStr(w))
	}
}

func TestHandleTopic_consumerRevoked(t *testing.T) {
	// No rules → grant revoked; consumer should be denied on next topic operation.
	srv := testServer(nil)
	w := postForm(srv.handleTopic, url.Values{
		"username":    {"consumer-1"},
		"permission":  {"read"},
		"routing_key": {"telemetry.#"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny for revoked consumer, got %q", bodyStr(w))
	}
}

func TestHandleTopic_consumerWriteDenied(t *testing.T) {
	// Consumers may only read; write is reserved for the publisher.
	rules := []AuthRule{{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"}}
	srv := testServer(rules)
	w := postForm(srv.handleTopic, url.Values{
		"username":    {"consumer-1"},
		"permission":  {"write"},
		"routing_key": {"telemetry.robot-001"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny for consumer write, got %q", bodyStr(w))
	}
}

func TestHandleTopic_caError_denies(t *testing.T) {
	as := newAuthServer(config{rmqAdminUser: "admin", rmqAdminPass: "admin-secret", publisherUser: "robot-fleet"}, fetchRulesError())
	w := postForm(as.handleTopic, url.Values{
		"username":    {"consumer-1"},
		"permission":  {"read"},
		"routing_key": {"telemetry.#"},
	})
	if bodyStr(w) != "deny" {
		t.Fatalf("expected deny on CA error, got %q", bodyStr(w))
	}
}

// ── HTTP method guard ────────────────────────────────────────────────────────

func TestHandlers_rejectGet(t *testing.T) {
	srv := testServer(nil)
	for _, handler := range []http.HandlerFunc{srv.handleUser, srv.handleVhost, srv.handleTopic} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for GET, got %d", w.Code)
		}
	}
}
