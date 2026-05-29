package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"arrowhead/core/internal/api"
	"arrowhead/core/internal/model"
	"arrowhead/core/internal/repository"
	"arrowhead/core/internal/service"
)

func newAH5Handler() http.Handler {
	return api.NewAH5Handler(service.NewAH5RegistryService(repository.NewAH5Store()))
}

func ah5Post(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func ah5Put(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func ah5Delete(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// ─── Device Discovery ─────────────────────────────────────────────────────────

func TestAH5DeviceRegisterCreated(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{
		"name": "GW1",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var dev model.Device
	json.NewDecoder(w.Body).Decode(&dev) //nolint
	if dev.Name != "GW1" {
		t.Errorf("Name = %q", dev.Name)
	}
}

func TestAH5DeviceRegisterUpdated(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{"name": "GW1"})
	w := ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{
		"name": "GW1", "metadata": map[string]string{"k": "v"},
	})
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for update, got %d", w.Code)
	}
}

func TestAH5DeviceRegisterMissingName(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAH5DeviceRegisterWrongMethod(t *testing.T) {
	h := newAH5Handler()
	req := httptest.NewRequest(http.MethodGet, "/serviceregistry/device-discovery/register", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestAH5DeviceLookupAll(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{"name": "D1"})
	ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{"name": "D2"})
	w := ah5Post(t, h, "/serviceregistry/device-discovery/lookup", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.DeviceLookupResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestAH5DeviceLookupWrongMethod(t *testing.T) {
	h := newAH5Handler()
	req := httptest.NewRequest(http.MethodGet, "/serviceregistry/device-discovery/lookup", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestAH5DeviceRevokeFound(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{"name": "GW3"})
	w := ah5Delete(t, h, "/serviceregistry/device-discovery/revoke/GW3")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAH5DeviceRevokeNotFound(t *testing.T) {
	h := newAH5Handler()
	w := ah5Delete(t, h, "/serviceregistry/device-discovery/revoke/missing")
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestAH5DeviceRevokeWrongMethod(t *testing.T) {
	h := newAH5Handler()
	req := httptest.NewRequest(http.MethodGet, "/serviceregistry/device-discovery/revoke/x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ─── System Discovery ─────────────────────────────────────────────────────────

func TestAH5SystemRegisterCreated(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{
		"name":    "MySystem",
		"version": "1.0",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var sys model.AH5System
	json.NewDecoder(w.Body).Decode(&sys) //nolint
	if sys.Name != "MySystem" {
		t.Errorf("Name = %q", sys.Name)
	}
}

func TestAH5SystemRegisterUpdated(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{"name": "Sys1", "version": "1"})
	w := ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{"name": "Sys1", "version": "2"})
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for update, got %d", w.Code)
	}
}

func TestAH5SystemRegisterMissingName(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAH5SystemLookupAll(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{"name": "Sys1"})
	ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{"name": "Sys2"})
	w := ah5Post(t, h, "/serviceregistry/system-discovery/lookup", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.SystemLookupResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestAH5SystemRevokeFound(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{"name": "RemSys"})
	w := ah5Delete(t, h, "/serviceregistry/system-discovery/revoke?name=RemSys")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAH5SystemRevokeNotFound(t *testing.T) {
	h := newAH5Handler()
	w := ah5Delete(t, h, "/serviceregistry/system-discovery/revoke?name=ghost")
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestAH5SystemRevokeMissingName(t *testing.T) {
	h := newAH5Handler()
	w := ah5Delete(t, h, "/serviceregistry/system-discovery/revoke")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── Service Discovery ────────────────────────────────────────────────────────

func TestAH5ServiceRegisterCreated(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName":            "Prov1",
		"serviceDefinitionName": "temperature",
		"version":               "1.0",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var inst model.AH5ServiceInstance
	json.NewDecoder(w.Body).Decode(&inst) //nolint
	if inst.InstanceID == "" {
		t.Error("expected instanceId")
	}
	if inst.ServiceDefinitionName != "temperature" {
		t.Errorf("ServiceDefinitionName = %q", inst.ServiceDefinitionName)
	}
}

func TestAH5ServiceRegisterUpdated(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName": "P1", "serviceDefinitionName": "s", "version": "1",
	})
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName": "P1", "serviceDefinitionName": "s", "version": "1",
		"metadata": map[string]string{"k": "v"},
	})
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for update, got %d", w.Code)
	}
}

func TestAH5ServiceRegisterMissingSystemName(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"serviceDefinitionName": "svc",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAH5ServiceRegisterMissingDefinitionName(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName": "Sys1",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAH5ServiceLookupByProvider(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName": "P1", "serviceDefinitionName": "svc",
	})
	ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName": "P2", "serviceDefinitionName": "svc",
	})
	w := ah5Post(t, h, "/serviceregistry/service-discovery/lookup", map[string]any{
		"providerNames": []string{"P1"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.ServiceLookupResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

// ─── Cycle 16.2 — ServiceLookupRequest at-least-one-filter ───────────────────

func TestAH5ServiceLookupRequiresFilter(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/service-discovery/lookup", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty lookup: expected 400, got %d", w.Code)
	}
}

// ─── Cycle 16.1 — Structured and flat-string interface backward-compat ────────

func TestAH5ServiceRegisterStructuredInterface(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName":            "IntfSys",
		"serviceDefinitionName": "tempService",
		"version":               "1.0.0",
		"interfaces": []map[string]any{
			{"templateName": "http-json", "protocol": "http", "policy": "NONE"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAH5ServiceRegisterFlatStringInterfaceBackwardCompat(t *testing.T) {
	h := newAH5Handler()
	// Flat string interface (old style) must still be accepted.
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName":            "FlatSys",
		"serviceDefinitionName": "flatSvc",
		"version":               "1.0.0",
		"interfaces":            []string{"HTTP-INSECURE-JSON"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("backward-compat flat string interface: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var inst model.AH5ServiceInstance
	json.NewDecoder(w.Body).Decode(&inst) //nolint
	if len(inst.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(inst.Interfaces))
	}
	if inst.Interfaces[0].TemplateName != "HTTP-INSECURE-JSON" {
		t.Errorf("TemplateName = %q, want HTTP-INSECURE-JSON", inst.Interfaces[0].TemplateName)
	}
}

// ─── Cycle 16.3 — Template-absent registration accepted without validation ────

func TestInterfaceValidationTemplateAbsent(t *testing.T) {
	h := newAH5Handler()
	// No interface template registered — should be accepted without validation.
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName":            "NoTmplSys",
		"serviceDefinitionName": "noTmplSvc",
		"version":               "1.0.0",
		"interfaces":            []map[string]any{{"templateName": "custom", "protocol": "mqtt", "policy": "NONE"}},
	})
	if w.Code != http.StatusCreated {
		t.Errorf("absent template: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAH5ServiceRegisterInvalidPolicy(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName":            "BadPolSys",
		"serviceDefinitionName": "badPolSvc",
		"interfaces":            []map[string]any{{"templateName": "httpJson", "protocol": "http", "policy": "INVALID_POLICY"}},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid policy: expected 400, got %d", w.Code)
	}
}

func TestAH5ServiceRevokeFound(t *testing.T) {
	h := newAH5Handler()
	w1 := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName": "P1", "serviceDefinitionName": "s",
	})
	var inst model.AH5ServiceInstance
	json.NewDecoder(w1.Body).Decode(&inst) //nolint
	w2 := ah5Delete(t, h, "/serviceregistry/service-discovery/revoke/"+inst.InstanceID)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
}

func TestAH5ServiceRevokeNotFound(t *testing.T) {
	h := newAH5Handler()
	w := ah5Delete(t, h, "/serviceregistry/service-discovery/revoke/9999")
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestAH5DeviceRevoke423WhenSystemDependent(t *testing.T) {
	h := newAH5Handler()
	// Register device
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST",
		"/serviceregistry/device-discovery/register",
		strings.NewReader(`{"name":"GW01"}`)))
	// Register system referencing device
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST",
		"/serviceregistry/system-discovery/register",
		strings.NewReader(`{"name":"Sensor1","deviceName":"GW01","addresses":[{"type":"IP","address":"192.0.2.1"}]}`)))
	// Attempt revoke
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("DELETE",
		"/serviceregistry/device-discovery/revoke/GW01", nil))
	if rr.Code != http.StatusLocked {
		t.Errorf("expected 423, got %d", rr.Code)
	}
}

func TestAH5ServiceRevokeByCompositeID(t *testing.T) {
	h := newAH5Handler()
	// Register a service — version defaults to 1.0.0
	body := `{"systemName":"Provider1","serviceDefinitionName":"temperature","version":"1.0.0"}`
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST",
		"/serviceregistry/service-discovery/register", strings.NewReader(body)))
	if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
		t.Fatalf("register failed: %d", rr.Code)
	}

	// Revoke using URL-encoded composite ID
	encodedID := url.PathEscape("Provider1|temperature|1.0.0")
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, httptest.NewRequest("DELETE",
		"/serviceregistry/service-discovery/revoke/"+encodedID, nil))
	if rr2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

