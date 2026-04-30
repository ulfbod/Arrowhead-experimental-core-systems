// stats.go — per-robot record keeping and latency statistics for edge-adapter.
package main

import (
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// robotRecord holds the latest message and rolling statistics for one robot.
type robotRecord struct {
	seq        int64
	payload    json.RawMessage
	receivedAt time.Time
	latencyMs  float64

	msgCount  int64
	msgTimes  []time.Time // rolling window (last 100) for rate computation
	latencies []float64   // rolling window (last 100) for percentile computation
	byteWin   []byteEntry // rolling window (last 100) for throughput computation
}

type byteEntry struct {
	ts    time.Time
	bytes int
}

// minimalPayload extracts only the fields needed for latency computation.
type minimalPayload struct {
	RobotID   string `json:"robotId"`
	Timestamp string `json:"timestamp"`
}

// parseRobotID returns the robotId from a raw JSON payload; falls back to
// "unknown" if parsing fails.
func parseRobotID(raw json.RawMessage) string {
	var mp minimalPayload
	json.Unmarshal(raw, &mp) //nolint:errcheck
	if mp.RobotID == "" {
		return "unknown"
	}
	return mp.RobotID
}

// computeLatency parses the timestamp in the payload and returns milliseconds
// between the publish time and now.  Returns 0 if the timestamp is missing or
// unparseable.
func computeLatency(raw json.RawMessage, receivedAt time.Time) float64 {
	var mp minimalPayload
	if err := json.Unmarshal(raw, &mp); err != nil || mp.Timestamp == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, mp.Timestamp)
	if err != nil {
		t, err = time.Parse(time.RFC3339, mp.Timestamp)
		if err != nil {
			return 0
		}
	}
	d := receivedAt.Sub(t)
	if d < 0 {
		return 0
	}
	return float64(d.Milliseconds())
}

// maxWindow is the rolling-window depth kept per robot.
const maxWindow = 100

func appendCapped[T any](s []T, v T) []T {
	s = append(s, v)
	if len(s) > maxWindow {
		s = s[len(s)-maxWindow:]
	}
	return s
}

// LatencyStats is the stats returned per robot on /telemetry/stats.
type LatencyStats struct {
	Mean float64 `json:"mean"`
	P50  float64 `json:"p50"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
	Max  float64 `json:"max"`
}

// computeLatencyStats computes statistics from a latency window (not sorted).
func computeLatencyStats(lats []float64) LatencyStats {
	if len(lats) == 0 {
		return LatencyStats{}
	}
	cp := make([]float64, len(lats))
	copy(cp, lats)
	sort.Float64s(cp)
	var sum float64
	for _, v := range cp {
		sum += v
	}
	return LatencyStats{
		Mean: sum / float64(len(cp)),
		P50:  percentile(cp, 50),
		P95:  percentile(cp, 95),
		P99:  percentile(cp, 99),
		Max:  cp[len(cp)-1],
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p / 100 * float64(len(sorted)-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[lo]
	}
	return sorted[lo]*(1-(idx-float64(lo))) + sorted[hi]*(idx-float64(lo))
}

// RobotStatsEntry is one robot's contribution to the /telemetry/stats response.
type RobotStatsEntry struct {
	LastSeq        int64        `json:"lastSeq"`
	MsgCount       int64        `json:"msgCount"`
	RateHz         float64      `json:"rateHz"`
	LatencyMs      LatencyStats `json:"latencyMs"`
	LastReceivedAt string       `json:"lastReceivedAt"`
}

// AggregateStats summarises all robots in /telemetry/stats.
type AggregateStats struct {
	RobotCount    int          `json:"robotCount"`
	TotalMsgCount int64        `json:"totalMsgCount"`
	TotalRateHz   float64      `json:"totalRateHz"`
	TotalKbps     float64      `json:"totalKbps"`
	LatencyMs     LatencyStats `json:"latencyMs"`
}

// TelemetryStatsResponse is the full /telemetry/stats payload.
type TelemetryStatsResponse struct {
	Robots    map[string]RobotStatsEntry `json:"robots"`
	Aggregate AggregateStats             `json:"aggregate"`
}

// LatestEntry is one robot's data in /telemetry/all.
type LatestEntry struct {
	ReceivedAt string          `json:"receivedAt"`
	Payload    json.RawMessage `json:"payload"`
}

// State is the guarded store of per-robot records.
type State struct {
	mu     sync.RWMutex
	robots map[string]*robotRecord
}

func newState() *State {
	return &State{robots: make(map[string]*robotRecord)}
}

// Update ingests a new AMQP message.
func (s *State) Update(raw json.RawMessage) {
	now := time.Now()
	id := parseRobotID(raw)
	lat := computeLatency(raw, now)

	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.robots[id]
	if !ok {
		rec = &robotRecord{}
		s.robots[id] = rec
	}
	rec.payload = raw
	rec.receivedAt = now
	rec.latencyMs = lat
	rec.msgCount++
	rec.msgTimes = appendCapped(rec.msgTimes, now)
	rec.latencies = appendCapped(rec.latencies, lat)
	rec.byteWin = appendCapped(rec.byteWin, byteEntry{ts: now, bytes: len(raw)})
}

// Latest returns the most recently received record (any robot) for backward-
// compat with the original /telemetry/latest endpoint.
func (s *State) Latest() (json.RawMessage, time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *robotRecord
	for _, rec := range s.robots {
		if latest == nil || rec.receivedAt.After(latest.receivedAt) {
			latest = rec
		}
	}
	if latest == nil {
		return nil, time.Time{}, false
	}
	return latest.payload, latest.receivedAt, true
}

// All returns a snapshot of the latest entry per robot.
func (s *State) All() map[string]LatestEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]LatestEntry, len(s.robots))
	for id, rec := range s.robots {
		out[id] = LatestEntry{
			ReceivedAt: rec.receivedAt.UTC().Format(time.RFC3339),
			Payload:    rec.payload,
		}
	}
	return out
}

// Stats computes the full /telemetry/stats response.
func (s *State) Stats() TelemetryStatsResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := TelemetryStatsResponse{
		Robots: make(map[string]RobotStatsEntry, len(s.robots)),
	}

	now := time.Now()
	window10s := now.Add(-10 * time.Second)
	var allLats []float64
	var totalRate, totalKbps float64
	var totalMsg int64

	for id, rec := range s.robots {
		// Compute message rate from rolling msgTimes.
		rateCount := 0
		for _, t := range rec.msgTimes {
			if t.After(window10s) {
				rateCount++
			}
		}
		rate := float64(rateCount) / 10.0

		// Compute throughput KB/s from byteWin.
		var byteSum int
		for _, e := range rec.byteWin {
			if e.ts.After(window10s) {
				byteSum += e.bytes
			}
		}
		kbps := float64(byteSum) / 10.0 / 1000.0

		lstats := computeLatencyStats(rec.latencies)
		allLats = append(allLats, rec.latencies...)
		totalRate += rate
		totalKbps += kbps
		totalMsg += rec.msgCount

		out.Robots[id] = RobotStatsEntry{
			LastSeq:        rec.seq,
			MsgCount:       rec.msgCount,
			RateHz:         rate,
			LatencyMs:      lstats,
			LastReceivedAt: rec.receivedAt.UTC().Format(time.RFC3339),
		}
	}

	out.Aggregate = AggregateStats{
		RobotCount:    len(s.robots),
		TotalMsgCount: totalMsg,
		TotalRateHz:   totalRate,
		TotalKbps:     totalKbps,
		LatencyMs:     computeLatencyStats(allLats),
	}
	return out
}
