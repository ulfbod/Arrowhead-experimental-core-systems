// Package service implements Device QoS Evaluator business logic.
package service

import (
	"math"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"

	"arrowhead/core/internal/deviceqoseval/model"
	"arrowhead/core/internal/deviceqoseval/repository"
)

const probeCount = 5

// Evaluator performs QoS measurements and queries stored results.
type Evaluator struct {
	repo repository.Repository
}

// NewEvaluator creates a new Evaluator backed by repo.
func NewEvaluator(repo repository.Repository) *Evaluator {
	return &Evaluator{repo: repo}
}

// probeTimeout returns the per-probe timeout from QOS_PROBE_TIMEOUT_SECONDS (default 5s).
func probeTimeout() time.Duration {
	if v := os.Getenv("QOS_PROBE_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 5 * time.Second
}

// Measure performs multi-probe TCP measurements to host:port and records the result.
// Computes latency (mean of 5 probes), jitter (stddev of latencies), bandwidth
// (64KB throughput probe), and packet loss (failed probe fraction).
func (e *Evaluator) Measure(host, port string) *model.QoSRecord {
	timeout := probeTimeout()
	addr := net.JoinHostPort(host, port)

	// 5-probe RTT measurement.
	var latencies []int64
	failures := 0
	for i := 0; i < probeCount; i++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, timeout)
		elapsed := time.Since(start).Milliseconds()
		if err != nil {
			failures++
		} else {
			latencies = append(latencies, elapsed)
			conn.Close() //nolint:errcheck
		}
	}

	reachable := failures < probeCount
	packetLoss := float64(failures) / float64(probeCount) * 100.0

	var latencyMs int64
	var jitterMs int64
	if len(latencies) > 0 {
		var sum int64
		for _, l := range latencies {
			sum += l
		}
		latencyMs = sum / int64(len(latencies))

		// Jitter = stddev of latencies.
		if len(latencies) > 1 {
			mean := float64(latencyMs)
			var variance float64
			for _, l := range latencies {
				diff := float64(l) - mean
				variance += diff * diff
			}
			variance /= float64(len(latencies))
			jitterMs = int64(math.Round(math.Sqrt(variance)))
		}
	}

	// Bandwidth probe: connect, write 64KB, measure throughput.
	var bandwidthBps int64
	if reachable {
		bwBuf := make([]byte, 64*1024)
		start := time.Now()
		bwConn, err := net.DialTimeout("tcp", addr, timeout)
		if err == nil {
			bwConn.SetDeadline(time.Now().Add(timeout)) //nolint:errcheck
			n, _ := bwConn.Write(bwBuf)
			elapsed := time.Since(start)
			bwConn.Close() //nolint:errcheck
			if elapsed > 0 && n > 0 {
				bandwidthBps = int64(float64(n) / elapsed.Seconds())
			}
		}
	}

	rec := &model.QoSRecord{
		ID:           uuid.New().String(),
		Host:         host,
		Port:         port,
		LatencyMs:    latencyMs,
		MeasuredAt:   time.Now().UTC().Format(time.RFC3339),
		Reachable:    reachable,
		BandwidthBps: bandwidthBps,
		JitterMs:     jitterMs,
		PacketLoss:   packetLoss,
	}
	e.repo.Save(rec)
	return rec
}

// Query returns all stored records filtered by the request parameters.
func (e *Evaluator) Query(req model.QueryRequest) []*model.QoSRecord {
	all := e.repo.All()
	var out []*model.QoSRecord
	for _, r := range all {
		if req.Host != "" && r.Host != req.Host {
			continue
		}
		if req.Port != "" && r.Port != req.Port {
			continue
		}
		out = append(out, r)
	}
	return out
}