// ─── Management — Devices ─────────────────────────────────────────────────────

func TestAH5MgmtDeviceCreate(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/mgmt/devices", map[string]any{
		"devices": []map[string]any{{"name": "D1"}, {"name": "D2"}},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.DeviceListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestAH5MgmtDeviceCreateDuplicate(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/devices", map[string]any{
		"devices": []map[string]any{{"name": "D1"}},
	})
	w := ah5Post(t, h, "/serviceregistry/mgmt/devices", map[string]any{
		"devices": []map[string]any{{"name": "D1"}},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAH5MgmtDeviceQuery(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/devices", map[string]any{
		"devices": []map[string]any{{"name": "D1"}},
	})
	w := ah5Post(t, h, "/serviceregistry/mgmt/devices/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.DeviceListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

func TestAH5MgmtDeviceUpdate(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/devices", map[string]any{
		"devices": []map[string]any{{"name": "D1", "metadata": map[string]string{"v": "1"}}},
	})
	w := ah5Put(t, h, "/serviceregistry/mgmt/devices", map[string]any{
		"devices": []map[string]any{{"name": "D1", "metadata": map[string]string{"v": "2"}}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.DeviceListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Devices[0].Metadata["v"] != "2" {
		t.Errorf("metadata not updated: %v", resp.Devices[0].Metadata)
	}
}

func TestAH5MgmtDeviceRemove(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/devices", map[string]any{
		"devices": []map[string]any{{"name": "D1"}, {"name": "D2"}},
	})
	w := ah5Delete(t, h, "/serviceregistry/mgmt/devices?names=D1")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	qw := ah5Post(t, h, "/serviceregistry/mgmt/devices/query", map[string]any{})
	var resp model.DeviceListResponse
	json.NewDecoder(qw.Body).Decode(&resp) //nolint
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1 after remove", resp.Count)
	}
}

