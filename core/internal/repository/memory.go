// Package repository provides storage for the Arrowhead Core Service Registry.
package repository

import (
	"sync"
	"sync/atomic"

	"arrowhead/core/internal/model"
)

// Repository defines storage operations for service instances.
type Repository interface {
	Save(svc *model.ServiceInstance) *model.ServiceInstance
	All() []*model.ServiceInstance
	Delete(serviceDefinition, systemName, address string, port, version int) bool
}

type key struct {
	serviceDefinition string
	systemName        string
	address           string
	port              int
	version           int
}

// MemoryRepository is a thread-safe, in-memory Repository implementation.
type MemoryRepository struct {
	mu      sync.RWMutex
	byKey   map[key]*model.ServiceInstance
	counter int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{byKey: make(map[key]*model.ServiceInstance)}
}

func (r *MemoryRepository) Save(svc *model.ServiceInstance) *model.ServiceInstance {
	k := key{
		serviceDefinition: svc.ServiceDefinition,
		systemName:        svc.ProviderSystem.SystemName,
		address:           svc.ProviderSystem.Address,
		port:              svc.ProviderSystem.Port,
		version:           svc.Version,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.byKey[k]; ok {
		existing.ServiceUri = svc.ServiceUri
		existing.Interfaces = svc.Interfaces
		existing.Version = svc.Version
		existing.Metadata = svc.Metadata
		existing.Secure = svc.Secure
		existing.ProviderSystem.AuthenticationInfo = svc.ProviderSystem.AuthenticationInfo
		return existing
	}
	svc.ID = atomic.AddInt64(&r.counter, 1)
	r.byKey[k] = svc
	return svc
}

func (r *MemoryRepository) All() []*model.ServiceInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*model.ServiceInstance, 0, len(r.byKey))
	for _, svc := range r.byKey {
		out = append(out, svc)
	}
	return out
}

func (r *MemoryRepository) Delete(serviceDefinition, systemName, address string, port, version int) bool {
	k := key{
		serviceDefinition: serviceDefinition,
		systemName:        systemName,
		address:           address,
		port:              port,
		version:           version,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byKey[k]; !ok {
		return false
	}
	delete(r.byKey, k)
	return true
}
