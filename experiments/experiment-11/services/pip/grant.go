package main

import (
	"sync"
	"time"
)

// Grant mirrors the ConsumerAuthorization rule wire type.
type Grant struct {
	ID                 int    `json:"id"`
	ConsumerSystemName string `json:"consumerSystemName"`
	ProviderSystemName string `json:"providerSystemName,omitempty"`
	ServiceDefinition  string `json:"serviceDefinition"`
}

// LookupResponse is the ConsumerAuthorization /authorization/lookup response.
type LookupResponse struct {
	Rules []Grant `json:"rules"`
	Count int     `json:"count"`
}

// GrantStore holds a cached copy of ConsumerAuthorization grants.
// Version increments only when the grant set actually changes.
// All methods are safe for concurrent use.
type GrantStore struct {
	mu         sync.RWMutex
	grants     []Grant
	version    int
	synced     bool
	lastSyncAt time.Time
}

// NewGrantStore returns an empty, unsynced GrantStore.
func NewGrantStore() *GrantStore {
	return &GrantStore{}
}

// Update replaces the grant list. Returns true if the grants changed.
// The first call always returns true (marks the store as synced).
func (s *GrantStore) Update(grants []Grant) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Normalise nil to empty slice for comparison.
	if grants == nil {
		grants = []Grant{}
	}

	if s.synced && grantsEqual(s.grants, grants) {
		return false
	}
	s.grants = grants
	s.version++
	s.synced = true
	s.lastSyncAt = time.Now().UTC()
	return true
}

// GetAll returns a snapshot of the current grant list.
func (s *GrantStore) GetAll() []Grant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Grant, len(s.grants))
	copy(out, s.grants)
	return out
}

// GetBySubject returns grants for a given consumer system name.
func (s *GrantStore) GetBySubject(subject string) []Grant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Grant
	for _, g := range s.grants {
		if g.ConsumerSystemName == subject {
			out = append(out, g)
		}
	}
	return out
}

// IsGranted reports whether subject is granted access to resource.
func (s *GrantStore) IsGranted(subject, resource string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, g := range s.grants {
		if g.ConsumerSystemName == subject && g.ServiceDefinition == resource {
			return true
		}
	}
	return false
}

// Version returns the current change counter.
func (s *GrantStore) Version() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

// Synced reports whether at least one successful sync has occurred.
func (s *GrantStore) Synced() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.synced
}

// LastSyncAt returns the time of the last successful sync.
func (s *GrantStore) LastSyncAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncAt
}

// grantsEqual reports whether two grant slices have identical content
// (by ConsumerSystemName + ServiceDefinition, order-insensitive).
func grantsEqual(a, b []Grant) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, g := range a {
		set[g.ConsumerSystemName+"\x00"+g.ServiceDefinition] = struct{}{}
	}
	for _, g := range b {
		if _, ok := set[g.ConsumerSystemName+"\x00"+g.ServiceDefinition]; !ok {
			return false
		}
	}
	return true
}
