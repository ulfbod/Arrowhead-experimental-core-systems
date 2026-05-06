package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── /hello ────────────────────────────────────────────────────────────────────

func TestHandleHello_GET(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	rec := httptest.NewRecorder()
	handleHello(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}

	var m map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := m["message"]; !ok {
		t.Error("response missing 'message' field")
	}
	if m["service"] != "example-service" {
		t.Errorf("service: got %q, want example-service", m["service"])
	}
}

func TestHandleHello_rejectsNonGET(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/hello", nil)
		rec := httptest.NewRecorder()
		handleHello(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /hello: got %d, want 405", method, rec.Code)
		}
	}
}

// ── /query (CORS proxy) ───────────────────────────────────────────────────────

func TestQueryProxy_OPTIONS(t *testing.T) {
	// Upstream never called for preflight.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should not be called for OPTIONS")
	}))
	defer upstream.Close()

	handler := makeQueryProxy(upstream.URL)
	req := httptest.NewRequest(http.MethodOptions, "/query", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("OPTIONS: got %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("OPTIONS response missing CORS header")
	}
}

func TestQueryProxy_POST_forwards(t *testing.T) {
	// Upstream echoes whatever was POSTed.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"services":[]}`))
	}))
	defer upstream.Close()

	handler := makeQueryProxy(upstream.URL)
	body := `{"serviceDefinition":"example-service"}`
	req := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST proxy: got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "services") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestQueryProxy_rejectsNonPOST(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer upstream.Close()

	handler := makeQueryProxy(upstream.URL)
	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /query: got %d, want 405", rec.Code)
	}
}

func TestQueryProxy_upstreamError(t *testing.T) {
	// Point at a non-listening port to force a connection error.
	handler := makeQueryProxy("http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("upstream error: got %d, want 502", rec.Code)
	}
}

func TestQueryProxy_CORSHeadersOnPost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	handler := makeQueryProxy(upstream.URL)
	req := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS header missing on POST response")
	}
}
