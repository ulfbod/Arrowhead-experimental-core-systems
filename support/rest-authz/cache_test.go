package main

import (
	"testing"
	"time"
)

func TestDecisionCache_zeroTTL_alwaysMiss(t *testing.T) {
	c := newDecisionCache(0)
	c.set("consumer", "service", "invoke", true)

	_, ok := c.get("consumer", "service", "invoke")
	if ok {
		t.Error("zero-TTL cache should always miss")
	}
}

func TestDecisionCache_withinTTL_hit(t *testing.T) {
	c := newDecisionCache(10 * time.Second)
	c.set("consumer-1", "telemetry", "invoke", true)

	permit, ok := c.get("consumer-1", "telemetry", "invoke")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !permit {
		t.Error("expected permit=true")
	}
}

func TestDecisionCache_deny_cached(t *testing.T) {
	c := newDecisionCache(10 * time.Second)
	c.set("bad-consumer", "telemetry", "invoke", false)

	permit, ok := c.get("bad-consumer", "telemetry", "invoke")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if permit {
		t.Error("expected permit=false")
	}
}

func TestDecisionCache_expired_miss(t *testing.T) {
	c := newDecisionCache(1 * time.Nanosecond)
	c.set("consumer", "service", "invoke", true)

	time.Sleep(2 * time.Millisecond) // enough for 1ns TTL to expire

	_, ok := c.get("consumer", "service", "invoke")
	if ok {
		t.Error("expired entry should be a miss")
	}
}

func TestDecisionCache_differentKeys(t *testing.T) {
	c := newDecisionCache(10 * time.Second)
	c.set("consumer-A", "svc-1", "invoke", true)
	c.set("consumer-B", "svc-1", "invoke", false)

	permitA, okA := c.get("consumer-A", "svc-1", "invoke")
	permitB, okB := c.get("consumer-B", "svc-1", "invoke")

	if !okA || !permitA {
		t.Error("consumer-A should be permitted")
	}
	if !okB || permitB {
		t.Error("consumer-B should be denied")
	}
}

func TestDecisionCache_missingKey(t *testing.T) {
	c := newDecisionCache(10 * time.Second)
	_, ok := c.get("nobody", "nothing", "invoke")
	if ok {
		t.Error("missing key should be a miss")
	}
}
