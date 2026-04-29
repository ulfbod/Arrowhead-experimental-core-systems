package repository

import (
	"sync"
	"sync/atomic"

	"arrowhead/core/internal/orchestration/flexiblestore/model"
)

type Repository interface {
	Save(rule model.FlexibleRule) model.FlexibleRule
	Delete(id int64) bool
	All() []model.FlexibleRule
}

type MemoryRepository struct {
	mu      sync.RWMutex
	rules   map[int64]model.FlexibleRule
	counter int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{rules: make(map[int64]model.FlexibleRule)}
}

func (r *MemoryRepository) Save(rule model.FlexibleRule) model.FlexibleRule {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule.ID = atomic.AddInt64(&r.counter, 1)
	r.rules[rule.ID] = rule
	return rule
}

func (r *MemoryRepository) Delete(id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rules[id]; !ok {
		return false
	}
	delete(r.rules, id)
	return true
}

func (r *MemoryRepository) All() []model.FlexibleRule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.FlexibleRule, 0, len(r.rules))
	for _, rule := range r.rules {
		out = append(out, rule)
	}
	return out
}
