package service

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// HistoryEntry records a single orchestration job.
type HistoryEntry struct {
	ID                string     `json:"id"`
	Status            string     `json:"status"`            // DONE | ERROR
	Type              string     `json:"type"`              // PULL
	RequesterSystem   string     `json:"requesterSystem,omitempty"`
	ServiceDefinition string     `json:"serviceDefinition,omitempty"`
	Message           string     `json:"message,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	FinishedAt        *time.Time `json:"finishedAt,omitempty"`
}

// HistoryQueryFilter holds optional filter parameters for history queries.
// Empty fields are ignored (no filter for that field).
type HistoryQueryFilter struct {
	// RequesterSystemName filters by exact match on RequesterSystem. Empty = no filter.
	RequesterSystemName string
	// ServiceDefinition filters by exact match on ServiceDefinition. Empty = no filter.
	ServiceDefinition string
	// Status filters by exact match on Status (DONE, ERROR, etc.). Empty = no filter.
	Status string
	// From is an inclusive RFC3339 lower bound on CreatedAt. Empty = no lower bound.
	From string
	// To is an inclusive RFC3339 upper bound on CreatedAt. Empty = no upper bound.
	To string
}

// HistoryQueryResponse is returned by POST mgmt/history/query.
type HistoryQueryResponse struct {
	Entries []HistoryEntry `json:"entries"`
	Count   int            `json:"count"`
}

// HistoryStore is the exported handle for a history store (used by tests).
// Internal code uses the unexported historyStore directly.
type HistoryStore struct {
	s *historyStore
}

// NewHistoryStoreForTest creates an exported HistoryStore for use in tests.
func NewHistoryStoreForTest() *HistoryStore {
	return &HistoryStore{s: newHistoryStore()}
}

// AddEntry adds a history entry to the test store.
func (h *HistoryStore) AddEntry(e HistoryEntry) {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	h.s.add(e)
}

// Query applies the filter and returns matching entries.
func (h *HistoryStore) Query(f HistoryQueryFilter) HistoryQueryResponse {
	return h.s.query(f)
}

type historyStore struct {
	mu      sync.RWMutex
	entries []HistoryEntry
}

func newHistoryStore() *historyStore {
	return &historyStore{}
}

// add appends a history entry and returns its ID.
func (s *historyStore) add(e HistoryEntry) string {
	s.mu.Lock()
	s.entries = append(s.entries, e)
	s.mu.Unlock()
	return e.ID
}

// updateStatus finds an entry by ID and updates its status and FinishedAt.
func (s *historyStore) updateStatus(id, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i, e := range s.entries {
		if e.ID == id {
			s.entries[i].Status = status
			s.entries[i].FinishedAt = &now
			return
		}
	}
}

// query returns entries matching the given filter. Empty fields = no filter.
func (s *historyStore) query(f HistoryQueryFilter) HistoryQueryResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Parse time bounds once.
	var fromT, toT time.Time
	if f.From != "" {
		fromT, _ = time.Parse(time.RFC3339, f.From)
	}
	if f.To != "" {
		toT, _ = time.Parse(time.RFC3339, f.To)
	}

	out := make([]HistoryEntry, 0, len(s.entries))
	for _, e := range s.entries {
		if f.RequesterSystemName != "" && e.RequesterSystem != f.RequesterSystemName {
			continue
		}
		if f.ServiceDefinition != "" && e.ServiceDefinition != f.ServiceDefinition {
			continue
		}
		if f.Status != "" && e.Status != f.Status {
			continue
		}
		if !fromT.IsZero() && e.CreatedAt.Before(fromT) {
			continue
		}
		if !toT.IsZero() && e.CreatedAt.After(toT) {
			continue
		}
		out = append(out, e)
	}
	return HistoryQueryResponse{Entries: out, Count: len(out)}
}

func newHistoryEntry(requester, service, status, message string) HistoryEntry {
	return newHistoryEntryTyped(requester, service, status, message, "PULL")
}

func newHistoryEntryTyped(requester, service, status, message, entryType string) HistoryEntry {
	now := time.Now()
	return HistoryEntry{
		ID:                uuid.NewString(),
		Status:            status,
		Type:              entryType,
		RequesterSystem:   requester,
		ServiceDefinition: service,
		Message:           message,
		CreatedAt:         now,
		FinishedAt:        &now,
	}
}
