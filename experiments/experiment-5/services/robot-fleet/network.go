// network.go — 5G network profile emulation for robot-fleet.
//
// Each robot is assigned a NetworkProfile that controls per-message latency,
// jitter, packet-loss probability, and bandwidth cap.  All effects are applied
// at the application layer (sleep before publish) so no kernel privileges are
// needed.
package main

import (
	"math"
	"math/rand"
	"time"
)

// NetworkProfile describes the simulated radio link for one robot.
type NetworkProfile struct {
	// Preset names a built-in profile; "custom" means the numeric fields are
	// used as-is.
	Preset string `json:"preset"`

	// One-way added latency in milliseconds (Gaussian mean).
	LatencyMs float64 `json:"latencyMs"`

	// One-way jitter in milliseconds (Gaussian σ = JitterMs/2).
	JitterMs float64 `json:"jitterMs"`

	// Fraction of messages to silently drop (0–100).
	PacketLossPercent float64 `json:"packetLossPercent"`

	// Maximum bandwidth in Kbps; 0 means unlimited.
	BandwidthKbps float64 `json:"bandwidthKbps"`
}

// preset definitions — values match 3GPP NR Release-15 field observations.
var presets = map[string]NetworkProfile{
	"5g_excellent": {Preset: "5g_excellent", LatencyMs: 2, JitterMs: 1, PacketLossPercent: 0, BandwidthKbps: 0},
	"5g_good":      {Preset: "5g_good", LatencyMs: 12, JitterMs: 3, PacketLossPercent: 0, BandwidthKbps: 10000},
	"5g_moderate":  {Preset: "5g_moderate", LatencyMs: 40, JitterMs: 12, PacketLossPercent: 0.1, BandwidthKbps: 1000},
	"5g_poor":      {Preset: "5g_poor", LatencyMs: 100, JitterMs: 35, PacketLossPercent: 0.5, BandwidthKbps: 200},
	"5g_edge":      {Preset: "5g_edge", LatencyMs: 200, JitterMs: 70, PacketLossPercent: 2.0, BandwidthKbps: 50},
}

// ResolveProfile fills in numeric fields from the named preset when
// Preset != "custom".  If the preset name is unknown it returns p unchanged.
func ResolveProfile(p NetworkProfile) NetworkProfile {
	if p.Preset == "custom" || p.Preset == "" {
		return p
	}
	if base, ok := presets[p.Preset]; ok {
		return base
	}
	return p
}

// ShouldDrop returns true with probability p.PacketLossPercent / 100.
func ShouldDrop(p NetworkProfile) bool {
	if p.PacketLossPercent <= 0 {
		return false
	}
	if p.PacketLossPercent >= 100 {
		return true
	}
	return rand.Float64()*100 < p.PacketLossPercent
}

// DelayDuration returns a non-negative duration sampled from
// Normal(LatencyMs, JitterMs/2).  Returns 0 when LatencyMs == 0.
func DelayDuration(p NetworkProfile) time.Duration {
	if p.LatencyMs <= 0 {
		return 0
	}
	sigma := p.JitterMs / 2
	var ms float64
	if sigma <= 0 {
		ms = p.LatencyMs
	} else {
		ms = p.LatencyMs + sigma*normalSample()
	}
	if ms < 0 {
		ms = 0
	}
	return time.Duration(ms * float64(time.Millisecond))
}

// ByteDelay returns the additional sleep needed to respect BandwidthKbps.
// Returns 0 when BandwidthKbps == 0 (unlimited).
func ByteDelay(p NetworkProfile, nBytes int) time.Duration {
	if p.BandwidthKbps <= 0 || nBytes <= 0 {
		return 0
	}
	// bits / (bits/s) = seconds
	bits := float64(nBytes) * 8
	bps := p.BandwidthKbps * 1000
	seconds := bits / bps
	return time.Duration(seconds * float64(time.Second))
}

// normalSample returns a single standard-normal sample via Box–Muller.
func normalSample() float64 {
	u1 := rand.Float64()
	u2 := rand.Float64()
	if u1 == 0 {
		u1 = 1e-10
	}
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}
