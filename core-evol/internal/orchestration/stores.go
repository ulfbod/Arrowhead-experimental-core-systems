package orchestration

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"
)

// newUUID returns a random UUID v4 string using crypto/rand.
func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b) //nolint:errcheck — crypto/rand.Read never fails on supported platforms
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// ---- Lock store -------------------------------------------------------------

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

type lockStore struct {
	mu      sync.RWMutex
	locks   map[int]Lock
	counter int64
}

func newLockStore() *lockStore {
	return &lockStore{locks: make(map[int]Lock)}
}

func (s *lockStore) create(req CreateLockRequest) Lock {
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

func (s *lockStore) query() LockQueryResponse {
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

func (s *lockStore) removeByOwner(owner string) int {
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

// ---- History store ----------------------------------------------------------

// HistoryEntry records a single orchestration job.
type HistoryEntry struct {
	ID                string     `json:"id"`
	Status            string     `json:"status"`  // DONE | ERROR | PENDING
	Type              string     `json:"type"`    // PULL | PUSH
	RequesterSystem   string     `json:"requesterSystem,omitempty"`
	ServiceDefinition string     `json:"serviceDefinition,omitempty"`
	Message           string     `json:"message,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	FinishedAt        *time.Time `json:"finishedAt,omitempty"`
}

// HistoryQueryResponse is returned by POST mgmt/history/query.
type HistoryQueryResponse struct {
	Entries []HistoryEntry `json:"entries"`
	Count   int            `json:"count"`
}

type historyStore struct {
	mu      sync.RWMutex
	entries []HistoryEntry
}

func newHistoryStore() *historyStore { return &historyStore{} }

func (s *historyStore) add(e HistoryEntry) {
	s.mu.Lock()
	s.entries = append(s.entries, e)
	s.mu.Unlock()
}

func (s *historyStore) query() HistoryQueryResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]HistoryEntry, len(s.entries))
	copy(out, s.entries)
	return HistoryQueryResponse{Entries: out, Count: len(out)}
}

func newHistoryEntry(requester, service, status, entryType string) HistoryEntry {
	now := time.Now()
	var finished *time.Time
	if status != "PENDING" {
		finished = &now
	}
	return HistoryEntry{
		ID:                newUUID(),
		Status:            status,
		Type:              entryType,
		RequesterSystem:   requester,
		ServiceDefinition: service,
		CreatedAt:         now,
		FinishedAt:        finished,
	}
}

// ---- Subscription store -----------------------------------------------------

// Subscription is a push orchestration subscription.
type Subscription struct {
	ID                   string         `json:"id"`
	OwnerSystemName      string         `json:"ownerSystemName"`
	TargetSystemName     string         `json:"targetSystemName"`
	OrchestrationRequest map[string]any `json:"orchestrationRequest"`
	NotifyInterface      map[string]any `json:"notifyInterface,omitempty"`
	ExpiredAt            *string        `json:"expiredAt,omitempty"`
	CreatedAt            time.Time      `json:"createdAt"`
}

// CreateSubscriptionRequest is the body for POST subscribe or mgmt/push/subscribe.
type CreateSubscriptionRequest struct {
	OwnerSystemName      string         `json:"ownerSystemName"`
	TargetSystemName     string         `json:"targetSystemName"`
	OrchestrationRequest map[string]any `json:"orchestrationRequest"`
	NotifyInterface      map[string]any `json:"notifyInterface,omitempty"`
	ExpiredAt            *string        `json:"expiredAt,omitempty"`
}

// SubscriptionQueryResponse is returned by mgmt/push/query.
type SubscriptionQueryResponse struct {
	Subscriptions []Subscription `json:"subscriptions"`
	Count         int            `json:"count"`
}

type subscriptionStore struct {
	mu            sync.RWMutex
	byID          map[string]Subscription
	byOwnerTarget map[string]string // "owner|target" → id
}

func newSubscriptionStore() *subscriptionStore {
	return &subscriptionStore{
		byID:          make(map[string]Subscription),
		byOwnerTarget: make(map[string]string),
	}
}

// subscribe creates or overwrites a subscription.
// Returns (subscription, created): created=true on new, false on overwrite.
func (s *subscriptionStore) subscribe(req CreateSubscriptionRequest) (Subscription, bool) {
	key := req.OwnerSystemName + "|" + req.TargetSystemName
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID, ok := s.byOwnerTarget[key]; ok {
		existing := s.byID[existingID]
		existing.OrchestrationRequest = req.OrchestrationRequest
		existing.NotifyInterface = req.NotifyInterface
		existing.ExpiredAt = req.ExpiredAt
		s.byID[existingID] = existing
		return existing, false
	}
	sub := Subscription{
		ID:                   newUUID(),
		OwnerSystemName:      req.OwnerSystemName,
		TargetSystemName:     req.TargetSystemName,
		OrchestrationRequest: req.OrchestrationRequest,
		NotifyInterface:      req.NotifyInterface,
		ExpiredAt:            req.ExpiredAt,
		CreatedAt:            time.Now(),
	}
	s.byID[sub.ID] = sub
	s.byOwnerTarget[key] = sub.ID
	return sub, true
}

func (s *subscriptionStore) unsubscribe(id string) bool {
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

func (s *subscriptionStore) unsubscribeMany(ids []string) int {
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

func (s *subscriptionStore) get(id string) (Subscription, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.byID[id]
	return sub, ok
}

func (s *subscriptionStore) queryAll() SubscriptionQueryResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	subs := make([]Subscription, 0, len(s.byID))
	for _, sub := range s.byID {
		subs = append(subs, sub)
	}
	return SubscriptionQueryResponse{Subscriptions: subs, Count: len(subs)}
}
