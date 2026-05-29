package repository

import (
	"sync"
	"time"

	"arrowhead/core/internal/blacklist/model"
)

// Entry is an alias for model.Entry kept for internal convenience.
type Entry = model.Entry

// Repository is the storage interface for blacklist entries.
type Repository interface {
	Save(e Entry) Entry
	All() []Entry
	SetActive(systemName string, active bool) bool
}

// MemoryRepository is a thread-safe in-memory Repository.
type MemoryRepository struct {
	mu      sync.RWMutex
	entries []Entry
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{}
}

func (r *MemoryRepository) Save(e Entry) Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	e.CreatedAt = now
	e.UpdatedAt = now
	r.entries = append(r.entries, e)
	return e
}

func (r *MemoryRepository) All() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, len(r.entries))
	copy(out, r.entries)
	return out
}

func (r *MemoryRepository) SetActive(systemName string, active bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	found := false
	for i, e := range r.entries {
		if e.SystemName == systemName {
			r.entries[i].Active = active
			r.entries[i].UpdatedAt = time.Now().UTC()
			found = true
		}
	}
	return found
}
