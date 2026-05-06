package main

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestFleetConfigRoundTrip(t *testing.T) {
	cfg := FleetConfig{
		PayloadType: "imu",
		PayloadHz:   10,
		Robots: []RobotConfig{
			{ID: "robot-1", Network: NetworkProfile{Preset: "5g_good"}},
			{ID: "robot-2", Network: NetworkProfile{Preset: "custom", LatencyMs: 50, JitterMs: 10, PacketLossPercent: 0.5, BandwidthKbps: 500}},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded FleetConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.PayloadType != "imu" {
		t.Errorf("PayloadType: got %q", decoded.PayloadType)
	}
	if len(decoded.Robots) != 2 {
		t.Fatalf("Robots len: got %d", len(decoded.Robots))
	}
	if decoded.Robots[1].Network.LatencyMs != 50 {
		t.Errorf("custom LatencyMs: got %v", decoded.Robots[1].Network.LatencyMs)
	}
}

func TestFleetStatsRoundTrip(t *testing.T) {
	fs := FleetStats{
		Robots: map[string]RobotStats{
			"robot-1": {MsgSent: 50, MsgDropped: 1, BytesSent: 10000, RateHz: 4.9, KbpsSent: 9.5},
		},
		Aggregate: RobotStats{MsgSent: 50, MsgDropped: 1},
	}
	data, err := json.Marshal(fs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded FleetStats
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Robots["robot-1"].MsgSent != 50 {
		t.Errorf("MsgSent: got %d", decoded.Robots["robot-1"].MsgSent)
	}
}

func TestIMUPayloadShape(t *testing.T) {
	p := imuPayload{
		RobotID:     "robot-1",
		Timestamp:   "2026-01-01T00:00:00Z",
		Seq:         1,
		IMU:         imuData{Roll: 0.1, Pitch: -0.05, Yaw: 1.57},
		Position:    positionData{X: 10.5, Y: 5.3, Z: 0},
		Temperature: 22.1,
		Humidity:    55.3,
	}
	data, _ := json.Marshal(p)
	var m map[string]any
	json.Unmarshal(data, &m)
	for _, key := range []string{"robotId", "timestamp", "seq", "imu", "position", "temperature", "humidity"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in imu payload", key)
		}
	}
}

func TestBasicPayloadShape(t *testing.T) {
	p := basicPayload{
		RobotID:     "robot-1",
		Timestamp:   "2026-01-01T00:00:00Z",
		Seq:         1,
		Temperature: 22.1,
		Humidity:    55.3,
	}
	data, _ := json.Marshal(p)
	var m map[string]any
	json.Unmarshal(data, &m)
	for _, key := range []string{"robotId", "timestamp", "seq", "temperature", "humidity"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in basic payload", key)
		}
	}
	if _, ok := m["imu"]; ok {
		t.Error("basic payload must not contain imu field")
	}
}

// TestNewFleetWithPublisher verifies that the custom publish callback is invoked
// for each robot message (exp-5 adds dual-publish via PublishFn).
func TestNewFleetWithPublisher_callsPublishFn(t *testing.T) {
	var mu sync.Mutex
	var published []string

	fn := func(routingKey string, payload []byte) {
		mu.Lock()
		published = append(published, routingKey)
		mu.Unlock()
	}

	cfg := FleetConfig{
		PayloadType: "basic",
		PayloadHz:   50, // fast so test doesn't take long
		Robots: []RobotConfig{
			{ID: "robot-a", Network: NetworkProfile{Preset: "5g_excellent"}},
		},
	}

	fleet := NewFleetWithPublisher(nil, cfg, fn)
	defer fleet.UpdateConfig(FleetConfig{Robots: nil, PayloadHz: 1})

	// Wait for at least one message to be published (up to 500ms).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(published)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	n := len(published)
	mu.Unlock()

	if n == 0 {
		t.Fatal("publish function was never called")
	}
	mu.Lock()
	key := published[0]
	mu.Unlock()
	if key != "telemetry.robot-a" {
		t.Errorf("routing key: got %q, want %q", key, "telemetry.robot-a")
	}
}

func TestNewFleetWithPublisher_defaultsApplied(t *testing.T) {
	fn := func(string, []byte) {}
	cfg := FleetConfig{
		PayloadHz: 0, // should default to 1
	}
	fleet := NewFleetWithPublisher(nil, cfg, fn)
	defer fleet.UpdateConfig(FleetConfig{})

	if fleet.Config().PayloadHz != 1 {
		t.Errorf("PayloadHz should default to 1, got %v", fleet.Config().PayloadHz)
	}
	if fleet.Config().PayloadType != "imu" {
		t.Errorf("PayloadType should default to imu, got %q", fleet.Config().PayloadType)
	}
}

func TestFleet_RobotCount(t *testing.T) {
	fn := func(string, []byte) {}
	cfg := FleetConfig{
		PayloadHz: 1,
		Robots: []RobotConfig{
			{ID: "r1"},
			{ID: "r2"},
			{ID: "r3"},
		},
	}
	fleet := NewFleetWithPublisher(nil, cfg, fn)
	defer fleet.UpdateConfig(FleetConfig{})

	if fleet.RobotCount() != 3 {
		t.Errorf("RobotCount: got %d, want 3", fleet.RobotCount())
	}
}
