package main

import (
	"context"
	"log"
	"time"

	pb "arrowhead/core-evol/proto/certlifecycle"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// certEventStream is the receiving side of the CertificateLifecycle stream.
// Defined as an interface so tests can inject a mock without network.
type certEventStream interface {
	Recv() (*pb.CertEvent, error)
}

// dialFn dials profile-ca and returns a ready certEventStream.
// The real implementation uses gRPC; tests inject a fake.
type dialFn func(ctx context.Context) (certEventStream, error)

// subscriber connects to profile-ca's CertificateLifecycle gRPC stream and
// keeps the SubjectStore in sync automatically.
//
// Event → SubjectStore mapping:
//   ISSUED, SNAPSHOT → store.Register(cn, ou, valid=true)
//   REVOKED, EXPIRED → store.Register(cn, ou, valid=false)
//
// Reconnect uses exponential backoff (baseBackoff → 2× → ... → 30s max).
// While disconnected the store retains its last known state (not purged).
type subscriber struct {
	store       *SubjectStore
	dial        dialFn
	baseBackoff time.Duration
}

// newSubscriber constructs a subscriber.
// baseBackoff is the starting delay between reconnect attempts; pass 0 to
// disable all sleeping (useful in tests).
func newSubscriber(store *SubjectStore, dial dialFn, baseBackoff time.Duration) *subscriber {
	return &subscriber{store: store, dial: dial, baseBackoff: baseBackoff}
}

// run is the reconnect loop. Call it in a goroutine; it exits when ctx is done.
func (s *subscriber) run(ctx context.Context) {
	backoff := s.baseBackoff
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		stream, err := s.dial(ctx)
		if err != nil {
			log.Printf("PIP subscriber: dial error: %v", err)
			backoff = s.nextBackoff(backoff, maxBackoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}

		// Reset backoff on successful connect.
		backoff = s.baseBackoff

		if err := s.consume(ctx, stream); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("PIP subscriber: stream error: %v — reconnecting", err)
		}

		backoff = s.nextBackoff(backoff, maxBackoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// consume reads events from stream until an error or ctx cancellation.
func (s *subscriber) consume(ctx context.Context, stream certEventStream) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ev, err := stream.Recv()
		if err != nil {
			return err
		}
		s.handleEvent(ev)
	}
}

// handleEvent maps a CertEvent to a SubjectStore operation.
func (s *subscriber) handleEvent(ev *pb.CertEvent) {
	switch ev.Type {
	case pb.EventType_ISSUED, pb.EventType_SNAPSHOT:
		if _, err := s.store.Register(ev.Cn, ev.Ou, true); err != nil {
			log.Printf("PIP subscriber: register %q failed: %v", ev.Cn, err)
		}
	case pb.EventType_REVOKED, pb.EventType_EXPIRED:
		if _, err := s.store.Register(ev.Cn, ev.Ou, false); err != nil {
			log.Printf("PIP subscriber: register %q failed: %v", ev.Cn, err)
		}
	default:
		// EVENT_UNSPECIFIED and unknown values are ignored per proto contract.
	}
}

// nextBackoff doubles the current backoff up to max. If current is 0, returns 0.
func (s *subscriber) nextBackoff(current, max time.Duration) time.Duration {
	if current == 0 {
		return 0
	}
	next := current * 2
	if next > max {
		return max
	}
	return next
}

// newGRPCDialFn returns a real dialFn that connects to addr using insecure
// credentials and subscribes with include_snapshot=true.
func newGRPCDialFn(addr string) dialFn {
	return func(ctx context.Context) (certEventStream, error) {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		client := pb.NewCertificateLifecycleClient(conn)
		stream, err := client.Subscribe(ctx, &pb.SubscribeRequest{IncludeSnapshot: true})
		if err != nil {
			conn.Close()
			return nil, err
		}
		return stream, nil
	}
}
