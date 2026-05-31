package service_test

import (
	"testing"
	"time"

	"arrowhead/core/internal/orchestration/dynamic/service"
)

// newHistoryStoreWithEntries creates a historyStore via the exported constructor
// and populates it via the orchestrator add path. Since historyStore is unexported,
// we use the exported HistoryEntry type and test through the public orchestrator API.
// But we can also test the filter directly by testing QueryHistory with a filter.

// buildEntry is a helper to create a service.HistoryEntry for testing.
func buildEntry(requester, svcDef, status string, startedAt time.Time) service.HistoryEntry {
	return service.HistoryEntry{
		RequesterSystem:   requester,
		ServiceDefinition: svcDef,
		Status:            status,
		CreatedAt:         startedAt,
	}
}

// TestHistoryQueryFilterByRequester confirms that filtering by requesterSystemName
// returns only entries with matching RequesterSystem.
func TestHistoryQueryFilterByRequester(t *testing.T) {
	hs := service.NewHistoryStoreForTest()
	hs.AddEntry(buildEntry("alpha", "temp", "DONE", time.Now()))
	hs.AddEntry(buildEntry("beta", "temp", "DONE", time.Now()))
	hs.AddEntry(buildEntry("alpha", "humidity", "DONE", time.Now()))

	resp := hs.Query(service.HistoryQueryFilter{RequesterSystemName: "alpha"})
	if resp.Count != 2 {
		t.Errorf("filter by requester alpha: want 2, got %d", resp.Count)
	}
	for _, e := range resp.Entries {
		if e.RequesterSystem != "alpha" {
			t.Errorf("unexpected requester %q in filtered results", e.RequesterSystem)
		}
	}
}

// TestHistoryQueryFilterByStatus confirms that filtering by status returns only
// entries with the matching status.
func TestHistoryQueryFilterByStatus(t *testing.T) {
	hs := service.NewHistoryStoreForTest()
	hs.AddEntry(buildEntry("sys", "svc", "DONE", time.Now()))
	hs.AddEntry(buildEntry("sys", "svc", "ERROR", time.Now()))
	hs.AddEntry(buildEntry("sys", "svc", "DONE", time.Now()))

	resp := hs.Query(service.HistoryQueryFilter{Status: "ERROR"})
	if resp.Count != 1 {
		t.Errorf("filter by ERROR status: want 1, got %d", resp.Count)
	}
	if resp.Entries[0].Status != "ERROR" {
		t.Errorf("expected status ERROR, got %q", resp.Entries[0].Status)
	}
}

// TestHistoryQueryFilterByDateRange confirms that from/to bounds on StartedAt work.
func TestHistoryQueryFilterByDateRange(t *testing.T) {
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	hs := service.NewHistoryStoreForTest()
	hs.AddEntry(buildEntry("sys", "svc", "DONE", base.Add(-1*time.Hour)))   // before range
	hs.AddEntry(buildEntry("sys", "svc", "DONE", base))                     // at from bound
	hs.AddEntry(buildEntry("sys", "svc", "DONE", base.Add(1*time.Hour)))    // inside
	hs.AddEntry(buildEntry("sys", "svc", "DONE", base.Add(2*time.Hour)))    // at to bound
	hs.AddEntry(buildEntry("sys", "svc", "DONE", base.Add(3*time.Hour)))    // after range

	from := base.Format(time.RFC3339)
	to := base.Add(2 * time.Hour).Format(time.RFC3339)
	resp := hs.Query(service.HistoryQueryFilter{From: from, To: to})
	if resp.Count != 3 {
		t.Errorf("date range filter: want 3, got %d (entries: %+v)", resp.Count, resp.Entries)
	}
}

// TestHistoryQueryNoFilterReturnsAll confirms that an empty filter returns all entries.
func TestHistoryQueryNoFilterReturnsAll(t *testing.T) {
	hs := service.NewHistoryStoreForTest()
	hs.AddEntry(buildEntry("a", "svc1", "DONE", time.Now()))
	hs.AddEntry(buildEntry("b", "svc2", "ERROR", time.Now()))
	hs.AddEntry(buildEntry("c", "svc3", "DONE", time.Now()))

	resp := hs.Query(service.HistoryQueryFilter{})
	if resp.Count != 3 {
		t.Errorf("no filter: want 3, got %d", resp.Count)
	}
}
