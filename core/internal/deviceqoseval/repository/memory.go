// Package repository provides in-memory storage for Device QoS Evaluator records.
package repository

import (
	"sync"

	"arrowhead/core/internal/deviceqoseval/model"
)

// Repository defines the storage interface for QoS records.
type Repository interface {
	Save(r *model.QoSRecord)
	All() []*model.QoSRecord
}

// MemoryRepository is a thread-safe in-memory QoS record store.
type MemoryRepository struct {
	mu      sync.RWMutex
	records []*model.QoSRecord
}

// NewMemoryRepository creates a new empty MemoryRepository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{}
}

// Save appends a record to the store.
func (r *MemoryRepository) Save(rec *model.QoSRecord) {
	r.mu.Lock()
	r.records = append(r.records, rec)
	r.mu.Unlock()
}

// All returns a snapshot of all stored records.
func (r *MemoryRepository) All() []*model.QoSRecord {
	r.mu.RLock()
	out := make([]*model.QoSRecord, len(r.records))
	copy(out, r.records)
	r.mu.RUnlock()
	return out
}
