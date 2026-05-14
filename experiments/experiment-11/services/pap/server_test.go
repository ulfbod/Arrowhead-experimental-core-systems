package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── test doubles ─────────────────────────────────────────────────────────────

type noopPusher struct {
	pushCount   int
	lastGrants  []ExternalGrant
	failNext    bool
}

func (n *noopPusher) Push(policies []*Policy, pipGrants []ExternalGrant, version int) error {
	n.pushCount++
	n.lastGrants = pipGrants
	if n.failNext {
		n.failNext = false
		return errPushFailed
	}
	return nil
}

var errPushFailed = &pushErr{"simulated push failure"}

type pushErr struct{ msg string }

func (e *pushErr) Error() string { return e.msg }

// staticGrantFetcher returns a fixed list of grants.
type staticGrantFetcher struct {
	grants  []ExternalGrant
	version int
	err     error
}

func (f *staticGrantFetcher) FetchGrants() ([]ExternalGrant, int, error) {
	return f.grants, f.version, f.err
}

func newTestServer() (http.Handler, *PolicyStore, *noopPusher, *staticGrantFetcher) {
	store := NewPolicyStore()
	pusher := &noopPusher{}
	fetcher := &staticGrantFetcher{version: 1}
	srv := NewServer(store, pusher, fetcher, "arrowhead-exp11")
	return srv, store, pusher, fetcher
}

// ── /health ───────────────────────────────────────────────────────────────────

func TestHealth_OK(t *testing.T) {
	srv, _, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Errorf("health: code = %d, want 200", w.Code)
	}
}

// ── /status ───────────────────────────────────────────────────────────────────

func TestStatus_Fields(t *testing.T) {
	srv, store, _, _ := newTestServer()
	store.Add("sp1", "svc", "consume", "Permit")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/status", nil))
	if w.Code != http.StatusOK {
		t.Errorf("status: code = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	for _, f := range []string{"policies", "version", "domainExternalId", "pipGrants"} {
		if _, ok := body[f]; !ok {
			t.Errorf("status: missing field %q", f)
		}
	}
	if body["domainExternalId"] != "arrowhead-exp11" {
		t.Errorf("status domainExternalId = %v, want arrowhead-exp11", body["domainExternalId"])
	}
}

// ── POST /policies ────────────────────────────────────────────────────────────

func TestCreatePolicy_201_PushReceivesPIPGrants(t *testing.T) {
	srv, _, pusher, fetcher := newTestServer()
	fetcher.grants = []ExternalGrant{
		{Subject: "portal-cloud-ml", Resource: "telemetry"},
	}
	body := `{"subject":"sp1","resource":"telemetry-rest","action":"consume"}`
	req := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("create: code = %d, want 201", w.Code)
	}
	if pusher.pushCount != 1 {
		t.Errorf("create: pushCount = %d, want 1", pusher.pushCount)
	}
	// Pusher must have received the PIP grant.
	if len(pusher.lastGrants) != 1 {
		t.Errorf("create: lastGrants len = %d, want 1 (from PIP)", len(pusher.lastGrants))
	}
}

func TestCreatePolicy_400_EmptySubject(t *testing.T) {
	srv, _, _, _ := newTestServer()
	body := `{"subject":"","resource":"svc","action":"consume"}`
	req := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("create empty subject: code = %d, want 400", w.Code)
	}
}

func TestCreatePolicy_400_BadJSON(t *testing.T) {
	srv, _, _, _ := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewBufferString(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad JSON: code = %d, want 400", w.Code)
	}
}

func TestCreatePolicy_DefaultsEffect(t *testing.T) {
	srv, _, _, _ := newTestServer()
	body := `{"subject":"sp1","resource":"svc","action":"consume"}`
	req := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var p Policy
	json.NewDecoder(w.Body).Decode(&p)
	if p.Effect != "Permit" {
		t.Errorf("default effect = %q, want Permit", p.Effect)
	}
}

// ── GET /policies ─────────────────────────────────────────────────────────────

func TestListPolicies_Empty(t *testing.T) {
	srv, _, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/policies", nil))
	if w.Code != http.StatusOK {
		t.Errorf("list: code = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["count"].(float64) != 0 {
		t.Errorf("list empty: count = %v, want 0", body["count"])
	}
}

func TestListPolicies_WithEntries(t *testing.T) {
	srv, store, _, _ := newTestServer()
	store.Add("sp1", "svc", "consume", "Permit")
	store.Add("sp2", "svc", "consume", "Permit")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/policies", nil))
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["count"].(float64) != 2 {
		t.Errorf("list: count = %v, want 2", body["count"])
	}
}

// ── DELETE /policies/{id} ─────────────────────────────────────────────────────

func TestDeletePolicy_204_TriggersPush(t *testing.T) {
	srv, store, pusher, _ := newTestServer()
	p, _ := store.Add("sp1", "svc", "consume", "Permit")
	pusher.pushCount = 0
	req := httptest.NewRequest(http.MethodDelete, "/policies/"+p.ID, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("delete: code = %d, want 204", w.Code)
	}
	if pusher.pushCount != 1 {
		t.Errorf("delete: pushCount = %d, want 1", pusher.pushCount)
	}
}

func TestDeletePolicy_404(t *testing.T) {
	srv, _, _, _ := newTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/policies/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("delete nonexistent: code = %d, want 404", w.Code)
	}
}

