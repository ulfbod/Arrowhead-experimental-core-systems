// Package repository provides storage for the Arrowhead Core Service Registry.
//
// DO NOT MODIFY FOR EXPERIMENTS.
// The Repository interface and its implementations are internal to core.
// Experiments must not import this package.
package repository

import (
	"arrowhead/serviceregistry/internal/model"
	"sync"
	"sync/atomic"
)

// Repository defines storage operations for service instances.
type Repository interface {
	// Save inserts or replaces the service instance identified by
	// (ServiceDefinition, SystemName, Address, Port, Version). Returns the saved entry.
	Save(svc *model.ServiceInstance) *model.ServiceInstance
	// All returns every registered service instance.
	All() []*model.ServiceInstance
}

// key uniquely identifies a service instance.
// Version is included so that different versions of the same service
// are stored as independent entries; re-registering the exact same tuple
// (including version) overwrites the existing entry.
type key struct {
	serviceDefinition string
	systemName        string
	address           string
	port              int
	version           int
}

// MemoryRepository is a thread-safe, in-memory Repository implementation.
type MemoryRepository struct {
	mu       sync.RWMutex
	byKey    map[key]*model.ServiceInstance
	counter  int64
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
		// Overwrite in place, keep same ID.
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
