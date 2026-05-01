package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func makeClient(t *testing.T, handler http.Handler) (*rmqClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return newRMQClient(srv.URL, "admin", "secret", "/"), srv
}

func TestEnsureUser(t *testing.T) {
	var recorded struct {
		method string
		path   string
		body   rmqUserBody
		auth   string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.method = r.Method
		recorded.path = r.URL.Path
		recorded.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&recorded.body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := newRMQClient(srv.URL, "admin", "secret", "/")
	if err := c.ensureUser("test-user", "test-pass"); err != nil {
		t.Fatalf("ensureUser: %v", err)
	}

	if recorded.method != http.MethodPut {
		t.Errorf("method: got %q, want PUT", recorded.method)
	}
	if recorded.path != "/api/users/test-user" {
		t.Errorf("path: got %q, want /api/users/test-user", recorded.path)
	}
	if recorded.body.Password != "test-pass" {
		t.Errorf("password: got %q, want test-pass", recorded.body.Password)
	}
	if recorded.body.Tags != managedTag {
		t.Errorf("tags: got %q, want %q", recorded.body.Tags, managedTag)
	}
	if recorded.auth == "" {
		t.Error("no Authorization header sent")
	}
}

func TestListManagedUsers_filtersTag(t *testing.T) {
	users := []rmqUserResp{
		{Name: "guest", Tags: "administrator"},
		{Name: "managed-user", Tags: managedTag},
		{Name: "other-managed", Tags: "other, " + managedTag},
		{Name: "unmanaged", Tags: ""},
	}

	c, _ := makeClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(users)
	}))

	got, err := c.listManagedUsers()
	if err != nil {
		t.Fatalf("listManagedUsers: %v", err)
	}

	wantSet := map[string]bool{"managed-user": true, "other-managed": true}
	if len(got) != len(wantSet) {
		t.Fatalf("got %d managed users, want %d: %v", len(got), len(wantSet), got)
	}
	for _, name := range got {
		if !wantSet[name] {
			t.Errorf("unexpected managed user %q", name)
		}
	}
}

func TestSetTopicPermission(t *testing.T) {
	var recorded struct {
		method string
		path   string
		body   rmqTopicPermission
	}

	c, _ := makeClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.method = r.Method
		recorded.path = r.URL.EscapedPath()
		_ = json.NewDecoder(r.Body).Decode(&recorded.body)
		w.WriteHeader(http.StatusCreated)
	}))

	tp := rmqTopicPermission{Exchange: "arrowhead", Write: "", Read: "^telemetry\\."}
	if err := c.setTopicPermission("consumer-1", tp); err != nil {
		t.Fatalf("setTopicPermission: %v", err)
	}

	if recorded.method != http.MethodPut {
		t.Errorf("method: got %q, want PUT", recorded.method)
	}
	// vhost "/" is encoded as "%2F"
	wantPath := "/api/topic-permissions/%2F/consumer-1"
	if recorded.path != wantPath {
		t.Errorf("path: got %q, want %q", recorded.path, wantPath)
	}
	if recorded.body.Exchange != "arrowhead" {
		t.Errorf("exchange: got %q", recorded.body.Exchange)
	}
	if recorded.body.Read != "^telemetry\\." {
		t.Errorf("read: got %q", recorded.body.Read)
	}
}

func TestSetPermissions(t *testing.T) {
	var recorded struct {
		method string
		path   string
		body   rmqPermission
	}

	c, _ := makeClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.method = r.Method
		recorded.path = r.URL.EscapedPath()
		_ = json.NewDecoder(r.Body).Decode(&recorded.body)
		w.WriteHeader(http.StatusCreated)
	}))

	perm := rmqPermission{Configure: "", Write: "", Read: ".*"}
	if err := c.setPermissions("consumer-1", perm); err != nil {
		t.Fatalf("setPermissions: %v", err)
	}

	wantPath := "/api/permissions/%2F/consumer-1"
	if recorded.path != wantPath {
		t.Errorf("path: got %q, want %q", recorded.path, wantPath)
	}
	if recorded.body.Read != ".*" {
		t.Errorf("read: got %q, want .*", recorded.body.Read)
	}
}

func TestDeleteUser(t *testing.T) {
	var recorded struct {
		method string
		path   string
	}

	c, _ := makeClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.method = r.Method
		recorded.path = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := c.deleteUser("stale-user"); err != nil {
		t.Fatalf("deleteUser: %v", err)
	}

	if recorded.method != http.MethodDelete {
		t.Errorf("method: got %q, want DELETE", recorded.method)
	}
	if recorded.path != "/api/users/stale-user" {
		t.Errorf("path: got %q, want /api/users/stale-user", recorded.path)
	}
}

func TestDeleteTopicPermissions(t *testing.T) {
	var recorded struct {
		method string
		path   string
	}

	c, _ := makeClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.method = r.Method
		recorded.path = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := c.deleteTopicPermissions("stale-user"); err != nil {
		t.Fatalf("deleteTopicPermissions: %v", err)
	}

	if recorded.method != http.MethodDelete {
		t.Errorf("method: got %q, want DELETE", recorded.method)
	}
	wantPath := "/api/topic-permissions/%2F/stale-user"
	if recorded.path != wantPath {
		t.Errorf("path: got %q, want %q", recorded.path, wantPath)
	}
}

func TestContainsTag(t *testing.T) {
	cases := []struct {
		tags   string
		target string
		want   bool
	}{
		{"", managedTag, false},
		{managedTag, managedTag, true},
		{"administrator," + managedTag, managedTag, true},
		{"administrator, " + managedTag, managedTag, true},
		{"administrator", managedTag, false},
		{"arrowhead-managedextra", managedTag, false},
	}

	for _, tc := range cases {
		got := containsTag(tc.tags, tc.target)
		if got != tc.want {
			t.Errorf("containsTag(%q, %q) = %v, want %v", tc.tags, tc.target, got, tc.want)
		}
	}
}

