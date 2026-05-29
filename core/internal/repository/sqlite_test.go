package repository_test

import (
	"testing"

	"arrowhead/core/internal/model"
	"arrowhead/core/internal/repository"
)

func TestSQLiteServiceRegistryPersists(t *testing.T) {
	dbPath := t.TempDir() + "/sr_test.db"

	repo1, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	repo1.Save(&model.ServiceInstance{
		ServiceDefinition: "temperature-service",
		ProviderSystem:    model.System{SystemName: "sensor-1", Address: "10.0.0.1", Port: 9000},
		ServiceUri:        "/temperature",
		Interfaces:        []string{"HTTP-INSECURE-JSON"},
		Version:           1,
	})
	repo1.Close()

	repo2, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer repo2.Close()
	all := repo2.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 persisted service, got %d", len(all))
	}
	s := all[0]
	if s.ServiceDefinition != "temperature-service" || s.ProviderSystem.SystemName != "sensor-1" {
		t.Errorf("unexpected service: %+v", s)
	}
}

func TestSQLiteServiceRegistryUpsert(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	defer repo.Close()

	svc := &model.ServiceInstance{
		ServiceDefinition: "svc", ProviderSystem: model.System{SystemName: "p", Address: "a", Port: 1},
		ServiceUri: "/old", Interfaces: []string{"HTTP"}, Version: 1,
	}
	saved1 := repo.Save(svc)
	svc.ServiceUri = "/new"
	saved2 := repo.Save(svc)

	if saved1.ID != saved2.ID {
		t.Errorf("upsert should return same ID: %d vs %d", saved1.ID, saved2.ID)
	}
	all := repo.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 after upsert, got %d", len(all))
	}
	if all[0].ServiceUri != "/new" {
		t.Errorf("ServiceUri not updated: %q", all[0].ServiceUri)
	}
}

func TestSQLiteServiceRegistryDelete(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	defer repo.Close()

	svc := &model.ServiceInstance{
		ServiceDefinition: "svc", ProviderSystem: model.System{SystemName: "p", Address: "a", Port: 1},
		ServiceUri: "/x", Interfaces: []string{"HTTP"}, Version: 1,
	}
	repo.Save(svc)
	if !repo.Delete("svc", "p", "a", 1, 1) {
		t.Fatal("Delete returned false for existing service")
	}
	if len(repo.All()) != 0 {
		t.Fatal("expected empty after delete")
	}
}
