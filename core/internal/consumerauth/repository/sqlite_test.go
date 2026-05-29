package repository_test

import (
	"testing"

	"arrowhead/core/internal/consumerauth/model"
	"arrowhead/core/internal/consumerauth/repository"
)

func TestSQLiteAuthPolicyPersists(t *testing.T) {
	dbPath := t.TempDir() + "/ca_test.db"

	policy := model.AuthPolicy{
		InstanceID:     model.BuildInstanceID("provider-1", model.TargetServiceDef, "temperature-service"),
		AuthLevel:      "PR",
		Cloud:          "LOCAL",
		Provider:       "provider-1",
		TargetType:     model.TargetServiceDef,
		Target:         "temperature-service",
		DefaultPolicy:  model.PolicyDef{PolicyType: model.PolicyWhitelist, PolicyList: []string{"consumer-1"}},
		ScopedPolicies: map[string]model.PolicyDef{},
	}

	repo1, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	repo1.Save(policy)
	repo1.Close()

	repo2, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer repo2.Close()

	all := repo2.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 persisted policy, got %d", len(all))
	}
	p := all[0]
	if p.InstanceID != policy.InstanceID {
		t.Errorf("InstanceID = %q, want %q", p.InstanceID, policy.InstanceID)
	}
	if p.DefaultPolicy.PolicyType != model.PolicyWhitelist {
		t.Errorf("PolicyType = %q, want WHITELIST", p.DefaultPolicy.PolicyType)
	}
	if len(p.DefaultPolicy.PolicyList) != 1 || p.DefaultPolicy.PolicyList[0] != "consumer-1" {
		t.Errorf("PolicyList = %v", p.DefaultPolicy.PolicyList)
	}
}

func TestSQLiteAuthPolicyDelete(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	defer repo.Close()

	instanceID := model.BuildInstanceID("prov", model.TargetServiceDef, "svc")
	repo.Save(model.AuthPolicy{
		InstanceID:     instanceID,
		AuthLevel:      "PR",
		Cloud:          "LOCAL",
		Provider:       "prov",
		TargetType:     model.TargetServiceDef,
		Target:         "svc",
		DefaultPolicy:  model.PolicyDef{PolicyType: model.PolicyAll},
		ScopedPolicies: map[string]model.PolicyDef{},
	})

	if !repo.Delete(instanceID) {
		t.Fatal("Delete returned false for existing policy")
	}
	if repo.Delete(instanceID) {
		t.Fatal("Delete returned true for already-deleted policy")
	}
	if len(repo.All()) != 0 {
		t.Fatal("expected empty after delete")
	}
}
