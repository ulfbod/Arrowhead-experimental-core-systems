package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"arrowhead/core/internal/api"
	"arrowhead/core/internal/model"
	"arrowhead/core/internal/repository"
	"arrowhead/core/internal/service"
)

func newTestHandler() http.Handler {
	return api.NewHandler(service.NewRegistryService(repository.NewMemoryRepository()))
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func decodeInstance(t *testing.T, w *httptest.ResponseRecorder) model.ServiceInstance {
	t.Helper()
	var v model.ServiceInstance
	if err := json.NewDecoder(w.Body).Decode(&v); err != nil {
		t.Fatalf("decode ServiceInstance: %v", err)
	}
	return v
}

func decodeQuery(t *testing.T, w *httptest.ResponseRecorder) model.QueryResponse {
	t.Helper()
	var v model.QueryResponse
	if err := json.NewDecoder(w.Body).Decode(&v); err != nil {
		t.Fatalf("decode QueryResponse: %v", err)
	}
	return v
}

var fullRegisterBody = map[string]any{
	"serviceDefinition": "temperature-service",
	"providerSystem": map[string]any{
		"systemName": "sensor-1",
		"address":    "192.168.0.10",
		"port":       8080,
	},
	"serviceUri": "/temperature",
	"interfaces": []string{"HTTP-SECURE-JSON"},
	"version":    1,
	"metadata":   map[string]string{"region": "eu", "unit": "celsius"},
	"secure":     "NOT_SECURE",
}

// ---- Register ----

func TestHandlerRegisterValid(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/serviceregistry/register", fullRegisterBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	got := decodeInstance(t, w)
	if got.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if got.ServiceDefinition != "temperature-service" {
		t.Errorf("ServiceDefinition = %q", got.ServiceDefinition)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.Metadata["region"] != "eu" {
		t.Errorf("Metadata region = %q", got.Metadata["region"])
	}
	if got.Secure != "NOT_SECURE" {
		t.Errorf("Secure = %q", got.Secure)
	}
}

func TestHandlerRegisterMissingFields(t *testing.T) {
	tests := []struct {
		name string
		body map[string]any
	}{
		{
			"missing serviceDefinition",
			map[string]any{
				"providerSystem": map[string]any{"systemName": "s1", "address": "10.0.0.1", "port": 9000},
				"serviceUri":     "/svc",
				"interfaces":     []string{"HTTP"},
			},
		},
		{
			"missing providerSystem",
			map[string]any{
				"serviceDefinition": "svc",
				"serviceUri":        "/svc",
				"interfaces":        []string{"HTTP"},
			},
		},
		{
			"missing serviceUri",
			map[string]any{
				"serviceDefinition": "svc",
				"providerSystem":    map[string]any{"systemName": "s1", "address": "10.0.0.1", "port": 9000},
				"interfaces":        []string{"HTTP"},
			},
		},
		{
			"missing interfaces",
			map[string]any{
				"serviceDefinition": "svc",
				"providerSystem":    map[string]any{"systemName": "s1", "address": "10.0.0.1", "port": 9000},
				"serviceUri":        "/svc",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(t, newTestHandler(), "/serviceregistry/register", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestHandlerRegisterInvalidJSON(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/serviceregistry/register",
		bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerRegisterWrongTypes(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			"port as string",
			`{"serviceDefinition":"svc","providerSystem":{"systemName":"s1","address":"10.0.0.1","port":"notanumber"},"serviceUri":"/svc","interfaces":["HTTP"]}`,
		},
		{
			"interfaces as string instead of array",
			`{"serviceDefinition":"svc","providerSystem":{"systemName":"s1","address":"10.0.0.1","port":9000},"serviceUri":"/svc","interfaces":"HTTP"}`,
		},
		{
			"version as string",
			`{"serviceDefinition":"svc","providerSystem":{"systemName":"s1","address":"10.0.0.1","port":9000},"serviceUri":"/svc","interfaces":["HTTP"],"version":"two"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/serviceregistry/register",
				bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			newTestHandler().ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestHandlerQueryWrongTypes(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"versionRequirement as string", `{"versionRequirement":"two"}`},
		{"interfaces as string instead of array", `{"interfaces":"HTTP"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/serviceregistry/query",
				bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			newTestHandler().ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestHandlerRegisterWrongMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/serviceregistry/register", nil)
	w := httptest.NewRecorder()
	newTestHandler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---- Unregister ----

func TestHandlerUnregister(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/serviceregistry/register", fullRegisterBody)

	req := httptest.NewRequest(http.MethodDelete, "/serviceregistry/unregister",
		bytes.NewBufferString(`{"serviceDefinition":"temperature-service","providerSystem":{"systemName":"sensor-1","address":"192.168.0.10","port":8080},"version":1}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Should be gone
	qw := postJSON(t, h, "/serviceregistry/query", map[string]any{"serviceDefinition": "temperature-service"})
	resp := decodeQuery(t, qw)
	if len(resp.ServiceQueryData) != 0 {
		t.Errorf("expected 0 after unregister, got %d", len(resp.ServiceQueryData))
	}
}

func TestHandlerLookup(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/serviceregistry/register", fullRegisterBody)

	req := httptest.NewRequest(http.MethodGet, "/serviceregistry/lookup", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	resp := decodeQuery(t, w)
	if len(resp.ServiceQueryData) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp.ServiceQueryData))
	}
}

// ---- Query: basic ----

func TestHandlerQueryExactMatch(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/serviceregistry/register", fullRegisterBody)

	w := postJSON(t, h, "/serviceregistry/query", map[string]any{
		"serviceDefinition": "temperature-service",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	resp := decodeQuery(t, w)
	if len(resp.ServiceQueryData) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp.ServiceQueryData))
	}
}

func TestHandlerQueryNoMatch(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/serviceregistry/register", fullRegisterBody)

	w := postJSON(t, h, "/serviceregistry/query", map[string]any{
		"serviceDefinition": "unknown-service",
	})
	resp := decodeQuery(t, w)
	if len(resp.ServiceQueryData) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.ServiceQueryData))
	}
}

func TestHandlerQueryInterfaceMatch(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/serviceregistry/register", map[string]any{
		"serviceDefinition": "svc",
		"providerSystem":    map[string]any{"systemName": "s1", "address": "10.0.0.1", "port": 9000},
		"serviceUri":        "/svc",
		"interfaces":        []string{"HTTP", "HTTPS"},
	})

	tests := []struct {
		query     []string
		wantCount int
	}{
		{[]string{"HTTPS"}, 1},
		{[]string{"HTTP", "HTTPS"}, 1},
		{[]string{"COAP"}, 0},
	}
	for _, tc := range tests {
		w := postJSON(t, h, "/serviceregistry/query", map[string]any{"interfaces": tc.query})
		resp := decodeQuery(t, w)
		if len(resp.ServiceQueryData) != tc.wantCount {
			t.Errorf("query %v: expected %d, got %d", tc.query, tc.wantCount, len(resp.ServiceQueryData))
		}
	}
}

func TestHandlerQueryMetadataMatch(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/serviceregistry/register", fullRegisterBody)

	tests := []struct {
		name      string
		meta      map[string]string
		wantCount int
	}{
		{"matching region", map[string]string{"region": "eu"}, 1},
		{"non-matching region", map[string]string{"region": "us"}, 0},
		{"key absent", map[string]string{"country": "se"}, 0},
		{"subset of service meta", map[string]string{"unit": "celsius"}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(t, h, "/serviceregistry/query", map[string]any{"metadata": tc.meta})
			resp := decodeQuery(t, w)
			if len(resp.ServiceQueryData) != tc.wantCount {
				t.Errorf("expected %d, got %d", tc.wantCount, len(resp.ServiceQueryData))
			}
		})
	}
}

func TestHandlerQueryVersionRequirement(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/serviceregistry/register", fullRegisterBody)
	body2 := map[string]any{
		"serviceDefinition": "temperature-service",
		"providerSystem":    map[string]any{"systemName": "sensor-1", "address": "192.168.0.10", "port": 8080},
		"serviceUri":        "/temperature",
		"interfaces":        []string{"HTTP-SECURE-JSON"},
		"version":           2,
		"metadata":          map[string]string{"region": "eu", "unit": "celsius"},
	}
	postJSON(t, h, "/serviceregistry/register", body2)

	tests := []struct {
		requirement int
		wantCount   int
	}{
		{0, 2},
		{1, 1},
		{2, 1},
		{3, 0},
	}
	for _, tc := range tests {
		w := postJSON(t, h, "/serviceregistry/query", map[string]any{"versionRequirement": tc.requirement})
		resp := decodeQuery(t, w)
		if len(resp.ServiceQueryData) != tc.wantCount {
			t.Errorf("versionRequirement=%d: expected %d, got %d", tc.requirement, tc.wantCount, len(resp.ServiceQueryData))
		}
	}
}

func TestHandlerQueryInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/serviceregistry/query", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newTestHandler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestIntegrationRegisterAndQuery(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/serviceregistry/register", fullRegisterBody)

	w := postJSON(t, h, "/serviceregistry/query", map[string]any{
		"serviceDefinition":  "temperature-service",
		"interfaces":         []string{"HTTP-SECURE-JSON"},
		"metadata":           map[string]string{"region": "eu"},
		"versionRequirement": 1,
	})
	resp := decodeQuery(t, w)
	if len(resp.ServiceQueryData) != 1 {
		t.Fatalf("expected 1, got %d", len(resp.ServiceQueryData))
	}
	got := resp.ServiceQueryData[0]
	if got.ServiceUri != "/temperature" {
		t.Errorf("ServiceUri = %q", got.ServiceUri)
	}
	if got.ProviderSystem.SystemName != "sensor-1" {
		t.Errorf("SystemName = %q", got.ProviderSystem.SystemName)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d", got.Version)
	}
	if got.Metadata["region"] != "eu" {
		t.Errorf("Metadata region = %q", got.Metadata["region"])
	}
}

func TestHandlerHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	newTestHandler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
