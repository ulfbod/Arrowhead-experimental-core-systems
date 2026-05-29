package generalmgmt_test

import (
	"fmt"
	"testing"
	"time"

	"arrowhead/core/internal/generalmgmt"
)

func TestRingBufferCapacity(t *testing.T) {
	buf := generalmgmt.NewLogBuffer(5)
	for i := 0; i < 10; i++ {
		buf.Append(generalmgmt.LogEntry{Message: fmt.Sprintf("msg%d", i), Severity: "INFO"})
	}
	entries := buf.All()
	if len(entries) != 5 {
		t.Errorf("ring buffer len = %d, want 5 (capacity)", len(entries))
	}
	// Most recent 5 should be msg5..msg9.
	if entries[0].Message != "msg5" {
		t.Errorf("oldest entry = %q, want msg5", entries[0].Message)
	}
}

func TestRingBufferSeverityFilter(t *testing.T) {
	buf := generalmgmt.NewLogBuffer(100)
	buf.Append(generalmgmt.LogEntry{Message: "info-msg", Severity: "INFO"})
	buf.Append(generalmgmt.LogEntry{Message: "warn-msg", Severity: "WARN"})
	buf.Append(generalmgmt.LogEntry{Message: "error-msg", Severity: "ERROR"})
	// Exact severity match: only WARN.
	entries := buf.Filter(generalmgmt.LogFilter{Severity: "WARN"})
	if len(entries) != 1 {
		t.Errorf("filtered len = %d, want 1 (exact WARN match)", len(entries))
	}
}

func TestRingBufferTimeRange(t *testing.T) {
	buf := generalmgmt.NewLogBuffer(100)
	past := time.Now().Add(-2 * time.Hour)
	present := time.Now()
	buf.Append(generalmgmt.LogEntry{Message: "old", Severity: "INFO", EntryDate: past})
	buf.Append(generalmgmt.LogEntry{Message: "new", Severity: "INFO", EntryDate: present})
	// From is RFC3339 string.
	from := time.Now().Add(-time.Hour).Format(time.RFC3339)
	entries := buf.Filter(generalmgmt.LogFilter{From: from})
	if len(entries) != 1 || entries[0].Message != "new" {
		t.Errorf("time filter: got %v, want [new]", entries)
	}
}
