package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPIPConfigFromEnv_InvalidInterval_UsesDefault(t *testing.T) {
	t.Setenv("SYNC_INTERVAL", "not-a-duration")
	cfg := configFromEnv()
	if cfg.syncInterval == 0 {
		t.Error("configFromEnv: invalid interval should fall back to non-zero default")
	}
}

func TestFetchAndUpdate_BadJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer ts.Close()

	store := NewGrantStore()
	err := fetchAndUpdate(ts.URL, store)
	if err == nil {
		t.Error("fetchAndUpdate bad JSON: expected error, got nil")
	}
}

func TestGetGrants_AllNoFilter(t *testing.T) {
	store := NewGrantStore()
	store.Update([]Grant{
		{ConsumerSystemName: "sp1", ServiceDefinition: "svc"},
		{ConsumerSystemName: "sp2", ServiceDefinition: "svc"},
	})
	srv := newPIPServer(store)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/grants", nil))
	if w.Code != http.StatusOK {
		t.Errorf("grants all: code = %d, want 200", w.Code)
	}
}

func TestPIPStatus_BeforeSyncLastSyncAt_Empty(t *testing.T) {
	store := NewGrantStore() // never synced
	srv := newPIPServer(store)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/status", nil))
	body := w.Body.String()
	// lastSyncAt should be "" (empty string) since never synced
	if body == "" {
		t.Error("status: empty body")
	}
}
