package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"arrowhead/authzforce"
)

// fakeAF is a minimal httptest server that accepts PUT policy and PUT pdp.properties.
func newFakeAF(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		// Accept any PUT — return 200 with a bare XML body.
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<ok/>`))
	}))
}

func TestPush_Permit(t *testing.T) {
	ts := newFakeAF(t)
	defer ts.Close()

	client := authzforce.New(ts.URL)
	pusher := &authzforcePusher{
		client:      client,
		domainID:    "d1",
		policySetID: "urn:test:pap",
	}

	policies := []*Policy{
		{ID: "pol-1", Subject: "sp1", Resource: "svc", Action: "consume", Effect: "Permit"},
		{ID: "pol-2", Subject: "sp2", Resource: "svc", Action: "consume", Effect: "Deny"},
	}
	// Only Permit policies become grants; Deny policies are skipped.
	if err := pusher.Push(policies, 1); err != nil {
		t.Fatalf("Push: unexpected error: %v", err)
	}
}

func TestPush_EmptyPolicies(t *testing.T) {
	ts := newFakeAF(t)
	defer ts.Close()

	client := authzforce.New(ts.URL)
	pusher := &authzforcePusher{
		client:      client,
		domainID:    "d1",
		policySetID: "urn:test:pap",
	}

	if err := pusher.Push(nil, 0); err != nil {
		t.Fatalf("Push empty: unexpected error: %v", err)
	}
}

func TestPush_AuthzForceError(t *testing.T) {
	// Server returns 500 — Push should propagate the error.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer ts.Close()

	client := authzforce.New(ts.URL)
	pusher := &authzforcePusher{
		client:      client,
		domainID:    "d1",
		policySetID: "urn:test:pap",
	}

	err := pusher.Push([]*Policy{{Subject: "sp1", Resource: "svc", Effect: "Permit"}}, 1)
	if err == nil {
		t.Error("Push with 500 response: expected error, got nil")
	}
}

// ── additional server method-not-allowed coverage ─────────────────────────────

func TestHealth_MethodNotAllowed(t *testing.T) {
	srv, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/health", strings.NewReader("")))
	if w.Code != http.StatusNotFound {
		t.Errorf("health POST: code = %d, want 404", w.Code)
	}
}

func TestStatus_MethodNotAllowed(t *testing.T) {
	srv, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/status", strings.NewReader("")))
	if w.Code != http.StatusNotFound {
		t.Errorf("status POST: code = %d, want 404", w.Code)
	}
}

func TestPoliciesByID_MethodNotAllowed(t *testing.T) {
	srv, store, _ := newTestServer()
	p, _ := store.Add("sp1", "svc", "", "consume", "Permit")
	req := httptest.NewRequest(http.MethodPut, "/policies/"+p.ID, strings.NewReader(""))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("policies PUT: code = %d, want 404", w.Code)
	}
}

func TestPolicies_MethodNotAllowed(t *testing.T) {
	srv, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/policies", strings.NewReader("")))
	if w.Code != http.StatusNotFound {
		t.Errorf("policies PUT: code = %d, want 404", w.Code)
	}
}

func TestPoliciesByID_EmptyID(t *testing.T) {
	srv, _, _ := newTestServer()
	// "/policies/" with no trailing ID
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/policies/", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("policies empty id: code = %d, want 404", w.Code)
	}
}
