package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockRMQ is a simple request recorder for the RabbitMQ Management API mock.
type mockRMQ struct {
	requests []string // "METHOD /path"
	users    []rmqUserResp
}

func (m *mockRMQ) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.requests = append(m.requests, r.Method+" "+r.URL.EscapedPath())

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/users":
		_ = json.NewEncoder(w).Encode(m.users)
	default:
		w.WriteHeader(http.StatusCreated)
	}
}

func (m *mockRMQ) saw(method, path string) bool {
	target := method + " " + path
	for _, req := range m.requests {
		if req == target {
			return true
		}
	}
	return false
}

// mockConsumerAuth serves the /authorization/lookup endpoint.
type mockConsumerAuth struct {
	rules      []AuthRule
	statusCode int
}

func (m *mockConsumerAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/authorization/lookup" {
		http.NotFound(w, r)
		return
	}
	if m.statusCode != 0 && m.statusCode != http.StatusOK {
		http.Error(w, "server error", m.statusCode)
		return
	}
	lr := LookupResponse{Rules: m.rules, Count: len(m.rules)}
	_ = json.NewEncoder(w).Encode(lr)
}

func newTestSyncer(caURL, rmqURL string) *syncer {
	cfg := config{
		consumerAuthURL:  caURL,
		rmqBase:          rmqURL,
		rmqAdminUser:     "admin",
		rmqAdminPass:     "secret",
		rmqVhost:         "/",
		rmqExchange:      "arrowhead",
		consumerPassword: "consumer-secret",
		publisherUser:    "robot-fleet",
		publisherPass:    "fleet-secret",
	}
	return newSyncer(cfg)
}

func TestSync_noRules(t *testing.T) {
	ca := &mockConsumerAuth{rules: nil}
	caSrv := httptest.NewServer(ca)
	defer caSrv.Close()

	rmq := &mockRMQ{}
	rmqSrv := httptest.NewServer(rmq)
	defer rmqSrv.Close()

	s := newTestSyncer(caSrv.URL, rmqSrv.URL)
	if err := s.sync(); err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Publisher must be created.
	if !rmq.saw(http.MethodPut, "/api/users/robot-fleet") {
		t.Error("expected PUT /api/users/robot-fleet")
	}
	// No consumer users should be created.
	for _, req := range rmq.requests {
		if strings.Contains(req, "consumer") {
			t.Errorf("unexpected consumer request: %s", req)
		}
	}
}

func TestSync_singleRule(t *testing.T) {
	ca := &mockConsumerAuth{rules: []AuthRule{
		{ConsumerSystemName: "demo-consumer-1", ProviderSystemName: "robot-fleet", ServiceDefinition: "telemetry"},
	}}
	caSrv := httptest.NewServer(ca)
	defer caSrv.Close()

	rmq := &mockRMQ{}
	rmqSrv := httptest.NewServer(rmq)
	defer rmqSrv.Close()

	s := newTestSyncer(caSrv.URL, rmqSrv.URL)
	if err := s.sync(); err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Consumer user must be created.
	if !rmq.saw(http.MethodPut, "/api/users/demo-consumer-1") {
		t.Error("expected PUT /api/users/demo-consumer-1")
	}
	// Regular permissions must be set.
	if !rmq.saw(http.MethodPut, "/api/permissions/%2F/demo-consumer-1") {
		t.Errorf("expected PUT /api/permissions/%%2F/demo-consumer-1; requests: %v", rmq.requests)
	}
	// Topic permissions must be set.
	if !rmq.saw(http.MethodPut, "/api/topic-permissions/%2F/demo-consumer-1") {
		t.Errorf("expected PUT /api/topic-permissions/%%2F/demo-consumer-1; requests: %v", rmq.requests)
	}
}

func TestSync_staleUser(t *testing.T) {
	// No rules → stale-consumer should be deleted.
	ca := &mockConsumerAuth{rules: nil}
	caSrv := httptest.NewServer(ca)
	defer caSrv.Close()

	rmq := &mockRMQ{
		users: []rmqUserResp{
			{Name: "stale-consumer", Tags: []string{managedTag}},
		},
	}
	rmqSrv := httptest.NewServer(rmq)
	defer rmqSrv.Close()

	s := newTestSyncer(caSrv.URL, rmqSrv.URL)
	if err := s.sync(); err != nil {
		t.Fatalf("sync error: %v", err)
	}

	if !rmq.saw(http.MethodDelete, "/api/users/stale-consumer") {
		t.Error("expected DELETE /api/users/stale-consumer")
	}
}

func TestSync_publisherNotDeleted(t *testing.T) {
	// No rules → publisher should be preserved even though it is managed and not a consumer.
	ca := &mockConsumerAuth{rules: nil}
	caSrv := httptest.NewServer(ca)
	defer caSrv.Close()

	rmq := &mockRMQ{
		users: []rmqUserResp{
			{Name: "robot-fleet", Tags: []string{managedTag}},
		},
	}
	rmqSrv := httptest.NewServer(rmq)
	defer rmqSrv.Close()

	s := newTestSyncer(caSrv.URL, rmqSrv.URL)
	if err := s.sync(); err != nil {
		t.Fatalf("sync error: %v", err)
	}

	if rmq.saw(http.MethodDelete, "/api/users/robot-fleet") {
		t.Error("publisher user robot-fleet must not be deleted")
	}
}

func TestSync_consumerAuthError(t *testing.T) {
	ca := &mockConsumerAuth{statusCode: http.StatusInternalServerError}
	caSrv := httptest.NewServer(ca)
	defer caSrv.Close()

	rmq := &mockRMQ{}
	rmqSrv := httptest.NewServer(rmq)
	defer rmqSrv.Close()

	s := newTestSyncer(caSrv.URL, rmqSrv.URL)
	err := s.sync()
	if err == nil {
		t.Fatal("expected sync to return an error when ConsumerAuth returns 500")
	}
}
