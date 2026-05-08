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
		"name": "gw-1",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var dev model.Device
	json.NewDecoder(w.Body).Decode(&dev) //nolint
	if dev.Name != "gw-1" {
		t.Errorf("Name = %q", dev.Name)
	}
}

func TestAH5DeviceRegisterUpdated(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{"name": "gw-1"})
	w := ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{
		"name": "gw-1", "metadata": map[string]string{"k": "v"},
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
	ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{"name": "d1"})
	ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{"name": "d2"})
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
	ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{"name": "rem"})
	w := ah5Delete(t, h, "/serviceregistry/device-discovery/revoke/rem")
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
		"name":    "my-system",
		"version": "1.0",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var sys model.AH5System
	json.NewDecoder(w.Body).Decode(&sys) //nolint
	if sys.Name != "my-system" {
		t.Errorf("Name = %q", sys.Name)
	}
}

func TestAH5SystemRegisterUpdated(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{"name": "s1", "version": "1"})
	w := ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{"name": "s1", "version": "2"})
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
	ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{"name": "s1"})
	ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{"name": "s2"})
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
	ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{"name": "rem-sys"})
	w := ah5Delete(t, h, "/serviceregistry/system-discovery/revoke?name=rem-sys")
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
		"systemName":            "prov-1",
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
		"systemName": "p", "serviceDefinitionName": "s", "version": "1",
	})
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName": "p", "serviceDefinitionName": "s", "version": "1",
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
		"systemName": "sys",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAH5ServiceLookupAll(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName": "p1", "serviceDefinitionName": "svc",
	})
	ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName": "p2", "serviceDefinitionName": "svc",
	})
	w := ah5Post(t, h, "/serviceregistry/service-discovery/lookup", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp model.ServiceLookupResponse
	json.NewDecoder(w.Body).Decode(&resp) //nolint
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestAH5ServiceRevokeFound(t *testing.T) {
	h := newAH5Handler()
	w1 := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"systemName": "p", "serviceDefinitionName": "s",
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

// ─── Management — Devices ─────────────────────────────────────────────────────

func TestAH5MgmtDeviceCreate(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/mgmt/devices", map[string]any{
		"devices": []map[string]any{{"name": "d1"}, {"name": "d2"}},
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
		"devices": []map[string]any{{"name": "d1"}},
	})
	w := ah5Post(t, h, "/serviceregistry/mgmt/devices", map[string]any{
		"devices": []map[string]any{{"name": "d1"}},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAH5MgmtDeviceQuery(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/mgmt/devices", map[string]any{
		"devices": []map[string]any{{"name": "d1"}},
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
		"devices": []map[string]any{{"name": "d1", "metadata": map[string]string{"v": "1"}}},
	})
	w := ah5Put(t, h, "/serviceregistry/mgmt/devices", map[string]any{
		"devices": []map[string]any{{"name": "d1", "metadata": map[string]string{"v": "2"}}},
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
		"devices": []map[string]any{{"name": "d1"}, {"name": "d2"}},
	})
	w := ah5Delete(t, h, "/serviceregistry/mgmt/devices?names=d1")
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
		"systems": []map[string]any{{"name": "s1", "version": "1.0"}},
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
		"systems": []map[string]any{{"name": "s1", "version": "1"}},
	})
	w := ah5Put(t, h, "/serviceregistry/mgmt/systems", map[string]any{
		"systems": []map[string]any{{"name": "s1", "version": "2"}},
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
		"systems": []map[string]any{{"name": "s1"}, {"name": "s2"}},
	})
	w := ah5Delete(t, h, "/serviceregistry/mgmt/systems?names=s1")
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
			{"name": "HTTP-SECURE-JSON", "protocol": "HTTP"},
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
		"interfaceTemplates": []map[string]any{{"name": "T1", "protocol": "HTTP"}},
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
		"interfaceTemplates": []map[string]any{{"name": "T1", "protocol": "HTTP"}, {"name": "T2", "protocol": "HTTP"}},
	})
	w := ah5Delete(t, h, "/serviceregistry/mgmt/interface-templates?names=T1")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ─── Management — Service Instances ──────────────────────────────────────────

func TestAH5MgmtServiceInstanceCreate(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/mgmt/service-instances", map[string]any{
		"instances": []map[string]any{
			{"systemName": "s1", "serviceDefinitionName": "svc"},
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
			{"systemName": "s1", "serviceDefinitionName": "a"},
			{"systemName": "s1", "serviceDefinitionName": "b"},
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
		"instances": []map[string]any{{"systemName": "s1", "serviceDefinitionName": "svc", "expiresAt": "2025-01-01T00:00:00Z"}},
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
			{"systemName": "s1", "serviceDefinitionName": "a"},
			{"systemName": "s1", "serviceDefinitionName": "b"},
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
