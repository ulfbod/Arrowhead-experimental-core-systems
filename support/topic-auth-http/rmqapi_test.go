package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type rmqTestServer struct {
	requests []string
	users    []rmqUserResp
}

func (m *rmqTestServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		m.requests = append(m.requests, r.Method+" /api/users")
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(m.users)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		m.requests = append(m.requests, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func newRMQTestServer(t *testing.T, users []rmqUserResp) (*httptest.Server, *rmqTestServer) {
	t.Helper()
	m := &rmqTestServer{users: users}
	srv := httptest.NewServer(m.handler())
	t.Cleanup(srv.Close)
	return srv, m
}

func TestRMQClient_ensureUser(t *testing.T) {
	srv, m := newRMQTestServer(t, nil)
	c := newRMQClient(srv.URL, "admin", "admin", "/")
	if err := c.ensureUser("consumer-1", "secret"); err != nil {
		t.Fatal(err)
	}
	if len(m.requests) != 1 || !strings.HasPrefix(m.requests[0], "PUT /api/users/") {
		t.Fatalf("unexpected requests: %v", m.requests)
	}
}

func TestRMQClient_ensureUser_setsTag(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newRMQClient(srv.URL, "admin", "admin", "/")
	if err := c.ensureUser("consumer-1", "secret"); err != nil {
		t.Fatal(err)
	}
	var body rmqUserBody
	json.Unmarshal(captured, &body)
	if body.Tags != managedTag {
		t.Fatalf("expected tag %q, got %q", managedTag, body.Tags)
	}
	if body.Password != "secret" {
		t.Fatalf("unexpected password: %q", body.Password)
	}
}

func TestRMQClient_listManagedUsers_filtersTag(t *testing.T) {
	users := []rmqUserResp{
		{Name: "managed-user", Tags: []string{managedTag}},
		{Name: "admin", Tags: []string{"administrator"}},
		{Name: "untagged", Tags: []string{}},
	}
	srv, _ := newRMQTestServer(t, users)
	c := newRMQClient(srv.URL, "admin", "admin", "/")
	managed, err := c.listManagedUsers()
	if err != nil {
		t.Fatal(err)
	}
	if len(managed) != 1 || managed[0] != "managed-user" {
		t.Fatalf("unexpected managed users: %v", managed)
	}
}

func TestRMQClient_deleteUser(t *testing.T) {
	srv, m := newRMQTestServer(t, nil)
	c := newRMQClient(srv.URL, "admin", "admin", "/")
	if err := c.deleteUser("consumer-1"); err != nil {
		t.Fatal(err)
	}
	if len(m.requests) != 1 || !strings.HasPrefix(m.requests[0], "DELETE /api/users/") {
		t.Fatalf("unexpected requests: %v", m.requests)
	}
}

func TestRMQClient_setPermissions(t *testing.T) {
	srv, m := newRMQTestServer(t, nil)
	c := newRMQClient(srv.URL, "admin", "admin", "/")
	perm := rmqPermission{Configure: ".*", Write: ".*", Read: ".*"}
	if err := c.setPermissions("consumer-1", perm); err != nil {
		t.Fatal(err)
	}
	if len(m.requests) != 1 || !strings.Contains(m.requests[0], "/api/permissions/") {
		t.Fatalf("unexpected requests: %v", m.requests)
	}
}

func TestRMQClient_setTopicPermission_vhostEscaped(t *testing.T) {
	var capturedURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// RequestURI preserves the raw percent-encoding sent by the client.
		capturedURI = r.RequestURI
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newRMQClient(srv.URL, "admin", "admin", "/")
	tp := rmqTopicPermission{Exchange: "arrowhead", Write: "", Read: `^telemetry\.`}
	if err := c.setTopicPermission("consumer-1", tp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(capturedURI, "%2F") {
		t.Fatalf("vhost '/' should be URL-escaped to %%2F in path, got %q", capturedURI)
	}
	if !strings.Contains(capturedURI, "consumer-1") {
		t.Fatalf("path should contain username, got %q", capturedURI)
	}
}

func TestRMQClient_deleteTopicPermissions(t *testing.T) {
	srv, m := newRMQTestServer(t, nil)
	c := newRMQClient(srv.URL, "admin", "admin", "/")
	if err := c.deleteTopicPermissions("consumer-1"); err != nil {
		t.Fatal(err)
	}
	if len(m.requests) != 1 || !strings.Contains(m.requests[0], "DELETE") {
		t.Fatalf("unexpected requests: %v", m.requests)
	}
}

func TestContainsTag(t *testing.T) {
	if !containsTag([]string{"a", "b", managedTag}, managedTag) {
		t.Fatal("should find managed tag")
	}
	if containsTag([]string{"a", "b"}, managedTag) {
		t.Fatal("should not find managed tag")
	}
	if containsTag(nil, managedTag) {
		t.Fatal("should not find tag in nil slice")
	}
}

func TestRMQClient_listConnections(t *testing.T) {
	conns := []rmqConnection{
		{Name: "172.0.0.1:1234 -> 172.0.0.2:5672", User: "consumer-1"},
		{Name: "172.0.0.1:5678 -> 172.0.0.2:5672", User: "robot-fleet"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/connections" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(conns)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newRMQClient(srv.URL, "admin", "admin", "/")
	got, err := c.listConnections()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(got))
	}
	if got[0].User != "consumer-1" {
		t.Fatalf("expected user consumer-1, got %q", got[0].User)
	}
}

func TestRMQClient_deleteConnection(t *testing.T) {
	var deletedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletedPath = r.RequestURI
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newRMQClient(srv.URL, "admin", "admin", "/")
	connName := "172.0.0.1:1234 -> 172.0.0.2:5672"
	if err := c.deleteConnection(connName); err != nil {
		t.Fatal(err)
	}
	if deletedPath == "" || !strings.Contains(deletedPath, "/api/connections/") {
		t.Fatalf("expected DELETE /api/connections/..., got %q", deletedPath)
	}
	// Verify the name was URL-encoded (colons, spaces, arrow)
	if strings.Contains(deletedPath, " ") || strings.Contains(deletedPath, ">") {
		t.Fatalf("connection name should be URL-encoded, got %q", deletedPath)
	}
}
