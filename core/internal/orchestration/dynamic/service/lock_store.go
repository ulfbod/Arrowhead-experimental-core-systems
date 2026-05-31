package service

import (
	"sync"
	"sync/atomic"
	"time"
)

// LockChecker is the interface used by DynamicOrchestrator to check whether a
// provider is currently under an exclusive lock.
type LockChecker interface {
	IsLocked(providerName string) bool
}

// NopLockChecker implements LockChecker and always returns false (no locks).
// Use in tests and when no LockStore is wired.
type NopLockChecker struct{}

func (NopLockChecker) IsLocked(_ string) bool { return false }

// Lock represents an exclusive lock on a service instance.
type Lock struct {
	ID                 int        `json:"id"`
	OrchestrationJobId string     `json:"orchestrationJobId"`
	ServiceInstanceId  string     `json:"serviceInstanceId"`
	Owner              string     `json:"owner"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
	Temporary          bool       `json:"temporary"`
}

// CreateLockRequest is the body for POST mgmt/lock/create.
type CreateLockRequest struct {
	OrchestrationJobId string  `json:"orchestrationJobId"`
	ServiceInstanceId  string  `json:"serviceInstanceId"`
	Owner              string  `json:"owner"`
	ExpiresAt          *string `json:"expiresAt,omitempty"` // RFC3339
	Temporary          bool    `json:"temporary"`
}

// LockQueryResponse is returned by POST mgmt/lock/query.
type LockQueryResponse struct {
	Locks []Lock `json:"locks"`
	Count int    `json:"count"`
}

// LockStore is an in-memory store for orchestration locks.
type LockStore struct {
	mu      sync.RWMutex
	locks   map[int]Lock
	counter int64
}

func NewLockStore() *LockStore {
	return &LockStore{locks: make(map[int]Lock)}
}

// Create adds a new lock and returns it.
func (s *LockStore) Create(req CreateLockRequest) Lock {
	id := int(atomic.AddInt64(&s.counter, 1))
	lock := Lock{
		ID:                 id,
		OrchestrationJobId: req.OrchestrationJobId,
		ServiceInstanceId:  req.ServiceInstanceId,
		Owner:              req.Owner,
		Temporary:          req.Temporary,
	}
	if req.ExpiresAt != nil {
		if t, err := time.Parse(time.RFC3339, *req.ExpiresAt); err == nil {
			lock.ExpiresAt = &t
		}
	}
	s.mu.Lock()
	s.locks[id] = lock
	s.mu.Unlock()
	return lock
}

// Query returns all non-expired locks.
func (s *LockStore) Query() LockQueryResponse {
	now := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var active []Lock
	for _, l := range s.locks {
		if l.ExpiresAt != nil && l.ExpiresAt.Before(now) {
			continue
		}
		active = append(active, l)
	}
	if active == nil {
		active = []Lock{}
	}
	return LockQueryResponse{Locks: active, Count: len(active)}
}

// IsLocked returns true if any active (non-expired) lock exists for providerName.
// A lock is considered to belong to a provider when its ServiceInstanceId equals
// or starts with "<providerName>|" (matching the composite service-instance ID
// format "<providerName>|<serviceDef>|<version>").
// It implements LockChecker.
func (s *LockStore) IsLocked(providerName string) bool {
	now := time.Now()
	prefix := providerName + "|"
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, l := range s.locks {
		if l.ServiceInstanceId != providerName && !startsWith(l.ServiceInstanceId, prefix) {
			continue
		}
		if l.ExpiresAt != nil && l.ExpiresAt.Before(now) {
			continue // expired — not locked
		}
		return true
	}
	return false
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// RemoveByOwner deletes all locks belonging to owner. Returns the count removed.
func (s *LockStore) RemoveByOwner(owner string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, l := range s.locks {
		if l.Owner == owner {
			delete(s.locks, id)
			removed++
		}
	}
	return removed
}
