package common

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StreamLogger writes gnuplot-friendly CSV rows for a sensor-reading stream.
// Each logger owns one file; rows are appended on every call to WriteRow.
type StreamLogger struct {
	mu   sync.Mutex
	file *os.File
}

// NewStreamLogger opens (or creates) logDir/filename for appending.
// Writes the header line if the file is new.
func NewStreamLogger(logDir, filename, header string) (*StreamLogger, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("qoslog mkdir %s: %w", logDir, err)
	}
	path := filepath.Join(logDir, filename)
	isNew := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		isNew = true
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("qoslog open %s: %w", path, err)
	}
	if isNew {
		fmt.Fprintln(f, header)
	}
	return &StreamLogger{file: f}, nil
}

// WriteRow appends one CSV row. Values are formatted as follows:
//   - float64 → 6 significant figures
//   - bool     → 0 or 1
//   - time.Time → Unix milliseconds
//   - anything else → %v
func (l *StreamLogger) WriteRow(values ...interface{}) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, v := range values {
		if i > 0 {
			fmt.Fprint(l.file, ",")
		}
		switch val := v.(type) {
		case float64:
			fmt.Fprintf(l.file, "%.6g", val)
		case bool:
			if val {
				fmt.Fprint(l.file, "1")
			} else {
				fmt.Fprint(l.file, "0")
			}
		case time.Time:
			fmt.Fprintf(l.file, "%d", val.UnixMilli())
		default:
			fmt.Fprintf(l.file, "%v", val)
		}
	}
	fmt.Fprintln(l.file)
}

// FailoverLogger writes one CSV row per FailoverEvent to a dedicated file.
// Columns include orchestration mode and network delay for experiment analysis.
type FailoverLogger struct {
	mu   sync.Mutex
	file *os.File
}

// NewFailoverLogger opens (or creates) logDir/filename for appending.
func NewFailoverLogger(logDir, filename string) (*FailoverLogger, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("failoverlog mkdir %s: %w", logDir, err)
	}
	path := filepath.Join(logDir, filename)
	isNew := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		isNew = true
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failoverlog open %s: %w", path, err)
	}
	if isNew {
		fmt.Fprintln(f,
			"event_id,cdt_id,capability,prev_provider,next_provider,"+
				"failure_time_ms,detection_time_ms,switch_time_ms,"+
				"fail_to_switch_ms,decision_delay_ms,"+
				"orchestration_mode,network_delay_ms,"+
				"accuracy_before,latency_before_ms,reliability_before,"+
				"accuracy_after,latency_after_ms,reliability_after")
	}
	return &FailoverLogger{file: f}, nil
}

// Write appends one FailoverEvent row.
func (fl *FailoverLogger) Write(ev FailoverEvent) {
	if fl == nil {
		return
	}
	fl.mu.Lock()
	defer fl.mu.Unlock()
	fmt.Fprintf(fl.file,
		"%s,%s,%s,%s,%s,%d,%d,%d,%.2f,%.2f,%s,%.1f,%.6g,%.4f,%.6g,%.6g,%.4f,%.6g\n",
		ev.EventID,
		ev.CDTID,
		ev.Capability,
		ev.PrevProvider,
		ev.NextProvider,
		ev.FailureTime.UnixMilli(),
		ev.DetectionTime.UnixMilli(),
		ev.SwitchTime.UnixMilli(),
		ev.FailToSwitchMs,
		ev.DecisionDelayMs,
		ev.OrchestrationMode,
		ev.NetworkDelayMs,
		ev.QoSBefore.Accuracy,
		ev.QoSBefore.LatencyMs,
		ev.QoSBefore.Reliability,
		ev.QoSAfter.Accuracy,
		ev.QoSAfter.LatencyMs,
		ev.QoSAfter.Reliability,
	)
}
