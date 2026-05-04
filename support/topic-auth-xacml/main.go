// topic-auth-xacml — RabbitMQ HTTP auth backend that delegates to AuthzForce.
//
// Replaces topic-auth-http for experiment-5. Instead of querying
// ConsumerAuthorization directly, every RabbitMQ auth decision is evaluated
// by the AuthzForce XACML PDP. This makes the RabbitMQ enforcement adapter
// a pure policy enforcement point (PEP) with no direct CA dependency.
//
// Environment variables:
//   AUTHZFORCE_URL      AuthzForce base URL (default: http://authzforce:8080/authzforce-ce)
//   AUTHZFORCE_DOMAIN   AuthzForce domain externalId (default: arrowhead-exp5)
//   RABBITMQ_ADMIN_USER RabbitMQ admin username (default: admin)
//   RABBITMQ_ADMIN_PASS RabbitMQ admin password (default: admin)
//   PUBLISHER_USER      Publisher AMQP username (default: robot-fleet)
//   PUBLISHER_PASSWORD  Publisher AMQP password (default: fleet-secret)
//   CONSUMER_PASSWORD   Shared consumer AMQP password (default: consumer-secret)
//   CACHE_TTL           Decision cache TTL (default: 0s = no caching)
//   PORT                HTTP server port (default: 9090)
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
	azDomainExt := envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp5")
	port        := envOr("PORT", "9090")

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
	}

	client := az.New(azURL)

	// Resolve the AuthzForce domain ID (created by policy-sync at startup).
	log.Printf("[topic-auth-xacml] resolving AuthzForce domain %q at %s", azDomainExt, azURL)
	var domainID string
	for attempt := 1; ; attempt++ {
		id, err := client.EnsureDomain(azDomainExt)
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
	log.Printf("[topic-auth-xacml] using AuthzForce domain=%s (cache_ttl=%s)", domainID, cacheTTL)

	cache := newDecisionCache(cacheTTL)
	srv := newAuthServer(cfg, client, cache)

	// Proactive revocation: close AMQP connections for consumers whose grants
	// were removed.  RabbitMQ only calls the auth backend on connect/bind, so
	// without this loop a revoked consumer keeps receiving messages indefinitely.
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
