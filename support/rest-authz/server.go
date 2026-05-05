// server.go — HTTP handlers for the rest-authz REST PEP.
//
// rest-authz sits in front of an upstream REST service and enforces
// authorization by querying AuthzForce on every request.
//
// Consumer identity is taken from the X-Consumer-Name request header (or
// the `consumer` query parameter). The service name is taken from the
// X-Service-Name header, falling back to the DEFAULT_SERVICE env var.
//
// For every proxied request the decision triple is:
//   subject  = consumerSystemName  (from X-Consumer-Name)
//   resource = serviceDefinition   (from X-Service-Name or DEFAULT_SERVICE)
//   action   = "invoke"
//
// Endpoints:
//   GET  /health       — liveness probe
//   GET  /status       — request counters
//   POST /auth/check   — explicit AuthzForce decision (for dashboard / tests)
//   *    /*            — PEP: check AuthzForce, proxy to upstream if Permit
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"

	az "arrowhead/authzforce"
)

type config struct {
	azDomainID     string
	upstreamURL    string
	defaultService string
	port           string
}

type stats struct {
	total     atomic.Int64
	permitted atomic.Int64
	denied    atomic.Int64
}

type authzServer struct {
	cfg    config
	client *az.Client
	cache  *decisionCache
	stats  stats
}

func newAuthzServer(cfg config, client *az.Client, cache *decisionCache) *authzServer {
	return &authzServer{cfg: cfg, client: client, cache: cache}
}

func (s *authzServer) register(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/auth/check", s.handleAuthCheck)
	// Catch-all: PEP proxy for all other paths.
	mux.HandleFunc("/", s.handleProxy)
}

func (s *authzServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}

func (s *authzServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"requestsTotal": s.stats.total.Load(),
		"permitted":     s.stats.permitted.Load(),
		"denied":        s.stats.denied.Load(),
	})
}

// handleAuthCheck accepts POST /auth/check {"consumer":"...", "service":"..."}
// and returns the AuthzForce decision.  Used by the dashboard and test scripts.
func (s *authzServer) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Consumer string `json:"consumer"`
		Service  string `json:"service"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Consumer == "" || req.Service == "" {
		http.Error(w, "consumer and service required", http.StatusBadRequest)
		return
	}
	permit, err := s.decide(r.Context(), req.Consumer, req.Service, "invoke")
	if err != nil {
		log.Printf("[rest-authz] AuthzForce error check consumer=%q service=%q: %v", req.Consumer, req.Service, err)
		http.Error(w, "PDP unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"consumer": req.Consumer,
		"service":  req.Service,
		"permit":   permit,
		"decision": map[bool]string{true: "Permit", false: "Deny"}[permit],
	})
}

// handleProxy is the main PEP handler.  It:
//  1. Extracts consumer identity from X-Consumer-Name header (or query param).
//  2. Determines service from X-Service-Name header or DEFAULT_SERVICE.
//  3. Queries AuthzForce for (consumer, service, "invoke").
//  4. If Permit: reverse-proxies the request to UPSTREAM_URL.
//  5. If Deny: returns 403 Forbidden.
func (s *authzServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Skip non-GET methods for now unless needed (data-provider only serves GET).
	consumer := r.Header.Get("X-Consumer-Name")
	if consumer == "" {
		consumer = r.URL.Query().Get("consumer")
	}
	if consumer == "" {
		http.Error(w, `{"error":"X-Consumer-Name header required"}`, http.StatusUnauthorized)
		return
	}

	service := r.Header.Get("X-Service-Name")
	if service == "" {
		service = s.cfg.defaultService
	}

	s.stats.total.Add(1)

	permit, err := s.decide(r.Context(), consumer, service, "invoke")
	if err != nil {
		log.Printf("[rest-authz] PDP error consumer=%q service=%q: %v", consumer, service, err)
		http.Error(w, `{"error":"PDP unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	if !permit {
		s.stats.denied.Add(1)
		log.Printf("[rest-authz] DENY  consumer=%q service=%q path=%s", consumer, service, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"not authorized"}`, http.StatusForbidden)
		return
	}

	s.stats.permitted.Add(1)
	log.Printf("[rest-authz] PERMIT consumer=%q service=%q → %s%s", consumer, service, s.cfg.upstreamURL, r.URL.Path)
	s.proxyRequest(w, r)
}

// proxyRequest reverse-proxies the request to the configured upstream URL.
// X-Consumer-Name and X-Service-Name headers are stripped before forwarding.
func (s *authzServer) proxyRequest(w http.ResponseWriter, r *http.Request) {
	targetURL := s.cfg.upstreamURL + r.URL.Path
	if q := r.URL.RawQuery; q != "" {
		// Strip the `consumer` query param added by clients that prefer it.
		targetURL += "?" + stripParam(q, "consumer")
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		log.Printf("[rest-authz] proxy build error: %v", err)
		http.Error(w, `{"error":"proxy error"}`, http.StatusBadGateway)
		return
	}

	// Copy all request headers except identity headers that should not leak downstream.
	for k, vv := range r.Header {
		switch k {
		case "X-Consumer-Name", "X-Service-Name":
			continue
		}
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[rest-authz] upstream error: %v", err)
		http.Error(w, `{"error":"upstream unavailable"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("[rest-authz] response copy error: %v", err)
	}
}

// decide queries AuthzForce with caching.
func (s *authzServer) decide(ctx context.Context, subject, resource, action string) (bool, error) {
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

// stripParam removes a single query parameter by name from a raw query string.
func stripParam(raw, key string) string {
	if raw == "" {
		return ""
	}
	out := ""
	for _, seg := range splitQuery(raw) {
		k, _, _ := cutParam(seg)
		if k == key {
			continue
		}
		if out != "" {
			out += "&"
		}
		out += seg
	}
	return out
}


func splitQuery(q string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '&' {
			parts = append(parts, q[start:i])
			start = i + 1
		}
	}
	parts = append(parts, q[start:])
	return parts
}

func cutParam(seg string) (key, val string, ok bool) {
	for i := 0; i < len(seg); i++ {
		if seg[i] == '=' {
			return seg[:i], seg[i+1:], true
		}
	}
	return seg, "", false
}
