package service_test

import (
	"testing"

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
		Name:      "gateway-1",
		Metadata:  map[string]string{"location": "room-a"},
		Addresses: []model.Address{{Type: "MAC", Address: "aa:bb:cc:dd:ee:ff"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true for first registration")
	}
	if dev.Name != "gateway-1" {
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
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "dev-1", Metadata: map[string]string{"v": "1"}}) //nolint
	dev, created, _ := svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "dev-1", Metadata: map[string]string{"v": "2"}})
	if created {
		t.Error("expected created=false for duplicate")
	}
	if dev.Metadata["v"] != "2" {
		t.Errorf("metadata not updated, got %q", dev.Metadata["v"])
	}
}

func TestDeviceLookupAll(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "d1"}) //nolint
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "d2"}) //nolint
	resp := svc.LookupDevices(model.DeviceLookupRequest{})
	if resp.Count != 2 {
		t.Errorf("Count = %d, want 2", resp.Count)
	}
}

func TestDeviceLookupByName(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "alpha"}) //nolint
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "beta"})  //nolint
	resp := svc.LookupDevices(model.DeviceLookupRequest{DeviceNames: []string{"alpha"}})
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
	if resp.Entries[0].Name != "alpha" {
		t.Errorf("Name = %q, want alpha", resp.Entries[0].Name)
	}
}

func TestDeviceRevokeFound(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "rem"}) //nolint
	if !svc.RevokeDevice("rem") {
		t.Error("expected true for existing device")
	}
	resp := svc.LookupDevices(model.DeviceLookupRequest{})
	if resp.Count != 0 {
		t.Errorf("expected 0 after revoke, got %d", resp.Count)
	}
}

func TestDeviceRevokeNotFound(t *testing.T) {
	svc := newAH5Service()
	if svc.RevokeDevice("nonexistent") {
		t.Error("expected false for missing device")
	}
}

// ─── System Discovery ─────────────────────────────────────────────────────────

