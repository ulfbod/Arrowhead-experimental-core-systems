package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"arrowhead/authzforce"
)

type config struct {
	authzforceURL string
	domainExtID   string
	port          string
	pipURL        string
	syncInterval  time.Duration
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func configFromEnv() config {
	intervalStr := envOr("SYNC_INTERVAL", "10s")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		interval = 10 * time.Second
	}
	return config{
		authzforceURL: envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce"),
		domainExtID:   envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp11"),
		port:          envOr("PORT", "9405"),
		pipURL:        envOr("PIP_URL", "http://pip:9406"),
		syncInterval:  interval,
	}
}

// ── authzforce pusher ─────────────────────────────────────────────────────────

const policySetID = "urn:arrowhead:exp11:pap"

type authzforcePusher struct {
	client    *authzforce.Client
	domainID  string
	version   int
}

// Push compiles and uploads a XACML PolicySet merging native policies and PIP grants.
// Deny-effect native policies are excluded (BuildPolicy always generates Permit rules).
func (p *authzforcePusher) Push(policies []*Policy, pipGrants []ExternalGrant, version int) error {
	seen := map[string]struct{}{}
	var grants []authzforce.Grant

	// PAP-native Permit policies take precedence.
	for _, pol := range policies {
		if pol.Effect == "Permit" {
			key := pol.Subject + "\x00" + pol.Resource
			seen[key] = struct{}{}
			grants = append(grants, authzforce.Grant{Consumer: pol.Subject, Service: pol.Resource})
		}
	}
	// PIP grants (from ConsumerAuth) fill in the rest.
	for _, g := range pipGrants {
		key := g.Subject + "\x00" + g.Resource
		if _, exists := seen[key]; !exists {
			grants = append(grants, authzforce.Grant{Consumer: g.Subject, Service: g.Resource})
		}
	}

	p.version++
	ver := fmt.Sprintf("%d", p.version)
	xml := authzforce.BuildPolicy(policySetID, ver, grants)
	return p.client.SetPolicy(p.domainID, xml, policySetID, ver)
}

// ── PIP HTTP client ───────────────────────────────────────────────────────────

type pipGrantFetcher struct {
	pipURL string
	client *http.Client
}

type pipGrantsResponse struct {
	Grants  []ExternalGrant `json:"grants"`
	Count   int             `json:"count"`
	Version int             `json:"version"`
}

func (f *pipGrantFetcher) FetchGrants() ([]ExternalGrant, int, error) {
	resp, err := f.client.Get(f.pipURL + "/grants")
	if err != nil {
		return nil, 0, fmt.Errorf("GET PIP grants: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("PIP returned %d", resp.StatusCode)
	}
	var body pipGrantsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, 0, fmt.Errorf("decode PIP response: %w", err)
	}
	return body.Grants, body.Version, nil
}

// ── main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg := configFromEnv()

	af := authzforce.New(cfg.authzforceURL)
	domainID, err := af.EnsureDomain(cfg.domainExtID)
	if err != nil {
		log.Fatalf("PAP: EnsureDomain %q: %v", cfg.domainExtID, err)
	}
	log.Printf("PAP: AuthzForce domain %q → %s", cfg.domainExtID, domainID)

	pusher := &authzforcePusher{client: af, domainID: domainID}
	fetcher := &pipGrantFetcher{
		pipURL: cfg.pipURL,
		client: &http.Client{Timeout: 5 * time.Second},
	}

	store := NewPolicyStore()
	srv := newServerImpl(store, pusher, fetcher, cfg.domainExtID)

	// Initial push.
	if err := srv.SyncFromPIP(); err != nil {
		log.Printf("PAP: initial PIP sync error: %v", err)
	}

	// Background sync loop: re-push when PIP version changes.
	go func(s *papServer, interval time.Duration) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := s.SyncFromPIP(); err != nil {
				log.Printf("PAP: sync error: %v", err)
			}
		}
	}(srv, cfg.syncInterval)

	addr := ":" + cfg.port
	log.Printf("PAP: listening on %s (PIP sync interval %s)", addr, cfg.syncInterval)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("PAP: %v", err)
	}
}

