package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"arrowhead/core/internal/orchestration/dynamic/api"
	dynservice "arrowhead/core/internal/orchestration/dynamic/service"
	orchmodel "arrowhead/core/internal/orchestration/model"
)

func newTestHandler(srURL, caURL string, checkAuth bool) http.Handler {
	orch := dynservice.NewDynamicOrchestrator(srURL, caURL, checkAuth)
	return api.NewHandler(orch)
}

func fakeSR(providers ...string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type sys struct {
			SystemName string `json:"systemName"`
			Address    string `json:"address"`
			Port       int    `json:"port"`
		}
		type inst struct {
			ServiceDefinition string   `json:"serviceDefinition"`
			ProviderSystem    sys      `json:"providerSystem"`
			ServiceUri        string   `json:"serviceUri"`
			Interfaces        []string `json:"interfaces"`
			Version           int      `json:"version"`
		}
		type resp struct {
			ServiceQueryData []inst `json:"serviceQueryData"`
			UnfilteredHits   int    `json:"unfilteredHits"`
		}
		var instances []inst
		for i, p := range providers {
			instances = append(instances, inst{
				ServiceDefinition: "temperature-service",
				ProviderSystem:    sys{SystemName: p, Address: "10.0.0.1", Port: 9000 + i},
				ServiceUri:        "/temperature",
				Interfaces:        []string{"HTTP"},
				Version:           1,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp{ServiceQueryData: instances, UnfilteredHits: len(instances)})
	}))
}

func fakeCA(authorized bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"authorized": authorized})
	}))
}

func postOrchestrate(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/orchestration/dynamic", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

var validBody = map[string]any{
	"requesterSystem":  map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
	"requestedService": map[string]any{"serviceDefinition": "temperature-service"},
}

func TestHandlerOrchestrateMatchNoAuth(t *testing.T) {
	sr := fakeSR("sensor-1")
	defer sr.Close()
	ca := fakeCA(true)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, false)
	w := postOrchestrate(t, h, validBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Response) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Response))
	}
	if resp.Response[0].Provider.SystemName != "sensor-1" {
		t.Errorf("expected sensor-1, got %q", resp.Response[0].Provider.SystemName)
	}
}

func TestHandlerOrchestrateNoMatchEmpty(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	ca := fakeCA(true)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, false)
	w := postOrchestrate(t, h, validBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Response) != 0 {
		t.Errorf("expected empty response, got %d", len(resp.Response))
	}
}

func TestHandlerOrchestrateWithAuthAllDenied(t *testing.T) {
	sr := fakeSR("sensor-1", "sensor-2")
	defer sr.Close()
	ca := fakeCA(false)
	defer ca.Close()

	h := newTestHandler(sr.URL, ca.URL, true)
	w := postOrchestrate(t, h, validBody)
	var resp orchmodel.OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Response) != 0 {
		t.Errorf("expected 0 results (all denied), got %d", len(resp.Response))
	}
}

func TestHandlerOrchestrateInvalidJSON(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	req := httptest.NewRequest(http.MethodPost, "/orchestration/dynamic", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerOrchestrateWrongMethod(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	req := httptest.NewRequest(http.MethodGet, "/orchestration/dynamic", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandlerHealth(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	for _, path := range []string{"/health", "/orchestration/dynamic/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, w.Code)
		}
	}
}
