// stats.go — per-robot send-side counters and rolling-window rate computation.
package main

import (
	"sort"
	"sync"
	"time"
)

// windowEntry records a single publish event.
type windowEntry struct {
	ts    time.Time
	bytes int
}

// robotCounter accumulates send-side metrics for one robot goroutine.
// All methods are safe for concurrent use.
type robotCounter struct {
	mu       sync.Mutex
	sent     int64
	dropped  int64
	window   []windowEntry // rolling 10-second window
}

func (rc *robotCounter) recordSent(nBytes int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.sent++
	now := time.Now()
	rc.window = append(rc.window, windowEntry{ts: now, bytes: nBytes})
	rc.prune(now)
}

func (rc *robotCounter) recordDropped() {
	rc.mu.Lock()
	rc.dropped++
	rc.mu.Unlock()
}

func (rc *robotCounter) prune(now time.Time) {
	cutoff := now.Add(-10 * time.Second)
	i := 0
	for i < len(rc.window) && rc.window[i].ts.Before(cutoff) {
		i++
	}
	rc.window = rc.window[i:]
}

func (rc *robotCounter) snapshot() RobotStats {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	now := time.Now()
	rc.prune(now)
	var byteSum int
	for _, e := range rc.window {
		byteSum += e.bytes
	}
	n := float64(len(rc.window))
	rateHz := n / 10.0
	kbps := float64(byteSum) / 10.0 / 1000.0
	return RobotStats{
		MsgSent:    rc.sent,
		MsgDropped: rc.dropped,
		BytesSent:  int64(byteSum),
		RateHz:     rateHz,
		KbpsSent:   kbps,
	}
}

// RobotStats is the snapshot exported via the /stats endpoint.
type RobotStats struct {
	MsgSent    int64   `json:"msgSent"`
	MsgDropped int64   `json:"msgDropped"`
	BytesSent  int64   `json:"bytesSent"`
	RateHz     float64 `json:"rateHz"`
	KbpsSent   float64 `json:"kbpsSent"`
}

// FleetStats aggregates per-robot stats for the /stats endpoint.
type FleetStats struct {
	Robots    map[string]RobotStats `json:"robots"`
	Aggregate RobotStats            `json:"aggregate"`
}

// Percentile returns the p-th percentile (0–100) of the given sorted slice.
// The slice must be sorted ascending.  Returns 0 if empty.
func Percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	idx := p / 100 * float64(len(sorted)-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// ComputePercentiles sorts a copy of vals and returns p50, p95, p99.
func ComputePercentiles(vals []float64) (mean, p50, p95, p99, max float64) {
	if len(vals) == 0 {
		return
	}
	cp := make([]float64, len(vals))
	copy(cp, vals)
	sort.Float64s(cp)
	var sum float64
	for _, v := range cp {
		sum += v
	}
	mean = sum / float64(len(cp))
	p50 = Percentile(cp, 50)
	p95 = Percentile(cp, 95)
	p99 = Percentile(cp, 99)
	max = cp[len(cp)-1]
	return
}