func TestSystemRegisterValid(t *testing.T) {
	svc := newAH5Service()
	sys, created, err := svc.RegisterSystem(model.SystemRegistrationRequest{
		Name:      "sensor-system",
		Version:   "1.0",
		Addresses: []model.Address{{Type: "IP", Address: "10.0.0.1"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true for first registration")
	}
	if sys.Name != "sensor-system" {
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
	svc.RegisterSystem(model.SystemRegistrationRequest{Name: "sys-1", Version: "1"}) //nolint
	sys, created, _ := svc.RegisterSystem(model.SystemRegistrationRequest{Name: "sys-1", Version: "2"})
	if created {
		t.Error("expected created=false on update")
	}
	if sys.Version != "2" {
		t.Errorf("Version = %q, want 2", sys.Version)
	}
}

func TestSystemLookupByName(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterSystem(model.SystemRegistrationRequest{Name: "s1"}) //nolint
	svc.RegisterSystem(model.SystemRegistrationRequest{Name: "s2"}) //nolint
	resp := svc.LookupSystems(model.SystemLookupRequest{SystemNames: []string{"s1"}})
	if resp.Count != 1 || resp.Entries[0].Name != "s1" {
		t.Errorf("unexpected lookup result: %+v", resp)
	}
}

func TestSystemRevokeFound(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterSystem(model.SystemRegistrationRequest{Name: "rem-sys"}) //nolint
	if !svc.RevokeSystem("rem-sys") {
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
		SystemName:            "provider-1",
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
	if inst.Provider == nil || inst.Provider.Name != "provider-1" {
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
		SystemName: "sys",
	})
	if err == nil {
		t.Error("expected error for missing serviceDefinitionName")
	}
}

func TestServiceRegisterUpsert(t *testing.T) {
	svc := newAH5Service()
	inst1, _, _ := svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName:            "p1",
		ServiceDefinitionName: "svc",
		Version:               "1",
		Metadata:              map[string]string{"k": "v1"},
	})
	inst2, created, _ := svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName:            "p1",
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
	svc.RegisterService(model.ServiceRegistrationRequest{SystemName: "p1", ServiceDefinitionName: "svc"}) //nolint
	svc.RegisterService(model.ServiceRegistrationRequest{SystemName: "p2", ServiceDefinitionName: "svc"}) //nolint
	resp := svc.LookupServices(model.ServiceLookupRequest{ProviderNames: []string{"p1"}})
	if resp.Count != 1 || resp.Entries[0].Provider.Name != "p1" {
		t.Errorf("unexpected result: %+v", resp)
	}
}

func TestServiceLookupByDefinitionName(t *testing.T) {
	svc := newAH5Service()
	svc.RegisterService(model.ServiceRegistrationRequest{SystemName: "p1", ServiceDefinitionName: "temp"})     //nolint
	svc.RegisterService(model.ServiceRegistrationRequest{SystemName: "p1", ServiceDefinitionName: "humidity"}) //nolint
	resp := svc.LookupServices(model.ServiceLookupRequest{ServiceDefinitionNames: []string{"temp"}})
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

func TestServiceLookupByInstanceID(t *testing.T) {
	svc := newAH5Service()
	inst, _, _ := svc.RegisterService(model.ServiceRegistrationRequest{
		SystemName:            "p",
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
		SystemName:            "p",
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

// ─── Management — Devices ─────────────────────────────────────────────────────

func TestMgmtDeviceCreateSuccess(t *testing.T) {
	svc := newAH5Service()
	resp, err := svc.CreateDevices(model.DeviceListRequest{
		Devices: []*model.DeviceRegistrationRequest{
			{Name: "d1"},
			{Name: "d2"},
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
	svc.CreateDevices(model.DeviceListRequest{Devices: []*model.DeviceRegistrationRequest{{Name: "d1"}}}) //nolint
	_, err := svc.CreateDevices(model.DeviceListRequest{Devices: []*model.DeviceRegistrationRequest{{Name: "d1"}}})
	if err == nil {
		t.Error("expected error for duplicate device")
	}
}

func TestMgmtDeviceUpdateSuccess(t *testing.T) {
	svc := newAH5Service()
	svc.CreateDevices(model.DeviceListRequest{Devices: []*model.DeviceRegistrationRequest{{Name: "d1", Metadata: map[string]string{"v": "1"}}}}) //nolint
	resp, err := svc.UpdateDevices(model.DeviceListRequest{
		Devices: []*model.DeviceRegistrationRequest{{Name: "d1", Metadata: map[string]string{"v": "2"}}},
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
	svc.CreateDevices(model.DeviceListRequest{Devices: []*model.DeviceRegistrationRequest{{Name: "d1"}, {Name: "d2"}}}) //nolint
	svc.RemoveDevices([]string{"d1"})
	resp := svc.QueryDevices(model.DeviceLookupRequest{})
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}

// ─── Management — Systems ─────────────────────────────────────────────────────

func TestMgmtSystemCreateSuccess(t *testing.T) {
	svc := newAH5Service()
	resp, err := svc.CreateSystems(model.SystemListRequest{
		Systems: []*model.SystemRegistrationRequest{{Name: "s1"}, {Name: "s2"}},
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
	svc.CreateSystems(model.SystemListRequest{Systems: []*model.SystemRegistrationRequest{{Name: "s1"}}}) //nolint
	_, err := svc.CreateSystems(model.SystemListRequest{Systems: []*model.SystemRegistrationRequest{{Name: "s1"}}})
	if err == nil {
		t.Error("expected error for duplicate system")
	}
}

func TestMgmtSystemUpdateSuccess(t *testing.T) {
	svc := newAH5Service()
	svc.CreateSystems(model.SystemListRequest{Systems: []*model.SystemRegistrationRequest{{Name: "s1", Version: "1"}}}) //nolint
	resp, err := svc.UpdateSystems(model.SystemListRequest{
		Systems: []*model.SystemRegistrationRequest{{Name: "s1", Version: "2"}},
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
	svc.CreateSystems(model.SystemListRequest{Systems: []*model.SystemRegistrationRequest{{Name: "s1"}, {Name: "s2"}}}) //nolint
	svc.RemoveSystems([]string{"s1"})
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
			{SystemName: "s1", ServiceDefinitionName: "svc-a"},
			{SystemName: "s1", ServiceDefinitionName: "svc-b"},
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
		Instances: []*model.ServiceCreateRequest{{SystemName: "s1", ServiceDefinitionName: "svc"}},
	}) //nolint
	_, err := svc.CreateServiceInstances(model.ServiceCreateListRequest{
		Instances: []*model.ServiceCreateRequest{{SystemName: "s1", ServiceDefinitionName: "svc"}},
	})
	if err == nil {
		t.Error("expected error for duplicate service instance")
	}
}

func TestMgmtServiceInstanceUpdateSuccess(t *testing.T) {
	svc := newAH5Service()
	createResp, _ := svc.CreateServiceInstances(model.ServiceCreateListRequest{
		Instances: []*model.ServiceCreateRequest{{SystemName: "s1", ServiceDefinitionName: "svc", ExpiresAt: "2025-01-01T00:00:00Z"}},
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
			{SystemName: "s1", ServiceDefinitionName: "a"},
			{SystemName: "s2", ServiceDefinitionName: "b"},
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
			{SystemName: "s1", ServiceDefinitionName: "a"},
			{SystemName: "s1", ServiceDefinitionName: "b"},
		},
	})
	id := createResp.Instances[0].InstanceID
	svc.RemoveServiceInstances([]string{id})
	resp := svc.QueryServiceInstances(model.ServiceLookupRequest{})
	if resp.Count != 1 {
		t.Errorf("Count = %d, want 1", resp.Count)
	}
}
