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
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	blclient "arrowhead/core/internal/blacklist/client"
	"arrowhead/core/internal/generalmgmt"
	dynclient "arrowhead/core/internal/orchestration/dynamic/client"
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

	var bl blclient.BlacklistClient = blclient.NopClient{}
	if blURL := os.Getenv("BLACKLIST_URL"); blURL != "" {
		bl = blclient.NewHTTPClient(blURL, httpClient)
	}

	var idClient dynclient.IdentityClient
	if checkIdentity {
		idClient = dynclient.NewIdentityHTTPClient(authSysURL, httpClient)
	}
	orch := dynsvc.NewDynamicOrchestratorWithClients(
		dynclient.NewSRHTTPClient(srURL, httpClient),
		dynclient.NewCAHTTPClient(caURL, httpClient),
		idClient,
		bl,
		checkAuth, checkIdentity,
	)

	// G54 — Token relay: when RELAY_TOKENS=true, embed ConsumerAuth tokens in each result.
	if os.Getenv("RELAY_TOKENS") == "true" {
		orch.SetRelayTokens(true)
		orch.SetTokenRelayClient(dynclient.NewCATokenRelayHTTPClient(caURL, httpClient))
	}

	pushTimeoutSec := 5
	if v := os.Getenv("PUSH_DELIVERY_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pushTimeoutSec = n
		}
	}
	orch.SetPushClient(&http.Client{Timeout: time.Duration(pushTimeoutSec) * time.Second})

	srPollURL := os.Getenv("SR_POLL_URL")
	pushPollIntervalSec := 30
	if v := os.Getenv("PUSH_POLL_INTERVAL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pushPollIntervalSec = n
		}
	}
	pushPollInterval := time.Duration(pushPollIntervalSec) * time.Second

	sysHandler := dynapi.NewHandlerWithPoller(orch, os.Getenv("MGMT_AUTH_URL"), srPollURL, pushPollInterval)

	mgmtHandler := generalmgmt.NewHandler(buf, "serviceorchestration/orchestration", map[string]string{
		"PORT":                  port,
		"SERVICE_REGISTRY_URL":  srURL,
		"CONSUMER_AUTH_URL":     caURL,
		"AUTH_SYSTEM_URL":       authSysURL,
		"ENABLE_AUTH":                   os.Getenv("ENABLE_AUTH"),
		"ENABLE_IDENTITY_CHECK":         os.Getenv("ENABLE_IDENTITY_CHECK"),
		"TLS_PORT":                      os.Getenv("TLS_PORT"),
		"MGMT_AUTH_URL":                 os.Getenv("MGMT_AUTH_URL"),
		"BLACKLIST_URL":                 os.Getenv("BLACKLIST_URL"),
		"PUSH_DELIVERY_TIMEOUT_SECONDS": os.Getenv("PUSH_DELIVERY_TIMEOUT_SECONDS"),
		"RELAY_TOKENS":                  os.Getenv("RELAY_TOKENS"),
	})

	root := http.NewServeMux()
	root.Handle("/serviceorchestration/orchestration/general/", mgmtHandler)
	root.Handle("/", sysHandler)

	serverTLSCfg, err := tlsutil.LoadServerTLSConfig(certFile, keyFile, caFile)
	if err != nil {
		log.Fatalf("[DynamicOrchestration] server TLS config: %v", err)
	}
	httpsOnly := os.Getenv("HTTPS_ONLY") == "true"
	tlsAddr := ""
	if tlsPort := os.Getenv("TLS_PORT"); tlsPort != "" {
		tlsAddr = ":" + tlsPort
	}

	slog.Info("Listening", "system", "DynamicOrchestration", "port", port,
		"sr", srURL, "ca", caURL, "auth", authSysURL,
		"checkAuth", checkAuth, "checkIdentity", checkIdentity)
	log.Fatal(tlsutil.ServeHTTPS(":"+port, tlsAddr, root, serverTLSCfg, httpsOnly))
}
