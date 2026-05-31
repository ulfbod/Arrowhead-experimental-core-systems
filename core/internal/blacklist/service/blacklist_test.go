package service_test

import (
	"testing"
	"time"

	"arrowhead/core/internal/blacklist/repository"
	"arrowhead/core/internal/blacklist/service"
)

func TestIsBlacklistedTrue(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	svc.Add("bad-actor", "malicious behavior", time.Time{}, "admin")
	if !svc.IsBlacklisted("bad-actor") {
		t.Error("IsBlacklisted = false, want true")
	}
}

func TestIsBlacklistedFalse(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	if svc.IsBlacklisted("clean") {
		t.Error("IsBlacklisted = true for unknown system")
	}
}

func TestRemoveInactivatesNotDeletes(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	svc.Add("sys-a", "reason", time.Time{}, "admin")
	svc.Remove("sys-a")
	if svc.IsBlacklisted("sys-a") {
		t.Error("IsBlacklisted = true after Remove — should be inactivated")
	}
	// Entry must still exist in query results (with active: false).
	entries := svc.Query(nil)
	found := false
	for _, e := range entries {
		if e.SystemName == "sys-a" {
			found = true
			if e.Active {
				t.Error("entry active = true after Remove")
			}
		}
	}
	if !found {
		t.Error("entry not found after Remove — was deleted instead of inactivated")
	}
}

func TestIsBlacklistedExpiredEntry(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	past := time.Now().Add(-time.Hour)
	svc.Add("expired-sys", "temp ban", past, "admin")
	if svc.IsBlacklisted("expired-sys") {
		t.Error("IsBlacklisted = true for expired entry")
	}
}

// ─── Step 45 — PurgeExpired ───────────────────────────────────────────────────

func TestBlacklistPurgeExpiredRemovesExpiredEntry(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	past := time.Now().Add(-1 * time.Second)
	svc.Add("expired-sys", "temp ban", past, "admin")

	svc.PurgeExpired()

	entries := svc.Query(nil)
	for _, e := range entries {
		if e.SystemName == "expired-sys" {
			t.Errorf("PurgeExpired: expired entry still present: %+v", e)
		}
	}
}

func TestBlacklistPurgeExpiredKeepsFutureEntry(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	future := time.Now().Add(1 * time.Hour)
	svc.Add("future-sys", "temp ban", future, "admin")

	svc.PurgeExpired()

	entries := svc.Query(nil)
	found := false
	for _, e := range entries {
		if e.SystemName == "future-sys" {
			found = true
		}
	}
	if !found {
		t.Error("PurgeExpired: future entry was incorrectly removed")
	}
}

func TestBlacklistPurgeExpiredKeepsPermanentEntry(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	// Zero-value expiresAt = permanent
	svc.Add("permanent-sys", "permanent ban", time.Time{}, "admin")

	svc.PurgeExpired()

	entries := svc.Query(nil)
	found := false
	for _, e := range entries {
		if e.SystemName == "permanent-sys" {
			found = true
		}
	}
	if !found {
		t.Error("PurgeExpired: permanent entry (no expiry) was incorrectly removed")
	}
}