// ── GET /policies/{id} ────────────────────────────────────────────────────────

func TestGetPolicy_200(t *testing.T) {
	srv, store, _, _ := newTestServer()
	p, _ := store.Add("sp1", "svc", "consume", "Permit")
	req := httptest.NewRequest(http.MethodGet, "/policies/"+p.ID, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("get policy: code = %d, want 200", w.Code)
	}
}

func TestGetPolicy_404(t *testing.T) {
	srv, _, _, _ := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/policies/missing", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("get missing: code = %d, want 404", w.Code)
	}
}

// ── unknown / method-not-allowed ──────────────────────────────────────────────

func TestUnknownRoute_404(t *testing.T) {
	srv, _, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown: code = %d, want 404", w.Code)
	}
}

func TestHealth_MethodNotAllowed(t *testing.T) {
	srv, _, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/health", strings.NewReader("")))
	if w.Code != http.StatusNotFound {
		t.Errorf("health POST: code = %d, want 404", w.Code)
	}
}

func TestPoliciesByID_MethodNotAllowed(t *testing.T) {
	srv, store, _, _ := newTestServer()
	p, _ := store.Add("sp1", "svc", "consume", "Permit")
	req := httptest.NewRequest(http.MethodPut, "/policies/"+p.ID, strings.NewReader(""))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("policies PUT: code = %d, want 404", w.Code)
	}
}

func TestPoliciesByID_EmptyID(t *testing.T) {
	srv, _, _, _ := newTestServer()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/policies/", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("empty id: code = %d, want 404", w.Code)
	}
}

// ── SyncFromPIP ───────────────────────────────────────────────────────────────

func TestSyncFromPIP_CallsPushWhenVersionChanges(t *testing.T) {
	store := NewPolicyStore()
	pusher := &noopPusher{}
	fetcher := &staticGrantFetcher{version: 1,
		grants: []ExternalGrant{{Subject: "sp1", Resource: "svc"}},
	}
	srv := newServerImpl(store, pusher, fetcher, "arrowhead-exp11")

	if err := srv.SyncFromPIP(); err != nil {
		t.Fatalf("SyncFromPIP: %v", err)
	}
	if pusher.pushCount != 1 {
		t.Errorf("SyncFromPIP: pushCount = %d, want 1", pusher.pushCount)
	}
}

func TestSyncFromPIP_NoPushIfVersionUnchanged(t *testing.T) {
	store := NewPolicyStore()
	pusher := &noopPusher{}
	fetcher := &staticGrantFetcher{version: 1}
	srv := newServerImpl(store, pusher, fetcher, "arrowhead-exp11")

	srv.SyncFromPIP() // first call: pushes (new version)
	pusher.pushCount = 0
	srv.SyncFromPIP() // second call with same version: no push
	if pusher.pushCount != 0 {
		t.Errorf("SyncFromPIP same version: pushCount = %d, want 0", pusher.pushCount)
	}
}

func TestSyncFromPIP_PushesWhenVersionChanges(t *testing.T) {
	store := NewPolicyStore()
	pusher := &noopPusher{}
	fetcher := &staticGrantFetcher{version: 1}
	srv := newServerImpl(store, pusher, fetcher, "arrowhead-exp11")

	srv.SyncFromPIP()
	pusher.pushCount = 0
	fetcher.version = 2 // PIP version changed
	fetcher.grants = []ExternalGrant{{Subject: "sp2", Resource: "svc"}}

	if err := srv.SyncFromPIP(); err != nil {
		t.Fatalf("SyncFromPIP v2: %v", err)
	}
	if pusher.pushCount != 1 {
		t.Errorf("SyncFromPIP v2: pushCount = %d, want 1", pusher.pushCount)
	}
}

func TestSyncFromPIP_FetcherError_DegradeGracefully(t *testing.T) {
	store := NewPolicyStore()
	pusher := &noopPusher{}
	fetcher := &staticGrantFetcher{err: errPushFailed}
	srv := newServerImpl(store, pusher, fetcher, "arrowhead-exp11")

	// Fetch error should not cause SyncFromPIP to fail; it degrades gracefully.
	err := srv.SyncFromPIP()
	if err != nil {
		t.Errorf("SyncFromPIP fetch error: expected nil (degraded), got %v", err)
	}
}
