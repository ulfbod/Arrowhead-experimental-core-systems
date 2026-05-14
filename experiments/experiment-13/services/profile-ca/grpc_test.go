package main

import (
	"context"
	"testing"
	"time"

	pb "arrowhead/core-evol/proto/certlifecycle"
	"google.golang.org/grpc/metadata"
)

// fakeSubscribeStream is a fake implementation of
// pb.CertificateLifecycle_SubscribeServer for unit testing.
// It captures sent events and uses a context for cancellation.
type fakeSubscribeStream struct {
	ctx    context.Context
	cancel context.CancelFunc
	events []*pb.CertEvent
	sendFn func(*pb.CertEvent) error // optional hook for send errors
}

func newFakeStream() *fakeSubscribeStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &fakeSubscribeStream{ctx: ctx, cancel: cancel}
}

func (f *fakeSubscribeStream) Send(e *pb.CertEvent) error {
	if f.sendFn != nil {
		return f.sendFn(e)
	}
	f.events = append(f.events, e)
	return nil
}

func (f *fakeSubscribeStream) Context() context.Context        { return f.ctx }
func (f *fakeSubscribeStream) SetHeader(metadata.MD) error     { return nil }
func (f *fakeSubscribeStream) SendHeader(metadata.MD) error    { return nil }
func (f *fakeSubscribeStream) SetTrailer(metadata.MD)          {}
func (f *fakeSubscribeStream) SendMsg(m any) error             { return nil }
func (f *fakeSubscribeStream) RecvMsg(m any) error             { return nil }

// TestSubscribe_SnapshotDeliveredFirst verifies that when include_snapshot=true
// and there are pre-existing certs, SNAPSHOT events arrive before ISSUED events.
func TestSubscribe_SnapshotDeliveredFirst(t *testing.T) {
	ca := newTestCA(t)
	// Issue two certs before subscribing.
	ca.IssueOnboardingCert("snap-1") //nolint:errcheck
	ca.IssueInfraCert("snap-2")      //nolint:errcheck

	srv := &certLifecycleServer{ca: ca}
	stream := newFakeStream()

	// Cancel the stream after a short delay so Subscribe returns.
	go func() {
		time.Sleep(50 * time.Millisecond)
		stream.cancel()
	}()

	req := &pb.SubscribeRequest{IncludeSnapshot: true}
	srv.Subscribe(req, stream) //nolint:errcheck

	// Find snapshot events.
	var snapshots []*pb.CertEvent
	for _, e := range stream.events {
		if e.Type == pb.EventType_SNAPSHOT {
			snapshots = append(snapshots, e)
		}
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 SNAPSHOT events, got %d (all events: %v)", len(snapshots), stream.events)
	}

	// Snapshot events must appear before any ISSUED events.
	for i, e := range stream.events {
		if e.Type == pb.EventType_ISSUED {
			// Find the last snapshot index.
			for j := i; j < len(stream.events); j++ {
				if stream.events[j].Type == pb.EventType_SNAPSHOT {
					t.Error("SNAPSHOT event found after ISSUED event")
				}
			}
		}
	}
}

// TestSubscribe_LiveOnlyWhenSnapshotFalse verifies that with include_snapshot=false,
// no SNAPSHOT events are sent even if certs exist.
func TestSubscribe_LiveOnlyWhenSnapshotFalse(t *testing.T) {
	ca := newTestCA(t)
	ca.IssueOnboardingCert("existing-1") //nolint:errcheck
	ca.IssueInfraCert("existing-2")      //nolint:errcheck

	srv := &certLifecycleServer{ca: ca}
	stream := newFakeStream()

	// Cancel immediately — no live events will arrive.
	stream.cancel()

	req := &pb.SubscribeRequest{IncludeSnapshot: false}
	srv.Subscribe(req, stream) //nolint:errcheck

	for _, e := range stream.events {
		if e.Type == pb.EventType_SNAPSHOT {
			t.Errorf("unexpected SNAPSHOT event when include_snapshot=false: %+v", e)
		}
	}
}

