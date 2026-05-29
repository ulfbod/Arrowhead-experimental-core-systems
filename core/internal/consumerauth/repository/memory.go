// Package repository provides in-memory policy storage for ConsumerAuthorization.
package repository

import (
	"sort"
	"sync"

	"arrowhead/core/internal/consumerauth/model"
)

// Repository defines policy storage operations.
type Repository interface {
	Save(policy model.AuthPolicy) model.AuthPolicy
	Delete(instanceID string) bool
	FindByInstanceID(instanceID string) (model.AuthPolicy, bool)
	All() []model.AuthPolicy
}

// MemoryRepository is a thread-safe, in-memory policy store.
type MemoryRepository struct {
	mu       sync.RWMutex
	policies map[string]model.AuthPolicy
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{policies: make(map[string]model.AuthPolicy)}
}

func (r *MemoryRepository) Save(policy model.AuthPolicy) model.AuthPolicy {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policies[policy.InstanceID] = policy
	return policy
}

func (r *MemoryRepository) Delete(instanceID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.policies[instanceID]; !ok {
		return false
	}
	delete(r.policies, instanceID)
	return true
}

func (r *MemoryRepository) FindByInstanceID(instanceID string) (model.AuthPolicy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.policies[instanceID]
	return p, ok
}

func (r *MemoryRepository) All() []model.AuthPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.AuthPolicy, 0, len(r.policies))
	for _, p := range r.policies {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstanceID < out[j].InstanceID })
	return out
}
