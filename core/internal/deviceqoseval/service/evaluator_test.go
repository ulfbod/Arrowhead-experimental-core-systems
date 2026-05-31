package service_test

import (
	"net"
	"testing"

	"arrowhead/core/internal/deviceqoseval/model"
	"arrowhead/core/internal/deviceqoseval/repository"
	"arrowhead/core/internal/deviceqoseval/service"
)

func newEvaluator() *service.Evaluator {
	return service.NewEvaluator(repository.NewMemoryRepository())
}

func TestMeasureLocalhostReturnsPositiveLatency(t *testing.T) {
	// Start a TCP listener on a random port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close() //nolint:errcheck
	}()

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	eval := newEvaluator()
	rec := eval.Measure("127.0.0.1", port)
	if rec == nil {
		t.Fatal("expected non-nil record")
	}
	if !rec.Reachable {
		t.Errorf("reachable = false, want true")
	}
	if rec.LatencyMs < 0 {
		t.Errorf("latencyMs = %d, want >= 0", rec.LatencyMs)
	}
	if rec.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestMeasureUnreachableHostReturnsRecord(t *testing.T) {
	eval := newEvaluator()
	// Port 1 is typically not listening
	rec := eval.Measure("127.0.0.1", "1")
	if rec == nil {
		t.Fatal("expected non-nil record even for unreachable host")
	}
	if rec.Reachable {
		t.Errorf("reachable = true for port 1, expected false")
	}
}

func TestMgmtQueryReturnsMeasurements(t *testing.T) {
	eval := newEvaluator()
	eval.Measure("host-a", "9999") // will fail but creates record
	eval.Measure("host-b", "9999")

	records := eval.Query(model.QueryRequest{})
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
}

func TestMgmtQueryFilterByHost(t *testing.T) {
	eval := newEvaluator()
	eval.Measure("host-a", "9999")
	eval.Measure("host-b", "9999")

	records := eval.Query(model.QueryRequest{Host: "host-a"})
	if len(records) != 1 {
		t.Errorf("expected 1 record for host-a, got %d", len(records))
	}
	if records[0].Host != "host-a" {
		t.Errorf("host = %q, want host-a", records[0].Host)
	}
}
