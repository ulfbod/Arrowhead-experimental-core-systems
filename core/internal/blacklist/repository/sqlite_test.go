package repository_test

import (
	"os"
	"testing"

	"arrowhead/core/internal/blacklist/repository"
)

func TestSQLiteBlacklistSaveAndQuery(t *testing.T) {
	f, _ := os.CreateTemp("", "blacklist-*.db")
	f.Close()
	defer os.Remove(f.Name())

	repo, err := repository.NewSQLiteRepository(f.Name())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo.Save(repository.Entry{SystemName: "sql-bad", Reason: "r", Active: true})
	entries := repo.All()
	if len(entries) != 1 {
		t.Errorf("All() len = %d, want 1", len(entries))
	}
}

func TestSQLiteSetActive(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo.Save(repository.Entry{SystemName: "sys", Reason: "r", Active: true})
	ok := repo.SetActive("sys", false)
	if !ok {
		t.Error("SetActive returned false")
	}
	entries := repo.All()
	if len(entries) != 1 || entries[0].Active {
		t.Errorf("entry still active after SetActive(false): %+v", entries)
	}
}
