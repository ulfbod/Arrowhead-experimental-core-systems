// server.go implements the RabbitMQ HTTP auth backend API endpoints.
//
// RabbitMQ calls these endpoints on every auth decision. Instead of querying
// ConsumerAuthorization directly (as topic-auth-http does), this service
// delegates every decision to the AuthzForce PDP using XACML 3.0 requests.
//
// Endpoint semantics (same wire protocol as topic-auth-http):
//
//	POST /auth/user     — validates credentials; returns "allow [tags]" or "deny"
//	POST /auth/vhost    — checks vhost access; returns "allow" or "deny"
//	POST /auth/resource — always "allow" (resource-level authz delegated to topic)
//	POST /auth/topic    — live XACML decision per routing-key operation
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	az "arrowhead/authzforce"
)

type config struct {
	rmqAdminUser     string
	rmqAdminPass     string
	publisherUser    string
	publisherPass    string
	consumerPassword string
	azDomainID       string
}

type authServer struct {
	cfg    config
	client *az.Client
	cache  *decisionCache
}

func newAuthServer(cfg config, client *az.Client, cache *decisionCache) *authServer {
	return &authServer{cfg: cfg, client: client, cache: cache}
}

func (s *authServer) register(mux *http.ServeMux) {
	mux.HandleFunc("/auth/user",     s.requirePOST(s.handleUser))
	mux.HandleFunc("/auth/vhost",    s.requirePOST(s.handleVhost))
	mux.HandleFunc("/auth/resource", s.requirePOST(s.handleResource))
	mux.HandleFunc("/auth/topic",    s.requirePOST(s.handleTopic))
}

func (s *authServer) requirePOST(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		h(w, r)
	}
}

// handleUser validates credentials. Admin and publisher are validated locally.
// Consumer credentials are verified locally (shared password) and then their
// grant existence is confirmed via an AuthzForce vhost-level decision.
func (s *authServer) handleUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		fmt.Fprint(w, "deny")
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")

	// Admin: hard-coded credentials + management tags.
	if username == s.cfg.rmqAdminUser {
		if password == s.cfg.rmqAdminPass {
			fmt.Fprint(w, "allow administrator management")
		} else {
			fmt.Fprint(w, "deny")
		}
		return
	}

	// Publisher: hard-coded credentials.
	if username == s.cfg.publisherUser {
		if password == s.cfg.publisherPass {
			fmt.Fprint(w, "allow")
		} else {
			fmt.Fprint(w, "deny")
		}
		return
	}

	// Consumer: shared password + XACML grant check.
	if password != s.cfg.consumerPassword {
		fmt.Fprint(w, "deny")
		return
	}
	// Use a generic service query — we just need to know if this consumer
	// has ANY grant (the specific service check happens in /auth/topic).
	ok, err := s.decide(r.Context(), username, "telemetry", "subscribe")
	if err != nil {
		log.Printf("[topic-auth-xacml] AuthzForce error for user=%q: %v", username, err)
		fmt.Fprint(w, "deny")
		return
	}
	if ok {
		fmt.Fprint(w, "allow")
	} else {
		fmt.Fprint(w, "deny")
	}
}

// handleVhost checks vhost access. Admin and publisher are always allowed.
// Consumers require a valid grant in AuthzForce.
func (s *authServer) handleVhost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		fmt.Fprint(w, "deny")
		return
	}
	username := r.FormValue("username")

	if username == s.cfg.rmqAdminUser || username == s.cfg.publisherUser {
		fmt.Fprint(w, "allow")
		return
	}

	ok, err := s.decide(r.Context(), username, "telemetry", "subscribe")
	if err != nil {
		log.Printf("[topic-auth-xacml] AuthzForce error for vhost user=%q: %v", username, err)
		fmt.Fprint(w, "deny")
		return
	}
	if ok {
		fmt.Fprint(w, "allow")
	} else {
		fmt.Fprint(w, "deny")
	}
}

// handleResource always allows — fine-grained enforcement is at the topic level.
func (s *authServer) handleResource(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "allow")
}

// handleTopic is the primary live enforcement point. Called by RabbitMQ on
// every basic.publish and queue.bind. Routing keys are of the form
// "telemetry.robot-001" or "telemetry.#".
func (s *authServer) handleTopic(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		fmt.Fprint(w, "deny")
		return
	}
	username   := r.FormValue("username")
	permission := r.FormValue("permission") // "read" or "write"
	routingKey := r.FormValue("routing_key")

	// Admin: always allowed.
	if username == s.cfg.rmqAdminUser {
		fmt.Fprint(w, "allow")
		return
	}

	// Publisher: write only.
	if username == s.cfg.publisherUser {
		if permission == "write" {
			fmt.Fprint(w, "allow")
		} else {
			fmt.Fprint(w, "deny")
		}
		return
	}

	// Consumers: read only; derive service from routing key prefix.
	if permission != "read" {
		fmt.Fprint(w, "deny")
		return
	}

	service := serviceFromRoutingKey(routingKey)
	if service == "" {
		fmt.Fprint(w, "deny")
		return
	}

	ok, err := s.decide(r.Context(), username, service, "subscribe")
	if err != nil {
		log.Printf("[topic-auth-xacml] AuthzForce error topic user=%q key=%q: %v", username, routingKey, err)
		fmt.Fprint(w, "deny")
		return
	}
	if ok {
		fmt.Fprint(w, "allow")
	} else {
		fmt.Fprint(w, "deny")
	}
}

// decide queries the AuthzForce PDP with caching.
func (s *authServer) decide(ctx context.Context, subject, resource, action string) (bool, error) {
	if permit, ok := s.cache.get(subject, resource, action); ok {
		return permit, nil
	}
	permit, err := s.client.Decide(s.cfg.azDomainID, subject, resource, action)
	if err != nil {
		return false, err
	}
	s.cache.set(subject, resource, action, permit)
	return permit, nil
}

// serviceFromRoutingKey extracts the service definition from a routing key.
// "telemetry.robot-001" → "telemetry", "telemetry.#" → "telemetry".
// Returns "" if the key has no recognizable prefix.
func serviceFromRoutingKey(key string) string {
	// Strip AMQP wildcard suffixes.
	for _, suffix := range []string{".#", ".*"} {
		if strings.HasSuffix(key, suffix) {
			return key[:len(key)-len(suffix)]
		}
	}
	// Exact key: take everything before the last dot.
	idx := strings.LastIndex(key, ".")
	if idx > 0 {
		return key[:idx]
	}
	return key
}
