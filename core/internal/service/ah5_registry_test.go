package service_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"arrowhead/core/internal/model"
	"arrowhead/core/internal/repository"
	"arrowhead/core/internal/service"
)

func newAH5Service() *service.AH5RegistryService {
	return service.NewAH5RegistryService(repository.NewAH5Store())
}

// ─── Device Discovery ─────────────────────────────────────────────────────────

func TestDeviceRegisterValid(t *testing.T) {
	svc := newAH5Service()
	dev, created, err := svc.RegisterDevice(model.DeviceRegistrationRequest{
		Name:      "GW1",
		Metadata:  map[string]string{"location": "room-a"},
		Addresses: []model.Address{{Type: "MAC", Address: "aa:bb:cc:dd:ee:ff"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true for first registration")
	}
	if dev.Name != "GW1" {
		t.Errorf("Name = %q, want gateway-1", dev.Name)
	}
	if dev.CreatedAt == "" || dev.UpdatedAt == "" {
		t.Error("expected timestamps to be set")
	}
}

func TestDeviceRegisterMissingName(t *testing.T) {
	svc := newAH5Service()
	_, _, err := svc.RegisterDevice(model.DeviceRegistrationRequest{Name: ""})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestDeviceRegisterUpsert(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "DEV1", Metadata: map[string]string{"v": "1"}}) //nolint
	dev, created, _ := svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "DEV1", Metadata: map[string]string{"v": "2"}})
	if created {
		t.Error("expected created=false for duplicate")
	}
	if dev.Metadata["v"] != "2" {
		t.Errorf("metadata not updated, got %q", dev.Metadata["v"])
	}
}

func TestDeviceLookupAll(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "D1"}) //nolint
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "D2"}) //nolint
	resp := svc.LookupDevices(model.DeviceLookupRequest{})
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestDeviceLookupByName(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "ALPHA"}) //nolint
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "BETA"})  //nolint
	resp := svc.LookupDevices(model.DeviceLookupRequest{DeviceNames: []string{"ALPHA"}})
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
	if resp.Entries[0].Name != "ALPHA" {
		t.Errorf("Name = %q, want alpha", resp.Entries[0].Name)
	}
}

func TestDeviceRevokeFound(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "GW3"}) //nolint
	ok, err := svc.RevokeDevice("GW3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true for existing device")
	}
	resp := svc.LookupDevices(model.DeviceLookupRequest{})
	if resp.Count != 0 {
		t.Errorf("expected 0 after revoke, got %d", resp.Count)
	}
}

func TestDeviceRevokeNotFound(t *testing.T) {
	svc := newAH5Service()
	ok, err := svc.RevokeDevice("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false for missing device")
	}
}

// ─── System Discovery ─────────────────────────────────────────────────────────

