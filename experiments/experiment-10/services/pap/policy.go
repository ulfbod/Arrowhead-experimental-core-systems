package main

import (
	"fmt"
	"sync"
	"time"
)

// validEffects are the allowed values for Policy.Effect.
var validEffects = map[string]bool{"Permit": true, "Deny": true}

// Policy is a single access-control rule managed by the PAP.
type Policy struct {
	ID        string    `json:"id"`
	Subject   string    `json:"subject"`   // consumer system name (XACML subject-id)
	Resource  string    `json:"resource"`  // service definition (XACML resource-id)
	Action    string    `json:"action"`    // e.g. "consume"
	Effect    string    `json:"effect"`    // "Permit" or "Deny"
	CreatedAt time.Time `json:"createdAt"`
}

// PolicyStore holds policies in memory and maintains a monotonic version
// counter that increments on every Add or Delete. All methods are safe for
// concurrent use.
type PolicyStore struct {
	mu       sync.RWMutex
	policies map[string]*Policy
	nextID   int
	version  int
}

// NewPolicyStore returns an empty, ready-to-use PolicyStore.
func NewPolicyStore() *PolicyStore {
	return &PolicyStore{policies: make(map[string]*Policy)}
}

// Add validates and stores a new Policy, returning a pointer to the stored
// copy. Effect defaults to "Permit" when empty.
func (s *PolicyStore) Add(subject, resource, action, effect string) (*Policy, error) {
	if effect == "" {
		effect = "Permit"
	}
	if subject == "" {
		return nil, fmt.Errorf("subject must not be empty")
	}
	if resource == "" {
		return nil, fmt.Errorf("resource must not be empty")
	}
	if action == "" {
		return nil, fmt.Errorf("action must not be empty")
	}
	if !validEffects[effect] {
		return nil, fmt.Errorf("effect must be Permit or Deny, got %q", effect)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	s.version++
	id := fmt.Sprintf("pol-%d", s.nextID)
	p := &Policy{
		ID:        id,
		Subject:   subject,
		Resource:  resource,
		Action:    action,
		Effect:    effect,
		CreatedAt: time.Now().UTC(),
	}
	s.policies[id] = p
	return p, nil
}

// Get returns the policy with the given ID, or (nil, false) if not found.
func (s *PolicyStore) Get(id string) (*Policy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.policies[id]
	return p, ok
}

// Delete removes the policy with the given ID. Returns true if a policy was
// removed, false if the ID was not found.
func (s *PolicyStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.policies[id]; !ok {
		return false
	}
	delete(s.policies, id)
	s.version++
	return true
}

// GetAll returns a snapshot of all policies in arbitrary order.
func (s *PolicyStore) GetAll() []*Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Policy, 0, len(s.policies))
	for _, p := range s.policies {
		out = append(out, p)
	}
	return out
}

// Version returns the current store version. It starts at 0 and increments
// on every Add or Delete.
func (s *PolicyStore) Version() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}
