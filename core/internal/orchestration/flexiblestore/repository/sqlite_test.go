package repository_test

import (
	"testing"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/flexiblestore/model"
	"arrowhead/core/internal/orchestration/flexiblestore/repository"
)

func TestSQLiteFlexStoreRulePersists(t *testing.T) {
	dbPath := t.TempDir() + "/fs_test.db"

	repo1, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	saved := repo1.Save(model.FlexibleRule{
		ConsumerSystemName: "consumer-1",
		ServiceDefinition:  "temperature-service",
		Provider:           orchmodel.System{SystemName: "sensor-1", Address: "10.0.0.1", Port: 9000},
		ServiceUri:         "/temperature",
		Interfaces:         []string{"HTTP-INSECURE-JSON"},
		Priority:           5,
		MetadataFilter:     map[string]string{"region": "eu"},
	})
	if saved.ID == 0 {
		t.Fatal("expected non-zero ID")
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
	if r.Priority != 5 {
		t.Errorf("priority not persisted: got %d", r.Priority)
	}
	if r.MetadataFilter["region"] != "eu" {
		t.Errorf("metadataFilter not persisted: %v", r.MetadataFilter)
	}
}

func TestSQLiteFlexStoreDelete(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	defer repo.Close()
	saved := repo.Save(model.FlexibleRule{
		ConsumerSystemName: "c", ServiceDefinition: "s",
		Provider:   orchmodel.System{SystemName: "p", Address: "a", Port: 1},
		ServiceUri: "/x", Interfaces: []string{"HTTP"}, Priority: 1,
	})
	if !repo.Delete(saved.ID) {
		t.Fatal("Delete returned false")
	}
	if len(repo.All()) != 0 {
		t.Fatal("expected empty after delete")
	}
}
