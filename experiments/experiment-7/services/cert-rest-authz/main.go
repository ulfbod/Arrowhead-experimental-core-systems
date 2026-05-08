// cert-rest-authz — mTLS reverse-proxy PEP backed by AuthzForce XACML.
//
// Acts as a TLS-terminating reverse proxy. Every mTLS request must present a
// client certificate signed by the Arrowhead CA.  cert-rest-authz reads the
// consumer identity from the client certificate CN (Common Name) and queries
// AuthzForce for the triple (consumer, service, "invoke"); if Permit it
// forwards the request to UPSTREAM_URL, otherwise it returns 403 Forbidden.
//
// Two listeners are started:
//   - Plain HTTP on PORT: /health, /status, /auth/check (no TLS, for internal use)
//   - HTTPS with mTLS on TLS_PORT: all other paths (PEP proxy)
//
// Environment variables:
//
//	CA_URL             Arrowhead CA base URL (default: http://ca:8086)
//	AUTHZFORCE_URL     AuthzForce base URL (default: http://authzforce:8080/authzforce-ce)
//	AUTHZFORCE_DOMAIN  AuthzForce domain externalId (default: arrowhead-exp7)
//	UPSTREAM_URL       Upstream service base URL, no trailing slash (required)
//	DEFAULT_SERVICE    Service name when X-Service-Name is absent (default: telemetry-rest)
//	PORT               Plain HTTP port (default: 9099)
//	TLS_PORT           mTLS HTTPS port (default: 9098)
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

// fetchCACertWithRetry wraps fetchCACert with a retry loop.
func fetchCACertWithRetry(caURL string, maxAttempts int) (*x509.CertPool, []byte) {
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pool, pem, err := fetchCACert(caURL)
		if err != nil {
			if attempt < maxAttempts {
				log.Printf("[cert-rest-authz] CA fetch attempt %d/%d: %v — retrying in 3s", attempt, maxAttempts, err)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[cert-rest-authz] CA fetch failed after %d attempts: %v", maxAttempts, err)
		}
		return pool, pem
	}
	// unreachable
	return nil, nil
}

// issueCertWithRetry wraps issueCert with a retry loop.
func issueCertWithRetry(caURL, name string, maxAttempts int) tls.Certificate {
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		cert, err := issueCert(caURL, name)
		if err != nil {
			if attempt < maxAttempts {
				log.Printf("[cert-rest-authz] cert issue attempt %d/%d: %v — retrying in 3s", attempt, maxAttempts, err)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("[cert-rest-authz] cert issue failed after %d attempts: %v", maxAttempts, err)
		}
		return cert
	}
	// unreachable
	return tls.Certificate{}
}

func main() {
	caURL       := envOr("CA_URL", "http://ca:8086")
	azURL       := envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce")
	azDomainExt := envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp7")
	upstreamURL := os.Getenv("UPSTREAM_URL")
	port        := envOr("PORT", "9099")
	tlsPort     := envOr("TLS_PORT", "9098")

	if upstreamURL == "" {
		log.Fatal("[cert-rest-authz] UPSTREAM_URL is required")
	}

	cacheTTL, err := time.ParseDuration(envOr("CACHE_TTL", "0s"))
	if err != nil {
		log.Fatalf("[cert-rest-authz] invalid CACHE_TTL: %v", err)
	}

	// Fetch CA certificate and issue own cert with retry.
	log.Printf("[cert-rest-authz] fetching CA cert from %s", caURL)
	caPool, _ := fetchCACertWithRetry(caURL, 10)
	log.Printf("[cert-rest-authz] issuing own cert from %s", caURL)
	ownCert := issueCertWithRetry(caURL, "cert-rest-authz", 10)

	// Build mTLS server TLS config (for inbound connections from cert-consumer).
	serverTLSCfg := buildServerTLSConfig(ownCert, caPool)

	// Build upstream HTTP client (for outbound connections to data-provider-tls).
	// Must use the CA pool as RootCAs so the CA-issued server cert is accepted.
	upstreamTLSCfg := buildClientTLSConfig(ownCert, caPool)
	upstreamClient := buildMTLSUpstreamClient(upstreamTLSCfg)

	// Resolve the AuthzForce domain (created by policy-sync at startup).
	azClient := az.New(azURL)
	log.Printf("[cert-rest-authz] resolving AuthzForce domain %q at %s", azDomainExt, azURL)
	var domainID string
	for attempt := 1; ; attempt++ {
		id, resolveErr := azClient.EnsureDomain(azDomainExt)
		if resolveErr != nil || id == "" {
			if attempt == 1 || attempt%5 == 0 {
				log.Printf("[cert-rest-authz] domain lookup attempt %d failed: %v — retrying", attempt, resolveErr)
			}
			time.Sleep(3 * time.Second)
			continue
		}
		domainID = id
		break
	}
	log.Printf("[cert-rest-authz] using AuthzForce domain=%s upstream=%s (cache_ttl=%s)",
		domainID, upstreamURL, cacheTTL)

	cfg := serverConfig{
		azDomainID:     domainID,
		upstreamURL:    upstreamURL,
		defaultService: envOr("DEFAULT_SERVICE", "telemetry-rest"),
		port:           port,
		tlsPort:        tlsPort,
	}

	cache := newDecisionCache(cacheTTL)
	srv := newCertAuthzServer(cfg, azClient, cache, upstreamClient)

	// Start plain HTTP server (health + status + auth/check).
	plainMux := http.NewServeMux()
	srv.registerPlain(plainMux)
	go func() {
		ln, listenErr := net.Listen("tcp", ":"+port)
		if listenErr != nil {
			log.Fatalf("[cert-rest-authz] listen plain HTTP :%s: %v", port, listenErr)
		}
		log.Printf("[cert-rest-authz] plain HTTP listening on :%s", port)
		if serveErr := http.Serve(ln, plainMux); serveErr != nil {
			log.Fatal(serveErr)
		}
	}()

	// Start mTLS HTTPS server.
	mtlsMux := http.NewServeMux()
	srv.registerMTLS(mtlsMux)

	tlsLn, err := tls.Listen("tcp", ":"+tlsPort, serverTLSCfg)
	if err != nil {
		log.Fatalf("[cert-rest-authz] listen mTLS HTTPS :%s: %v", tlsPort, err)
	}
	log.Printf("[cert-rest-authz] mTLS HTTPS listening on :%s", tlsPort)
	if err := http.Serve(tlsLn, mtlsMux); err != nil {
		log.Fatal(err)
	}
}