func TestSystemRegisterValid(t *testing.T) {
	svc := newAH5Service()
	sys, created, err := svc.RegisterSystem(model.SystemRegistrationRequest{
		Name:      "SensorSystem",
		Version:   "1.0",
		Addresses: []model.Address{{Type: "IP", Address: "10.0.0.1"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true for first registration")
	}
	if sys.Name != "SensorSystem" {
		t.Errorf("Name = %q", sys.Name)
	}
}

func TestSystemRegisterMissingName(t *testing.T) {
	svc := newAH5Service()
	_, _, err := svc.RegisterSystem(model.SystemRegistrationRequest{Name: ""})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestSystemRegisterUpsert(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterSystem(model.SystemRegistrationRequest{Name: "Sys1", Version: "1"}) //nolint
	sys, created, _ := svc.RegisterSystem(model.SystemRegistrationRequest{Name: "Sys1", Version: "2"})
	if created {
		t.Error("expected created=false on update")
	}
	if sys.Version != "2" {
		t.Errorf("Version = %q, want 2", sys.Version)
	}
}

func TestSystemLookupByName(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterSystem(model.SystemRegistrationRequest{Name: "Sys1"}) //nolint
	svc.RegisterSystem(model.SystemRegistrationRequest{Name: "Sys2"}) //nolint
	resp := svc.LookupSystems(model.SystemLookupRequest{SystemNames: []string{"Sys1"}})
	if resp.Count != 1 || resp.Entries[0].Name != "Sys1" {
		t.Errorf("unexpected lookup result: %+v", resp)
	}
}

func TestSystemRevokeFound(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterSystem(model.SystemRegistrationRequest{Name: "RemSys"}) //nolint
	if !svc.RevokeSystem("RemSys") {
		t.Error("expected true")
	}
}

func TestSystemRevokeNotFound(t *testing.T) {
	svc := newAH5Service()
	if svc.RevokeSystem("ghost") {
		t.Error("expected false")
	}
}

// ─── Service Discovery ────────────────────────────────────────────────────────

func TestServiceRegisterValid(t *testing.T) {
	svc := newAH5Service()
	inst, created, err := svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName:            "Provider1",
		ServiceDefinitionName: "temperature",
		Version:               "1.0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if inst.InstanceID == "" {
		t.Error("expected non-empty instanceId")
	}
	if inst.ServiceDefinitionName != "temperature" {
		t.Errorf("ServiceDefinitionName = %q", inst.ServiceDefinitionName)
	}
	if inst.Provider == nil || inst.Provider.Name != "Provider1" {
		t.Error("expected provider to be set")
	}
}

func TestServiceRegisterMissingSystemName(t *testing.T) {
	svc := newAH5Service()
	_, _, err := svc.RegisterService(model.ServiceRegistrationRequest{
		ServiceDefinitionName: "temp",
	})
	if err == nil {
		t.Error("expected error for missing systemName")
	}
}

func TestServiceRegisterMissingDefinitionName(t *testing.T) {
	svc := newAH5Service()
	_, _, err := svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName: "Sys1",
	})
	if err == nil {
		t.Error("expected error for missing serviceDefinitionName")
	}
}

func TestServiceRegisterUpsert(t *testing.T) {
	svc := newAH5Service()
	inst1, _, _ := svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName:            "P1",
		ServiceDefinitionName: "svc",
		Version:               "1",
		Metadata:              map[string]string{"k": "v1"},
	})
	inst2, created, _ := svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName:            "P1",
		ServiceDefinitionName: "svc",
		Version:               "1",
		Metadata:              map[string]string{"k": "v2"},
	})
	if created {
		t.Error("expected created=false on update")
	}
	if inst1.InstanceID != inst2.InstanceID {
		t.Error("instanceId must not change on update")
	}
	if inst2.Metadata["k"] != "v2" {
		t.Errorf("metadata not updated: %v", inst2.Metadata)
	}
}

func TestServiceLookupByProviderName(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterService(model.ServiceRegistrationRequest{SystemName: "P1", ServiceDefinitionName: "svc"}) //nolint
	svc.RegisterService(model.ServiceRegistrationRequest{SystemName: "P2", ServiceDefinitionName: "svc"}) //nolint
	resp := svc.LookupServices(model.ServiceLookupRequest{ProviderNames: []string{"P1"}})
	if resp.Count != 1 || resp.Entries[0].Provider.Name != "P1" {
		t.Errorf("unexpected result: %+v", resp)
	}
}

func TestServiceLookupByDefinitionName(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterService(model.ServiceRegistrationRequest{SystemName: "P1", ServiceDefinitionName: "temp"})     //nolint
	svc.RegisterService(model.ServiceRegistrationRequest{SystemName: "P1", ServiceDefinitionName: "humidity"}) //nolint
	resp := svc.LookupServices(model.ServiceLookupRequest{ServiceDefinitionNames: []string{"temp"}})
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

func TestServiceLookupByInstanceID(t *testing.T) {
	svc := newAH5Service()
	inst, _, _ := svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName:            "P1",
		ServiceDefinitionName: "s",
	})
	resp := svc.LookupServices(model.ServiceLookupRequest{InstanceIDs: []string{inst.InstanceID}})
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

func TestServiceRevokeFound(t *testing.T) {
	svc := newAH5Service()
	inst, _, _ := svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName:            "P1",
		ServiceDefinitionName: "s",
	})
	if !svc.RevokeService(inst.InstanceID) {
		t.Error("expected true")
	}
	resp := svc.LookupServices(model.ServiceLookupRequest{})
	if resp.Count != 0 {
		t.Error("expected empty after revoke")
	}
}

func TestServiceRevokeNotFound(t *testing.T) {
	svc := newAH5Service()
	if svc.RevokeService("nonexistent-id") {
		t.Error("expected false")
	}
}

// ─── alivesAt filter (Step 3 / G17) ──────────────────────────────────────────

