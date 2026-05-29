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
	"log/slog"
	"net/http"
	"os"

	"arrowhead/core/internal/generalmgmt"
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

	buf := generalmgmt.NewLogBuffer(1000)
	slog.SetDefault(slog.New(generalmgmt.NewSlogHandler(buf)))

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
	sysHandler := dynapi.NewHandler(orch)

	mgmtHandler := generalmgmt.NewHandler(buf, "serviceorchestration/orchestration", map[string]string{
		"PORT":                 port,
		"SERVICE_REGISTRY_URL": srURL,
		"CONSUMER_AUTH_URL":    caURL,
		"AUTH_SYSTEM_URL":      authSysURL,
		"ENABLE_AUTH":          os.Getenv("ENABLE_AUTH"),
		"ENABLE_IDENTITY_CHECK": os.Getenv("ENABLE_IDENTITY_CHECK"),
		"TLS_PORT":             os.Getenv("TLS_PORT"),
	})

	root := http.NewServeMux()
	root.Handle("/serviceorchestration/orchestration/general/", mgmtHandler)
	root.Handle("/", sysHandler)

	// Optional TLS listener on TLS_PORT for incoming connections.
	if tlsPort := os.Getenv("TLS_PORT"); tlsPort != "" {
		serverTLSCfg, err := tlsutil.LoadServerTLSConfig(certFile, keyFile, caFile)
		if err != nil {
			log.Fatalf("[DynamicOrchestration] server TLS config: %v", err)
		}
		if serverTLSCfg != nil {
			go startTLS(root, tlsPort, serverTLSCfg, "DynamicOrchestration")
		}
	}

	slog.Info("Listening", "system", "DynamicOrchestration", "port", port,
		"sr", srURL, "ca", caURL, "auth", authSysURL,
		"checkAuth", checkAuth, "checkIdentity", checkIdentity)
	log.Fatal(http.ListenAndServe(":"+port, root))
}

func startTLS(handler http.Handler, port string, tlsCfg *tls.Config, name string) {
	ln, err := tls.Listen("tcp", ":"+port, tlsCfg)
	if err != nil {
		log.Fatalf("[%s] TLS listen on :%s: %v", name, port, err)
	}
	slog.Info("Listening (HTTPS/mTLS)", "system", name, "port", port)
	if err := http.Serve(ln, handler); err != nil {
		log.Fatalf("[%s] TLS serve: %v", name, err)
	}
}
