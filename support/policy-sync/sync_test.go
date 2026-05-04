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

// mockCA serves a fixed set of authorization rules.
func mockCA(rules []AuthRule) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lr := LookupResponse{Rules: rules, Count: len(rules)}
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

	s := newSyncer(az.New(azSrv.URL), ca.URL)
	if err := s.init(); err != nil {
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

func TestSync_threeRules_threePolicies(t *testing.T) {
	rules := []AuthRule{
		{ID: 1, ConsumerSystemName: "consumer-1", ServiceDefinition: "telemetry"},
		{ID: 2, ConsumerSystemName: "consumer-2", ServiceDefinition: "telemetry"},
		{ID: 3, ConsumerSystemName: "consumer-3", ServiceDefinition: "telemetry"},
	}

	m := &mockAuthzForce{}
	azSrv := httptest.NewServer(m.handler())
	defer azSrv.Close()

	ca := mockCA(rules)
	defer ca.Close()

	s := newSyncer(az.New(azSrv.URL), ca.URL)
	if err := s.init(); err != nil {
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

	s := newSyncer(az.New(azSrv.URL), ca.URL)
	if err := s.init(); err != nil {
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

	s := newSyncer(az.New(azSrv.URL), ca.URL)
	if err := s.init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if len(m.rootPolicySet) == 0 {
		t.Fatal("expected root policy to be set after init")
	}
	if !strings.Contains(m.rootPolicySet[0], policySetID) {
		t.Fatalf("root policy ref missing policy set ID %q", policySetID)
	}
}
