package service

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Subscription is a push orchestration subscription.
type Subscription struct {
	ID                   string         `json:"id"`
	OwnerSystemName      string         `json:"ownerSystemName"`
	TargetSystemName     string         `json:"targetSystemName"`
	ServiceDefinition    string         `json:"serviceDefinition,omitempty"` // used by auto-push poller (G26)
	OrchestrationRequest map[string]any `json:"orchestrationRequest"`
	NotifyInterface      map[string]any `json:"notifyInterface,omitempty"`
	ExpiredAt            *string        `json:"expiredAt,omitempty"`
	CreatedAt            time.Time      `json:"createdAt"`
}

// CreateSubscriptionRequest is the body for POST subscribe.
type CreateSubscriptionRequest struct {
	OwnerSystemName      string         `json:"ownerSystemName"`
	TargetSystemName     string         `json:"targetSystemName"`
	ServiceDefinition    string         `json:"serviceDefinition,omitempty"` // when set, auto-push polls SR for provider changes
	OrchestrationRequest map[string]any `json:"orchestrationRequest"`
	NotifyInterface      map[string]any `json:"notifyInterface,omitempty"`
	ExpiredAt            *string        `json:"expiredAt,omitempty"`
}

// SubscriptionQueryResponse is returned by mgmt/push/query.
type SubscriptionQueryResponse struct {
	Subscriptions []Subscription `json:"subscriptions"`
	Count         int            `json:"count"`
}

// SubscriptionStore is an in-memory store for push orchestration subscriptions.
// A duplicate subscribe (same ownerSystemName + targetSystemName) overwrites the existing entry.
type SubscriptionStore struct {
	mu            sync.RWMutex
	byID          map[string]Subscription
	byOwnerTarget map[string]string // "owner|target" → id
}

func NewSubscriptionStore() *SubscriptionStore {
	return &SubscriptionStore{
		byID:          make(map[string]Subscription),
		byOwnerTarget: make(map[string]string),
	}
}

// Subscribe creates or overwrites a subscription.
// Returns (subscription, created): created=true on new, false on overwrite.
func (s *SubscriptionStore) Subscribe(req CreateSubscriptionRequest) (Subscription, bool) {
	key := req.OwnerSystemName + "|" + req.TargetSystemName
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID, ok := s.byOwnerTarget[key]; ok {
		existing := s.byID[existingID]
		existing.OrchestrationRequest = req.OrchestrationRequest
		existing.NotifyInterface = req.NotifyInterface
		existing.ExpiredAt = req.ExpiredAt
		existing.ServiceDefinition = req.ServiceDefinition
		s.byID[existingID] = existing
		return existing, false
	}
	sub := Subscription{
		ID:                   uuid.NewString(),
		OwnerSystemName:      req.OwnerSystemName,
		TargetSystemName:     req.TargetSystemName,
		ServiceDefinition:    req.ServiceDefinition,
		OrchestrationRequest: req.OrchestrationRequest,
		NotifyInterface:      req.NotifyInterface,
		ExpiredAt:            req.ExpiredAt,
		CreatedAt:            time.Now(),
	}
	s.byID[sub.ID] = sub
	s.byOwnerTarget[key] = sub.ID
	return sub, true
}

// Unsubscribe removes a subscription by ID. Returns true if found.
func (s *SubscriptionStore) Unsubscribe(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.byID[id]
	if !ok {
		return false
	}
	delete(s.byOwnerTarget, sub.OwnerSystemName+"|"+sub.TargetSystemName)
	delete(s.byID, id)
	return true
}

// UnsubscribeMany removes subscriptions by IDs. Returns count removed.
func (s *SubscriptionStore) UnsubscribeMany(ids []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for _, id := range ids {
		sub, ok := s.byID[id]
		if !ok {
			continue
		}
		delete(s.byOwnerTarget, sub.OwnerSystemName+"|"+sub.TargetSystemName)
		delete(s.byID, id)
		removed++
	}
	return removed
}

// Get returns a subscription by ID.
func (s *SubscriptionStore) Get(id string) (Subscription, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.byID[id]
	return sub, ok
}

// Query returns all subscriptions.
func (s *SubscriptionStore) Query() SubscriptionQueryResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	subs := make([]Subscription, 0, len(s.byID))
	for _, sub := range s.byID {
		subs = append(subs, sub)
	}
	return SubscriptionQueryResponse{Subscriptions: subs, Count: len(subs)}
}
