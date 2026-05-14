// Command authz-pdp is the AuthorizationPDP gRPC server for Arrowhead experiment-12.
//
// It implements the interface defined in proto/authorize/authorize.proto and
// translates each DecisionRequest to a XACML 3.0 request with separate
// resource attributes (resource-id for service, provider-id for provider)
// before evaluating against AuthzForce CE.
//
// gRPC reflection is enabled: use grpcurl to inspect the service.
//
// Environment variables:
//
//	AUTHZFORCE_URL     AuthzForce base URL    (default: http://authzforce:8080/authzforce-ce)
//	AUTHZFORCE_DOMAIN  XACML domain name      (default: arrowhead-exp12)
//	PORT               gRPC listening port    (default: 9550)
package main

import (
	"log"
	"net"
	"os"

	"arrowhead/authzforce"
	"arrowhead/core-evol/internal/pdpserver"
	pb "arrowhead/core-evol/proto/authorize"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	afURL := envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce")
	afDomain := envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp12")
	port := envOr("PORT", "9550")

	afClient := authzforce.New(afURL)

	domainID, err := afClient.EnsureDomain(afDomain)
	if err != nil {
		log.Fatalf("authz-pdp: EnsureDomain %q: %v", afDomain, err)
	}
	log.Printf("authz-pdp: AuthzForce domain %q → %s", afDomain, domainID)

	srv := pdpserver.New(afClient, domainID)

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("authz-pdp: listen :%s: %v", port, err)
	}

	grpcSrv := grpc.NewServer()
	pb.RegisterAuthorizationPDPServer(grpcSrv, srv)

	// Enable gRPC server reflection for grpcurl introspection.
	// Usage: grpcurl -plaintext localhost:9550 list
	reflection.Register(grpcSrv)

	log.Printf("authz-pdp: gRPC server listening on :%s (reflection enabled)", port)
	if err := grpcSrv.Serve(lis); err != nil {
		log.Fatalf("authz-pdp: serve: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
