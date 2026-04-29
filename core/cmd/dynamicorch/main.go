// Arrowhead Core – DynamicServiceOrchestration entry point.
// Default port: 8083 (set PORT env var to override).
//
// Configuration:
//   PORT                 — listening port (default: 8083)
//   SERVICE_REGISTRY_URL — ServiceRegistry base URL (default: http://localhost:8080)
//   CONSUMER_AUTH_URL    — ConsumerAuthorization base URL (default: http://localhost:8082)
//   ENABLE_AUTH          — "true" to cross-check ConsumerAuthorization (default: false)
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
	authURL := os.Getenv("CONSUMER_AUTH_URL")
	if authURL == "" {
		authURL = "http://localhost:8082"
	}
	checkAuth := os.Getenv("ENABLE_AUTH") == "true"

	orch := dynsvc.NewDynamicOrchestrator(srURL, authURL, checkAuth)
	handler := dynapi.NewHandler(orch)

	log.Printf("[DynamicOrchestration] Listening on :%s  (sr=%s  auth=%s  checkAuth=%v)", port, srURL, authURL, checkAuth)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
