package main

import (
	"fmt"
	"sync"
	"time"
)

var validEffects = map[string]bool{"Permit": true, "Deny": true}

// Policy is a PAP-native access-control rule.
type Policy struct {
	ID        string    `json:"id"`
	Subject   string    `json:"subject"`
	Resource  string    `json:"resource"`
	Action    string    `json:"action"`
	Effect    string    `json:"effect"`
	CreatedAt time.Time `json:"createdAt"`
}

// PolicyStore holds PAP-native policies in memory.
// All methods are safe for concurrent use.
type PolicyStore struct {
	mu       sync.RWMutex
	policies map[string]*Policy
	nextID   int
	version  int
}

func NewPolicyStore() *PolicyStore {
	return &PolicyStore{policies: make(map[string]*Policy)}
}

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

func (s *PolicyStore) Get(id string) (*Policy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.policies[id]
	return p, ok
}

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

func (s *PolicyStore) GetAll() []*Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Policy, 0, len(s.policies))
	for _, p := range s.policies {
		out = append(out, p)
	}
	return out
}

func (s *PolicyStore) Version() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}
