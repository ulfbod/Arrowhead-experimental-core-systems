package repository

import (
	"sync"
	"sync/atomic"
	"time"
)

// MemoryRepository is a thread-safe in-memory CA repository (for tests).
type MemoryRepository struct {
	serial     atomic.Int64
	mu         sync.RWMutex
	revocations []Revocation
	revokedSet  map[string]struct{}
}

func NewMemoryRepository() *MemoryRepository {
	r := &MemoryRepository{revokedSet: make(map[string]struct{})}
	r.serial.Store(2) // 1 is the CA root
	return r
}

func (r *MemoryRepository) NextSerial() int64 { return r.serial.Load() }

func (r *MemoryRepository) IncrementSerial() int64 { return r.serial.Add(1) }

func (r *MemoryRepository) AddRevocation(serial, systemName string, revokedAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.revokedSet[serial]; ok {
		return
	}
	r.revokedSet[serial] = struct{}{}
	r.revocations = append(r.revocations, Revocation{Serial: serial, SystemName: systemName, RevokedAt: revokedAt})
}

func (r *MemoryRepository) IsRevoked(serial string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.revokedSet[serial]
	return ok
}

func (r *MemoryRepository) AllRevocations() []Revocation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Revocation, len(r.revocations))
	copy(out, r.revocations)
	return out
}
