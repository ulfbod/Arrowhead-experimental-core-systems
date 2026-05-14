package main

import (
	"fmt"
	"sync"
	"time"
)

// validCertLevels are the Arrowhead 5.2 certificate profile tiers.
var validCertLevels = map[string]bool{"lo": true, "on": true, "de": true, "sy": true}

// Subject represents a system and its PKI certificate attributes.
type Subject struct {
	Name         string    `json:"name"`
	CertLevel    string    `json:"certLevel"`    // lo, on, de, sy
	Valid        bool      `json:"valid"`         // cert not revoked/expired
	RegisteredAt time.Time `json:"registeredAt"`
}

// SubjectStore holds subject attribute records in memory.
// All methods are safe for concurrent use.
type SubjectStore struct {
	mu       sync.RWMutex
	subjects map[string]*Subject
}

// NewSubjectStore returns an empty, ready-to-use SubjectStore.
func NewSubjectStore() *SubjectStore {
	return &SubjectStore{subjects: make(map[string]*Subject)}
}

// Register adds or updates a subject's attributes. Returns the stored subject.
func (s *SubjectStore) Register(name, certLevel string, valid bool) (*Subject, error) {
	if name == "" {
		return nil, fmt.Errorf("name must not be empty")
	}
	if !validCertLevels[certLevel] {
		return nil, fmt.Errorf("certLevel must be lo, on, de, or sy; got %q", certLevel)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.subjects[name]; ok {
		// Upsert: update in place, preserve RegisteredAt
		existing.CertLevel = certLevel
		existing.Valid = valid
		return existing, nil
	}

	sub := &Subject{
		Name:         name,
		CertLevel:    certLevel,
		Valid:         valid,
		RegisteredAt: time.Now().UTC(),
	}
	s.subjects[name] = sub
	return sub, nil
}

// Get returns the subject with the given name, or (nil, false) if not found.
func (s *SubjectStore) Get(name string) (*Subject, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.subjects[name]
	return sub, ok
}

// Delete removes the subject with the given name. Returns true if removed.
func (s *SubjectStore) Delete(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subjects[name]; !ok {
		return false
	}
	delete(s.subjects, name)
	return true
}

// GetAll returns a snapshot of all subjects in arbitrary order.
func (s *SubjectStore) GetAll() []*Subject {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Subject, 0, len(s.subjects))
	for _, sub := range s.subjects {
		out = append(out, sub)
	}
	return out
}
