package main

import (
	"testing"
)

func TestStatsTracker_initialState(t *testing.T) {
	st := &statsTracker{}
	snap := st.snapshot("test-consumer")

	if snap["msgCount"] != int64(0) {
		t.Errorf("msgCount: got %v, want 0", snap["msgCount"])
	}
	if snap["deniedCount"] != int64(0) {
		t.Errorf("deniedCount: got %v, want 0", snap["deniedCount"])
	}
	if snap["lastReceivedAt"] != "" {
		t.Errorf("lastReceivedAt: got %q, want empty", snap["lastReceivedAt"])
	}
	if snap["lastDeniedAt"] != "" {
		t.Errorf("lastDeniedAt: got %q, want empty", snap["lastDeniedAt"])
	}
}

func TestStatsTracker_recordMsg(t *testing.T) {
	st := &statsTracker{}
	st.recordMsg()
	st.recordMsg()

	snap := st.snapshot("c")
	if snap["msgCount"] != int64(2) {
		t.Errorf("msgCount: got %v, want 2", snap["msgCount"])
	}
	if snap["lastReceivedAt"] == "" {
		t.Error("lastReceivedAt should be set after recordMsg")
	}
}

func TestStatsTracker_recordDenied(t *testing.T) {
	st := &statsTracker{}
	st.recordDenied()

	snap := st.snapshot("c")
	if snap["deniedCount"] != int64(1) {
		t.Errorf("deniedCount: got %v, want 1", snap["deniedCount"])
	}
	if snap["lastDeniedAt"] == "" {
		t.Error("lastDeniedAt should be set after recordDenied")
	}
}

func TestStatsTracker_snapshotFields(t *testing.T) {
	st := &statsTracker{}
	snap := st.snapshot("my-consumer")

	if snap["name"] != "my-consumer" {
		t.Errorf("name: got %v, want my-consumer", snap["name"])
	}
	if snap["transport"] != "rest" {
		t.Errorf("transport: got %v, want rest", snap["transport"])
	}
}

func TestStatsTracker_independentCounters(t *testing.T) {
	st := &statsTracker{}
	st.recordMsg()
	st.recordMsg()
	st.recordMsg()
	st.recordDenied()

	snap := st.snapshot("c")
	if snap["msgCount"] != int64(3) {
		t.Errorf("msgCount: got %v, want 3", snap["msgCount"])
	}
	if snap["deniedCount"] != int64(1) {
		t.Errorf("deniedCount: got %v, want 1", snap["deniedCount"])
	}
}
