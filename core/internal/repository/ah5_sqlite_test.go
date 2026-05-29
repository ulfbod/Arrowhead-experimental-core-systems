package repository_test

import (
	"testing"

	"arrowhead/core/internal/model"
	"arrowhead/core/internal/repository"
)

func TestSQLiteAH5ServiceRegistryPersists(t *testing.T) {
	dbPath := t.TempDir() + "/ah5_test.db"

	store1, err := repository.NewAH5SQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewAH5SQLiteStore: %v", err)
	}
	store1.SaveDevice(&model.DeviceRegistrationRequest{Name: "gateway-1"})
	store1.SaveSystem(&model.SystemRegistrationRequest{Name: "sensor-1", Version: "1.0.0"})
	store1.SaveServiceDefinitions([]string{"temperature"})
	store1.SaveServiceInstance(&model.ServiceRegistrationRequest{
		SystemName: "sensor-1", ServiceDefinitionName: "temperature", Version: "1.0.0",
	})
	store1.Close()

	store2, err := repository.NewAH5SQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer store2.Close()

	devs := store2.AllDevices()
	if len(devs) != 1 || devs[0].Name != "gateway-1" {
		t.Errorf("devices not persisted: %v", devs)
	}
	syss := store2.AllSystems()
	if len(syss) != 1 || syss[0].Name != "sensor-1" {
		t.Errorf("systems not persisted: %v", syss)
	}
	defs := store2.AllServiceDefinitions()
	if len(defs) != 1 || defs[0].Name != "temperature" {
		t.Errorf("service definitions not persisted: %v", defs)
	}
	insts := store2.AllServiceInstances()
	if len(insts) != 1 {
		t.Fatalf("service instances not persisted: %v", insts)
	}
	if insts[0].ServiceDefinitionName != "temperature" {
		t.Errorf("wrong service def name: %q", insts[0].ServiceDefinitionName)
	}
}

func TestSQLiteAH5HasDependentSystems(t *testing.T) {
	store, err := repository.NewAH5SQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewAH5SQLiteStore: %v", err)
	}
	defer store.Close()

	store.SaveDevice(&model.DeviceRegistrationRequest{Name: "gw"})
	store.SaveSystem(&model.SystemRegistrationRequest{Name: "sys-1", DeviceName: "gw", Version: "1.0"})

	if !store.HasDependentSystems("gw") {
		t.Error("expected dependent systems for gw")
	}
	if store.HasDependentSystems("other") {
		t.Error("expected no dependent systems for other")
	}
}
