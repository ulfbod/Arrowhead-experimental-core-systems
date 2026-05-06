package main

import (
	"testing"
	"time"
)

func TestResolveProfile_Good(t *testing.T) {
	p := ResolveProfile(NetworkProfile{Preset: "5g_good"})
	if p.LatencyMs != 12 {
		t.Errorf("5g_good LatencyMs: got %v, want 12", p.LatencyMs)
	}
	if p.BandwidthKbps != 10000 {
		t.Errorf("5g_good BandwidthKbps: got %v, want 10000", p.BandwidthKbps)
	}
}

func TestResolveProfile_AllPresets(t *testing.T) {
	for _, name := range []string{"5g_excellent", "5g_good", "5g_moderate", "5g_poor", "5g_edge"} {
		p := ResolveProfile(NetworkProfile{Preset: name})
		_ = p
	}
}

func TestResolveProfile_Custom(t *testing.T) {
	p := ResolveProfile(NetworkProfile{Preset: "custom", LatencyMs: 77})
	if p.LatencyMs != 77 {
		t.Errorf("custom LatencyMs: got %v, want 77", p.LatencyMs)
	}
}

func TestShouldDrop_ZeroLoss(t *testing.T) {
	p := NetworkProfile{PacketLossPercent: 0}
	for i := 0; i < 1000; i++ {
		if ShouldDrop(p) {
			t.Fatal("expected no drops with 0% loss")
		}
	}
}

func TestShouldDrop_FullLoss(t *testing.T) {
	p := NetworkProfile{PacketLossPercent: 100}
	for i := 0; i < 100; i++ {
		if !ShouldDrop(p) {
			t.Fatal("expected all drops with 100% loss")
		}
	}
}

func TestDelayDuration_ZeroLatency(t *testing.T) {
	if d := DelayDuration(NetworkProfile{LatencyMs: 0}); d != 0 {
		t.Errorf("expected 0, got %v", d)
	}
}

func TestDelayDuration_NonNegative(t *testing.T) {
	p := NetworkProfile{LatencyMs: 10, JitterMs: 5}
	for i := 0; i < 100; i++ {
		if d := DelayDuration(p); d < 0 {
			t.Errorf("duration must be non-negative, got %v", d)
		}
	}
}

func TestByteDelay_Unlimited(t *testing.T) {
	if d := ByteDelay(NetworkProfile{BandwidthKbps: 0}, 1000); d != 0 {
		t.Errorf("unlimited bandwidth: want 0, got %v", d)
	}
}

func TestByteDelay_Limited(t *testing.T) {
	p := NetworkProfile{BandwidthKbps: 1000}
	d := ByteDelay(p, 1000) // 8000 bits / 1_000_000 bps = 8ms
	want := 8 * time.Millisecond
	if d < want-time.Millisecond || d > want+time.Millisecond {
		t.Errorf("expected ~%v, got %v", want, d)
	}
}
