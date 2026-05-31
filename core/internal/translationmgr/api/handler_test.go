package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tmapi "arrowhead/core/internal/translationmgr/api"
	"arrowhead/core/internal/translationmgr/model"
	"arrowhead/core/internal/translationmgr/service"
)

func newHandler() http.Handler {
	return tmapi.NewHandler(service.NewTranslationService())
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

func TestCreateBridgeAndTranslate(t *testing.T) {
	h := newHandler()

	// Create bridge: map "temperature" → "temp"
	bridgeBody := model.Bridge{
		SourceFormat: "sensor-v1",
		TargetFormat: "sensor-v2",
		FieldMappings: map[string]string{
			"temperature": "temp",
		},
	}
	w := postJSON(t, h, "/translationmanager/translation/mgmt/bridges", bridgeBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create bridge: want 201, got %d: %s", w.Code, w.Body.String())
	}
	var bridge model.Bridge
	json.NewDecoder(w.Body).Decode(&bridge) //nolint:errcheck
	if bridge.ID == "" {
		t.Fatal("expected non-empty bridge ID")
	}

	// Translate using the created bridge
	translateBody := model.TranslateRequest{
		BridgeID: bridge.ID,
		Payload:  json.RawMessage(`{"temperature":25}`),
	}
	w = postJSON(t, h, "/translationmanager/translation/translate", translateBody)
	if w.Code != http.StatusOK {
		t.Fatalf("translate: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.TranslateResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp.BridgeID != bridge.ID {
		t.Errorf("bridgeId = %q, want %q", resp.BridgeID, bridge.ID)
	}
	// Translated payload should have "temp" key instead of "temperature"
	var translated map[string]any
	json.Unmarshal(resp.TranslatedPayload, &translated) //nolint:errcheck
	if _, ok := translated["temp"]; !ok {
		t.Error("translated payload missing 'temp' key")
	}
	if _, ok := translated["temperature"]; ok {
		t.Error("translated payload should not have 'temperature' key")
	}
}

func TestTranslateUnknownBridgeReturns404(t *testing.T) {
	h := newHandler()
	w := postJSON(t, h, "/translationmanager/translation/translate", model.TranslateRequest{
		BridgeID: "nonexistent-bridge",
		Payload:  json.RawMessage(`{}`),
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestBridgeCRUD(t *testing.T) {
	h := newHandler()

	// Create
	w := postJSON(t, h, "/translationmanager/translation/mgmt/bridges", model.Bridge{
		SourceFormat: "a",
		TargetFormat: "b",
		FieldMappings: map[string]string{"x": "y"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", w.Code)
	}
	var bridge model.Bridge
	json.NewDecoder(w.Body).Decode(&bridge) //nolint:errcheck

	// List
	req := httptest.NewRequest(http.MethodGet, "/translationmanager/translation/mgmt/bridges", nil)
	wr := httptest.NewRecorder()
	h.ServeHTTP(wr, req)
	if wr.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", wr.Code)
	}
	var list []*model.Bridge
	json.NewDecoder(wr.Body).Decode(&list) //nolint:errcheck
	if len(list) != 1 {
		t.Errorf("list count = %d, want 1", len(list))
	}

	// Delete
	req = httptest.NewRequest(http.MethodDelete, "/translationmanager/translation/mgmt/bridges/"+bridge.ID, nil)
	wr = httptest.NewRecorder()
	h.ServeHTTP(wr, req)
	if wr.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", wr.Code)
	}

	// List again — should be empty
	req = httptest.NewRequest(http.MethodGet, "/translationmanager/translation/mgmt/bridges", nil)
	wr = httptest.NewRecorder()
	h.ServeHTTP(wr, req)
	json.NewDecoder(wr.Body).Decode(&list) //nolint:errcheck
	if len(list) != 0 {
		t.Errorf("after delete: list count = %d, want 0", len(list))
	}
}
