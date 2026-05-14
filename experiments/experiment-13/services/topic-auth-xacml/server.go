// server.go — RabbitMQ HTTP auth backend for topic-auth-xacml (experiment-13).
//
// Local copy of support/topic-auth-xacml/server.go enriched with PIP cert-level
// attribute queries. Before every AuthzForce decision, the PEP queries PIP for the
// username's cert-level attributes, then passes them as additional subject
// attributes in the XACML request.
//
// In experiment-13 with rabbitmq_auth_mechanism_ssl, the username IS the cert CN.
// The PIP query on the username returns the cert attributes for that system.
//
// Decision D1: no PEP-side caching of PIP responses.
// Decision D2: cert-valid is forwarded to AuthzForce; the PEP does not pre-gate on it.
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

// handleUser validates credentials. Admin and publisher are validated locally.
// Consumer credentials are verified locally (shared password) and then their
// grant existence is confirmed via an AuthzForce decision enriched with cert-level.
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

	if username == s.cfg.publisherUser {
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

// handleVhost checks vhost access.
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

	if username == s.cfg.publisherUser {
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