// ─── Management — Systems ─────────────────────────────────────────────────────

func TestAH5MgmtSystemCreate(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/mgmt/systems", map[string]any{
		"systems": []map[string]any{{"name": "Sys1", "version": "1.0"}},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.SystemListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

func TestAH5MgmtSystemUpdate(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/systems", map[string]any{
		"systems": []map[string]any{{"name": "Sys1", "version": "1"}},
	})
	w := ah5Put(t, h, "/serviceregistry/mgmt/systems", map[string]any{
		"systems": []map[string]any{{"name": "Sys1", "version": "2"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.SystemListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Systems[0].Version != "2" {
		t.Error("version not updated")
	}
}

func TestAH5MgmtSystemRemove(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/systems", map[string]any{
		"systems": []map[string]any{{"name": "Sys1"}, {"name": "Sys2"}},
	})
	w := ah5Delete(t, h, "/serviceregistry/mgmt/systems?names=Sys1")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ─── Management — Service Definitions ────────────────────────────────────────

func TestAH5MgmtServiceDefCreate(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/mgmt/service-definitions", map[string]any{
		"serviceDefinitionNames": []string{"temp", "humidity"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.ServiceDefinitionListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestAH5MgmtServiceDefQuery(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/service-definitions", map[string]any{
		"serviceDefinitionNames": []string{"a", "b"},
	})
	w := ah5Post(t, h, "/serviceregistry/mgmt/service-definitions/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.ServiceDefinitionListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestAH5MgmtServiceDefRemove(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/service-definitions", map[string]any{
		"serviceDefinitionNames": []string{"a", "b"},
	})
	w := ah5Delete(t, h, "/serviceregistry/mgmt/service-definitions?names=a")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ─── Management — Interface Templates ────────────────────────────────────────

func TestAH5MgmtInterfaceTemplateCreate(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/mgmt/interface-templates", map[string]any{
		"interfaceTemplates": []map[string]any{
			{"name": "http_secure_json", "protocol": "HTTP"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.InterfaceTemplateListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

func TestAH5MgmtInterfaceTemplateQuery(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/interface-templates", map[string]any{
		"interfaceTemplates": []map[string]any{{"name": "t1", "protocol": "HTTP"}},
	})
	w := ah5Post(t, h, "/serviceregistry/mgmt/interface-templates/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.InterfaceTemplateListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

func TestAH5MgmtInterfaceTemplateRemove(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/interface-templates", map[string]any{
		"interfaceTemplates": []map[string]any{{"name": "t1", "protocol": "HTTP"}, {"name": "t2", "protocol": "HTTP"}},
	})
	w := ah5Delete(t, h, "/serviceregistry/mgmt/interface-templates?names=t1")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ─── Management — Service Instances ──────────────────────────────────────────

func TestAH5MgmtServiceInstanceCreate(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/mgmt/service-instances", map[string]any{
		"instances": []map[string]any{
			{"systemName": "Sys1", "serviceDefinitionName": "svc"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.ServiceListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

func TestAH5MgmtServiceInstanceQuery(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/service-instances", map[string]any{
		"instances": []map[string]any{
			{"systemName": "Sys1", "serviceDefinitionName": "a"},
			{"systemName": "Sys1", "serviceDefinitionName": "b"},
		},
	})
	w := ah5Post(t, h, "/serviceregistry/mgmt/service-instances/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.ServiceListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestAH5MgmtServiceInstanceUpdate(t *testing.T) {
	h := newAH5Handler()
	cw := ah5Post(t, h, "/serviceregistry/mgmt/service-instances", map[string]any{
		"instances": []map[string]any{{"systemName": "Sys1", "serviceDefinitionName": "svc", "expiresAt": "2025-01-01T00:00:00Z"}},
	})
	var cr model.ServiceListResponse
	json.NewDecoder(cw.Body).Decode(&cr) //nolint
	id := cr.Instances[0].InstanceID

	w := ah5Put(t, h, "/serviceregistry/mgmt/service-instances", map[string]any{
		"instances": []map[string]any{{"instanceId": id, "expiresAt": "2027-01-01T00:00:00Z"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.ServiceListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Instances[0].ExpiresAt != "2027-01-01T00:00:00Z" {
		t.Errorf("ExpiresAt = %q", resp.Instances[0].ExpiresAt)
	}
}

func TestAH5MgmtServiceInstanceRemove(t *testing.T) {
	h := newAH5Handler()
	cw := ah5Post(t, h, "/serviceregistry/mgmt/service-instances", map[string]any{
		"instances": []map[string]any{
			{"systemName": "Sys1", "serviceDefinitionName": "a"},
			{"systemName": "Sys1", "serviceDefinitionName": "b"},
		},
	})
	var cr model.ServiceListResponse
	json.NewDecoder(cw.Body).Decode(&cr) //nolint
	id := cr.Instances[0].InstanceID

	w := ah5Delete(t, h, "/serviceregistry/mgmt/service-instances?serviceInstances="+id)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	qw := ah5Post(t, h, "/serviceregistry/mgmt/service-instances/query", map[string]any{})
	var qr model.ServiceListResponse
	json.NewDecoder(qw.Body).Decode(&qr) //nolint
	if qr.Count != 1 {
		t.Errorf("Count = %d, want 1", qr.Count)
	}
}

// ─── Coverage: uncovered method-not-allowed and query paths ──────────────────

func TestAH5MgmtSystemQuery(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/systems", map[string]any{
		"systems": []map[string]any{{"name": "Sys1"}},
	})
	w := ah5Post(t, h, "/serviceregistry/mgmt/systems/query", map[string]any{
		"systemNames": []string{"Sys1"},
	})
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp model.SystemListResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

func TestAH5SystemRegisterWrongMethod(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/serviceregistry/system-discovery/register", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAH5ServiceRegisterWrongMethod(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/serviceregistry/service-discovery/register", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAH5SystemLookupWrongMethod(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/serviceregistry/system-discovery/lookup", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAH5ServiceLookupWrongMethod(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/serviceregistry/service-discovery/lookup", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAH5ServiceRevokeWrongMethod(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/serviceregistry/service-discovery/revoke/someId", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAH5MgmtDevicesQueryWrongMethod(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/serviceregistry/mgmt/devices/query", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAH5MgmtSystemsQueryWrongMethod(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/serviceregistry/mgmt/systems/query", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAH5MgmtServiceDefsQueryWrongMethod(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/serviceregistry/mgmt/service-definitions/query", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAH5MgmtServiceInstancesQueryWrongMethod(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/serviceregistry/mgmt/service-instances/query", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAH5MgmtInterfaceTemplatesQueryWrongMethod(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/serviceregistry/mgmt/interface-templates/query", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

// ─── Step 4: naming convention validation ─────────────────────────────────────

func TestAH5SystemRegister_InvalidNameLowerStart(t *testing.T) {
	h := newAH5Handler()
	body := `{"name":"mySystem"}`
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST",
		"/serviceregistry/system-discovery/register", strings.NewReader(body)))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-PascalCase SystemName, got %d", rr.Code)
	}
}

func TestAH5SystemRegister_ValidName(t *testing.T) {
	h := newAH5Handler()
	body := `{"name":"MySystem"}`
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST",
		"/serviceregistry/system-discovery/register", strings.NewReader(body)))
	if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
		t.Errorf("expected 200/201 for valid SystemName, got %d: %s",
			rr.Code, rr.Body.String())
	}
}

func TestAH5DeviceRegister_InvalidNameLowercase(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST",
		"/serviceregistry/device-discovery/register",
		strings.NewReader(`{"name":"gw01"}`)))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for lowercase DeviceName, got %d", rr.Code)
	}
}

func TestAH5DeviceRegister_InvalidNameTrailingUnderscore(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST",
		"/serviceregistry/device-discovery/register",
		strings.NewReader(`{"name":"GW01_"}`)))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for trailing underscore, got %d", rr.Code)
	}
}

func TestAH5DeviceRegister_ValidName(t *testing.T) {
	h := newAH5Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST",
		"/serviceregistry/device-discovery/register",
		strings.NewReader(`{"name":"GW01"}`)))
	if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
		t.Errorf("expected 200/201, got %d", rr.Code)
	}
}

