// pki-rest-authz — mTLS reverse-proxy PEP backed by AuthzForce XACML.
//
// Extends cert-rest-authz (experiment-7) to obtain its identity through the
// Arrowhead 5.2 onboarding lifecycle (on → de → sy) instead of calling
// POST /ca/certificate/issue directly. The rest of the authorization logic
// (XACML, mTLS proxy) is identical to experiment-7.
//
// Environment variables:
//
//	CA_URL             profile-ca plain HTTP URL (default: http://profile-ca:8087)
//	CA_TLS_URL         profile-ca mTLS URL (default: https://profile-ca:8088)
//	AUTHZFORCE_URL     AuthzForce base URL (default: http://authzforce:8080/authzforce-ce)
//	AUTHZFORCE_DOMAIN  AuthzForce domain externalId (default: arrowhead-exp8)
//	UPSTREAM_URL       Upstream service base URL, no trailing slash (required)
//	DEFAULT_SERVICE    Service name when X-Service-Name is absent (default: telemetry-rest)
//	PORT               Plain HTTP port (default: 9109)
//	TLS_PORT           mTLS HTTPS port (default: 9108)
//	CACHE_TTL          Decision cache TTL; 0s = no caching (default: 0s)
package main

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	az "arrowhead/authzforce"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	caURL       := envOr("CA_URL", "http://profile-ca:8087")
	caTLSURL    := envOr("CA_TLS_URL", "https://profile-ca:8088")
	azURL       := envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce")
	azDomainExt := envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp8")
	upstreamURL := os.Getenv("UPSTREAM_URL")
	port        := envOr("PORT", "9109")
	tlsPort     := envOr("TLS_PORT", "9108")

	if upstreamURL == "" {
		log.Fatal("[pki-rest-authz] UPSTREAM_URL is required")
	}

	cacheTTL, err := time.ParseDuration(envOr("CACHE_TTL", "0s"))
	if err != nil {
		log.Fatalf("[pki-rest-authz] invalid CACHE_TTL: %v", err)
	}

	// Perform the full Arrowhead 5.2 onboarding lifecycle to acquire a System cert.
	log.Printf("[pki-rest-authz] starting PKI lifecycle: on → de → sy for CN=pki-rest-authz")
	var ownCert tls.Certificate
	var caPool *x509.CertPool
	for attempt := 1; attempt <= 10; attempt++ {
		cert, pool, lcErr := AcquireSystemCert(caURL, caTLSURL, "pki-rest-authz")
		if lcErr != nil {
			if attempt < 10 {
				log.Printf("[pki-rest-authz] lifecycle attempt %d/10: %v — retrying in 3s", attempt, lcErr)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[pki-rest-authz] lifecycle failed after 10 attempts: %v", lcErr)
		}
		ownCert = cert
		caPool = pool
		break
	}
	log.Printf("[pki-rest-authz] system cert acquired (OU=sy, CN=pki-rest-authz)")

	// Build mTLS server TLS config.
	serverTLSCfg := buildServerTLSConfig(ownCert, caPool)

	// Build upstream HTTP client with CA pool for outbound HTTPS.
	upstreamTLSCfg := buildClientTLSConfig(ownCert, caPool)
	upstreamClient := buildMTLSUpstreamClient(upstreamTLSCfg)

	// Resolve AuthzForce domain.
	azClient := az.New(azURL)
	log.Printf("[pki-rest-authz] resolving AuthzForce domain %q at %s", azDomainExt, azURL)
	var domainID string
	for attempt := 1; ; attempt++ {
		id, resolveErr := azClient.EnsureDomain(azDomainExt)
		if resolveErr != nil || id == "" {
			if attempt == 1 || attempt%5 == 0 {
				log.Printf("[pki-rest-authz] domain lookup attempt %d: %v — retrying", attempt, resolveErr)
			}
			time.Sleep(3 * time.Second)
			continue
		}
		domainID = id
		break
	}

	cfg := serverConfig{
		azDomainID:     domainID,
		upstreamURL:    upstreamURL,
		defaultService: envOr("DEFAULT_SERVICE", "telemetry-rest"),
		port:           port,
		tlsPort:        tlsPort,
	}

	cache := newDecisionCache(cacheTTL)
	srv := newCertAuthzServer(cfg, azClient, cache, upstreamClient)

	// Start plain HTTP server.
	plainMux := http.NewServeMux()
	srv.registerPlain(plainMux)
	go func() {
		ln, listenErr := net.Listen("tcp", ":"+port)
		if listenErr != nil {
			log.Fatalf("[pki-rest-authz] listen plain HTTP :%s: %v", port, listenErr)
		}
		log.Printf("[pki-rest-authz] plain HTTP listening on :%s", port)
		if serveErr := http.Serve(ln, plainMux); serveErr != nil {
			log.Fatal(serveErr)
		}
	}()

	// Start mTLS HTTPS server.
	mtlsMux := http.NewServeMux()
	srv.registerMTLS(mtlsMux)

	tlsLn, err := tls.Listen("tcp", ":"+tlsPort, serverTLSCfg)
	if err != nil {
		log.Fatalf("[pki-rest-authz] listen mTLS HTTPS :%s: %v", tlsPort, err)
	}
	log.Printf("[pki-rest-authz] mTLS HTTPS listening on :%s", tlsPort)
	if err := http.Serve(tlsLn, mtlsMux); err != nil {
		log.Fatal(err)
	}
}
