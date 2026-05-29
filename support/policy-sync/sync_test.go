package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	az "arrowhead/authzforce"
)

// mockAuthzForce captures policy-set uploads and serves stub PDP responses.
type mockAuthzForce struct {
	domainCreated bool
	policyUploads []string
	rootPolicySet []string
}

func (m *mockAuthzForce) handler() http.Handler {
	mux := http.NewServeMux()

	// Domain list / create.
	mux.HandleFunc("/domains", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			if m.domainCreated {
				w.Write([]byte(`<resources><link href="/domains/test-domain-id"/></resources>`))
			} else {
				w.Write([]byte(`<resources/>`))
			}
		case http.MethodPost:
			m.domainCreated = true
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`<link href="/domains/test-domain-id"/>`))
		}
	})

	// Policy upload.
	mux.HandleFunc("/domains/test-domain-id/pap/policies", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body := make([]byte, 64*1024)
			n, _ := r.Body.Read(body)
			m.policyUploads = append(m.policyUploads, string(body[:n]))
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<link href="/domains/test-domain-id/pap/policies/policy/1"/>`))
		}
	})

	// Root policy update.
	mux.HandleFunc("/domains/test-domain-id/pap/pdp.properties", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body := make([]byte, 1024)
			n, _ := r.Body.Read(body)
			m.rootPolicySet = append(m.rootPolicySet, string(body[:n]))
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<pdpProperties/>`))
		}
	})

	return mux
}

// mockCA serves authorization policies in the new provider-centric WHITELIST format.
// It handles POST /consumerauthorization/authorization/mgmt/query (no filter required).
func mockCA(policies []AuthPolicy) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		lr := LookupResponse{Policies: policies, Count: len(policies), TotalCount: len(policies)}
		json.NewEncoder(w).Encode(lr)
	}))
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestSync_noRules_emptyPolicy(t *testing.T) {
	m := &mockAuthzForce{}
	azSrv := httptest.NewServer(m.handler())
	defer azSrv.Close()

	ca := mockCA(nil)
	defer ca.Close()

	s := newSyncer(az.New(azSrv.URL), ca.URL, http.DefaultClient)
	if err := s.init("test-ext-id"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if len(m.policyUploads) == 0 {
		t.Fatal("expected at least one policy upload")
	}
	// Policy should not contain any Policy elements (no grants).
	if strings.Contains(m.policyUploads[0], "<Policy ") {
		t.Fatal("expected no Policy elements for empty grant set")
	}
}

func TestSync_whitelistExpanded(t *testing.T) {
	// One WHITELIST policy with three consumers — should expand to three grants.
	policies := []AuthPolicy{
		{
			InstanceID: "PR|LOCAL|robot-fleet-site-1|SERVICE_DEF|telemetry",
			Provider:   "robot-fleet-site-1",
			TargetType: "SERVICE_DEF",
			Target:     "telemetry",
			DefaultPolicy: PolicyDef{
				PolicyType: "WHITELIST",
				PolicyList: []string{"consumer-1", "consumer-2", "consumer-3"},
			},
		},
	}

	m := &mockAuthzForce{}
	azSrv := httptest.NewServer(m.handler())
	defer azSrv.Close()

	ca := mockCA(policies)
	defer ca.Close()

	s := newSyncer(az.New(azSrv.URL), ca.URL, http.DefaultClient)
	if err := s.init("test-ext-id"); err != nil {
		t.Fatalf("init: %v", err)
	}

	policy := m.policyUploads[0]
	for _, name := range []string{"consumer-1", "consumer-2", "consumer-3"} {
		if !strings.Contains(policy, name) {
			t.Fatalf("policy missing grant for %s", name)
		}
	}
}

func TestSync_multipleProviders(t *testing.T) {
	// Two WHITELIST policies for different providers — all consumers must appear.
	policies := []AuthPolicy{
		{
			InstanceID: "PR|LOCAL|provider-a|SERVICE_DEF|svc-x",
			Provider:   "provider-a",
			TargetType: "SERVICE_DEF",
			Target:     "svc-x",
			DefaultPolicy: PolicyDef{
				PolicyType: "WHITELIST",
				PolicyList: []string{"consumer-1", "consumer-2"},
			},
		},
		{
			InstanceID: "PR|LOCAL|provider-b|SERVICE_DEF|svc-y",
			Provider:   "provider-b",
			TargetType: "SERVICE_DEF",
			Target:     "svc-y",
			DefaultPolicy: PolicyDef{
				PolicyType: "WHITELIST",
				PolicyList: []string{"consumer-3"},
			},
		},
	}

	m := &mockAuthzForce{}
	azSrv := httptest.NewServer(m.handler())
	defer azSrv.Close()

	ca := mockCA(policies)
	defer ca.Close()

	s := newSyncer(az.New(azSrv.URL), ca.URL, http.DefaultClient)
	if err := s.init("test-ext-id"); err != nil {
		t.Fatalf("init: %v", err)
	}

	policy := m.policyUploads[0]
	for _, name := range []string{"consumer-1", "consumer-2", "consumer-3"} {
		if !strings.Contains(policy, name) {
			t.Fatalf("policy missing grant for %s", name)
		}
	}
}

func TestSync_versionIncrements(t *testing.T) {
	m := &mockAuthzForce{}
	azSrv := httptest.NewServer(m.handler())
	defer azSrv.Close()

	ca := mockCA(nil)
	defer ca.Close()

	s := newSyncer(az.New(azSrv.URL), ca.URL, http.DefaultClient)
	if err := s.init("test-ext-id"); err != nil {
		t.Fatalf("init: %v", err)
	}
	ver1 := s.version

	if err := s.sync(); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if s.version != ver1+1 {
		t.Fatalf("expected version %d, got %d", ver1+1, s.version)
	}
}

func TestSync_rootPolicyRefUpdated(t *testing.T) {
	m := &mockAuthzForce{}
	azSrv := httptest.NewServer(m.handler())
	defer azSrv.Close()

	ca := mockCA(nil)
	defer ca.Close()

	s := newSyncer(az.New(azSrv.URL), ca.URL, http.DefaultClient)
	if err := s.init("test-ext-id"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if len(m.rootPolicySet) == 0 {
		t.Fatal("expected root policy to be set after init")
	}
	if !strings.Contains(m.rootPolicySet[0], policySetID) {
		t.Fatalf("root policy ref missing policy set ID %q", policySetID)
	}
}

func TestSync_methodIsPost(t *testing.T) {
	// Verify that fetchPolicies uses POST (not GET).
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		lr := LookupResponse{}
		json.NewEncoder(w).Encode(lr)
	}))
	defer srv.Close()

	s := &syncer{caURL: srv.URL, httpClient: http.DefaultClient}
	s.fetchPolicies() //nolint:errcheck
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
}
