package repository

import (
	"sync"
	"time"
)

// Identity is a stored system credential record.
type Identity struct {
	SystemName   string
	PasswordHash string
	Sysop        bool
	CreatedBy    string
	CreatedAt    string
	UpdatedAt    string
}

// IdentityRepository defines persistence for system identity records.
type IdentityRepository interface {
	Save(id Identity)
	Get(systemName string) (Identity, bool)
	Delete(systemName string)
	All() []Identity
}

// MemoryIdentityRepository is a thread-safe, in-memory identity store.
type MemoryIdentityRepository struct {
	mu         sync.RWMutex
	identities map[string]Identity
}

func NewMemoryIdentityRepository() *MemoryIdentityRepository {
	return &MemoryIdentityRepository{identities: make(map[string]Identity)}
}

func (r *MemoryIdentityRepository) Save(id Identity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().Format(time.RFC3339)
	if id.CreatedAt == "" {
		id.CreatedAt = now
	}
	id.UpdatedAt = now
	r.identities[id.SystemName] = id
}

func (r *MemoryIdentityRepository) Get(systemName string) (Identity, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.identities[systemName]
	return id, ok
}

func (r *MemoryIdentityRepository) Delete(systemName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.identities, systemName)
}

func (r *MemoryIdentityRepository) All() []Identity {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Identity, 0, len(r.identities))
	for _, id := range r.identities {
		result = append(result, id)
	}
	return result
}
