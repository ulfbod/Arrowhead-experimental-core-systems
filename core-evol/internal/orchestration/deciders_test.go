package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	pb "arrowhead/core-evol/proto/authorize"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ─── CADecider tests ─────────────────────────────────────────────────────────

func TestCADecider_Permit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/consumerauth/verify" {
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		var req caVerifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(caVerifyResponse{Authorized: true})
	}))
	defer srv.Close()

	d := NewCADecider(srv.URL)
	ok, err := d.Decide("", "consumer", "telemetry", "provider-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true (authorized)")
	}
}

func TestCADecider_Deny(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(caVerifyResponse{Authorized: false})
	}))
	defer srv.Close()

	d := NewCADecider(srv.URL)
	ok, err := d.Decide("", "consumer", "telemetry", "provider-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false (not authorized)")
	}
}

func TestCADecider_Non200_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	d := NewCADecider(srv.URL)
	_, err := d.Decide("", "consumer", "telemetry", "provider-1", "")
	if err == nil {
		t.Fatal("expected error on non-200 response")
	}
}

func TestCADecider_BadJSON_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{not valid json")
	}))
	defer srv.Close()

	d := NewCADecider(srv.URL)
	_, err := d.Decide("", "consumer", "telemetry", "provider-1", "")
	if err == nil {
		t.Fatal("expected error on bad JSON body")
	}
}

func TestCADecider_NetworkError_ReturnsError(t *testing.T) {
	d := NewCADecider("http://127.0.0.1:1") // nothing listening
	_, err := d.Decide("", "consumer", "telemetry", "provider-1", "")
	if err == nil {
		t.Fatal("expected error on network failure")
	}
}

func TestCADecider_RequestFieldMapping(t *testing.T) {
	var captured caVerifyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&captured)
		json.NewEncoder(w).Encode(caVerifyResponse{Authorized: true})
	}))
	defer srv.Close()

	d := NewCADecider(srv.URL)
	d.Decide("ignored-domain", "my-consumer", "temperature", "my-provider", "ignored-action")

	if captured.ConsumerSystemName != "my-consumer" {
		t.Errorf("consumerSystemName: got %q", captured.ConsumerSystemName)
	}
	if captured.ProviderSystemName != "my-provider" {
		t.Errorf("providerSystemName: got %q", captured.ProviderSystemName)
	}
	if captured.ServiceDefinition != "temperature" {
		t.Errorf("serviceDefinition: got %q", captured.ServiceDefinition)
	}
}

// ─── GRPCDecider tests ───────────────────────────────────────────────────────

// fakePDPServer is an in-process gRPC server for testing GRPCDecider.
type fakePDPServer struct {
	pb.UnimplementedAuthorizationPDPServer
	decision pb.Decision
	err      error
}

func (f *fakePDPServer) Decide(_ context.Context, req *pb.DecisionRequest) (*pb.DecisionResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &pb.DecisionResponse{Decision: f.decision}, nil
}

// startFakePDP starts a real gRPC listener on a random port and returns its
// address plus a shutdown function.
func startFakePDP(t *testing.T, srv *fakePDPServer) (addr string, shutdown func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterAuthorizationPDPServer(s, srv)
	go s.Serve(lis)
	return lis.Addr().String(), s.Stop
}

func TestGRPCDecider_Permit(t *testing.T) {
	fake := &fakePDPServer{decision: pb.Decision_PERMIT}
	addr, stop := startFakePDP(t, fake)
	defer stop()

	d, conn, err := NewGRPCDecider(addr)
	if err != nil {
		t.Fatalf("NewGRPCDecider: %v", err)
	}
	defer conn.Close()

	ok, err := d.Decide("domain", "consumer", "telemetry", "provider", "orchestrate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true (PERMIT)")
	}
}

func TestGRPCDecider_Deny(t *testing.T) {
	fake := &fakePDPServer{decision: pb.Decision_DENY}
	addr, stop := startFakePDP(t, fake)
	defer stop()

	d, conn, err := NewGRPCDecider(addr)
	if err != nil {
		t.Fatalf("NewGRPCDecider: %v", err)
	}
	defer conn.Close()

	ok, err := d.Decide("domain", "consumer", "telemetry", "provider", "orchestrate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false (DENY)")
	}
}

