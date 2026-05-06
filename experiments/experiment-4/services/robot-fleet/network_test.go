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
	if p.JitterMs != 3 {
		t.Errorf("5g_good JitterMs: got %v, want 3", p.JitterMs)
	}
	if p.BandwidthKbps != 10000 {
		t.Errorf("5g_good BandwidthKbps: got %v, want 10000", p.BandwidthKbps)
	}
}

func TestResolveProfile_AllPresets(t *testing.T) {
	names := []string{"5g_excellent", "5g_good", "5g_moderate", "5g_poor", "5g_edge"}
	for _, name := range names {
		p := ResolveProfile(NetworkProfile{Preset: name})
		_ = p // verify no panic
	}
}

func TestResolveProfile_Custom(t *testing.T) {
	custom := NetworkProfile{
		Preset:            "custom",
		LatencyMs:         77,
		JitterMs:          11,
		PacketLossPercent: 1.5,
		BandwidthKbps:     500,
	}
	p := ResolveProfile(custom)
	if p.LatencyMs != 77 {
		t.Errorf("custom LatencyMs: got %v, want 77", p.LatencyMs)
	}
	if p.PacketLossPercent != 1.5 {
		t.Errorf("custom PacketLossPercent: got %v, want 1.5", p.PacketLossPercent)
	}
}

func TestResolveProfile_UnknownPreset(t *testing.T) {
	p := NetworkProfile{Preset: "nonexistent", LatencyMs: 42}
	got := ResolveProfile(p)
	if got.LatencyMs != 42 {
		t.Errorf("unknown preset should leave values unchanged, got LatencyMs=%v", got.LatencyMs)
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
	p := NetworkProfile{LatencyMs: 0, JitterMs: 0}
	if d := DelayDuration(p); d != 0 {
		t.Errorf("expected 0, got %v", d)
	}
}

func TestDelayDuration_NonNegative(t *testing.T) {
	p := NetworkProfile{LatencyMs: 10, JitterMs: 2}
	for i := 0; i < 100; i++ {
		if d := DelayDuration(p); d < 0 {
			t.Errorf("duration must be non-negative, got %v", d)
		}
	}
}

func TestByteDelay_Unlimited(t *testing.T) {
	p := NetworkProfile{BandwidthKbps: 0}
	if d := ByteDelay(p, 1000); d != 0 {
		t.Errorf("unlimited bandwidth should give 0 delay, got %v", d)
	}
}

func TestByteDelay_Limited(t *testing.T) {
	// 1000 Kbps = 1 Mbps; 1000 bytes = 8000 bits → 8ms
	p := NetworkProfile{BandwidthKbps: 1000}
	d := ByteDelay(p, 1000)
	want := 8 * time.Millisecond
	if d < want-time.Millisecond || d > want+time.Millisecond {
		t.Errorf("expected ~%v, got %v", want, d)
	}
}

func TestByteDelay_ZeroBytes(t *testing.T) {
	p := NetworkProfile{BandwidthKbps: 100}
	if d := ByteDelay(p, 0); d != 0 {
		t.Errorf("0 bytes should give 0 delay, got %v", d)
	}
}
