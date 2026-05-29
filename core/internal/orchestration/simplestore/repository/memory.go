package repository

import (
	"sync"

	"github.com/google/uuid"

	"arrowhead/core/internal/orchestration/simplestore/model"
)

type Repository interface {
	Save(rule model.StoreRule) model.StoreRule
	Delete(id string) bool
	UpdatePriority(id string, priority int) bool
	All() []model.StoreRule
}

type MemoryRepository struct {
	mu    sync.RWMutex
	rules map[string]model.StoreRule
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{rules: make(map[string]model.StoreRule)}
}

func (r *MemoryRepository) Save(rule model.StoreRule) model.StoreRule {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule.ID = uuid.NewString()
	r.rules[rule.ID] = rule
	return rule
}

func (r *MemoryRepository) Delete(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rules[id]; !ok {
		return false
	}
	delete(r.rules, id)
	return true
}

func (r *MemoryRepository) UpdatePriority(id string, priority int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule, ok := r.rules[id]
	if !ok {
		return false
	}
	rule.Priority = priority
	r.rules[id] = rule
	return true
}

func (r *MemoryRepository) All() []model.StoreRule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.StoreRule, 0, len(r.rules))
	for _, rule := range r.rules {
		out = append(out, rule)
	}
	return out
}
