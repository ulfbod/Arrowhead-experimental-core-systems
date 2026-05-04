package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockCA serves /authorization/lookup with a fixed rule set.
type mockCA struct {
	rules      []AuthRule
	statusCode int
}

func (m *mockCA) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/authorization/lookup", func(w http.ResponseWriter, r *http.Request) {
		if m.statusCode != 0 {
			w.WriteHeader(m.statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LookupResponse{Rules: m.rules, Count: len(m.rules)})
	})
	return mux
}

// mockRMQRecorder records all API requests and maintains a user list.
type mockRMQRecorder struct {
	puts    map[string][]byte
	deletes []string
	users   []rmqUserResp
}

func newMockRMQ(users []rmqUserResp) (*httptest.Server, *mockRMQRecorder) {
	rec := &mockRMQRecorder{
		puts:  make(map[string][]byte),
		users: users,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rec.users)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			rec.deletes = append(rec.deletes, r.URL.Path)
		} else if r.Method == http.MethodPut {
			// read body if present
			buf := make([]byte, 1024)
			n, _ := r.Body.Read(buf)
			rec.puts[r.URL.Path] = buf[:n]
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	return srv, rec
}

func (r *mockRMQRecorder) userPutPaths() []string {
	var out []string
	for p := range r.puts {
		out = append(out, p)
	}
	return out
}

func TestSync_noRules(t *testing.T) {
	ca := &mockCA{rules: nil}
	caSrv := httptest.NewServer(ca.handler())
	defer caSrv.Close()

	rmqSrv, rec := newMockRMQ(nil)
	defer rmqSrv.Close()

	cfg := config{
		consumerAuthURL: caSrv.URL,
		rmqExchange:     "arrowhead",
		publisherUser:   "robot-fleet",
		publisherPass:   "fleet-secret",
		consumerPassword: "secret",
	}
	s := newSyncer(cfg, newRMQClient(rmqSrv.URL, "admin", "admin", "/"))
	if err := s.sync(); err != nil {
		t.Fatal(err)
	}
	// Publisher should be created; no consumer users.
	found := false
	for p := range rec.puts {
		if p == "/api/users/robot-fleet" {
			found = true
		}
	}
	if !found {
		t.Fatalf("publisher user should be created; puts: %v", rec.userPutPaths())
	}
}

func TestSync_singleRule_createsConsumerUser(t *testing.T) {
	ca := &mockCA{rules: []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
	}}
	caSrv := httptest.NewServer(ca.handler())
	defer caSrv.Close()

	rmqSrv, rec := newMockRMQ(nil)
	defer rmqSrv.Close()

	cfg := config{
		consumerAuthURL:  caSrv.URL,
		rmqExchange:      "arrowhead",
		publisherUser:    "robot-fleet",
		publisherPass:    "fleet-secret",
		consumerPassword: "secret",
	}
	s := newSyncer(cfg, newRMQClient(rmqSrv.URL, "admin", "admin", "/"))
	if err := s.sync(); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec.puts["/api/users/consumer-1"]; !ok {
		t.Fatalf("consumer-1 user should be created; puts: %v", rec.userPutPaths())
	}
}

func TestSync_staleUser_deleted(t *testing.T) {
	ca := &mockCA{rules: nil}
	caSrv := httptest.NewServer(ca.handler())
	defer caSrv.Close()

	staleUsers := []rmqUserResp{
		{Name: "stale-consumer", Tags: []string{managedTag}},
	}
	rmqSrv, rec := newMockRMQ(staleUsers)
	defer rmqSrv.Close()

	cfg := config{
		consumerAuthURL:  caSrv.URL,
		rmqExchange:      "arrowhead",
		publisherUser:    "robot-fleet",
		publisherPass:    "fleet-secret",
		consumerPassword: "secret",
	}
	s := newSyncer(cfg, newRMQClient(rmqSrv.URL, "admin", "admin", "/"))
	if err := s.sync(); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range rec.deletes {
		if p == "/api/users/stale-consumer" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale-consumer should be deleted; deletes: %v", rec.deletes)
	}
}

