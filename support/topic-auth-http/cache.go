package main

import (
	gosync "sync"
	"time"
)

// rulesCache is a simple TTL cache for ConsumerAuthorization rules.
// When ttl is zero, get always returns a miss so every auth check hits CA live.
type rulesCache struct {
	mu     gosync.RWMutex
	rules  []AuthRule
	expiry time.Time
	ttl    time.Duration
}

func newRulesCache(ttl time.Duration) *rulesCache {
	return &rulesCache{ttl: ttl}
}

// get returns cached rules and true if the cache is valid, otherwise nil and false.
func (c *rulesCache) get() ([]AuthRule, bool) {
	if c.ttl == 0 {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.rules == nil || time.Now().After(c.expiry) {
		return nil, false
	}
	return c.rules, true
}

// set stores rules in the cache with the configured TTL.
// A no-op when ttl is zero.
func (c *rulesCache) set(rules []AuthRule) {
	if c.ttl == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rules = rules
	c.expiry = time.Now().Add(c.ttl)
}

// invalidate clears the cache, forcing the next call to fetchRules live.
func (c *rulesCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rules = nil
}
