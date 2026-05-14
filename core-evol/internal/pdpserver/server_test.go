package pdpserver

import (
	"context"
	"errors"
	"testing"

	pb "arrowhead/core-evol/proto/authorize"
)

// --- stubs ---

// stubAF is a minimal stand-in for *authzforce.Client.
// pdpserver depends only on DecideWithProvider; we stub that via the interface
// below rather than embedding the real client.
type stubAF struct {
	permit bool
	err    error
	calls  []afCall
}

type afCall struct {
	domainID, subject, service, provider, action string
}

// DecideWithProvider matches the signature used by Server.Decide.
func (s *stubAF) DecideWithProvider(domainID, subject, service, provider, action string) (bool, error) {
	s.calls = append(s.calls, afCall{domainID, subject, service, provider, action})
	return s.permit, s.err
}

// stubServer wraps Server but uses the stubAF interface rather than the real client.
// This avoids coupling tests to the authzforce HTTP implementation.
type stubServer struct {
	pb.UnimplementedAuthorizationPDPServer
	af       interface {
		DecideWithProvider(string, string, string, string, string) (bool, error)
	}
	domainID string
}

func (s *stubServer) Decide(_ context.Context, req *pb.DecisionRequest) (*pb.DecisionResponse, error) {
	if req.Subject == "" || req.Service == "" || req.Action == "" {
		return &pb.DecisionResponse{
			Decision:   pb.Decision_INDETERMINATE,
			StatusCode: "urn:oasis:names:tc:xacml:1.0:status:missing-attribute",
		}, nil
	}
	permitted, err := s.af.DecideWithProvider(s.domainID, req.Subject, req.Service, req.Provider, req.Action)
	if err != nil {
		return &pb.DecisionResponse{
			Decision:   pb.Decision_INDETERMINATE,
			StatusCode: "urn:oasis:names:tc:xacml:1.0:status:processing-error",
		}, nil
	}
	if permitted {
		return &pb.DecisionResponse{Decision: pb.Decision_PERMIT, StatusCode: "urn:oasis:names:tc:xacml:1.0:status:ok"}, nil
	}
	return &pb.DecisionResponse{Decision: pb.Decision_DENY, StatusCode: "urn:oasis:names:tc:xacml:1.0:status:ok"}, nil
}

func newTestServer(af *stubAF) *stubServer {
	return &stubServer{af: af, domainID: "test-domain"}
}

// --- tests ---

func TestDecide_Permit(t *testing.T) {
	af := &stubAF{permit: true}
	srv := newTestServer(af)

	resp, err := srv.Decide(context.Background(), &pb.DecisionRequest{
		Subject:  "portal-cloud-ml",
		Service:  "telemetry",
		Provider: "robot-fleet-site-1",
		Action:   "orchestrate",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Decision != pb.Decision_PERMIT {
		t.Errorf("decision: got %v, want PERMIT", resp.Decision)
	}
	if resp.StatusCode != "urn:oasis:names:tc:xacml:1.0:status:ok" {
		t.Errorf("statusCode: got %q", resp.StatusCode)
	}
}

func TestDecide_Deny(t *testing.T) {
	af := &stubAF{permit: false}
	srv := newTestServer(af)

	resp, err := srv.Decide(context.Background(), &pb.DecisionRequest{
		Subject: "unknown",
		Service: "telemetry",
		Action:  "orchestrate",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Decision != pb.Decision_DENY {
		t.Errorf("decision: got %v, want DENY", resp.Decision)
	}
}

func TestDecide_AFError_ReturnsIndeterminate(t *testing.T) {
	af := &stubAF{err: errors.New("connection refused")}
	srv := newTestServer(af)

	resp, err := srv.Decide(context.Background(), &pb.DecisionRequest{
		Subject: "consumer",
		Service: "telemetry",
		Action:  "orchestrate",
	})
	if err != nil {
		t.Fatalf("unexpected gRPC error: %v", err)
	}
	if resp.Decision != pb.Decision_INDETERMINATE {
		t.Errorf("decision: got %v, want INDETERMINATE on AF error", resp.Decision)
	}
}

func TestDecide_MissingSubject_ReturnsIndeterminate(t *testing.T) {
	af := &stubAF{permit: true}
	srv := newTestServer(af)

	resp, err := srv.Decide(context.Background(), &pb.DecisionRequest{
		Subject: "", // missing
		Service: "telemetry",
		Action:  "orchestrate",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Decision != pb.Decision_INDETERMINATE {
		t.Errorf("decision: got %v, want INDETERMINATE on missing subject", resp.Decision)
	}
	if len(af.calls) != 0 {
		t.Errorf("expected no AF calls on validation failure, got %d", len(af.calls))
	}
}

func TestDecide_SeparateFieldsPassedToAF(t *testing.T) {
	af := &stubAF{permit: true}
	srv := newTestServer(af)

	_, err := srv.Decide(context.Background(), &pb.DecisionRequest{
		Subject:  "portal-cloud-ml",
		Service:  "telemetry",
		Provider: "robot-fleet-site-1",
		Action:   "orchestrate",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(af.calls) != 1 {
		t.Fatalf("expected 1 AF call, got %d", len(af.calls))
	}
	c := af.calls[0]
	// Verify fields passed separately — no "telemetry@robot-fleet-site-1" string
	if c.service != "telemetry" {
		t.Errorf("service: got %q, want %q", c.service, "telemetry")
	}
	if c.provider != "robot-fleet-site-1" {
		t.Errorf("provider: got %q, want %q", c.provider, "robot-fleet-site-1")
	}
	if c.action != "orchestrate" {
		t.Errorf("action: got %q, want %q", c.action, "orchestrate")
	}
}

func TestDecide_EnforcementRequest_EmptyProvider(t *testing.T) {
	af := &stubAF{permit: true}
	srv := newTestServer(af)

	_, err := srv.Decide(context.Background(), &pb.DecisionRequest{
		Subject:  "portal-cloud-ml",
		Service:  "telemetry",
		Provider: "", // enforcement: no provider
		Action:   "consume",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(af.calls) != 1 {
		t.Fatalf("expected 1 AF call, got %d", len(af.calls))
	}
	if af.calls[0].provider != "" {
		t.Errorf("expected empty provider passed to AF for enforcement request, got %q", af.calls[0].provider)
	}
	if af.calls[0].action != "consume" {
		t.Errorf("action: got %q, want consume", af.calls[0].action)
	}
}
