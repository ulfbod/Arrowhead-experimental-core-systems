package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
)

// authServer handles RabbitMQ HTTP auth backend requests.
//
// RabbitMQ is configured with a split authn/authz model:
//
//	auth_backends.1.authn = rabbit_auth_backend_internal
//	auth_backends.1.authz = rabbit_auth_backend_http
//
// The internal backend validates usernames and passwords; this server handles
// all authorization decisions (vhost, resource, topic). Because authorization
// is evaluated live against ConsumerAuthorization on every broker operation,
// a revoked grant takes effect within the next publish or bind — eliminating
// the T_sync delay of the polling-only architecture.
//
// The /auth/user endpoint is also implemented for completeness: it is not
// called by RabbitMQ when authn=internal, but enables testing the full flow
// and supports configurations where authn is also delegated to this server.
type authServer struct {
	cfg        config
	fetchRules func(ctx context.Context) ([]AuthRule, error)
}

func newAuthServer(cfg config, fetchRules func(ctx context.Context) ([]AuthRule, error)) *authServer {
	return &authServer{cfg: cfg, fetchRules: fetchRules}
}

func (s *authServer) register(mux *http.ServeMux) {
	mux.HandleFunc("/auth/user", s.handleUser)
	mux.HandleFunc("/auth/vhost", s.handleVhost)
	mux.HandleFunc("/auth/resource", s.handleResource)
	mux.HandleFunc("/auth/topic", s.handleTopic)
}

// handleUser validates credentials for a connecting client.
// With auth_backends.1 = rabbit_auth_backend_http, this is the sole
// authentication point for all users — including the RabbitMQ admin.
//
// Responses follow the HTTP auth backend protocol (plain text):
//   "allow [tag1 tag2]"  — authenticated, with optional user tags
//   "deny"               — authentication failed
//
// Admin tags "administrator management" are required for management UI access.
func (s *authServer) handleUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		fmt.Fprint(w, "deny")
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" {
		fmt.Fprint(w, "deny")
		return
	}

	// Admin user: return management tags so the management UI and management
	// HTTP API continue to work. This is the sole auth point (no internal
	// backend fallback), so we must handle admin explicitly.
	if username == s.cfg.rmqAdminUser {
		if password == s.cfg.rmqAdminPass {
			fmt.Fprint(w, "allow administrator management")
		} else {
			fmt.Fprint(w, "deny")
		}
		return
	}

	// Publisher authenticates with its own password.
	if username == s.cfg.publisherUser {
		if password == s.cfg.publisherPass {
			fmt.Fprint(w, "allow")
		} else {
			fmt.Fprint(w, "deny")
		}
		return
	}

	// Consumer users authenticate with the shared consumer password, then
	// must have at least one active grant in ConsumerAuthorization.
	if password != s.cfg.consumerPassword {
		fmt.Fprint(w, "deny")
		return
	}
	rules, err := s.fetchRules(r.Context())
	if err != nil {
		log.Printf("[auth/user] fetchRules: %v", err)
		fmt.Fprint(w, "deny")
		return
	}
	if hasAnyGrant(rules, username) {
		fmt.Fprint(w, "allow")
		return
	}
	fmt.Fprint(w, "deny")
}

// handleVhost authorizes access to a virtual host.
// Called by RabbitMQ on every new connection when authz=http.
// Consumers are allowed only if they have at least one active grant in CA.
// A revoked consumer that tries to reconnect is denied here immediately.
func (s *authServer) handleVhost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		fmt.Fprint(w, "deny")
		return
	}
	username := r.FormValue("username")

	if s.isAdminOrPublisher(username) {
		fmt.Fprint(w, "allow")
		return
	}

	rules, err := s.fetchRules(r.Context())
	if err != nil {
		log.Printf("[auth/vhost] fetchRules: %v", err)
		fmt.Fprint(w, "deny")
		return
	}
	if hasAnyGrant(rules, username) {
		fmt.Fprint(w, "allow")
		return
	}
	fmt.Fprint(w, "deny")
}

// handleResource authorizes access to exchanges and queues.
// We allow all resource operations for users that have passed vhost authorization.
// Fine-grained control is enforced at the topic level.
func (s *authServer) handleResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fmt.Fprint(w, "allow")
}

// handleTopic authorizes a publish (write) or bind (read) on a topic exchange.
// This is the primary live enforcement point: called by RabbitMQ on every
// basic.publish and queue.bind. A revoked grant is effective on the consumer's
// next operation — no polling delay.
func (s *authServer) handleTopic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		fmt.Fprint(w, "deny")
		return
	}
	username := r.FormValue("username")
	permission := r.FormValue("permission") // "write" (publish) or "read" (bind/subscribe)
	routingKey := r.FormValue("routing_key")

	// Admin user: unrestricted access.
	if username == s.cfg.rmqAdminUser {
		fmt.Fprint(w, "allow")
		return
	}

	// Publisher may write any routing key; it does not subscribe.
	if username == s.cfg.publisherUser && permission == "write" {
		fmt.Fprint(w, "allow")
		return
	}

	rules, err := s.fetchRules(r.Context())
	if err != nil {
		log.Printf("[auth/topic] fetchRules: %v", err)
		fmt.Fprint(w, "deny")
		return
	}

	if permission == "read" && routingKeyAllowed(rules, username, routingKey) {
		fmt.Fprint(w, "allow")
		return
	}

	log.Printf("[auth/topic] denied %q %s %q", username, permission, routingKey)
	fmt.Fprint(w, "deny")
}

func (s *authServer) isAdminOrPublisher(username string) bool {
	return username == s.cfg.rmqAdminUser || username == s.cfg.publisherUser
}
