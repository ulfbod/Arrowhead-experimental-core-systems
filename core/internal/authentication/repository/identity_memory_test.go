package repository_test

import (
	"testing"

	"arrowhead/core/internal/authentication/repository"
)

func TestIdentityMemorySaveAndGet(t *testing.T) {
	repo := repository.NewMemoryIdentityRepository()
	repo.Save(repository.Identity{SystemName: "sys-a", PasswordHash: "hash1", Sysop: false})
	got, ok := repo.Get("sys-a")
	if !ok {
		t.Fatal("Get returned false after Save")
	}
	if got.SystemName != "sys-a" {
		t.Errorf("SystemName = %q", got.SystemName)
	}
}

func TestIdentityMemoryDelete(t *testing.T) {
	repo := repository.NewMemoryIdentityRepository()
	repo.Save(repository.Identity{SystemName: "to-delete"})
	repo.Delete("to-delete")
	_, ok := repo.Get("to-delete")
	if ok {
		t.Error("Get returned true after Delete")
	}
}

func TestIdentityMemoryAll(t *testing.T) {
	repo := repository.NewMemoryIdentityRepository()
	repo.Save(repository.Identity{SystemName: "a"})
	repo.Save(repository.Identity{SystemName: "b"})
	all := repo.All()
	if len(all) != 2 {
		t.Errorf("All() len = %d, want 2", len(all))
	}
}
