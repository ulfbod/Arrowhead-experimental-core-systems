// grpc.go — gRPC CertificateLifecycle server for experiment-13 profile-ca.
//
// Implements the CertificateLifecycle.Subscribe streaming RPC.
// When include_snapshot=true in the request, all current (non-revoked)
// certificates are sent as SNAPSHOT events before live events begin.
package main

import (
	"log"
	"time"

	pb "arrowhead/core-evol/proto/certlifecycle"
)

// certLifecycleServer implements pb.CertificateLifecycleServer.
type certLifecycleServer struct {
	pb.UnimplementedCertificateLifecycleServer
	ca *ProfileCA
}

// Subscribe streams CertEvents to the caller.
//
// If req.IncludeSnapshot is true, the server first sends one SNAPSHOT event
// per currently valid (non-revoked) certificate, then switches to live events.
// The stream stays open until the client cancels (stream.Context().Done()).
func (s *certLifecycleServer) Subscribe(req *pb.SubscribeRequest, stream pb.CertificateLifecycle_SubscribeServer) error {
	// Register for live events before sending snapshot to avoid missing events
	// that arrive between GetAll() and Subscribe().
	ch, unsubscribe := s.ca.Subscribe()
	defer unsubscribe()

	// Send snapshot if requested.
	if req.GetIncludeSnapshot() {
		for _, rec := range s.ca.GetAll() {
			evt := &pb.CertEvent{
				Cn:        rec.CN,
				Ou:        rec.OU,
				Type:      pb.EventType_SNAPSHOT,
				IssuedAt:  rec.IssuedAt.UTC().Format(time.RFC3339),
				ExpiresAt: rec.ExpiresAt.UTC().Format(time.RFC3339),
			}
			if err := stream.Send(evt); err != nil {
				log.Printf("[profile-ca] grpc: send snapshot error: %v", err)
				return err
			}
		}
	}

	// Forward live events until context is cancelled.
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			pbEvt := internalToPB(event)
			if err := stream.Send(pbEvt); err != nil {
				log.Printf("[profile-ca] grpc: send event error: %v", err)
				return err
			}
		}
	}
}

// internalToPB converts a CertEvent (internal Go type) to a proto CertEvent.
func internalToPB(e CertEvent) *pb.CertEvent {
	evt := &pb.CertEvent{
		Cn:        e.CN,
		Ou:        e.OU,
		IssuedAt:  e.IssuedAt,
		ExpiresAt: e.ExpiresAt,
	}
	switch e.Type {
	case "issued":
		evt.Type = pb.EventType_ISSUED
	case "revoked":
		evt.Type = pb.EventType_REVOKED
	case "expired":
		evt.Type = pb.EventType_EXPIRED
	case "snapshot":
		evt.Type = pb.EventType_SNAPSHOT
	default:
		evt.Type = pb.EventType_EVENT_UNSPECIFIED
	}
	return evt
}