func TestAlivesAtExcludesExpiredService(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName: "P1", ServiceDefinitionName: "svc", Version: "1.0.0",
		ExpiresAt: past,
	})
	now := time.Now().UTC().Format(time.RFC3339)
	results := svc.LookupServices(model.ServiceLookupRequest{AlivesAt: now})
	if results.Count != 0 {
		t.Errorf("expected 0 results (expired), got %d", results.Count)
	}
}

func TestAlivesAtIncludesLiveService(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName: "P1", ServiceDefinitionName: "svc", Version: "1.0.0",
		ExpiresAt: future,
	})
	now := time.Now().UTC().Format(time.RFC3339)
	results := svc.LookupServices(model.ServiceLookupRequest{AlivesAt: now})
	if results.Count != 1 {
		t.Errorf("expected 1 result (live), got %d", results.Count)
	}
}

func TestAlivesAtIncludesServiceWithNoExpiry(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName: "P1", ServiceDefinitionName: "svc", Version: "1.0.0",
	})
	now := time.Now().UTC().Format(time.RFC3339)
	results := svc.LookupServices(model.ServiceLookupRequest{AlivesAt: now})
	if results.Count != 1 {
		t.Errorf("expected 1 result (no expiry = immortal), got %d", results.Count)
	}
}

// ─── 423 Locked on device revoke (Step 3 / G18) ───────────────────────────────

func TestRevokeDeviceLockedWhenSystemDependent(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "GW01"})
	svc.RegisterSystem(model.SystemRegistrationRequest{Name: "Sensor1", DeviceName: "GW01"})
	_, err := svc.RevokeDevice("GW01")
	if !errors.Is(err, service.ErrLocked) {
		t.Errorf("expected ErrLocked, got %v", err)
	}
}

func TestRevokeDeviceSucceedsWithNoDependent(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "GW02"})
	ok, err := svc.RevokeDevice("GW02")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true (device found and removed)")
	}
}

// ─── Version normalisation (Step 2 / G14) ────────────────────────────────────

func TestRegisterServiceNormalisesEmptyVersion(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	req := model.ServiceRegistrationRequest{
		SystemName:            "Provider1",
		ServiceDefinitionName: "temperature",
		Version:               "",
	}
	resp, _, err := svc.RegisterService(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %q", resp.Version)
	}
}

func TestRegisterServicePreservesExplicitVersion(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	req := model.ServiceRegistrationRequest{
		SystemName:            "Provider1",
		ServiceDefinitionName: "temperature",
		Version:               "2.3.1",
	}
	resp, _, _ := svc.RegisterService(req)
	if resp.Version != "2.3.1" {
		t.Errorf("expected version 2.3.1, got %q", resp.Version)
	}
}

func TestRegisterSystemNormalisesEmptyVersion(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	req := model.SystemRegistrationRequest{Name: "Provider1", Version: ""}
	resp, _, _ := svc.RegisterSystem(req)
	if resp.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %q", resp.Version)
	}
}

// ─── Composite ServiceInstanceID (Step 2 / G13) ───────────────────────────────

func TestServiceInstanceIDIsComposite(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	req := model.ServiceRegistrationRequest{
		SystemName:            "Provider1",
		ServiceDefinitionName: "temperature",
		Version:               "1.0.0",
	}
	resp, _, _ := svc.RegisterService(req)
	want := "Provider1|temperature|1.0.0"
	if resp.InstanceID != want {
		t.Errorf("expected instanceId %q, got %q", want, resp.InstanceID)
	}
}

func TestServiceInstanceIDStableOnUpsert(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	req := model.ServiceRegistrationRequest{
		SystemName:            "Provider1",
		ServiceDefinitionName: "temperature",
		Version:               "1.0.0",
	}
	r1, _, _ := svc.RegisterService(req)
	r2, _, _ := svc.RegisterService(req) // upsert
	if r1.InstanceID != r2.InstanceID {
		t.Errorf("instanceId changed on upsert: %q → %q", r1.InstanceID, r2.InstanceID)
	}
}

func TestServiceRevokeByCompositeID(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	req := model.ServiceRegistrationRequest{
		SystemName: "Provider1", ServiceDefinitionName: "temperature", Version: "1.0.0",
	}
	inst, _, _ := svc.RegisterService(req)
	ok := svc.RevokeService(inst.InstanceID)
	if !ok {
		t.Error("expected RevokeService to return true for existing instance")
	}
	results := svc.LookupServices(model.ServiceLookupRequest{
		InstanceIDs: []string{inst.InstanceID},
	})
	if results.Count != 0 {
		t.Error("revoked instance still appears in lookup")
	}
}

