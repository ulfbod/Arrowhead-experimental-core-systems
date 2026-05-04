// kafka-authz — Kafka SSE proxy with AuthzForce enforcement for experiment-5.
//
// Analytics consumers connect to GET /stream/{consumerName}?service=<service>.
// kafka-authz checks AuthzForce; if Permit it subscribes to the matching Kafka
// topic and streams messages as Server-Sent Events.  If Deny it returns 403.
//
// This is the Kafka enforcement adapter in the unified policy projection model:
// both RabbitMQ (via topic-auth-xacml) and Kafka (via kafka-authz) delegate
// authorization to the same AuthzForce PDP, which evaluates the same XACML
// policy derived from ConsumerAuthorization grants.
//
// Environment variables:
//   KAFKA_BROKERS       Comma-separated broker list (default: kafka:9092)
//   AUTHZFORCE_URL      AuthzForce base URL (default: http://authzforce:8080/authzforce-ce)
//   AUTHZFORCE_DOMAIN   AuthzForce domain externalId (default: arrowhead-exp5)
//   PORT                HTTP port (default: 9091)
package main

import (
	"log"
	"net/http"
	"os"
	"strings"
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
	kafkaBrokers := strings.Split(envOr("KAFKA_BROKERS", "kafka:9092"), ",")
	port        := envOr("PORT", "9091")

	client := az.New(azURL)

	// Resolve the AuthzForce domain ID (set up by policy-sync).
	log.Printf("[kafka-authz] resolving AuthzForce domain %q at %s", azDomainExt, azURL)
	var domainID string
	for attempt := 1; ; attempt++ {
		id, err := client.EnsureDomain(azDomainExt)
		if err != nil || id == "" {
			if attempt == 1 || attempt%5 == 0 {
				log.Printf("[kafka-authz] domain lookup attempt %d: %v — retrying", attempt, err)
			}
			time.Sleep(3 * time.Second)
			continue
		}
		domainID = id
		break
	}
	log.Printf("[kafka-authz] using AuthzForce domain=%s kafka=%v", domainID, kafkaBrokers)

	cfg := serverConfig{
		kafkaBrokers: kafkaBrokers,
		azDomainID:   domainID,
	}
	srv := newAuthzServer(cfg, client)

	mux := http.NewServeMux()
	srv.register(mux)

	log.Printf("[kafka-authz] listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
