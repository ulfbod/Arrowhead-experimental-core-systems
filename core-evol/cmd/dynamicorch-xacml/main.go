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
	"log/slog"
	"net/http"
	"os"
	"strings"

	"arrowhead/core-evol/internal/generalmgmt"
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

	buf := generalmgmt.NewLogBuffer(1000)
	slog.SetDefault(slog.New(generalmgmt.NewSlogHandler(buf)))

	var decider orchestration.AuthDecider

	switch authBackend {
	case "consumerauth":
		decider = orchestration.NewCADecider(caURL)
		slog.Info("auth backend", "backend", "consumerauth", "url", caURL)

	default: // "grpc"
		grpcDecider, conn, err := orchestration.NewGRPCDecider(authzPDPAddr)
		if err != nil {
			log.Fatalf("authz-pdp: cannot connect to %s: %v", authzPDPAddr, err)
		}
		defer conn.Close()
		decider = grpcDecider
		slog.Info("auth backend", "backend", "grpc", "addr", authzPDPAddr)
	}

	sr := orchestration.NewSRClient(srURL)
	orch := orchestration.NewXACMLOrchestrator(sr, decider, domainID, enableAuth)

	mgmtHandler := generalmgmt.NewHandler(buf, "serviceorchestration/orchestration", map[string]string{
		"srUrl":       srURL,
		"authBackend": authBackend,
		"domainId":    domainID,
		"enableAuth":  envOr("ENABLE_AUTH", "true"),
		"port":        port,
	})

	sysHandler := http.NewServeMux()
	orchestration.RegisterRoutes(sysHandler, orch, domainID, enableAuth)

	root := http.NewServeMux()
	root.Handle("/serviceorchestration/orchestration/general/", mgmtHandler)
	root.Handle("/", sysHandler)

	addr := ":" + port
	slog.Info("dynamicorch-xacml listening", "addr", addr, "auth", enableAuth, "backend", authBackend)
	if err := http.ListenAndServe(addr, root); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
