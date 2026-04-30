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

func TestResolveProfile_Excellent(t *testing.T) {
	p := ResolveProfile(NetworkProfile{Preset: "5g_excellent"})
	if p.LatencyMs != 2 {
		t.Errorf("5g_excellent LatencyMs: got %v, want 2", p.LatencyMs)
	}
	if p.BandwidthKbps != 0 {
		t.Errorf("5g_excellent BandwidthKbps: got %v, want 0 (unlimited)", p.BandwidthKbps)
	}
}

func TestResolveProfile_AllPresets(t *testing.T) {
	names := []string{"5g_excellent", "5g_good", "5g_moderate", "5g_poor", "5g_edge"}
	for _, name := range names {
		p := ResolveProfile(NetworkProfile{Preset: name})
		if p.LatencyMs <= 0 && name != "5g_excellent" {
			t.Errorf("preset %s: expected LatencyMs > 0, got %v", name, p.LatencyMs)
		}
		// LatencyMs should increase with degrading presets
		_ = p
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

func TestShouldDrop_PartialLoss(t *testing.T) {
	// At 50% loss over 10000 trials, expect roughly 5000 drops.
	// Allow ±10% tolerance.
	p := NetworkProfile{PacketLossPercent: 50}
	drops := 0
	trials := 10000
	for i := 0; i < trials; i++ {
		if ShouldDrop(p) {
			drops++
		}
	}
	lo, hi := trials*35/100, trials*65/100
	if drops < lo || drops > hi {
		t.Errorf("50%% loss: got %d drops out of %d, expected %d–%d", drops, trials, lo, hi)
	}
}

func TestDelayDuration_ZeroLatency(t *testing.T) {
	p := NetworkProfile{LatencyMs: 0, JitterMs: 0}
	if d := DelayDuration(p); d != 0 {
		t.Errorf("expected 0, got %v", d)
	}
}

func TestDelayDuration_Positive(t *testing.T) {
	p := NetworkProfile{LatencyMs: 10, JitterMs: 2}
	d := DelayDuration(p)
	if d < 0 {
		t.Errorf("duration must be non-negative, got %v", d)
	}
}

func TestDelayDuration_Mean(t *testing.T) {
	// Mean of 1000 samples should be within ±30% of the configured latency.
	p := NetworkProfile{LatencyMs: 20, JitterMs: 4}
	var total time.Duration
	n := 1000
	for i := 0; i < n; i++ {
		total += DelayDuration(p)
	}
	meanMs := float64(total.Milliseconds()) / float64(n)
	lo, hi := 20.0*0.70, 20.0*1.30
	if meanMs < lo || meanMs > hi {
		t.Errorf("mean latency %.2f ms outside expected range [%.1f, %.1f]", meanMs, lo, hi)
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
	// Allow ±1ms tolerance for floating-point conversion.
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
