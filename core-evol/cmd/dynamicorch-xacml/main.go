// Command dynamicorch-xacml is the AH5-evolved DynamicOrchestration service
// that uses a single XACML decision (AuthzForce) instead of per-provider
// ConsumerAuthorization.verify.
//
// Environment variables:
//
//	SR_URL             ServiceRegistry base URL (default: http://localhost:8080)
//	AUTHZFORCE_URL     AuthzForce base URL      (default: http://localhost:8080/authzforce-ce)
//	AUTHZFORCE_DOMAIN  XACML domain name        (default: arrowhead)
//	ENABLE_AUTH        Enable XACML check       (default: true)
//	PORT               Listening port           (default: 8083)
package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"arrowhead/authzforce"
	"arrowhead/core-evol/internal/orchestration"
)

func main() {
	srURL := envOr("SR_URL", "http://localhost:8080")
	afURL := envOr("AUTHZFORCE_URL", "http://localhost:8080/authzforce-ce")
	afDomain := envOr("AUTHZFORCE_DOMAIN", "arrowhead")
	enableAuth := strings.ToLower(envOr("ENABLE_AUTH", "true")) != "false"
	port := envOr("PORT", "8083")

	afClient := authzforce.New(afURL)

	domainID, err := afClient.EnsureDomain(afDomain)
	if err != nil {
		log.Fatalf("authzforce: cannot ensure domain %q: %v", afDomain, err)
	}
	log.Printf("authzforce domain: %s → %s", afDomain, domainID)

	sr := orchestration.NewSRClient(srURL)
	orch := orchestration.NewXACMLOrchestrator(sr, afClient, domainID, enableAuth)

	mux := http.NewServeMux()
	orchestration.RegisterRoutes(mux, orch, domainID, enableAuth)

	addr := ":" + port
	log.Printf("dynamicorch-xacml listening on %s (xacml=%v domain=%s)", addr, enableAuth, domainID)
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