func TestAH5ServiceRegister_InvalidServiceDefNameUpperStart(t *testing.T) {
	h := newAH5Handler()
	body := `{"systemName":"Provider1","serviceDefinitionName":"Temperature","version":"1.0.0"}`
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST",
		"/serviceregistry/service-discovery/register", strings.NewReader(body)))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for UpperCase ServiceDefinitionName, got %d", rr.Code)
	}
}

func TestAH5MgmtInterfaceTemplateCreate_InvalidName(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/mgmt/interface-templates", map[string]any{
		"interfaceTemplates": []map[string]any{
			{"name": "HTTP-SECURE-JSON", "protocol": "HTTP"},
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-snake_case InterfaceTemplateName, got %d", w.Code)
	}
}

// ─── Pagination ───────────────────────────────────────────────────────────────

func TestAH5DeviceLookupPagination(t *testing.T) {
	h := newAH5Handler()
	// Seed 5 devices (names must be UPPER_SNAKE_CASE).
	for i := 0; i < 5; i++ {
		ah5Post(t, h, "/serviceregistry/device-discovery/register",
			map[string]any{"name": fmt.Sprintf("DEV%d", i)})
	}
	// Query page 0, size 2.
	w := ah5Post(t, h, "/serviceregistry/device-discovery/lookup", map[string]any{
		"pagination": map[string]any{"pageNumber": 0, "pageSize": 2, "pageDirection": "ASC"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count      int `json:"count"`
		TotalCount int `json:"totalCount"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2", resp.Count)
	}
	if resp.TotalCount != 5 {
		t.Errorf("totalCount = %d, want 5", resp.TotalCount)
	}
}

func TestAH5DeviceLookupNoPaginationReturnsAll(t *testing.T) {
	h := newAH5Handler()
	for i := 0; i < 3; i++ {
		ah5Post(t, h, "/serviceregistry/device-discovery/register",
			map[string]any{"name": fmt.Sprintf("DA%d", i)})
	}
	w := ah5Post(t, h, "/serviceregistry/device-discovery/lookup", map[string]any{})
	var resp struct {
		Count      int `json:"count"`
		TotalCount int `json:"totalCount"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 3 {
		t.Errorf("count = %d, want 3", resp.Count)
	}
	if resp.TotalCount != 3 {
		t.Errorf("totalCount = %d, want 3", resp.TotalCount)
	}
}
