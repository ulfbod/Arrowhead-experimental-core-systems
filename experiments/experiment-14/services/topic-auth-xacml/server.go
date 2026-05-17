// server.go — RabbitMQ HTTP auth backend for topic-auth-xacml (experiment-14).
//
// Extends experiment-13 with connection-time cert-validity pre-gate (design
// decision D2'). Before consulting AuthzForce, handleUser and handleVhost query
// PIP directly. If certValid=false the AMQP connection is rejected immediately
// without calling the PDP at all.
//
// In experiment-13 with rabbitmq_auth_mechanism_ssl, the username IS the cert CN.
// The PIP query on the username returns the cert attributes for that system.
//
// Design decisions:
//   D1  — no PEP-side caching of PIP responses.
//   D2' — cert-valid pre-gate at connection time: PIP checked before PDP.
//          A revoked cert causes an immediate deny without ever calling AuthzForce.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type config struct {
	rmqAdminUser     string
	rmqAdminPass     string
	publisherUser    string
	publisherPass    string
	consumerPassword string
	azDomainID       string
	azURL            string // AuthzForce base URL
	pipURL           string // PIP base URL
}

type authServer struct {
	cfg   config
	pip   *pipClient
	cache *decisionCache
}

func newAuthServer(cfg config, cache *decisionCache) *authServer {
	return &authServer{cfg: cfg, pip: newPIPClient(cfg.pipURL), cache: cache}
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

// isPublisher returns true if username is a publisher identity.
// Any username starting with the configured publisherUser prefix is a publisher
// (e.g. PUBLISHER_USER="robot-fleet" matches "robot-fleet-site-1", "robot-fleet-tls", etc.)
func (s *authServer) isPublisher(username string) bool {
	return strings.HasPrefix(username, s.cfg.publisherUser)
}

// handleUser validates credentials. Admin and publisher are validated locally.
// Consumer credentials are verified locally (shared password), then pre-gated on
// certValid from PIP (D2'), and finally authorized via AuthzForce.
func (s *authServer) handleUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		fmt.Fprint(w, "deny")
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == s.cfg.rmqAdminUser {
		if password == s.cfg.rmqAdminPass {
			fmt.Fprint(w, "allow administrator management")
		} else {
			fmt.Fprint(w, "deny")
		}
		return
	}

	if s.isPublisher(username) {
		if password == s.cfg.publisherPass {
			fmt.Fprint(w, "allow")
		} else {
			fmt.Fprint(w, "deny")
		}
		return
	}

	if password != s.cfg.consumerPassword {
		fmt.Fprint(w, "deny")
		return
	}

	// Experiment-14 — connection-time cert validity pre-gate (D2' design decision).
	// PIP is queried before AuthzForce. If certValid=false, the AMQP connection is
	// rejected here — without consulting the PDP at all. This enforces revocation
	// at connection setup, not only at message publication/consumption time.
	attrs, pipErr := s.pip.GetAttributes(username)
	if pipErr != nil || !attrs.CertValid {
		if pipErr != nil {
			log.Printf("[topic-auth-xacml] PIP error for user=%q: %v — failing closed", username, pipErr)
		} else {
			log.Printf("[topic-auth-xacml] connection rejected: cert revoked or unknown for %q", username)
		}
		fmt.Fprint(w, "deny")
		return
	}

	ok, err := s.decide(r.Context(), username, "telemetry", "consume")
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

// handleVhost checks vhost access. Consumers are pre-gated on certValid (D2')
// before consulting AuthzForce — revoked certs are rejected at connection time.
func (s *authServer) handleVhost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		fmt.Fprint(w, "deny")
		return
	}
	username := r.FormValue("username")

	if username == s.cfg.rmqAdminUser || s.isPublisher(username) {
		fmt.Fprint(w, "allow")
		return
	}

	// Experiment-14 — connection-time cert validity pre-gate (D2').
	attrs, pipErr := s.pip.GetAttributes(username)
	if pipErr != nil || !attrs.CertValid {
		if pipErr != nil {
			log.Printf("[topic-auth-xacml] PIP error for vhost user=%q: %v — failing closed", username, pipErr)
		} else {
			log.Printf("[topic-auth-xacml] vhost rejected: cert revoked or unknown for %q", username)
		}
		fmt.Fprint(w, "deny")
		return
	}

	ok, err := s.decide(r.Context(), username, "telemetry", "consume")
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

// handleTopic is the primary live enforcement point.
func (s *authServer) handleTopic(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		fmt.Fprint(w, "deny")
		return
	}
	username   := r.FormValue("username")
	permission := r.FormValue("permission")
	routingKey := r.FormValue("routing_key")

	if username == s.cfg.rmqAdminUser {
		fmt.Fprint(w, "allow")
		return
	}

	if s.isPublisher(username) {
		if permission == "write" {
			fmt.Fprint(w, "allow")
		} else {
			fmt.Fprint(w, "deny")
		}
		return
	}

	if permission != "read" {
		fmt.Fprint(w, "deny")
		return
	}

	service := serviceFromRoutingKey(routingKey)
	if service == "" {
		fmt.Fprint(w, "deny")
		return
	}

	ok, err := s.decide(r.Context(), username, service, "consume")
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

// decide queries PIP then AuthzForce with cert-level enrichment and optional caching.
// Decision D1: PIP is queried on every call (no caching of PIP responses).
func (s *authServer) decide(ctx context.Context, subject, resource, action string) (bool, error) {
	if permit, ok := s.cache.get(subject, resource, action); ok {
		return permit, nil
	}
	// Query PIP for cert-level attributes (D1: no caching).
	attrs, _ := s.pip.GetAttributes(subject)
	permit, err := decideWithCertLevel(s.cfg.azURL, s.cfg.azDomainID, subject, resource, action, attrs.CertLevel, attrs.CertValid)
	if err != nil {
		return false, err
	}
	s.cache.set(subject, resource, action, permit)
	return permit, nil
}

// serviceFromRoutingKey extracts the service definition from a routing key.
func serviceFromRoutingKey(key string) string {
	for _, suffix := range []string{".#", ".*"} {
		if strings.HasSuffix(key, suffix) {
			return key[:len(key)-len(suffix)]
		}
	}
	idx := strings.LastIndex(key, ".")
	if idx > 0 {
		return key[:idx]
	}
	return key
}
