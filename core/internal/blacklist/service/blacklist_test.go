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
