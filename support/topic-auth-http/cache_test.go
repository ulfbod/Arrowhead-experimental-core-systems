package main

import (
	"testing"
	"time"
)

func TestCache_zeroTTL_alwaysMiss(t *testing.T) {
	c := newRulesCache(0)
	c.set([]AuthRule{{ConsumerSystemName: "a"}})
	if _, ok := c.get(); ok {
		t.Fatal("expected cache miss with ttl=0")
	}
}

func TestCache_withinTTL_hit(t *testing.T) {
	c := newRulesCache(10 * time.Second)
	rules := []AuthRule{{ConsumerSystemName: "consumer-1"}}
	c.set(rules)
	got, ok := c.get()
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 1 || got[0].ConsumerSystemName != "consumer-1" {
		t.Fatalf("unexpected rules: %v", got)
	}
}

func TestCache_afterTTL_miss(t *testing.T) {
	c := newRulesCache(10 * time.Millisecond)
	c.set([]AuthRule{{ConsumerSystemName: "consumer-1"}})
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.get(); ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestCache_invalidate_clears(t *testing.T) {
	c := newRulesCache(10 * time.Second)
	c.set([]AuthRule{{ConsumerSystemName: "consumer-1"}})
	c.invalidate()
	if _, ok := c.get(); ok {
		t.Fatal("expected cache miss after invalidate")
	}
}

func TestCache_emptyRules(t *testing.T) {
	c := newRulesCache(10 * time.Second)
	c.set([]AuthRule{})
	got, ok := c.get()
	if !ok {
		t.Fatal("expected cache hit for empty rule set")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty rules, got %v", got)
	}
}
