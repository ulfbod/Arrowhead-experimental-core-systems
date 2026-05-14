// Package pdpserver implements the AuthorizationPDP gRPC service defined in
// proto/authorize/authorize.proto.
//
// The server translates each DecisionRequest into a XACML 3.0 request with
// separate resource attributes (resource-id for service, provider-id for
// provider) and evaluates it against AuthzForce CE.
package pdpserver

import (
	"context"
	"fmt"
	"log"

	"arrowhead/authzforce"
	pb "arrowhead/core-evol/proto/authorize"
)

// Server implements pb.AuthorizationPDPServer.
// It wraps an AuthzForce client and resolves its configured domain at startup.
type Server struct {
	pb.UnimplementedAuthorizationPDPServer
	af       *authzforce.Client
	domainID string // resolved AuthzForce internal domain UUID
}

// New returns a Server using the given AuthzForce client and domain UUID.
// Call authzforce.Client.EnsureDomain before constructing to obtain domainID.
func New(af *authzforce.Client, domainID string) *Server {
	return &Server{af: af, domainID: domainID}
}

// Decide evaluates a single access-control request.
//
// The request fields map to XACML attributes:
//   - Subject   → subject-id
//   - Service   → resource-id
//   - Provider  → urn:arrowhead:attribute:provider-id (when non-empty)
//   - Action    → action-id
//
// domain_id in the request is accepted but ignored — this server is
// pre-configured with a single domain.
func (s *Server) Decide(_ context.Context, req *pb.DecisionRequest) (*pb.DecisionResponse, error) {
	if req.Subject == "" || req.Service == "" || req.Action == "" {
		return &pb.DecisionResponse{
			Decision:   pb.Decision_INDETERMINATE,
			StatusCode: "urn:oasis:names:tc:xacml:1.0:status:missing-attribute",
		}, nil
	}

	permitted, err := s.af.DecideWithProvider(s.domainID, req.Subject, req.Service, req.Provider, req.Action)
	if err != nil {
		log.Printf("authz-pdp: DecideWithProvider error: %v", err)
		return &pb.DecisionResponse{
			Decision:   pb.Decision_INDETERMINATE,
			StatusCode: fmt.Sprintf("urn:oasis:names:tc:xacml:1.0:status:processing-error: %v", err),
		}, nil
	}

	decision := pb.Decision_NOT_APPLICABLE
	if permitted {
		decision = pb.Decision_PERMIT
	} else {
		decision = pb.Decision_DENY
	}

	return &pb.DecisionResponse{
		Decision:   decision,
		StatusCode: "urn:oasis:names:tc:xacml:1.0:status:ok",
	}, nil
}