func TestSync_publisherNotDeleted(t *testing.T) {
	ca := &mockCA{rules: nil}
	caSrv := httptest.NewServer(ca.handler())
	defer caSrv.Close()

	// Publisher is already in the managed user list.
	existingUsers := []rmqUserResp{
		{Name: "robot-fleet", Tags: []string{managedTag}},
	}
	rmqSrv, rec := newMockRMQ(existingUsers)
	defer rmqSrv.Close()

	cfg := config{
		consumerAuthURL:  caSrv.URL,
		rmqExchange:      "arrowhead",
		publisherUser:    "robot-fleet",
		publisherPass:    "fleet-secret",
		consumerPassword: "secret",
	}
	s := newSyncer(cfg, newRMQClient(rmqSrv.URL, "admin", "admin", "/"))
	if err := s.sync(); err != nil {
		t.Fatal(err)
	}
	for _, p := range rec.deletes {
		if p == "/api/users/robot-fleet" {
			t.Fatal("publisher user must never be deleted by sync")
		}
	}
}

func TestSync_caError_returnsError(t *testing.T) {
	ca := &mockCA{statusCode: http.StatusInternalServerError}
	caSrv := httptest.NewServer(ca.handler())
	defer caSrv.Close()

	rmqSrv, _ := newMockRMQ(nil)
	defer rmqSrv.Close()

	cfg := config{
		consumerAuthURL:  caSrv.URL,
		rmqExchange:      "arrowhead",
		publisherUser:    "robot-fleet",
		publisherPass:    "fleet-secret",
		consumerPassword: "secret",
	}
	s := newSyncer(cfg, newRMQClient(rmqSrv.URL, "admin", "admin", "/"))
	if err := s.sync(); err == nil {
		t.Fatal("expected error when CA returns 500")
	}
}

func TestEnforceRevocations_closesRevokedConsumer(t *testing.T) {
	// CA has a grant only for consumer-1; consumer-2 has been revoked.
	ca := &mockCA{rules: []AuthRule{
		{ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
	}}
	caSrv := httptest.NewServer(ca.handler())
	defer caSrv.Close()

	// Both consumer-1 and consumer-2 have active connections.
	type conn struct {
		Name string `json:"name"`
		User string `json:"user"`
	}
	connections := []conn{
		{Name: "conn-c1", User: "consumer-1"},
		{Name: "conn-c2", User: "consumer-2"},
	}
	var deletedConns []string
	rmqSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/connections" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(connections)
			return
		}
		if r.Method == http.MethodDelete {
			deletedConns = append(deletedConns, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer rmqSrv.Close()

	cfg := config{
		consumerAuthURL: caSrv.URL,
		rmqAdminUser:    "admin",
		publisherUser:   "robot-fleet",
	}
	s := newSyncer(cfg, newRMQClient(rmqSrv.URL, "admin", "admin", "/"))
	if err := s.enforceRevocations(); err != nil {
		t.Fatal(err)
	}
	// consumer-2 connection should be deleted; consumer-1 should not.
	if len(deletedConns) != 1 {
		t.Fatalf("expected 1 deletion, got %d: %v", len(deletedConns), deletedConns)
	}
	if !strings.Contains(deletedConns[0], "conn-c2") {
		t.Fatalf("expected conn-c2 to be deleted, got %v", deletedConns)
	}
}

func TestEnforceRevocations_skipsAdminAndPublisher(t *testing.T) {
	// No grants at all — but admin and publisher should never be disconnected.
	ca := &mockCA{rules: nil}
	caSrv := httptest.NewServer(ca.handler())
	defer caSrv.Close()

	type conn struct {
		Name string `json:"name"`
		User string `json:"user"`
	}
	connections := []conn{
		{Name: "conn-admin", User: "admin"},
		{Name: "conn-fleet", User: "robot-fleet"},
	}
	var deletedConns []string
	rmqSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/connections" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(connections)
			return
		}
		if r.Method == http.MethodDelete {
			deletedConns = append(deletedConns, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer rmqSrv.Close()

	cfg := config{
		consumerAuthURL: caSrv.URL,
		rmqAdminUser:    "admin",
		publisherUser:   "robot-fleet",
	}
	s := newSyncer(cfg, newRMQClient(rmqSrv.URL, "admin", "admin", "/"))
	if err := s.enforceRevocations(); err != nil {
		t.Fatal(err)
	}
	if len(deletedConns) != 0 {
		t.Fatalf("admin and publisher must not be disconnected, got deletes: %v", deletedConns)
	}
}

func TestEnforceRevocations_nilRMQ_noOp(t *testing.T) {
	ca := &mockCA{rules: nil}
	caSrv := httptest.NewServer(ca.handler())
	defer caSrv.Close()

	cfg := config{consumerAuthURL: caSrv.URL}
	s := newSyncer(cfg, nil)
	// Must not panic when rmq is nil.
	if err := s.enforceRevocations(); err != nil {
		t.Fatal(err)
	}
}
