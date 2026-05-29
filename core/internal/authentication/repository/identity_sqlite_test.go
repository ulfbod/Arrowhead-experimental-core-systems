package repository_test

import (
	"os"
	"testing"

	"arrowhead/core/internal/authentication/repository"
)

func TestSQLiteIdentitySaveAndGet(t *testing.T) {
	f, _ := os.CreateTemp("", "auth-identity-*.db")
	f.Close()
	defer os.Remove(f.Name())

	repo, err := repository.NewSQLiteIdentityRepository(f.Name())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo.Save(repository.Identity{SystemName: "sql-sys", PasswordHash: "h", Sysop: true})
	got, ok := repo.Get("sql-sys")
	if !ok {
		t.Fatal("not found after save")
	}
	if !got.Sysop {
		t.Error("Sysop = false, want true")
	}
}
