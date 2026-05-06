package main

import (
	"testing"
)

func TestPercentile_Empty(t *testing.T) {
	if v := Percentile(nil, 50); v != 0 {
		t.Errorf("empty slice: want 0, got %v", v)
	}
}

func TestPercentile_Single(t *testing.T) {
	if v := Percentile([]float64{42}, 50); v != 42 {
		t.Errorf("single element: want 42, got %v", v)
	}
}

func TestPercentile_Known(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	p50 := Percentile(vals, 50)
	if p50 < 5 || p50 > 6 {
		t.Errorf("p50: got %v, want ~5.5", p50)
	}
	if Percentile(vals, 100) != 10 {
		t.Errorf("p100: want 10")
	}
	if Percentile(vals, 0) != 1 {
		t.Errorf("p0: want 1")
	}
}

func TestComputePercentiles_Empty(t *testing.T) {
	mean, p50, p95, p99, max := ComputePercentiles(nil)
	if mean != 0 || p50 != 0 || p95 != 0 || p99 != 0 || max != 0 {
		t.Errorf("empty: all should be 0")
	}
}

func TestComputePercentiles_Known(t *testing.T) {
	vals := make([]float64, 100)
	for i := range vals {
		vals[i] = float64(i + 1)
	}
	mean, p50, p95, p99, max := ComputePercentiles(vals)

	if mean < 50 || mean > 51 {
		t.Errorf("mean: got %v, want ~50.5", mean)
	}
	if p50 < 49 || p50 > 51 {
		t.Errorf("p50: got %v, want ~50", p50)
	}
	if p95 < 93 || p95 > 97 {
		t.Errorf("p95: got %v, want ~95", p95)
	}
	if p99 < 97 || p99 > 100 {
		t.Errorf("p99: got %v, want ~99", p99)
	}
	if max != 100 {
		t.Errorf("max: got %v, want 100", max)
	}
}

func TestRobotCounter_RecordAndSnapshot(t *testing.T) {
	rc := &robotCounter{}
	rc.recordSent(200)
	rc.recordSent(200)
	rc.recordDropped()

	snap := rc.snapshot()
	if snap.MsgSent != 2 {
		t.Errorf("MsgSent: got %d, want 2", snap.MsgSent)
	}
	if snap.MsgDropped != 1 {
		t.Errorf("MsgDropped: got %d, want 1", snap.MsgDropped)
	}
}
