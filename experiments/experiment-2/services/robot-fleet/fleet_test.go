package main

import (
	"encoding/json"
	"testing"
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
	if decoded.PayloadHz != 10 {
		t.Errorf("PayloadHz: got %v", decoded.PayloadHz)
	}
	if len(decoded.Robots) != 2 {
		t.Fatalf("Robots len: got %d", len(decoded.Robots))
	}
	if decoded.Robots[0].ID != "robot-1" {
		t.Errorf("Robots[0].ID: got %q", decoded.Robots[0].ID)
	}
	if decoded.Robots[1].Network.LatencyMs != 50 {
		t.Errorf("custom LatencyMs: got %v", decoded.Robots[1].Network.LatencyMs)
	}
}

func TestFleetStatsRoundTrip(t *testing.T) {
	fs := FleetStats{
		Robots: map[string]RobotStats{
			"robot-1": {MsgSent: 100, MsgDropped: 2, BytesSent: 20000, RateHz: 9.9, KbpsSent: 19.5},
		},
		Aggregate: RobotStats{MsgSent: 100, MsgDropped: 2, BytesSent: 20000, RateHz: 9.9, KbpsSent: 19.5},
	}

	data, err := json.Marshal(fs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded FleetStats
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	r1, ok := decoded.Robots["robot-1"]
	if !ok {
		t.Fatal("robot-1 not in decoded robots")
	}
	if r1.MsgSent != 100 {
		t.Errorf("MsgSent: got %d", r1.MsgSent)
	}
}

func TestIMUPayloadShape(t *testing.T) {
	p := imuPayload{
		RobotID:   "robot-1",
		Timestamp: "2026-01-01T00:00:00Z",
		Seq:       1,
		IMU:       imuData{Roll: 0.1, Pitch: -0.05, Yaw: 1.57},
		Position:  positionData{X: 10.5, Y: 5.3, Z: 0},
		Temperature: 22.1,
		Humidity:    55.3,
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify expected keys exist in the JSON.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	for _, key := range []string{"robotId", "timestamp", "seq", "imu", "position", "temperature", "humidity"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in imu payload", key)
		}
	}

	imu, ok := m["imu"].(map[string]any)
	if !ok {
		t.Fatal("imu field is not a map")
	}
	if _, ok := imu["roll"]; !ok {
		t.Error("imu missing roll")
	}
	if _, ok := imu["yaw"]; !ok {
		t.Error("imu missing yaw")
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

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"robotId", "timestamp", "seq", "temperature", "humidity"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in basic payload", key)
		}
	}
	// IMU fields must NOT be present in basic payload.
	if _, ok := m["imu"]; ok {
		t.Error("basic payload must not contain imu field")
	}
}