// ─── Management — Devices ─────────────────────────────────────────────────────

func TestMgmtDeviceCreateSuccess(t *testing.T) {
	svc := newAH5Service()
	resp, err := svc.CreateDevices(model.DeviceListRequest{
		Devices: []*model.DeviceRegistrationRequest{
			{Name: "D1"},
			{Name: "D2"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestMgmtDeviceCreateDuplicate(t *testing.T) {
	svc := newAH5Service()
	svc.CreateDevices(model.DeviceListRequest{Devices: []*model.DeviceRegistrationRequest{{Name: "D1"}}}) //nolint
	_, err := svc.CreateDevices(model.DeviceListRequest{Devices: []*model.DeviceRegistrationRequest{{Name: "D1"}}})
	if err == nil {
		t.Error("expected error for duplicate device")
	}
}

func TestMgmtDeviceUpdateSuccess(t *testing.T) {
	svc := newAH5Service()
	svc.CreateDevices(model.DeviceListRequest{Devices: []*model.DeviceRegistrationRequest{{Name: "D1", Metadata: map[string]string{"v": "1"}}}}) //nolint
	resp, err := svc.UpdateDevices(model.DeviceListRequest{
		Devices: []*model.DeviceRegistrationRequest{{Name: "D1", Metadata: map[string]string{"v": "2"}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Devices[0].Metadata["v"] != "2" {
		t.Errorf("metadata not updated")
	}
}

func TestMgmtDeviceUpdateNotFound(t *testing.T) {
	svc := newAH5Service()
	_, err := svc.UpdateDevices(model.DeviceListRequest{
		Devices: []*model.DeviceRegistrationRequest{{Name: "missing"}},
	})
	if err == nil {
		t.Error("expected error for missing device")
	}
}

func TestMgmtDeviceQueryAll(t *testing.T) {
	svc := newAH5Service()
	svc.CreateDevices(model.DeviceListRequest{Devices: []*model.DeviceRegistrationRequest{{Name: "x"}, {Name: "y"}}}) //nolint
	resp := svc.QueryDevices(model.DeviceLookupRequest{})
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestMgmtDeviceRemove(t *testing.T) {
	svc := newAH5Service()
	svc.CreateDevices(model.DeviceListRequest{Devices: []*model.DeviceRegistrationRequest{{Name: "D1"}, {Name: "D2"}}}) //nolint
	svc.RemoveDevices([]string{"D1"})
	resp := svc.QueryDevices(model.DeviceLookupRequest{})
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

// ─── Management — Systems ─────────────────────────────────────────────────────

func TestMgmtSystemCreateSuccess(t *testing.T) {
	svc := newAH5Service()
	resp, err := svc.CreateSystems(model.SystemListRequest{
		Systems: []*model.SystemRegistrationRequest{{Name: "Sys1"}, {Name: "Sys2"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestMgmtSystemCreateDuplicate(t *testing.T) {
	svc := newAH5Service()
	svc.CreateSystems(model.SystemListRequest{Systems: []*model.SystemRegistrationRequest{{Name: "Sys1"}}}) //nolint
	_, err := svc.CreateSystems(model.SystemListRequest{Systems: []*model.SystemRegistrationRequest{{Name: "Sys1"}}})
	if err == nil {
		t.Error("expected error for duplicate system")
	}
}

func TestMgmtSystemUpdateSuccess(t *testing.T) {
	svc := newAH5Service()
	svc.CreateSystems(model.SystemListRequest{Systems: []*model.SystemRegistrationRequest{{Name: "Sys1", Version: "1"}}}) //nolint
	resp, err := svc.UpdateSystems(model.SystemListRequest{
		Systems: []*model.SystemRegistrationRequest{{Name: "Sys1", Version: "2"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Systems[0].Version != "2" {
		t.Error("version not updated")
	}
}

func TestMgmtSystemUpdateNotFound(t *testing.T) {
	svc := newAH5Service()
	_, err := svc.UpdateSystems(model.SystemListRequest{
		Systems: []*model.SystemRegistrationRequest{{Name: "ghost"}},
	})
	if err == nil {
		t.Error("expected error for missing system")
	}
}

func TestMgmtSystemRemove(t *testing.T) {
	svc := newAH5Service()
	svc.CreateSystems(model.SystemListRequest{Systems: []*model.SystemRegistrationRequest{{Name: "Sys1"}, {Name: "Sys2"}}}) //nolint
	svc.RemoveSystems([]string{"Sys1"})
	resp := svc.QuerySystems(model.SystemLookupRequest{})
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

// ─── Management — Service Definitions ────────────────────────────────────────

func TestMgmtServiceDefCreateSuccess(t *testing.T) {
	svc := newAH5Service()
	resp, err := svc.CreateServiceDefinitions(model.ServiceDefinitionListRequest{
		ServiceDefinitionNames: []string{"temperature", "humidity"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestMgmtServiceDefCreateDuplicate(t *testing.T) {
	svc := newAH5Service()
	svc.CreateServiceDefinitions(model.ServiceDefinitionListRequest{ServiceDefinitionNames: []string{"temp"}}) //nolint
	_, err := svc.CreateServiceDefinitions(model.ServiceDefinitionListRequest{ServiceDefinitionNames: []string{"temp"}})
	if err == nil {
		t.Error("expected error for duplicate service definition")
	}
}

func TestMgmtServiceDefQueryAll(t *testing.T) {
	svc := newAH5Service()
	svc.CreateServiceDefinitions(model.ServiceDefinitionListRequest{ServiceDefinitionNames: []string{"a", "b", "c"}}) //nolint
	resp := svc.QueryServiceDefinitions()
	if resp.Count != 3 {
		t.Errorf("Count = %d, want 3", resp.Count)
	}
}

func TestMgmtServiceDefRemove(t *testing.T) {
	svc := newAH5Service()
	svc.CreateServiceDefinitions(model.ServiceDefinitionListRequest{ServiceDefinitionNames: []string{"a", "b"}}) //nolint
	svc.RemoveServiceDefinitions([]string{"a"})
	resp := svc.QueryServiceDefinitions()
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

// ─── Management — Interface Templates ────────────────────────────────────────

func TestMgmtInterfaceTemplateCreateSuccess(t *testing.T) {
	svc := newAH5Service()
	resp, err := svc.CreateInterfaceTemplates(model.InterfaceTemplateListRequest{
		InterfaceTemplates: []*model.InterfaceTemplate{
			{Name: "HTTP-SECURE-JSON", Protocol: "HTTP"},
			{Name: "MQTT-INSECURE-JSON", Protocol: "MQTT"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestMgmtInterfaceTemplateCreateDuplicate(t *testing.T) {
	svc := newAH5Service()
	svc.CreateInterfaceTemplates(model.InterfaceTemplateListRequest{
		InterfaceTemplates: []*model.InterfaceTemplate{{Name: "HTTP-JSON", Protocol: "HTTP"}},
	}) //nolint
	_, err := svc.CreateInterfaceTemplates(model.InterfaceTemplateListRequest{
		InterfaceTemplates: []*model.InterfaceTemplate{{Name: "HTTP-JSON", Protocol: "HTTP"}},
	})
	if err == nil {
		t.Error("expected error for duplicate template")
	}
}

func TestMgmtInterfaceTemplateQueryAll(t *testing.T) {
	svc := newAH5Service()
	svc.CreateInterfaceTemplates(model.InterfaceTemplateListRequest{
		InterfaceTemplates: []*model.InterfaceTemplate{
			{Name: "T1", Protocol: "HTTP"},
			{Name: "T2", Protocol: "MQTT"},
		},
	}) //nolint
	resp := svc.QueryInterfaceTemplates()
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestMgmtInterfaceTemplateRemove(t *testing.T) {
	svc := newAH5Service()
	svc.CreateInterfaceTemplates(model.InterfaceTemplateListRequest{
		InterfaceTemplates: []*model.InterfaceTemplate{
			{Name: "T1", Protocol: "HTTP"},
			{Name: "T2", Protocol: "HTTP"},
		},
	}) //nolint
	svc.RemoveInterfaceTemplates([]string{"T1"})
	resp := svc.QueryInterfaceTemplates()
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

// ─── Management — Service Instances ──────────────────────────────────────────

func TestMgmtServiceInstanceCreateSuccess(t *testing.T) {
	svc := newAH5Service()
	resp, err := svc.CreateServiceInstances(model.ServiceCreateListRequest{
		Instances: []*model.ServiceCreateRequest{
			{SystemName: "Sys1", ServiceDefinitionName: "svcA"},
			{SystemName: "Sys1", ServiceDefinitionName: "svcB"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestMgmtServiceInstanceCreateDuplicate(t *testing.T) {
	svc := newAH5Service()
	svc.CreateServiceInstances(model.ServiceCreateListRequest{
		Instances: []*model.ServiceCreateRequest{{SystemName: "Sys1", ServiceDefinitionName: "svc"}},
	}) //nolint
	_, err := svc.CreateServiceInstances(model.ServiceCreateListRequest{
		Instances: []*model.ServiceCreateRequest{{SystemName: "Sys1", ServiceDefinitionName: "svc"}},
	})
	if err == nil {
		t.Error("expected error for duplicate service instance")
	}
}

func TestMgmtServiceInstanceUpdateSuccess(t *testing.T) {
	svc := newAH5Service()
	createResp, _ := svc.CreateServiceInstances(model.ServiceCreateListRequest{
		Instances: []*model.ServiceCreateRequest{{SystemName: "Sys1", ServiceDefinitionName: "svc", ExpiresAt: "2025-01-01T00:00:00Z"}},
	})
	id := createResp.Instances[0].InstanceID
	updateResp, err := svc.UpdateServiceInstances(model.ServiceUpdateListRequest{
		Instances: []*model.ServiceUpdateRequest{{InstanceID: id, ExpiresAt: "2026-01-01T00:00:00Z"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updateResp.Instances[0].ExpiresAt != "2026-01-01T00:00:00Z" {
		t.Errorf("ExpiresAt = %q", updateResp.Instances[0].ExpiresAt)
	}
}

func TestMgmtServiceInstanceUpdateNotFound(t *testing.T) {
	svc := newAH5Service()
	_, err := svc.UpdateServiceInstances(model.ServiceUpdateListRequest{
		Instances: []*model.ServiceUpdateRequest{{InstanceID: "99999"}},
	})
	if err == nil {
		t.Error("expected error for missing instance")
	}
}

func TestMgmtServiceInstanceQueryAll(t *testing.T) {
	svc := newAH5Service()
	svc.CreateServiceInstances(model.ServiceCreateListRequest{
		Instances: []*model.ServiceCreateRequest{
			{SystemName: "Sys1", ServiceDefinitionName: "a"},
			{SystemName: "Sys2", ServiceDefinitionName: "b"},
		},
	}) //nolint
	resp := svc.QueryServiceInstances(model.ServiceLookupRequest{})
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestMgmtServiceInstanceRemove(t *testing.T) {
	svc := newAH5Service()
	createResp, _ := svc.CreateServiceInstances(model.ServiceCreateListRequest{
		Instances: []*model.ServiceCreateRequest{
			{SystemName: "Sys1", ServiceDefinitionName: "a"},
			{SystemName: "Sys1", ServiceDefinitionName: "b"},
		},
	})
	id := createResp.Instances[0].InstanceID
	svc.RemoveServiceInstances([]string{id})
	resp := svc.QueryServiceInstances(model.ServiceLookupRequest{})
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

// ─── Metadata operator tests (Step 5 / G16) ──────────────────────────────────

func TestMetadataOpEqualsTo(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	svc.RegisterService(model.ServiceRegistrationRequest{ //nolint
		SystemName: "P1", ServiceDefinitionName: "temperature", Version: "1.0.0",
		Metadata: map[string]string{"env": "prod"},
	})
	results := svc.LookupServices(model.ServiceLookupRequest{
		MetadataRequirements: map[string]model.MetadataRequirement{
			"env": {Op: model.OpEqualsTo, Value: "prod"},
		},
	})
	if results.Count != 1 {
		t.Errorf("EQUALS_TO: expected 1 match, got %d", results.Count)
	}
	results2 := svc.LookupServices(model.ServiceLookupRequest{
		MetadataRequirements: map[string]model.MetadataRequirement{
			"env": {Op: model.OpEqualsTo, Value: "staging"},
		},
	})
	if results2.Count != 0 {
		t.Errorf("EQUALS_TO non-match: expected 0, got %d", results2.Count)
	}
}

func TestMetadataOpNotEqualsTo(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	svc.RegisterService(model.ServiceRegistrationRequest{ //nolint
		SystemName: "P1", ServiceDefinitionName: "temperature", Version: "1.0.0",
		Metadata: map[string]string{"env": "prod"},
	})
	results := svc.LookupServices(model.ServiceLookupRequest{
		MetadataRequirements: map[string]model.MetadataRequirement{
			"env": {Op: model.OpNotEqualsTo, Value: "staging"},
		},
	})
	if results.Count != 1 {
		t.Errorf("NOT_EQUALS_TO: expected 1 match, got %d", results.Count)
	}
}

func TestMetadataOpContains(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	svc.RegisterService(model.ServiceRegistrationRequest{ //nolint
		SystemName: "P1", ServiceDefinitionName: "temperature", Version: "1.0.0",
		Metadata: map[string]string{"desc": "hello world"},
	})
	results := svc.LookupServices(model.ServiceLookupRequest{
		MetadataRequirements: map[string]model.MetadataRequirement{
			"desc": {Op: model.OpContains, Value: "world"},
		},
	})
	if results.Count != 1 {
		t.Errorf("CONTAINS: expected 1 match, got %d", results.Count)
	}
}

func TestMetadataOpNotContains(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	svc.RegisterService(model.ServiceRegistrationRequest{ //nolint
		SystemName: "P1", ServiceDefinitionName: "temperature", Version: "1.0.0",
		Metadata: map[string]string{"desc": "hello world"},
	})
	results := svc.LookupServices(model.ServiceLookupRequest{
		MetadataRequirements: map[string]model.MetadataRequirement{
			"desc": {Op: model.OpNotContains, Value: "foo"},
		},
	})
	if results.Count != 1 {
		t.Errorf("NOT_CONTAINS: expected 1 match, got %d", results.Count)
	}
}

func TestMetadataOpLessThanOrEqualTo(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	svc.RegisterService(model.ServiceRegistrationRequest{ //nolint
		SystemName: "P1", ServiceDefinitionName: "temperature", Version: "1.0.0",
		Metadata: map[string]string{"error": "0.5"},
	})
	results := svc.LookupServices(model.ServiceLookupRequest{
		MetadataRequirements: map[string]model.MetadataRequirement{
			"error": {Op: model.OpLessThanOrEqualsTo, Value: 1.0},
		},
	})
	if results.Count != 1 {
		t.Errorf("LESS_THAN_OR_EQUALS_TO: expected 1 match, got %d", results.Count)
	}
}

func TestMetadataOpGreaterThanOrEqualTo(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	svc.RegisterService(model.ServiceRegistrationRequest{ //nolint
		SystemName: "P1", ServiceDefinitionName: "temperature", Version: "1.0.0",
		Metadata: map[string]string{"error": "0.5"},
	})
	results := svc.LookupServices(model.ServiceLookupRequest{
		MetadataRequirements: map[string]model.MetadataRequirement{
			"error": {Op: model.OpGreaterThanOrEqualsTo, Value: 0.1},
		},
	})
	if results.Count != 1 {
		t.Errorf("GREATER_THAN_OR_EQUALS_TO: expected 1 match, got %d", results.Count)
	}
}

func TestMetadataShorthandBool(t *testing.T) {
	store := repository.NewAH5Store()
	svc := service.NewAH5RegistryService(store)
	svc.RegisterService(model.ServiceRegistrationRequest{ //nolint
		SystemName: "P1", ServiceDefinitionName: "temperature", Version: "1.0.0",
		Metadata: map[string]string{"active": "true"},
	})
	results := svc.LookupServices(model.ServiceLookupRequest{
		MetadataRequirements: map[string]model.MetadataRequirement{
			"active": {Value: true},
		},
	})
	if results.Count != 1 {
		t.Errorf("bool shorthand: expected 1, got %d", results.Count)
	}
}

func TestMetadataRequirementUnmarshalStructured(t *testing.T) {
	raw := `{"op":"CONTAINS","value":"world"}`
	var req model.MetadataRequirement
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if req.Op != model.OpContains {
		t.Errorf("expected CONTAINS, got %q", req.Op)
	}
}

func TestMetadataRequirementUnmarshalShorthand(t *testing.T) {
	raw := `"prod"`
	var req model.MetadataRequirement
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if req.Op != model.OpEqualsTo {
		t.Errorf("expected EQUALS_TO for shorthand, got %q", req.Op)
	}
	if req.Value != "prod" {
		t.Errorf("expected value prod, got %v", req.Value)
	}
}
