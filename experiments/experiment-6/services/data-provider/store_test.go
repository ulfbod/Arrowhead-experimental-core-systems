package main

import (
	"testing"
)

func TestTelemetryStore_InitialState(t *testing.T) {
	s := newStore()

	if got := string(s.getLatest()); got != "null" {
		t.Errorf("empty store getLatest: got %q, want null", got)
	}

	_, ok := s.getByRobot("robot-1")
	if ok {
		t.Error("empty store: getByRobot should return false")
	}

	stats := s.stats()
	if stats["msgCount"] != int64(0) {
		t.Errorf("msgCount: got %v, want 0", stats["msgCount"])
	}
	if stats["robotCount"] != 0 {
		t.Errorf("robotCount: got %v, want 0", stats["robotCount"])
	}
}

func TestTelemetryStore_Record(t *testing.T) {
	s := newStore()
	msg := []byte(`{"robotId":"robot-1","temp":22.5}`)
	s.record("robot-1", msg)

	latest := s.getLatest()
	if string(latest) != string(msg) {
		t.Errorf("getLatest: got %q, want %q", latest, msg)
	}

	byRobot, ok := s.getByRobot("robot-1")
	if !ok {
		t.Fatal("getByRobot: expected entry for robot-1")
	}
	if string(byRobot) != string(msg) {
		t.Errorf("getByRobot: got %q, want %q", byRobot, msg)
	}

	stats := s.stats()
	if stats["msgCount"] != int64(1) {
		t.Errorf("msgCount: got %v, want 1", stats["msgCount"])
	}
	if stats["robotCount"] != 1 {
		t.Errorf("robotCount: got %v, want 1", stats["robotCount"])
	}
}

func TestTelemetryStore_MultipleRobots(t *testing.T) {
	s := newStore()
	s.record("robot-1", []byte(`{"id":"r1"}`))
	s.record("robot-2", []byte(`{"id":"r2"}`))
	s.record("robot-1", []byte(`{"id":"r1-updated"}`)) // overwrite robot-1

	stats := s.stats()
	if stats["msgCount"] != int64(3) {
		t.Errorf("msgCount: got %v, want 3", stats["msgCount"])
	}
	if stats["robotCount"] != 2 {
		t.Errorf("robotCount: got %v, want 2", stats["robotCount"])
	}

	r1, _ := s.getByRobot("robot-1")
	if string(r1) != `{"id":"r1-updated"}` {
		t.Errorf("robot-1 should have latest message, got %q", r1)
	}
}

func TestTelemetryStore_LatestIsLastReceived(t *testing.T) {
	s := newStore()
	s.record("robot-1", []byte(`{"seq":1}`))
	s.record("robot-2", []byte(`{"seq":2}`))

	// Latest should be the last message across all robots.
	latest := s.getLatest()
	if string(latest) != `{"seq":2}` {
		t.Errorf("latest: got %q, want seq=2", latest)
	}
}

func TestTelemetryStore_UnknownRobot(t *testing.T) {
	s := newStore()
	s.record("robot-1", []byte(`{}`))
	_, ok := s.getByRobot("robot-99")
	if ok {
		t.Error("unknown robot should return false")
	}
}

func TestTelemetryStore_EmptyRobotID(t *testing.T) {
	s := newStore()
	// An empty robotID should still update latest but not add to byRobot.
	s.record("", []byte(`{"key":"val"}`))

	if string(s.getLatest()) != `{"key":"val"}` {
		t.Error("latest should be updated even with empty robotID")
	}

	stats := s.stats()
	if stats["robotCount"] != 0 {
		t.Errorf("empty robotID should not increment robotCount, got %v", stats["robotCount"])
	}
}
