// Arrowhead Core – DynamicServiceOrchestration entry point.
// Default port: 8083 (set PORT env var to override).
//
// Configuration:
//   PORT                   — listening port (default: 8083)
//   SERVICE_REGISTRY_URL   — ServiceRegistry base URL (default: http://localhost:8080)
//   CONSUMER_AUTH_URL      — ConsumerAuthorization base URL (default: http://localhost:8082)
//   AUTH_SYSTEM_URL        — Authentication system base URL (default: http://localhost:8081)
//   ENABLE_AUTH            — "true" to cross-check ConsumerAuthorization (default: false)
//   ENABLE_IDENTITY_CHECK  — "true" to require and verify a Bearer token via the
//                            Authentication system (default: false). When enabled,
//                            the verified systemName from the token replaces the
//                            self-reported requesterSystem.systemName for all
//                            ConsumerAuthorization checks.
package main

import (
	"log"
	"net/http"
	"os"

	dynapi "arrowhead/core/internal/orchestration/dynamic/api"
	dynsvc "arrowhead/core/internal/orchestration/dynamic/service"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}
	srURL := os.Getenv("SERVICE_REGISTRY_URL")
	if srURL == "" {
		srURL = "http://localhost:8080"
	}
	caURL := os.Getenv("CONSUMER_AUTH_URL")
	if caURL == "" {
		caURL = "http://localhost:8082"
	}
	authSysURL := os.Getenv("AUTH_SYSTEM_URL")
	if authSysURL == "" {
		authSysURL = "http://localhost:8081"
	}
	checkAuth     := os.Getenv("ENABLE_AUTH") == "true"
	checkIdentity := os.Getenv("ENABLE_IDENTITY_CHECK") == "true"

	orch := dynsvc.NewDynamicOrchestrator(srURL, caURL, authSysURL, checkAuth, checkIdentity)
	handler := dynapi.NewHandler(orch)

	log.Printf("[DynamicOrchestration] Listening on :%s  (sr=%s  ca=%s  auth=%s  checkAuth=%v  checkIdentity=%v)",
		port, srURL, caURL, authSysURL, checkAuth, checkIdentity)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
