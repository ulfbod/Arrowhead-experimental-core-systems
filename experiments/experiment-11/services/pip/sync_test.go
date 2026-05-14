package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeConsumerAuth serves a fixed LookupResponse.
func newFakeConsumerAuth(rules []Grant) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/authorization/lookup" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LookupResponse{Rules: rules, Count: len(rules)})
	}))
}

func TestFetchAndUpdate_Populates(t *testing.T) {
	ts := newFakeConsumerAuth([]Grant{
		{ID: 1, ConsumerSystemName: "sp1", ProviderSystemName: "portal", ServiceDefinition: "svc"},
	})
	defer ts.Close()

	store := NewGrantStore()
	if err := fetchAndUpdate(ts.URL, store); err != nil {
		t.Fatalf("fetchAndUpdate: %v", err)
	}
	if len(store.GetAll()) != 1 {
		t.Errorf("grants len = %d, want 1", len(store.GetAll()))
	}
}

func TestFetchAndUpdate_EmptyResponse(t *testing.T) {
	ts := newFakeConsumerAuth(nil)
	defer ts.Close()

	store := NewGrantStore()
	if err := fetchAndUpdate(ts.URL, store); err != nil {
		t.Fatalf("fetchAndUpdate empty: %v", err)
	}
	if len(store.GetAll()) != 0 {
		t.Errorf("grants len = %d, want 0", len(store.GetAll()))
	}
}

func TestFetchAndUpdate_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	store := NewGrantStore()
	err := fetchAndUpdate(ts.URL, store)
	if err == nil {
		t.Error("fetchAndUpdate 500: expected error, got nil")
	}
}

func TestFetchAndUpdate_VersionStableOnRepeatSameGrants(t *testing.T) {
	grants := []Grant{{ID: 1, ConsumerSystemName: "sp1", ServiceDefinition: "svc"}}
	ts := newFakeConsumerAuth(grants)
	defer ts.Close()

	store := NewGrantStore()
	fetchAndUpdate(ts.URL, store)
	v1 := store.Version()
	fetchAndUpdate(ts.URL, store)
	v2 := store.Version()
	if v1 != v2 {
		t.Errorf("Version changed on same grants: %d → %d", v1, v2)
	}
}
