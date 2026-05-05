package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	az "arrowhead/authzforce"
)

const (
	policySetID = "urn:arrowhead:exp5:telemetry"
)

// AuthRule mirrors the ConsumerAuthorization rule wire type.
type AuthRule struct {
	ID                 int    `json:"id"`
	ConsumerSystemName string `json:"consumerSystemName"`
	ProviderSystemName string `json:"providerSystemName"`
	ServiceDefinition  string `json:"serviceDefinition"`
}

// LookupResponse is the ConsumerAuthorization /authorization/lookup response.
type LookupResponse struct {
	Rules []AuthRule `json:"rules"`
	Count int        `json:"count"`
}

// syncer holds the sync state: AuthzForce client, domain ID, current policy
// version counter, and the last known rule count for change detection.
type syncer struct {
	client       *az.Client
	caURL        string
	authToken    string
	domainID     string
	version      int
	grantsCount  int
	lastSyncedAt time.Time
}

func newSyncer(client *az.Client, caURL string) *syncer {
	return &syncer{client: client, caURL: caURL}
}

func (s *syncer) setToken(tok string) { s.authToken = tok }

// init creates or looks up the AuthzForce domain and performs the first sync.
func (s *syncer) init(domainExtID string) error {
	id, err := s.client.EnsureDomain(domainExtID)
	if err != nil {
		return fmt.Errorf("EnsureDomain: %w", err)
	}
	s.domainID = id
	return s.sync()
}

// sync fetches CA rules, compiles them into a XACML policy, and pushes it to
// AuthzForce. Increments the version counter on each call.
func (s *syncer) sync() error {
	rules, err := s.fetchRules()
	if err != nil {
		return fmt.Errorf("fetchRules: %w", err)
	}

	// Compile grants.
	grants := make([]az.Grant, 0, len(rules))
	for _, r := range rules {
		grants = append(grants, az.Grant{Consumer: r.ConsumerSystemName, Service: r.ServiceDefinition})
	}

	s.version++
	ver := strconv.Itoa(s.version)
	policyXML := az.BuildPolicy(policySetID, ver, grants)

	if err := s.client.SetPolicy(s.domainID, policyXML, policySetID, ver); err != nil {
		return fmt.Errorf("SetPolicy: %w", err)
	}
	s.grantsCount = len(grants)
	s.lastSyncedAt = time.Now()
	return nil
}

// fetchRules calls ConsumerAuth GET /authorization/lookup and returns the rules.
func (s *syncer) fetchRules() ([]AuthRule, error) {
	req, err := http.NewRequest(http.MethodGet, s.caURL+"/authorization/lookup", nil)
	if err != nil {
		return nil, err
	}
	if s.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.authToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ConsumerAuth lookup returned %d", resp.StatusCode)
	}
	var lr LookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, err
	}
	return lr.Rules, nil
}
