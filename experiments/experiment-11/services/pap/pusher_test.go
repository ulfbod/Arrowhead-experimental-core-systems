package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"arrowhead/authzforce"
)

// fakeAF accepts PUT /domains/d1/pap/policies and PUT /domains/d1/pap/pdp.properties.
func newFakeAF(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<ok/>`))
	}))
}

func TestAuthzforcePusher_Push_PermitAndPIPGrants(t *testing.T) {
	ts := newFakeAF(t)
	defer ts.Close()

	pusher := &authzforcePusher{
		client:   authzforce.New(ts.URL),
		domainID: "d1",
	}
	native := []*Policy{
		{Subject: "sp1", Resource: "svc", Effect: "Permit"},
		{Subject: "sp2", Resource: "svc", Effect: "Deny"}, // Deny — excluded from grants
	}
	pip := []ExternalGrant{
		{Subject: "sp3", Resource: "svc"},
	}
	if err := pusher.Push(native, pip, 1); err != nil {
		t.Fatalf("Push: %v", err)
	}
}

func TestAuthzforcePusher_Push_DeduplicatesNativeAndPIP(t *testing.T) {
	ts := newFakeAF(t)
	defer ts.Close()

	pusher := &authzforcePusher{client: authzforce.New(ts.URL), domainID: "d1"}
	native := []*Policy{{Subject: "sp1", Resource: "svc", Effect: "Permit"}}
	pip := []ExternalGrant{{Subject: "sp1", Resource: "svc"}} // same — should not duplicate
	if err := pusher.Push(native, pip, 1); err != nil {
		t.Fatalf("Push dedup: %v", err)
	}
}

func TestAuthzforcePusher_Push_Empty(t *testing.T) {
	ts := newFakeAF(t)
	defer ts.Close()
	pusher := &authzforcePusher{client: authzforce.New(ts.URL), domainID: "d1"}
	if err := pusher.Push(nil, nil, 1); err != nil {
		t.Fatalf("Push empty: %v", err)
	}
}

func TestAuthzforcePusher_Push_AFError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	pusher := &authzforcePusher{client: authzforce.New(ts.URL), domainID: "d1"}
	err := pusher.Push([]*Policy{{Subject: "sp1", Resource: "svc", Effect: "Permit"}}, nil, 1)
	if err == nil {
		t.Error("Push 500: expected error")
	}
}

// ── pipGrantFetcher ───────────────────────────────────────────────────────────

func newFakePIP(grants []ExternalGrant, version int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pipGrantsResponse{
			Grants:  grants,
			Count:   len(grants),
			Version: version,
		})
	}))
}

func TestPIPGrantFetcher_FetchGrants(t *testing.T) {
	grants := []ExternalGrant{{Subject: "sp1", Resource: "svc"}}
	ts := newFakePIP(grants, 3)
	defer ts.Close()

	fetcher := &pipGrantFetcher{
		pipURL: ts.URL,
		client: &http.Client{},
	}
	got, ver, err := fetcher.FetchGrants()
	if err != nil {
		t.Fatalf("FetchGrants: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("FetchGrants: len = %d, want 1", len(got))
	}
	if ver != 3 {
		t.Errorf("FetchGrants: version = %d, want 3", ver)
	}
}

func TestPIPGrantFetcher_FetchGrants_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	fetcher := &pipGrantFetcher{pipURL: ts.URL, client: &http.Client{}}
	_, _, err := fetcher.FetchGrants()
	if err == nil {
		t.Error("FetchGrants 503: expected error")
	}
}

func TestPIPGrantFetcher_FetchGrants_BadJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer ts.Close()
	fetcher := &pipGrantFetcher{pipURL: ts.URL, client: &http.Client{}}
	_, _, err := fetcher.FetchGrants()
	if err == nil {
		t.Error("FetchGrants bad JSON: expected error")
	}
}

// ── additional server coverage ────────────────────────────────────────────────

func TestStatus_MethodNotAllowed(t *testing.T) {
	srv, _, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/status", strings.NewReader("")))
	if w.Code != http.StatusNotFound {
		t.Errorf("status POST: code = %d, want 404", w.Code)
	}
}

func TestPolicies_MethodNotAllowed(t *testing.T) {
	srv, _, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/policies", strings.NewReader("")))
	if w.Code != http.StatusNotFound {
		t.Errorf("policies PUT: code = %d, want 404", w.Code)
	}
}

func TestConfigFromEnv_InvalidInterval(t *testing.T) {
	t.Setenv("SYNC_INTERVAL", "bad")
	cfg := configFromEnv()
	if cfg.syncInterval == 0 {
		t.Error("configFromEnv: invalid interval should fall back to non-zero default")
	}
}
