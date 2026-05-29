package repository_test

import (
	"testing"
	"time"

	"arrowhead/core/internal/ca/repository"
)

func TestSQLiteRevocationPersists(t *testing.T) {
	dbPath := t.TempDir() + "/ca_rev_test.db"

	repo1, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	repo1.AddRevocation("12345", "sensor-1", now)
	repo1.Close()

	repo2, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer repo2.Close()

	if !repo2.IsRevoked("12345") {
		t.Fatal("revocation not persisted after reopen")
	}
	revs := repo2.AllRevocations()
	if len(revs) != 1 {
		t.Fatalf("expected 1 revocation, got %d", len(revs))
	}
	if revs[0].SystemName != "sensor-1" {
		t.Errorf("systemName not persisted: %q", revs[0].SystemName)
	}
}

func TestSQLiteRevocationIdempotent(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	defer repo.Close()

	repo.AddRevocation("serial-1", "sys", time.Now())
	repo.AddRevocation("serial-1", "sys", time.Now()) // duplicate

	if len(repo.AllRevocations()) != 1 {
		t.Error("duplicate revocation should not be stored twice")
	}
}

func TestSQLiteSerialPersists(t *testing.T) {
	dbPath := t.TempDir() + "/serial_test.db"

	repo1, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	s1 := repo1.IncrementSerial()
	s2 := repo1.IncrementSerial()
	if s1 != 3 || s2 != 4 {
		t.Errorf("unexpected serials: %d, %d", s1, s2)
	}
	repo1.Close()

	repo2, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer repo2.Close()

	// After reopen, serial should continue from 4 (not restart at 2)
	s3 := repo2.IncrementSerial()
	if s3 != 5 {
		t.Errorf("serial should continue from 5 after reopen, got %d", s3)
	}
}
