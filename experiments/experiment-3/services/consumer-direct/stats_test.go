package main

import (
	"testing"
)

func TestStatsTracker_initialState(t *testing.T) {
	st := &statsTracker{}
	snap := st.snapshot("consumer-direct")

	if snap["msgCount"] != int64(0) {
		t.Errorf("msgCount: got %v, want 0", snap["msgCount"])
	}
	if snap["lastReceivedAt"] != "" {
		t.Errorf("lastReceivedAt: got %q, want empty", snap["lastReceivedAt"])
	}
	if snap["name"] != "consumer-direct" {
		t.Errorf("name: got %v, want consumer-direct", snap["name"])
	}
}

func TestStatsTracker_record(t *testing.T) {
	st := &statsTracker{}
	st.record()
	st.record()

	snap := st.snapshot("c")
	if snap["msgCount"] != int64(2) {
		t.Errorf("msgCount: got %v, want 2", snap["msgCount"])
	}
	if snap["lastReceivedAt"] == "" {
		t.Error("lastReceivedAt should be set after record()")
	}
}

func TestStatsTracker_snapshotKeys(t *testing.T) {
	st := &statsTracker{}
	snap := st.snapshot("my-consumer")

	for _, key := range []string{"name", "msgCount", "lastReceivedAt"} {
		if _, ok := snap[key]; !ok {
			t.Errorf("snapshot missing key %q", key)
		}
	}
}
