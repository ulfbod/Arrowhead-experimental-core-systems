// Package generalmgmt implements the AH5 GeneralManagement endpoints shared by
// every core system: POST /<prefix>/general/mgmt/logs and
// GET /<prefix>/general/mgmt/get-config.
package generalmgmt

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---- LogEntry ---------------------------------------------------------------

// LogEntry is one log record stored in the ring buffer.
type LogEntry struct {
	LogID     string    `json:"logId"`
	EntryDate time.Time `json:"entryDate"`
	Logger    string    `json:"logger"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	Exception string    `json:"exception,omitempty"`
}

// ---- LogFilter --------------------------------------------------------------

// LogFilter constrains which entries Filter returns.
// Zero-value fields are treated as "no constraint".
type LogFilter struct {
	From      string // RFC3339 — entries at or after this time
	To        string // RFC3339 — entries at or before this time
	Severity  string // exact match
	LoggerStr string // substring match against Logger
}

// ---- LogBuffer --------------------------------------------------------------

var logIDSeq int64

// LogBuffer is a thread-safe fixed-capacity ring buffer of LogEntry values.
type LogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	cap     int
	head    int // index of the next write slot
	count   int // number of valid entries (≤ cap)
}

// NewLogBuffer returns a LogBuffer with the given capacity.
func NewLogBuffer(capacity int) *LogBuffer {
	return &LogBuffer{entries: make([]LogEntry, capacity), cap: capacity}
}

// Append adds e to the buffer, overwriting the oldest entry when full.
// Sets EntryDate to time.Now() if zero, and assigns a unique LogID if empty.
func (b *LogBuffer) Append(e LogEntry) {
	if e.EntryDate.IsZero() {
		e.EntryDate = time.Now()
	}
	if e.LogID == "" {
		e.LogID = fmt.Sprintf("%d", atomic.AddInt64(&logIDSeq, 1))
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries[b.head] = e
	b.head = (b.head + 1) % b.cap
	if b.count < b.cap {
		b.count++
	}
}

// All returns all stored entries in insertion order (oldest first).
func (b *LogBuffer) All() []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]LogEntry, b.count)
	start := 0
	if b.count == b.cap {
		start = b.head // oldest entry is at head when buffer is full
	}
	for i := 0; i < b.count; i++ {
		out[i] = b.entries[(start+i)%b.cap]
	}
	return out
}

// Filter returns entries that match all non-zero fields of f.
func (b *LogBuffer) Filter(f LogFilter) []LogEntry {
	all := b.All()
	var from, to time.Time
	if f.From != "" {
		from, _ = time.Parse(time.RFC3339, f.From)
	}
	if f.To != "" {
		to, _ = time.Parse(time.RFC3339, f.To)
	}
	var out []LogEntry
	for _, e := range all {
		if f.Severity != "" && e.Severity != f.Severity {
			continue
		}
		if !from.IsZero() && e.EntryDate.Before(from) {
			continue
		}
		if !to.IsZero() && e.EntryDate.After(to) {
			continue
		}
		if f.LoggerStr != "" && !strings.Contains(e.Logger, f.LoggerStr) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ---- SlogHandler ------------------------------------------------------------

// SlogHandler implements slog.Handler backed by a LogBuffer.
// Use slog.SetDefault(slog.New(NewSlogHandler(buf))) to capture slog output.
type SlogHandler struct {
	buf    *LogBuffer
	logger string
	attrs  []slog.Attr
}

// NewSlogHandler returns a slog.Handler that writes records to buf.
func NewSlogHandler(buf *LogBuffer) *SlogHandler {
	return &SlogHandler{buf: buf}
}

func (h *SlogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *SlogHandler) Handle(_ context.Context, r slog.Record) error {
	h.buf.Append(LogEntry{
		EntryDate: r.Time,
		Logger:    h.logger,
		Severity:  levelName(r.Level),
		Message:   r.Message,
	})
	return nil
}

func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SlogHandler{buf: h.buf, logger: h.logger, attrs: append(h.attrs, attrs...)}
}

func (h *SlogHandler) WithGroup(name string) slog.Handler {
	return &SlogHandler{buf: h.buf, logger: name, attrs: h.attrs}
}

func levelName(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERROR"
	case l >= slog.LevelWarn:
		return "WARN"
	case l >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}
