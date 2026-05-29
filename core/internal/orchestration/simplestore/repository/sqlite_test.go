package repository_test

import (
	"testing"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/simplestore/model"
	"arrowhead/core/internal/orchestration/simplestore/repository"
)

func TestSQLiteSimpleStoreRulePersists(t *testing.T) {
	dbPath := t.TempDir() + "/ss_test.db"

	repo1, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	saved := repo1.Save(model.StoreRule{
		ConsumerSystemName: "consumer-1",
		ServiceDefinition:  "temperature-service",
		Provider:           orchmodel.System{SystemName: "sensor-1", Address: "10.0.0.1", Port: 9000},
		ServiceUri:         "/temperature",
		Interfaces:         []string{"HTTP-INSECURE-JSON"},
	})
	if saved.ID == "" {
		t.Fatal("expected non-empty UUID ID")
	}
	repo1.Close()

	repo2, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer repo2.Close()
	rules := repo2.All()
	if len(rules) != 1 {
		t.Fatalf("expected 1 persisted rule, got %d", len(rules))
	}
	r := rules[0]
	if r.ConsumerSystemName != "consumer-1" || r.Provider.SystemName != "sensor-1" {
		t.Errorf("unexpected rule: %+v", r)
	}
	if len(r.Interfaces) != 1 || r.Interfaces[0] != "HTTP-INSECURE-JSON" {
		t.Errorf("interfaces not persisted: %v", r.Interfaces)
	}
}

func TestSQLiteSimpleStoreDelete(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	defer repo.Close()
	saved := repo.Save(model.StoreRule{
		ConsumerSystemName: "c", ServiceDefinition: "s",
		Provider:   orchmodel.System{SystemName: "p", Address: "a", Port: 1},
		ServiceUri: "/x", Interfaces: []string{"HTTP"},
	})
	if !repo.Delete(saved.ID) {
		t.Fatal("Delete returned false")
	}
	if len(repo.All()) != 0 {
		t.Fatal("expected empty after delete")
	}
}

func TestSQLiteSimpleStoreUpdatePriority(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	defer repo.Close()
	saved := repo.Save(model.StoreRule{
		ConsumerSystemName: "c", ServiceDefinition: "s",
		Provider:   orchmodel.System{SystemName: "p", Address: "a", Port: 1},
		ServiceUri: "/x", Interfaces: []string{"HTTP"},
	})
	if !repo.UpdatePriority(saved.ID, 10) {
		t.Fatal("UpdatePriority returned false")
	}
	rules := repo.All()
	if len(rules) != 1 || rules[0].Priority != 10 {
		t.Errorf("priority not updated: %+v", rules)
	}
}
