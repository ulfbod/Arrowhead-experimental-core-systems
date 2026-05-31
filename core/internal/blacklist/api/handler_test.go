package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"arrowhead/core/internal/blacklist/api"
	"arrowhead/core/internal/blacklist/repository"
	"arrowhead/core/internal/blacklist/service"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	svc := service.NewBlacklistService(repository.NewMemoryRepository())
	return api.NewHandler(svc, "", "")
}

func newTestHandlerWithAuth(t *testing.T, authURL string) http.Handler {
	t.Helper()
	svc := service.NewBlacklistService(repository.NewMemoryRepository())
	return api.NewHandler(svc, authURL, "")
}

func getReq(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// ---- Cycle 20.2: Discovery endpoints ----

func TestCheckTrue(t *testing.T) {
	h := newTestHandler(t)
	// Seed via mgmt endpoint.
	postJSON(t, h, "/blacklist/mgmt/create", map[string]any{
		"entries": []map[string]any{{"systemName": "bad", "reason": "test"}},
	})
	w := getReq(t, h, "/blacklist/check/bad")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "true\n" && body != "true" {
		t.Errorf("body = %q, want true", body)
	}
}

func TestCheckFalse(t *testing.T) {
	h := newTestHandler(t)
	w := getReq(t, h, "/blacklist/check/unknown")
	if body := w.Body.String(); body != "false\n" && body != "false" {
		t.Errorf("body = %q, want false", body)
	}
}

func TestCheckExpiredEntry(t *testing.T) {
	// No easy way to create expired via API — use service directly.
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	_ = time.Now() // reference to confirm time package is used
	svc.Add("exp-sys", "temp", time.Now().Add(-time.Hour), "admin")
	handler := api.NewHandler(svc, "", "")
	w := getReq(t, handler, "/blacklist/check/exp-sys")
	if body := w.Body.String(); body != "false\n" && body != "false" {
		t.Errorf("expired entry: body = %q, want false", body)
	}
}

func TestLookupReturnsActiveEntries(t *testing.T) {
	h := newTestHandler(t)
	postJSON(t, h, "/blacklist/mgmt/create", map[string]any{
		"entries": []map[string]any{{"systemName": "sys-x", "reason": "r"}},
	})
	w := getReq(t, h, "/blacklist/lookup")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Count int `json:"count"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("lookup count = 0 after create")
	}
}

// ---- Cycle 20.3: Management endpoints ----

func TestMgmtCreateMissingReason400(t *testing.T) {
	h := newTestHandler(t)
	w := postJSON(t, h, "/blacklist/mgmt/create", map[string]any{
		"entries": []map[string]any{{"systemName": "sys-without-reason"}},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing reason, got %d", w.Code)
	}
}

func TestMgmtRemoveInactivates(t *testing.T) {
	h := newTestHandler(t)
	postJSON(t, h, "/blacklist/mgmt/create", map[string]any{
		"entries": []map[string]any{{"systemName": "removable", "reason": "test"}},
	})
	req := httptest.NewRequest(http.MethodDelete, "/blacklist/mgmt/remove?names=removable", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("remove: expected 200, got %d", w.Code)
	}
	// System should no longer be blacklisted.
	cw := getReq(t, h, "/blacklist/check/removable")
	if cw.Body.String() != "false\n" && cw.Body.String() != "false" {
		t.Error("after remove, check should return false")
	}
}

func TestMgmtQueryReturnsAll(t *testing.T) {
	h := newTestHandler(t)
	postJSON(t, h, "/blacklist/mgmt/create", map[string]any{
		"entries": []map[string]any{
			{"systemName": "a", "reason": "r1"},
			{"systemName": "b", "reason": "r2"},
		},
	})
	w := postJSON(t, h, "/blacklist/mgmt/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Count int `json:"count"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 2 {
		t.Errorf("count = %d, want >= 2", resp.Count)
	}
}

func TestHealth(t *testing.T) {
	h := newTestHandler(t)
	for _, path := range []string{"/health", "/blacklist/health"} {
		w := getReq(t, h, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, w.Code)
		}
	}
}

// ─── Step B: Tests for Step 23 (bearer enforcement + query modes) ─────────────

func TestLookupRequiresBearerWhenAuthConfigured(t *testing.T) {
	fakeAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"systemName": "caller"}) //nolint:errcheck
	}))
	defer fakeAuth.Close()

	h := newTestHandlerWithAuth(t, fakeAuth.URL)

	// no bearer → 401
	req := httptest.NewRequest(http.MethodGet, "/blacklist/lookup", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}

	// with bearer → 200
	req2 := httptest.NewRequest(http.MethodGet, "/blacklist/lookup", nil)
	req2.Header.Set("Authorization", "Bearer mytoken")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w2.Code)
	}
}

func TestCheckRequiresBearerWhenAuthConfigured(t *testing.T) {
	fakeAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"systemName": "caller"}) //nolint:errcheck
	}))
	defer fakeAuth.Close()

	h := newTestHandlerWithAuth(t, fakeAuth.URL)

	req := httptest.NewRequest(http.MethodGet, "/blacklist/check/somesystem", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestMgmtQueryModeActives(t *testing.T) {
	h := newTestHandler(t)
	// add one active entry
	body := `{"entries":[{"systemName":"sys1","reason":"test"}]}`
	req := httptest.NewRequest(http.MethodPost, "/blacklist/mgmt/create", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// query with ACTIVES mode
	req2 := httptest.NewRequest(http.MethodPost, "/blacklist/mgmt/query", strings.NewReader(`{"mode":"ACTIVES"}`))
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w2.Body).Decode(&resp) //nolint:errcheck
	if resp["count"].(float64) < 1 {
		t.Errorf("want at least 1 active entry, got %v", resp["count"])
	}
}

func TestMgmtQueryModeAll(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/blacklist/mgmt/query", strings.NewReader(`{"mode":"ALL"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMgmtQueryModeInactives(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/blacklist/mgmt/query", strings.NewReader(`{"mode":"INACTIVES"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMgmtQueryModeInvalid(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/blacklist/mgmt/query", strings.NewReader(`{"mode":"BOGUS"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}
