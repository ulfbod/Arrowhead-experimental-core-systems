// Package repository provides in-memory token storage for Authentication.
package repository

import (
	"sync"
	"time"

	"arrowhead/core/internal/authentication/model"
)

// Repository defines token storage operations.
type Repository interface {
	Save(token *model.IdentityToken)
	FindByToken(token string) (*model.IdentityToken, bool)
	Delete(token string) bool
	DeleteExpired()
}

// MemoryRepository is a thread-safe, in-memory token store.
type MemoryRepository struct {
	mu     sync.RWMutex
	tokens map[string]*model.IdentityToken
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{tokens: make(map[string]*model.IdentityToken)}
}

func (r *MemoryRepository) Save(t *model.IdentityToken) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[t.Token] = t
}

func (r *MemoryRepository) FindByToken(token string) (*model.IdentityToken, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tokens[token]
	return t, ok
}

func (r *MemoryRepository) Delete(token string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tokens[token]; !ok {
		return false
	}
	delete(r.tokens, token)
	return true
}

func (r *MemoryRepository) DeleteExpired() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for k, t := range r.tokens {
		if now.After(t.ExpiresAt) {
			delete(r.tokens, k)
		}
	}
}
