// Arrowhead Core – DynamicServiceOrchestration entry point.
// Default port: 8083 (set PORT env var to override).
//
// Configuration:
//
//	PORT                   — listening port (default: 8083)
//	SERVICE_REGISTRY_URL   — ServiceRegistry base URL (default: http://localhost:8080)
//	CONSUMER_AUTH_URL      — ConsumerAuthorization base URL (default: http://localhost:8082)
//	AUTH_SYSTEM_URL        — Authentication system base URL (default: http://localhost:8081)
//	ENABLE_AUTH            — "true" to cross-check ConsumerAuthorization (default: false)
//	ENABLE_IDENTITY_CHECK  — "true" to require and verify a Bearer token via the
//	                         Authentication system (default: false). When enabled,
//	                         the verified systemName from the token replaces the
//	                         self-reported requesterSystem.systemName for all
//	                         ConsumerAuthorization checks.
//
// Optional mutual TLS:
//
//	TLS_PORT      — HTTPS listen port for incoming connections
//	TLS_CERT_FILE — PEM certificate file (server cert, also used for outbound mTLS)
//	TLS_KEY_FILE  — PEM private key file
//	TLS_CA_FILE   — PEM CA file; required for server cert verification on outbound calls
package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"

	dynapi "arrowhead/core/internal/orchestration/dynamic/api"
	dynsvc "arrowhead/core/internal/orchestration/dynamic/service"
	"arrowhead/core/internal/tlsutil"
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
	checkAuth := os.Getenv("ENABLE_AUTH") == "true"
	checkIdentity := os.Getenv("ENABLE_IDENTITY_CHECK") == "true"

	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")
	caFile := os.Getenv("TLS_CA_FILE")

	// Build outbound HTTP client: use mTLS when TLS files are configured.
	clientTLSCfg, err := tlsutil.LoadClientTLSConfig(certFile, keyFile, caFile)
	if err != nil {
		log.Fatalf("[DynamicOrchestration] outbound TLS config: %v", err)
	}
	httpClient := tlsutil.NewHTTPClient(clientTLSCfg)

	orch := dynsvc.NewDynamicOrchestratorWithClient(srURL, caURL, authSysURL, checkAuth, checkIdentity, httpClient)
	handler := dynapi.NewHandler(orch)

	// Optional TLS listener on TLS_PORT for incoming connections.
	if tlsPort := os.Getenv("TLS_PORT"); tlsPort != "" {
		serverTLSCfg, err := tlsutil.LoadServerTLSConfig(certFile, keyFile, caFile)
		if err != nil {
			log.Fatalf("[DynamicOrchestration] server TLS config: %v", err)
		}
		if serverTLSCfg != nil {
			go startTLS(handler, tlsPort, serverTLSCfg, "DynamicOrchestration")
		}
	}

	log.Printf("[DynamicOrchestration] Listening on :%s  (sr=%s  ca=%s  auth=%s  checkAuth=%v  checkIdentity=%v)",
		port, srURL, caURL, authSysURL, checkAuth, checkIdentity)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func startTLS(handler http.Handler, port string, tlsCfg *tls.Config, name string) {
	ln, err := tls.Listen("tcp", ":"+port, tlsCfg)
	if err != nil {
		log.Fatalf("[%s] TLS listen on :%s: %v", name, port, err)
	}
	log.Printf("[%s] Listening on :%s (HTTPS/mTLS)", name, port)
	if err := http.Serve(ln, handler); err != nil {
		log.Fatalf("[%s] TLS serve: %v", name, err)
	}
}
