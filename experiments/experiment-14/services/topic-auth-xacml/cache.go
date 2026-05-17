package main

import (
	"sync"
	"time"
)

// decisionCache is a simple per-(subject,resource,action) TTL cache for
// AuthzForce PDP decisions. With TTL=0, every check bypasses the cache.
type decisionCache struct {
	ttl   time.Duration
	mu    sync.Mutex
	items map[string]cacheItem
}

type cacheItem struct {
	permit    bool
	expiresAt time.Time
}

func newDecisionCache(ttl time.Duration) *decisionCache {
	return &decisionCache{ttl: ttl, items: make(map[string]cacheItem)}
}

func (c *decisionCache) key(subject, resource, action string) string {
	return subject + "|" + resource + "|" + action
}

func (c *decisionCache) get(subject, resource, action string) (permit bool, ok bool) {
	if c.ttl == 0 {
		return false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	item, found := c.items[c.key(subject, resource, action)]
	if !found || time.Now().After(item.expiresAt) {
		return false, false
	}
	return item.permit, true
}

func (c *decisionCache) set(subject, resource, action string, permit bool) {
	if c.ttl == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[c.key(subject, resource, action)] = cacheItem{
		permit:    permit,
		expiresAt: time.Now().Add(c.ttl),
	}
}