// TestSubscribe_IssuedEventDelivered verifies that after calling Subscribe,
// a cert issued while the stream is open produces an ISSUED gRPC event.
func TestGRPCSubscribe_IssuedEventDelivered(t *testing.T) {
	ca := newTestCA(t)
	srv := &certLifecycleServer{ca: ca}

	collected := make(chan *pb.CertEvent, 10)
	stream := newFakeStream()
	stream.sendFn = func(e *pb.CertEvent) error {
		collected <- e
		return nil
	}

	// Run Subscribe in a goroutine; cancel it after issuing.
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := &pb.SubscribeRequest{IncludeSnapshot: false}
		srv.Subscribe(req, stream) //nolint:errcheck
	}()

	// Small delay to let Subscribe register its channel subscription.
	time.Sleep(20 * time.Millisecond)

	ca.IssueInfraCert("live-issued") //nolint:errcheck

	// Wait for the event.
	select {
	case event := <-collected:
		if event.Type != pb.EventType_ISSUED {
			t.Errorf("expected ISSUED, got %v", event.Type)
		}
		if event.Cn != "live-issued" {
			t.Errorf("expected CN=live-issued, got %s", event.Cn)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for gRPC ISSUED event")
	}

	stream.cancel()
	<-done
}

// TestGRPCSubscribe_RevokedEventDelivered verifies that revoking a cert
// produces a REVOKED gRPC event on an active stream.
func TestGRPCSubscribe_RevokedEventDelivered(t *testing.T) {
	ca := newTestCA(t)
	ca.IssueInfraCert("grpc-revoke-me") //nolint:errcheck

	srv := &certLifecycleServer{ca: ca}

	collected := make(chan *pb.CertEvent, 10)
	stream := newFakeStream()
	stream.sendFn = func(e *pb.CertEvent) error {
		collected <- e
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		req := &pb.SubscribeRequest{IncludeSnapshot: false}
		srv.Subscribe(req, stream) //nolint:errcheck
	}()

	time.Sleep(20 * time.Millisecond)

	if err := ca.Revoke("grpc-revoke-me"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	timeout := time.After(time.Second)
	for {
		select {
		case event := <-collected:
			if event.Type == pb.EventType_REVOKED {
				if event.Cn != "grpc-revoke-me" {
					t.Errorf("expected CN=grpc-revoke-me, got %s", event.Cn)
				}
				stream.cancel()
				<-done
				return
			}
		case <-timeout:
			stream.cancel()
			<-done
			t.Fatal("timeout waiting for gRPC REVOKED event")
		}
	}
}

// TestGRPCSubscribe_SnapshotFieldsPopulated verifies that SNAPSHOT events
// include CN, OU, IssuedAt, and ExpiresAt.
func TestGRPCSubscribe_SnapshotFieldsPopulated(t *testing.T) {
	ca := newTestCA(t)
	ca.IssueInfraCert("fields-check") //nolint:errcheck

	srv := &certLifecycleServer{ca: ca}
	stream := newFakeStream()

	go func() {
		time.Sleep(30 * time.Millisecond)
		stream.cancel()
	}()

	req := &pb.SubscribeRequest{IncludeSnapshot: true}
	srv.Subscribe(req, stream) //nolint:errcheck

	var snap *pb.CertEvent
	for _, e := range stream.events {
		if e.Type == pb.EventType_SNAPSHOT {
			snap = e
			break
		}
	}
	if snap == nil {
		t.Fatal("no SNAPSHOT event found")
	}
	if snap.Cn == "" {
		t.Error("SNAPSHOT event missing CN")
	}
	if snap.Ou == "" {
		t.Error("SNAPSHOT event missing OU")
	}
	if snap.IssuedAt == "" {
		t.Error("SNAPSHOT event missing IssuedAt")
	}
	if snap.ExpiresAt == "" {
		t.Error("SNAPSHOT event missing ExpiresAt")
	}
}