func TestGRPCDecider_ServerError_ReturnsFalseAndError(t *testing.T) {
	fake := &fakePDPServer{err: fmt.Errorf("internal error")}
	addr, stop := startFakePDP(t, fake)
	defer stop()

	d, conn, err := NewGRPCDecider(addr)
	if err != nil {
		t.Fatalf("NewGRPCDecider: %v", err)
	}
	defer conn.Close()

	ok, err := d.Decide("domain", "consumer", "telemetry", "provider", "orchestrate")
	if err == nil {
		t.Fatal("expected error from server failure")
	}
	if ok {
		t.Error("expected false on server error (fail-closed)")
	}
}

func TestGRPCDecider_FieldPassthrough(t *testing.T) {
	var captured *pb.DecisionRequest
	fake := &fakePDPServer{decision: pb.Decision_PERMIT}
	// Override Decide to capture the request.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	captureSrv := &capturingPDPServer{response: pb.Decision_PERMIT, captured: &captured}
	s := grpc.NewServer()
	pb.RegisterAuthorizationPDPServer(s, captureSrv)
	go s.Serve(lis)
	defer s.Stop()
	_ = fake // not used here

	d, conn, err := NewGRPCDecider(lis.Addr().String())
	if err != nil {
		t.Fatalf("NewGRPCDecider: %v", err)
	}
	defer conn.Close()

	d.Decide("my-domain", "my-consumer", "my-service", "my-provider", "orchestrate")

	if captured == nil {
		t.Fatal("no request captured")
	}
	if captured.DomainId != "my-domain" {
		t.Errorf("domainId: got %q", captured.DomainId)
	}
	if captured.Subject != "my-consumer" {
		t.Errorf("subject: got %q", captured.Subject)
	}
	if captured.Service != "my-service" {
		t.Errorf("service: got %q", captured.Service)
	}
	if captured.Provider != "my-provider" {
		t.Errorf("provider: got %q", captured.Provider)
	}
	if captured.Action != "orchestrate" {
		t.Errorf("action: got %q", captured.Action)
	}
}

type capturingPDPServer struct {
	pb.UnimplementedAuthorizationPDPServer
	response pb.Decision
	captured **pb.DecisionRequest
}

func (c *capturingPDPServer) Decide(_ context.Context, req *pb.DecisionRequest) (*pb.DecisionResponse, error) {
	*c.captured = req
	return &pb.DecisionResponse{Decision: c.response}, nil
}

// TestNewGRPCDecider_LazyDial verifies that NewGRPCDecider does not fail even
// when nothing is listening at the address (gRPC uses lazy dialing).
func TestNewGRPCDecider_LazyDial(t *testing.T) {
	d, conn, err := NewGRPCDecider("127.0.0.1:1")
	if err != nil {
		t.Fatalf("NewGRPCDecider should not fail with lazy dialing, got: %v", err)
	}
	if d == nil {
		t.Fatal("expected non-nil GRPCDecider")
	}
	conn.Close()
}

// TestNewGRPCDecider_WithInsecureCredentials checks that a Decide call to an
// unavailable server returns an error (fail-closed) — not a panic.
func TestGRPCDecider_UnavailableServer_ReturnsError(t *testing.T) {
	d, conn, err := NewGRPCDecider("127.0.0.1:1")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	defer conn.Close()

	ok, err := d.Decide("", "", "", "", "")
	if err == nil {
		t.Fatal("expected error when server is unavailable")
	}
	if ok {
		t.Error("expected false on unavailable server (fail-closed)")
	}
}

// TestGRPCDecider_WithDirectClient tests GRPCDecider.Decide by injecting a
// stub client directly, without starting a real gRPC server.
func TestGRPCDecider_WithDirectClient_Permit(t *testing.T) {
	conn, err := grpc.NewClient("passthrough:///localhost:9999",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer conn.Close()

	// Use a fake PDP server on a real port, so we can test the decision mapping.
	fake := &fakePDPServer{decision: pb.Decision_PERMIT}
	addr, stop := startFakePDP(t, fake)
	defer stop()

	d, realConn, err := NewGRPCDecider(addr)
	if err != nil {
		t.Fatalf("NewGRPCDecider: %v", err)
	}
	defer realConn.Close()

	ok, err := d.Decide("", "c", "svc", "p", "orchestrate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected PERMIT → true")
	}
}
