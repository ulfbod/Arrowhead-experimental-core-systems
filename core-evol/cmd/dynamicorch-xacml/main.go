// Command dynamicorch-xacml is the AH5-evolved DynamicOrchestration service.
//
// It supports two authorization backends selected via AUTH_BACKEND:
//
//	grpc         (default) — calls authz-pdp over gRPC (authorize.proto)
//	             Uses AUTHZ_PDP_ADDR to locate the gRPC server.
//	             The authz-pdp server translates to XACML with separate
//	             service and provider attributes (no "service@provider" encoding).
//
//	consumerauth — calls AH5 ConsumerAuthorization over HTTP
//	             Uses CA_URL to locate the ConsumerAuth service.
//	             Provides a direct comparison against the spec-compliant backend.
//
// Environment variables:
//
//	SR_URL          ServiceRegistry base URL     (default: http://localhost:8080)
//	AUTHZ_PDP_ADDR  authz-pdp gRPC address       (default: localhost:9550)
//	CA_URL          ConsumerAuth base URL         (default: http://localhost:8082)
//	AUTH_BACKEND    "grpc" or "consumerauth"      (default: grpc)
//	ENABLE_AUTH     Enable authorization check    (default: true)
//	PORT            HTTP listening port           (default: 8083)
//	DOMAIN_ID       Policy domain identifier      (default: "" — not used by gRPC backend)
package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"arrowhead/core-evol/internal/orchestration"
)

func main() {
	srURL := envOr("SR_URL", "http://localhost:8080")
	authBackend := strings.ToLower(envOr("AUTH_BACKEND", "grpc"))
	authzPDPAddr := envOr("AUTHZ_PDP_ADDR", "localhost:9550")
	caURL := envOr("CA_URL", "http://localhost:8082")
	domainID := envOr("DOMAIN_ID", "")
	enableAuth := strings.ToLower(envOr("ENABLE_AUTH", "true")) != "false"
	port := envOr("PORT", "8083")

	var decider orchestration.AuthDecider

	switch authBackend {
	case "consumerauth":
		decider = orchestration.NewCADecider(caURL)
		log.Printf("dynamicorch-xacml: auth backend=consumerauth url=%s", caURL)

	default: // "grpc"
		grpcDecider, conn, err := orchestration.NewGRPCDecider(authzPDPAddr)
		if err != nil {
			log.Fatalf("authz-pdp: cannot connect to %s: %v", authzPDPAddr, err)
		}
		defer conn.Close()
		decider = grpcDecider
		log.Printf("dynamicorch-xacml: auth backend=grpc addr=%s", authzPDPAddr)
	}

	sr := orchestration.NewSRClient(srURL)
	orch := orchestration.NewXACMLOrchestrator(sr, decider, domainID, enableAuth)

	mux := http.NewServeMux()
	orchestration.RegisterRoutes(mux, orch, domainID, enableAuth)

	addr := ":" + port
	log.Printf("dynamicorch-xacml listening on %s (auth=%v backend=%s)", addr, enableAuth, authBackend)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
