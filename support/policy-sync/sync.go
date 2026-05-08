package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	client          *az.Client
	caURL           string
	authToken       string
	httpClient      *http.Client
	domainExtID     string // AuthzForce externalId (from AUTHZFORCE_DOMAIN env)
	domainID        string
	version         int
	grantsCount     int
	lastSyncedAt    time.Time
}

func newSyncer(client *az.Client, caURL string, httpClient *http.Client) *syncer {
	return &syncer{client: client, caURL: caURL, httpClient: httpClient}
}

func (s *syncer) setToken(tok string) { s.authToken = tok }

// init creates or looks up the AuthzForce domain and performs the first sync.
func (s *syncer) init(domainExtID string) error {
	id, err := s.client.EnsureDomain(domainExtID)
	if err != nil {
		return fmt.Errorf("EnsureDomain: %w", err)
	}
	s.domainExtID = domainExtID
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
	resp, err := s.httpClient.Do(req)
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

// buildHTTPClient returns an *http.Client. When TLS_CERT_FILE, TLS_KEY_FILE, and
// TLS_CA_FILE environment variables are all set, the client uses mutual TLS so that
// policy-sync can call a TLS-enabled ConsumerAuthorization service.
// Falls back to a plain http.Client when any variable is absent (backward-compatible
// with experiments that use plain HTTP ConsumerAuthorization).
func buildHTTPClient() *http.Client {
	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile  := os.Getenv("TLS_KEY_FILE")
	caFile   := os.Getenv("TLS_CA_FILE")
	if certFile == "" || keyFile == "" || caFile == "" {
		return &http.Client{Timeout: 10 * time.Second}
	}
	tlsCfg, err := loadClientTLS(certFile, keyFile, caFile)
	if err != nil {
		// Log and fall back to plain HTTP rather than crashing.
		fmt.Printf("[policy-sync] TLS client config error: %v — falling back to plain HTTP\n", err)
		return &http.Client{Timeout: 10 * time.Second}
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   10 * time.Second,
	}
}

func loadClientTLS(certFile, keyFile, caFile string) (*tls.Config, error) {
	caData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA file %q: %w", caFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("parse CA PEM from %q", caFile)
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load key pair (%s, %s): %w", certFile, keyFile, err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}
