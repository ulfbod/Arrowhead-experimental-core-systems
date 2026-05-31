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

// HistoryQueryResponse is returned by POST mgmt/history/query.
type HistoryQueryResponse struct {
	Entries []HistoryEntry `json:"entries"`
	Count   int            `json:"count"`
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

func (s *historyStore) query() HistoryQueryResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]HistoryEntry, len(s.entries))
	copy(out, s.entries)
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
