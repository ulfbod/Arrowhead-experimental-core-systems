package main

import (
	"sync"
	"time"
)

// decisionKey is the cache key for an AuthzForce decision.
type decisionKey struct {
	subject  string
	resource string
	action   string
}

type decisionEntry struct {
	permit    bool
	expiresAt time.Time
}

// decisionCache caches AuthzForce Permit/Deny results.
// A TTL of zero means no caching.
type decisionCache struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[decisionKey]decisionEntry
}

func newDecisionCache(ttl time.Duration) *decisionCache {
	return &decisionCache{ttl: ttl, m: make(map[decisionKey]decisionEntry)}
}

// get returns (permit, true) if a valid cached entry exists.
func (c *decisionCache) get(subject, resource, action string) (bool, bool) {
	if c.ttl == 0 {
		return false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[decisionKey{subject, resource, action}]
	if !ok || time.Now().After(e.expiresAt) {
		return false, false
	}
	return e.permit, true
}

// set stores a decision in the cache.
func (c *decisionCache) set(subject, resource, action string, permit bool) {
	if c.ttl == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[decisionKey{subject, resource, action}] = decisionEntry{
		permit:    permit,
		expiresAt: time.Now().Add(c.ttl),
	}
}
