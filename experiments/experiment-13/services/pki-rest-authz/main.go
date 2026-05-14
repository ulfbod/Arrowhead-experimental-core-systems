// pki-rest-authz — mTLS reverse-proxy PEP with PIP cert-level enrichment (experiment-13).
//
// Extends experiment-8/pki-rest-authz with PIP cert-level attribute queries.
// Before calling AuthzForce, the PEP queries PIP (GET /pip/attributes/{name})
// to get the consumer's certLevel and valid attributes, included in the XACML request.
//
// Environment variables:
//
//	CA_URL             profile-ca plain HTTP URL (default: http://profile-ca:8087)
//	CA_TLS_URL         profile-ca mTLS URL (default: https://profile-ca:8088)
//	AUTHZFORCE_URL     AuthzForce base URL (default: http://authzforce:8080/authzforce-ce)
//	AUTHZFORCE_DOMAIN  AuthzForce domain externalId (default: arrowhead-exp13)
//	PIP_URL            PIP base URL (default: http://pip:9506)
//	UPSTREAM_URL       Upstream service base URL, no trailing slash (required)
//	DEFAULT_SERVICE    Service name when X-Service-Name is absent (default: telemetry-rest)
//	PORT               Plain HTTP port (default: 9109)
//	TLS_PORT           mTLS HTTPS port (default: 9108)
//	CACHE_TTL          Decision cache TTL; 0s = no caching (default: 0s)
package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// findDomain looks up an AuthzForce domain by externalID.
func findDomain(azURL, externalID string) (string, error) {
	url := fmt.Sprintf("%s/domains?externalId=%s", azURL, externalID)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("list domains returned %d", resp.StatusCode)
	}
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return extractHrefID(sb.String()), nil
}

func extractHrefID(s string) string {
	for _, prefix := range []string{`href="`, `href='`} {
		idx := strings.Index(s, prefix)
		if idx < 0 {
			continue
		}
		rest := s[idx+len(prefix):]
		end := strings.IndexAny(rest, `"'`)
		if end < 0 {
			continue
		}
		href := rest[:end]
		href = strings.TrimRight(href, "/")
		i := strings.LastIndex(href, "/")
		if i >= 0 {
			return href[i+1:]
		}
		return href
	}
	return ""
}

func main() {
	caURL       := envOr("CA_URL", "http://profile-ca:8087")
	caTLSURL    := envOr("CA_TLS_URL", "https://profile-ca:8088")
	azURL       := envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce")
	azDomainExt := envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp13")
	pipURL       := envOr("PIP_URL", "http://pip:9506")
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

	// Perform the full Arrowhead 5.2 onboarding lifecycle.
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

	serverTLSCfg := buildServerTLSConfig(ownCert, caPool)
	upstreamTLSCfg := buildClientTLSConfig(ownCert, caPool)
	upstreamClient := buildMTLSUpstreamClient(upstreamTLSCfg)

	// Resolve AuthzForce domain.
	log.Printf("[pki-rest-authz] resolving AuthzForce domain %q at %s", azDomainExt, azURL)
	var domainID string
	for attempt := 1; ; attempt++ {
		id, resolveErr := findDomain(azURL, azDomainExt)
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
		azURL:          azURL,
		pipURL:         pipURL,
		upstreamURL:    upstreamURL,
		defaultService: envOr("DEFAULT_SERVICE", "telemetry-rest"),
		port:           port,
		tlsPort:        tlsPort,
	}

	cache := newDecisionCache(cacheTTL)
	srv := newCertAuthzServer(cfg, cache, upstreamClient)

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
