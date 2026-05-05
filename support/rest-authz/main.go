// rest-authz — REST HTTP PEP backed by AuthzForce XACML.
//
// Acts as a reverse proxy for REST requests.  Every proxied request must carry
// an X-Consumer-Name header (or a `consumer` query parameter).  rest-authz
// queries AuthzForce for the triple (consumer, service, "invoke"); if the
// decision is Permit it forwards the request to UPSTREAM_URL, otherwise it
// returns 403 Forbidden.
//
// rest-authz is the third PEP in the unified policy projection model of
// experiment-6: it shares the same AuthzForce domain with topic-auth-xacml
// (AMQP) and kafka-authz (Kafka/SSE), so a single grant in ConsumerAuth
// simultaneously authorises a consumer on all three transports.
//
// Sync-delay caveat: REST enforcement lags ConsumerAuth by up to SYNC_INTERVAL
// (policy-sync period).  A revoked grant continues to produce Permit decisions
// until policy-sync uploads the next PolicySet version to AuthzForce.
//
// Environment variables:
//   AUTHZFORCE_URL     AuthzForce base URL (default: http://authzforce:8080/authzforce-ce)
//   AUTHZFORCE_DOMAIN  AuthzForce domain externalId (default: arrowhead-exp6)
//   UPSTREAM_URL       Upstream service base URL, no trailing slash (required)
//   DEFAULT_SERVICE    Service name used when X-Service-Name is absent (default: telemetry-rest)
//   CACHE_TTL          Decision cache TTL; 0s = no caching (default: 0s)
//   PORT               HTTP port (default: 9093)
package main

import (
	"fmt"
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
	azURL       := envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce")
	azDomainExt := envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp6")
	upstreamURL := os.Getenv("UPSTREAM_URL")
	port        := envOr("PORT", "9093")

	if upstreamURL == "" {
		log.Fatal("[rest-authz] UPSTREAM_URL is required")
	}

	cacheTTL, err := time.ParseDuration(envOr("CACHE_TTL", "0s"))
	if err != nil {
		log.Fatalf("invalid CACHE_TTL: %v", err)
	}

	cfg := config{
		upstreamURL:    upstreamURL,
		defaultService: envOr("DEFAULT_SERVICE", "telemetry-rest"),
		port:           port,
	}

	client := az.New(azURL)

	// Resolve the AuthzForce domain (created by policy-sync at startup).
	// Retry until available so rest-authz can start before policy-sync is healthy,
	// but won't serve requests until the domain exists.
	log.Printf("[rest-authz] resolving AuthzForce domain %q at %s", azDomainExt, azURL)
	var domainID string
	for attempt := 1; ; attempt++ {
		id, resolveErr := client.EnsureDomain(azDomainExt)
		if resolveErr != nil || id == "" {
			if attempt == 1 || attempt%5 == 0 {
				log.Printf("[rest-authz] domain lookup attempt %d failed: %v — retrying", attempt, resolveErr)
			}
			time.Sleep(3 * time.Second)
			continue
		}
		domainID = id
		break
	}
	cfg.azDomainID = domainID
	log.Printf("[rest-authz] using AuthzForce domain=%s upstream=%s (cache_ttl=%s)",
		domainID, upstreamURL, cacheTTL)

	cache := newDecisionCache(cacheTTL)
	srv := newAuthzServer(cfg, client, cache)

	mux := http.NewServeMux()
	srv.register(mux)

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("listen :%s: %v", port, err)
	}
	log.Printf("[rest-authz] listening on :%s", port)
	if err := http.Serve(ln, mux); err != nil {
		log.Fatal(err)
	}
}

// Compile-time check: ensure fmt is used.
var _ = fmt.Sprintf
