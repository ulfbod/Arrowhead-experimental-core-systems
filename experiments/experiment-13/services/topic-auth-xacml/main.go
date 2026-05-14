// topic-auth-xacml — RabbitMQ HTTP auth backend with PIP cert-level enrichment (experiment-13).
//
// Extends support/topic-auth-xacml with PIP cert-level attribute queries.
// Before every AuthzForce decision, the PEP queries PIP (GET /pip/attributes/{name})
// to get the subject's certLevel and valid attributes, included in the XACML request.
//
// In experiment-13 with rabbitmq_auth_mechanism_ssl, the RabbitMQ username IS the
// cert CN. The PIP query on the username returns the cert attributes for that system.
//
// Environment variables:
//
//	AUTHZFORCE_URL      AuthzForce base URL (default: http://authzforce:8080/authzforce-ce)
//	AUTHZFORCE_DOMAIN   AuthzForce domain externalId (default: arrowhead-exp13)
//	PIP_URL             PIP base URL (default: http://pip:9506)
//	RABBITMQ_ADMIN_USER RabbitMQ admin username (default: admin)
//	RABBITMQ_ADMIN_PASS RabbitMQ admin password (default: admin)
//	PUBLISHER_USER      Publisher AMQP username (default: robot-fleet)
//	PUBLISHER_PASSWORD  Publisher AMQP password (default: fleet-secret)
//	CONSUMER_PASSWORD   Shared consumer AMQP password (default: consumer-secret)
//	CACHE_TTL           Decision cache TTL (default: 0s = no caching)
//	PORT                HTTP server port (default: 9090)
package main

import (
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
	azURL        := envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce")
	azDomainExt  := envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp13")
	pipURL        := envOr("PIP_URL", "http://pip:9506")
	port         := envOr("PORT", "9090")

	cacheTTL, err := time.ParseDuration(envOr("CACHE_TTL", "0s"))
	if err != nil {
		log.Fatalf("invalid CACHE_TTL: %v", err)
	}

	cfg := config{
		rmqAdminUser:     envOr("RABBITMQ_ADMIN_USER", "admin"),
		rmqAdminPass:     envOr("RABBITMQ_ADMIN_PASS", "admin"),
		publisherUser:    envOr("PUBLISHER_USER", "robot-fleet"),
		publisherPass:    envOr("PUBLISHER_PASSWORD", "fleet-secret"),
		consumerPassword: envOr("CONSUMER_PASSWORD", "consumer-secret"),
		azURL:            azURL,
		pipURL:           pipURL,
	}

	// Resolve the AuthzForce domain ID.
	log.Printf("[topic-auth-xacml] resolving AuthzForce domain %q at %s", azDomainExt, azURL)
	var domainID string
	for attempt := 1; ; attempt++ {
		id, err := findDomain(azURL, azDomainExt)
		if err != nil || id == "" {
			if attempt == 1 || attempt%5 == 0 {
				log.Printf("[topic-auth-xacml] domain lookup attempt %d failed: %v — retrying", attempt, err)
			}
			time.Sleep(3 * time.Second)
			continue
		}
		domainID = id
		break
	}
	cfg.azDomainID = domainID
	log.Printf("[topic-auth-xacml] using AuthzForce domain=%s pip=%s (cache_ttl=%s)", domainID, pipURL, cacheTTL)

	cache := newDecisionCache(cacheTTL)
	srv := newAuthServer(cfg, cache)

	rmqMgmtURL := envOr("RABBITMQ_MGMT_URL", "http://rabbitmq:15672")
	revInterval, err := time.ParseDuration(envOr("REVOCATION_INTERVAL", "15s"))
	if err != nil {
		log.Fatalf("invalid REVOCATION_INTERVAL: %v", err)
	}
	rc := newRevocationChecker(srv, rmqMgmtURL, cfg.rmqAdminUser, cfg.rmqAdminPass, revInterval)
	go rc.run()

	mux := http.NewServeMux()
	srv.register(mux)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("listen :%s: %v", port, err)
	}
	log.Printf("[topic-auth-xacml] listening on :%s", port)
	if err := http.Serve(ln, mux); err != nil {
		log.Fatal(err)
	}
}
