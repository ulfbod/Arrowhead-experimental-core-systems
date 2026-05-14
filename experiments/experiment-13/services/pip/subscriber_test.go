package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	pb "arrowhead/core-evol/proto/certlifecycle"
)

// mockStream is a certEventStream that returns a pre-configured sequence of
// events followed by an error (io.EOF or a test error).
type mockStream struct {
	events []*pb.CertEvent
	idx    int
	err    error // returned after all events are exhausted
}

func (m *mockStream) Recv() (*pb.CertEvent, error) {
	if m.idx < len(m.events) {
		ev := m.events[m.idx]
		m.idx++
		return ev, nil
	}
	return nil, m.err
}

// newMockDialFn returns a dialFn that always succeeds and returns the given stream.
func newMockDialFn(stream certEventStream) dialFn {
	return func(ctx context.Context) (certEventStream, error) {
		return stream, nil
	}
}

// newErrorDialFn returns a dialFn that always returns an error (simulates
// unreachable gRPC server).
func newErrorDialFn(err error) dialFn {
	return func(ctx context.Context) (certEventStream, error) {
		return nil, err
	}
}

// waitForCondition polls fn every 1ms for up to timeout, returning true if
// fn ever returns true.
func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

// ── TestSubscriber_IssuedRegistersValid ──────────────────────────────────────
// An ISSUED event must call store.Register(cn, ou, valid=true).

func TestSubscriber_IssuedRegistersValid(t *testing.T) {
	store := NewSubjectStore()

	stream := &mockStream{
		events: []*pb.CertEvent{
			{Cn: "sys-a", Ou: "sy", Type: pb.EventType_ISSUED},
		},
		err: errors.New("stream ended"),
	}

	sub := newSubscriber(store, newMockDialFn(stream), 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sub.run(ctx)

	ok := waitForCondition(t, 2*time.Second, func() bool {
		s, found := store.Get("sys-a")
		return found && s.Valid
	})
	if !ok {
		t.Fatal("ISSUED event: subject not registered as valid within timeout")
	}

	s, _ := store.Get("sys-a")
	if s.CertLevel != "sy" {
		t.Errorf("ISSUED: certLevel = %q, want sy", s.CertLevel)
	}
}

// ── TestSubscriber_RevokedRegistersInvalid ───────────────────────────────────
// A REVOKED event must call store.Register(cn, ou, valid=false).

func TestSubscriber_RevokedRegistersInvalid(t *testing.T) {
	store := NewSubjectStore()
	// Pre-register as valid so we can observe the transition.
	store.Register("sys-b", "on", true)

	stream := &mockStream{
		events: []*pb.CertEvent{
			{Cn: "sys-b", Ou: "on", Type: pb.EventType_REVOKED},
		},
		err: errors.New("stream ended"),
	}

	sub := newSubscriber(store, newMockDialFn(stream), 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sub.run(ctx)

	ok := waitForCondition(t, 2*time.Second, func() bool {
		s, found := store.Get("sys-b")
		return found && !s.Valid
	})
	if !ok {
		t.Fatal("REVOKED event: subject not marked invalid within timeout")
	}
}

// ── TestSubscriber_SnapshotRegistersValid ────────────────────────────────────
// A SNAPSHOT event must call store.Register(cn, ou, valid=true).

func TestSubscriber_SnapshotRegistersValid(t *testing.T) {
	store := NewSubjectStore()

	stream := &mockStream{
		events: []*pb.CertEvent{
			{Cn: "sys-c", Ou: "de", Type: pb.EventType_SNAPSHOT},
		},
		err: errors.New("stream ended"),
	}

	sub := newSubscriber(store, newMockDialFn(stream), 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sub.run(ctx)

	ok := waitForCondition(t, 2*time.Second, func() bool {
		s, found := store.Get("sys-c")
		return found && s.Valid
	})
	if !ok {
		t.Fatal("SNAPSHOT event: subject not registered as valid within timeout")
	}

	s, _ := store.Get("sys-c")
	if s.CertLevel != "de" {
		t.Errorf("SNAPSHOT: certLevel = %q, want de", s.CertLevel)
	}
}

// ── TestSubscriber_ExpiredRegistersInvalid ───────────────────────────────────
// An EXPIRED event must call store.Register(cn, ou, valid=false).

func TestSubscriber_ExpiredRegistersInvalid(t *testing.T) {
	store := NewSubjectStore()
	store.Register("sys-d", "lo", true)

	stream := &mockStream{
		events: []*pb.CertEvent{
			{Cn: "sys-d", Ou: "lo", Type: pb.EventType_EXPIRED},
		},
		err: errors.New("stream ended"),
	}

	sub := newSubscriber(store, newMockDialFn(stream), 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sub.run(ctx)

	ok := waitForCondition(t, 2*time.Second, func() bool {
		s, found := store.Get("sys-d")
		return found && !s.Valid
	})
	if !ok {
		t.Fatal("EXPIRED event: subject not marked invalid within timeout")
	}
}

// ── TestSubscriber_ReconnectsAfterError ──────────────────────────────────────
// When the stream errors, the subscriber must reconnect (dial a second time).

func TestSubscriber_ReconnectsAfterError(t *testing.T) {
	store := NewSubjectStore()

	var dialCount int64

	// First dial returns a stream that immediately errors.
	// Second dial also returns an immediately-erroring stream.
	// We just need to confirm that dial is called at least twice.
	dial := func(ctx context.Context) (certEventStream, error) {
		atomic.AddInt64(&dialCount, 1)
		return &mockStream{err: errors.New("instant stream error")}, nil
	}

	// Use zero backoff so the test doesn't wait.
	sub := newSubscriber(store, dial, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sub.run(ctx)

	ok := waitForCondition(t, 2*time.Second, func() bool {
		return atomic.LoadInt64(&dialCount) >= 2
	})
	if !ok {
		t.Fatalf("reconnect: dial called %d times, want >= 2", atomic.LoadInt64(&dialCount))
	}
}
