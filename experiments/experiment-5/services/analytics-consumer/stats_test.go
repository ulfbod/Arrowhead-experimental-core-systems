package main

import (
	"testing"
)

func TestStatsTracker_initialState(t *testing.T) {
	st := &statsTracker{}
	snap := st.snapshot("analytics-consumer")

	if snap["msgCount"] != int64(0) {
		t.Errorf("msgCount: got %v, want 0", snap["msgCount"])
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
	st.recordMsg()

	snap := st.snapshot("c")
	if snap["msgCount"] != int64(3) {
		t.Errorf("msgCount: got %v, want 3", snap["msgCount"])
	}
	if snap["lastReceivedAt"] == "" {
		t.Error("lastReceivedAt should be set after recordMsg")
	}
}

func TestStatsTracker_recordDenied(t *testing.T) {
	st := &statsTracker{}
	st.recordDenied()

	snap := st.snapshot("c")
	if snap["lastDeniedAt"] == "" {
		t.Error("lastDeniedAt should be set after recordDenied")
	}
	// deniedCount is not tracked in analytics-consumer statsTracker;
	// only the timestamp is recorded. Verify no panic.
}

func TestStatsTracker_snapshotFields(t *testing.T) {
	st := &statsTracker{}
	snap := st.snapshot("analytics-consumer")

	for _, key := range []string{"name", "transport", "msgCount", "lastReceivedAt", "lastDeniedAt"} {
		if _, ok := snap[key]; !ok {
			t.Errorf("snapshot missing key %q", key)
		}
	}
	if snap["transport"] != "kafka-sse" {
		t.Errorf("transport: got %v, want kafka-sse", snap["transport"])
	}
	if snap["name"] != "analytics-consumer" {
		t.Errorf("name: got %v, want analytics-consumer", snap["name"])
	}
}
