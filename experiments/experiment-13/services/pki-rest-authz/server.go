// server.go — HTTP/mTLS handlers for pki-rest-authz (experiment-13).
//
// Local copy of experiment-8/pki-rest-authz/server.go enriched with PIP
// cert-level attribute queries. Before every AuthzForce decision, the PEP
// queries PIP for the consumer CN's cert-level attributes, then passes them
// as additional subject attributes in the XACML request.
//
// Decision D1: no PEP-side caching of PIP responses.
// Decision D2: cert-valid is forwarded to AuthzForce; PEP does not pre-gate on it.
// Fail-closed: PIP 404 or unreachable → certLevel="", certValid=false → AuthzForce likely DENY.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// serverConfig holds configuration resolved at startup.
type serverConfig struct {
	azDomainID     string
	azURL          string // AuthzForce base URL
	pipURL         string // PIP base URL
	upstreamURL    string
	defaultService string
	port           string
	tlsPort        string
}

// serverStats tracks request counters.
type serverStats struct {
	total     atomic.Int64
	permitted atomic.Int64
	denied    atomic.Int64
}

// certAuthzServer handles both plain-HTTP and mTLS-HTTPS endpoints.
type certAuthzServer struct {
	cfg            serverConfig
	pip            *pipClient
	cache          *decisionCache
	stats          serverStats
	upstreamClient *http.Client
}

func newCertAuthzServer(cfg serverConfig, cache *decisionCache, upstreamClient *http.Client) *certAuthzServer {
	return &certAuthzServer{
		cfg:            cfg,
		pip:            newPIPClient(cfg.pipURL),
		cache:          cache,
		upstreamClient: upstreamClient,
	}
}

// registerPlain registers the plain-HTTP endpoints onto mux.
func (s *certAuthzServer) registerPlain(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/auth/check", s.handleAuthCheck)
}

// registerMTLS registers the mTLS proxy endpoint onto mux.
func (s *certAuthzServer) registerMTLS(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleMTLSProxy)
}

func (s *certAuthzServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}

func (s *certAuthzServer) handleStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"requestsTotal": s.stats.total.Load(),
		"permitted":     s.stats.permitted.Load(),
		"denied":        s.stats.denied.Load(),
	})
}

// handleAuthCheck accepts POST /auth/check {"consumer":"...", "service":"..."}
// and returns the AuthzForce decision enriched with PIP cert-level attrs.
func (s *certAuthzServer) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("[pki-rest-authz] AuthzForce error check consumer=%q service=%q: %v", req.Consumer, req.Service, err)
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

// handleMTLSProxy is the mTLS PEP handler.
// It reads the consumer identity from the client certificate CN, queries PIP
// for cert-level attrs, enriches the XACML request, and reverse-proxies if Permit.
func (s *certAuthzServer) handleMTLSProxy(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, `{"error":"client certificate required"}`, http.StatusUnauthorized)
		return
	}
	consumer := r.TLS.PeerCertificates[0].Subject.CommonName
	if consumer == "" {
		http.Error(w, `{"error":"client certificate has empty CN"}`, http.StatusUnauthorized)
		return
	}

	service := r.Header.Get("X-Service-Name")
	if service == "" {
		service = s.cfg.defaultService
	}

	s.stats.total.Add(1)

	permit, err := s.decide(r.Context(), consumer, service, "invoke")
	if err != nil {
		log.Printf("[pki-rest-authz] PDP error consumer=%q service=%q: %v", consumer, service, err)
		http.Error(w, `{"error":"PDP unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	if !permit {
		s.stats.denied.Add(1)
		log.Printf("[pki-rest-authz] DENY  consumer=%q service=%q path=%s", consumer, service, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"not authorized"}`, http.StatusForbidden)
		return
	}

	s.stats.permitted.Add(1)
	log.Printf("[pki-rest-authz] PERMIT consumer=%q (CN) service=%q → %s%s",
		consumer, service, s.cfg.upstreamURL, r.URL.Path)
	s.proxyRequest(w, r)
}

// proxyRequest reverse-proxies the request to the configured upstream URL.
func (s *certAuthzServer) proxyRequest(w http.ResponseWriter, r *http.Request) {
	targetURL := s.cfg.upstreamURL + r.URL.Path
	if q := r.URL.RawQuery; q != "" {
		targetURL += "?" + q
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		log.Printf("[pki-rest-authz] proxy build error: %v", err)
		http.Error(w, `{"error":"proxy error"}`, http.StatusBadGateway)
		return
	}

	for k, vv := range r.Header {
		if k == "X-Service-Name" {
			continue
		}
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	resp, err := s.upstreamClient.Do(req)
	if err != nil {
		log.Printf("[pki-rest-authz] upstream error: %v", err)
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
		log.Printf("[pki-rest-authz] response copy error: %v", err)
	}
}

// decide queries PIP then AuthzForce with cert-level enrichment and optional caching.
// Decision D1: PIP is queried on every call (no caching of PIP responses).
func (s *certAuthzServer) decide(ctx context.Context, subject, resource, action string) (bool, error) {
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

// buildMTLSUpstreamClient builds an http.Client that uses the given TLS config
// for outgoing connections to the upstream service.
func buildMTLSUpstreamClient(tlsCfg *tls.Config) *http.Client {
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   10 * time.Second,
	}
}

// ── decisionCache ─────────────────────────────────────────────────────────────

type decisionKey struct {
	subject  string
	resource string
	action   string
}

type decisionEntry struct {
	permit    bool
	expiresAt time.Time
}

// decisionCache caches AuthzForce Permit/Deny results.
// A TTL of zero means no caching.
type decisionCache struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[decisionKey]decisionEntry
}

func newDecisionCache(ttl time.Duration) *decisionCache {
	return &decisionCache{ttl: ttl, m: make(map[decisionKey]decisionEntry)}
}

func (c *decisionCache) get(subject, resource, action string) (bool, bool) {
	if c.ttl == 0 {
		return false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[decisionKey{subject, resource, action}]
	if !ok || time.Now().After(e.expiresAt) {
		return false, false
	}
	return e.permit, true
}

func (c *decisionCache) set(subject, resource, action string, permit bool) {
	if c.ttl == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[decisionKey{subject, resource, action}] = decisionEntry{
		permit:    permit,
		expiresAt: time.Now().Add(c.ttl),
	}
}
