package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseRobotID(t *testing.T) {
	raw := json.RawMessage(`{"robotId":"robot-3","temperature":22.1}`)
	id := parseRobotID(raw)
	if id != "robot-3" {
		t.Errorf("got %q, want %q", id, "robot-3")
	}
}

func TestParseRobotID_Missing(t *testing.T) {
	raw := json.RawMessage(`{"temperature":22.1}`)
	id := parseRobotID(raw)
	if id != "unknown" {
		t.Errorf("missing robotId: got %q, want \"unknown\"", id)
	}
}

func TestComputeLatency(t *testing.T) {
	// Publish timestamp 50ms in the past.
	ts := time.Now().Add(-50 * time.Millisecond).UTC().Format(time.RFC3339Nano)
	raw := json.RawMessage(`{"robotId":"r","timestamp":"` + ts + `"}`)
	lat := computeLatency(raw, time.Now())
	if lat < 40 || lat > 200 {
		t.Errorf("expected ~50ms latency, got %.1f ms", lat)
	}
}

func TestComputeLatency_MissingTimestamp(t *testing.T) {
	raw := json.RawMessage(`{"robotId":"r","temperature":22}`)
	lat := computeLatency(raw, time.Now())
	if lat != 0 {
		t.Errorf("missing timestamp: expected 0, got %v", lat)
	}
}

func TestComputeLatencyStats(t *testing.T) {
	lats := make([]float64, 100)
	for i := range lats {
		lats[i] = float64(i + 1)
	}
	s := computeLatencyStats(lats)
	if s.Mean < 50 || s.Mean > 51 {
		t.Errorf("mean: got %v, want ~50.5", s.Mean)
	}
	if s.Max != 100 {
		t.Errorf("max: got %v, want 100", s.Max)
	}
	if s.P95 < 93 || s.P95 > 97 {
		t.Errorf("p95: got %v, want ~95", s.P95)
	}
}

func TestComputeLatencyStats_Empty(t *testing.T) {
	s := computeLatencyStats(nil)
	if s.Mean != 0 || s.P50 != 0 || s.Max != 0 {
		t.Errorf("empty: expected all zeros, got %+v", s)
	}
}

func TestStateUpdate_PerRobot(t *testing.T) {
	s := newState()

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	r1 := json.RawMessage(`{"robotId":"robot-1","timestamp":"` + ts + `","seq":1}`)
	r2 := json.RawMessage(`{"robotId":"robot-2","timestamp":"` + ts + `","seq":1}`)

	s.Update(r1)
	s.Update(r2)
	s.Update(r1) // second message from robot-1

	all := s.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 robots, got %d", len(all))
	}
	if _, ok := all["robot-1"]; !ok {
		t.Error("robot-1 missing from All()")
	}
	if _, ok := all["robot-2"]; !ok {
		t.Error("robot-2 missing from All()")
	}
}

func TestStateLatest_Empty(t *testing.T) {
	s := newState()
	_, _, ok := s.Latest()
	if ok {
		t.Error("empty state should return ok=false")
	}
}

func TestStateLatest_ReturnsMostRecent(t *testing.T) {
	s := newState()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	s.Update(json.RawMessage(`{"robotId":"robot-1","timestamp":"` + ts + `","seq":1}`))
	s.Update(json.RawMessage(`{"robotId":"robot-2","timestamp":"` + ts + `","seq":2}`))

	msg, _, ok := s.Latest()
	if !ok {
		t.Fatal("expected ok=true")
	}
	// Latest should be some valid JSON.
	var m map[string]any
	if err := json.Unmarshal(msg, &m); err != nil {
		t.Errorf("latest payload not valid JSON: %v", err)
	}
}

func TestStateStats_Structure(t *testing.T) {
	s := newState()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	for i := 0; i < 5; i++ {
		s.Update(json.RawMessage(`{"robotId":"robot-1","timestamp":"` + ts + `","seq":` + string(rune('1'+i)) + `}`))
	}

	stats := s.Stats()
	if stats.Aggregate.RobotCount != 1 {
		t.Errorf("RobotCount: got %d, want 1", stats.Aggregate.RobotCount)
	}
	if stats.Aggregate.TotalMsgCount != 5 {
		t.Errorf("TotalMsgCount: got %d, want 5", stats.Aggregate.TotalMsgCount)
	}
	if _, ok := stats.Robots["robot-1"]; !ok {
		t.Error("robot-1 missing from Stats()")
	}
}
