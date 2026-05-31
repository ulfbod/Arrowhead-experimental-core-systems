// Package service implements Device QoS Evaluator business logic.
package service

import (
	"net"
	"time"

	"github.com/google/uuid"

	"arrowhead/core/internal/deviceqoseval/model"
	"arrowhead/core/internal/deviceqoseval/repository"
)

// Evaluator performs QoS measurements and queries stored results.
type Evaluator struct {
	repo repository.Repository
}

// NewEvaluator creates a new Evaluator backed by repo.
func NewEvaluator(repo repository.Repository) *Evaluator {
	return &Evaluator{repo: repo}
}

// Measure performs a TCP dial to host:port and records the latency.
// The result is saved to the repository and returned.
func (e *Evaluator) Measure(host, port string) *model.QoSRecord {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	elapsed := time.Since(start).Milliseconds()
	reachable := err == nil
	if conn != nil {
		conn.Close() //nolint:errcheck
	}
	rec := &model.QoSRecord{
		ID:         uuid.New().String(),
		Host:       host,
		Port:       port,
		LatencyMs:  elapsed,
		MeasuredAt: time.Now().UTC().Format(time.RFC3339),
		Reachable:  reachable,
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
