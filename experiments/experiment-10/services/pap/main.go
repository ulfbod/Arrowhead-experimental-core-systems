package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"arrowhead/authzforce"
)

// config holds all runtime configuration for the PAP.
type config struct {
	authzforceURL string
	domainExtID   string
	port          string
}

// envOr returns the value of the named environment variable, or def if unset/empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// configFromEnv builds a config from environment variables with sane defaults.
func configFromEnv() config {
	return config{
		authzforceURL: envOr("AUTHZFORCE_URL", "http://authzforce:8080/authzforce-ce"),
		domainExtID:   envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp10"),
		port:          envOr("PORT", "9305"),
	}
}

// authzforcePusher implements Pusher using the authzforce client.
type authzforcePusher struct {
	client      *authzforce.Client
	domainID    string
	policySetID string
}

// Push compiles the current policy set into XACML and uploads it to AuthzForce.
func (p *authzforcePusher) Push(policies []*Policy, version int) error {
	grants := make([]authzforce.Grant, 0, len(policies))
	for _, pol := range policies {
		if pol.Effect == "Permit" {
			grants = append(grants, authzforce.Grant{
				Consumer: pol.Subject,
				Service:  pol.Resource,
			})
		}
	}
	ver := fmt.Sprintf("%d", version)
	xml := authzforce.BuildPolicy(p.policySetID, ver, grants)
	return p.client.SetPolicy(p.domainID, xml, p.policySetID, ver)
}

func main() {
	cfg := configFromEnv()

	af := authzforce.New(cfg.authzforceURL)
	domainID, err := af.EnsureDomain(cfg.domainExtID)
	if err != nil {
		log.Fatalf("PAP: EnsureDomain %q: %v", cfg.domainExtID, err)
	}
	log.Printf("PAP: AuthzForce domain %q → %s", cfg.domainExtID, domainID)

	policySetID := "urn:arrowhead:exp10:pap"
	pusher := &authzforcePusher{
		client:      af,
		domainID:    domainID,
		policySetID: policySetID,
	}

	store := NewPolicyStore()
	srv := NewServer(store, pusher, cfg.domainExtID)

	addr := ":" + cfg.port
	log.Printf("PAP: listening on %s", addr)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("PAP: %v", err)
	}
}
