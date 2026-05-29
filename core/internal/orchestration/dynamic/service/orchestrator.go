// Package service implements DynamicServiceOrchestration business logic.
//
// AH5 responsibility: find matching service instances by dynamically querying
// the ServiceRegistry and (optionally) checking ConsumerAuthorization.
//
// Strategy: "dynamic" — real-time lookup, no pre-configured rules.
package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	orchmodel "arrowhead/core/internal/orchestration/model"
)

var (
	ErrMissingRequester = errors.New("requesterSystem.systemName is required")
	ErrMissingService   = errors.New("requestedService.serviceDefinition is required")
	// ErrIdentityRequired is returned when ENABLE_IDENTITY_CHECK=true and no token was provided.
	ErrIdentityRequired = errors.New("identity token required: provide Authorization: Bearer <token>")
	// ErrIdentityInvalid is returned when the token is expired, unknown, or the Authentication system is unreachable.
	ErrIdentityInvalid = errors.New("identity token is invalid or expired")
)

// srLookupRequest is the body for POST /serviceregistry/service-discovery/lookup (AH5).
type srLookupRequest struct {
	ServiceDefinitionNames []string `json:"serviceDefinitionNames"`
	ProviderNames          []string `json:"providerNames,omitempty"`
}

// srAH5System mirrors the provider system in an AH5 service instance response.
type srAH5System struct {
	Name string `json:"name"`
}

// srAH5Interface mirrors the interface entry in an AH5 service instance response.
type srAH5Interface struct {
	TemplateName string `json:"templateName"`
}

// srAH5ServiceInstance mirrors the AH5 service instance response shape.
type srAH5ServiceInstance struct {
	InstanceID            string           `json:"instanceId"`
	Provider              *srAH5System     `json:"provider,omitempty"`
	ServiceDefinitionName string           `json:"serviceDefinitionName"`
	Version               string           `json:"version,omitempty"`
	ExpiresAt             string           `json:"expiresAt,omitempty"`
	Interfaces            []srAH5Interface `json:"interfaces,omitempty"`
}

// srAH5LookupResponse mirrors POST /serviceregistry/service-discovery/lookup response.
type srAH5LookupResponse struct {
	Entries []*srAH5ServiceInstance `json:"entries"`
	Count   int                     `json:"count"`
}

// caVerifyRequest mirrors the ConsumerAuthorization verify body (AH5 model).
// Provider is included so the check is scoped to the specific provider's policy.
type caVerifyRequest struct {
	Consumer   string `json:"consumer"`
	Provider   string `json:"provider,omitempty"`
	Target     string `json:"target"`
	TargetType string `json:"targetType"`
}

// authSysVerifyResponse mirrors the Authentication identity/verify response.
type authSysVerifyResponse struct {
	Verified   bool   `json:"verified"`
	SystemName string `json:"systemName"`
}

// DynamicOrchestrator performs real-time orchestration.
type DynamicOrchestrator struct {
	srURL         string
	caURL         string // ConsumerAuthorization base URL
	authSysURL    string // Authentication system base URL
	checkAuth     bool
	checkIdentity bool
	httpClient    *http.Client
	hist          *historyStore
}

// NewDynamicOrchestrator creates a new orchestrator with a default http.Client.
//
//   - srURL:         ServiceRegistry base URL
//   - caURL:         ConsumerAuthorization base URL (used when checkAuth=true)
//   - authSysURL:    Authentication system base URL (used when checkIdentity=true)
//   - checkAuth:     when true, filters providers through ConsumerAuthorization
//   - checkIdentity: when true, requires a valid Bearer token and uses the verified
//     systemName from the token instead of the self-reported requesterSystem.systemName
func NewDynamicOrchestrator(srURL, caURL, authSysURL string, checkAuth, checkIdentity bool) *DynamicOrchestrator {
	return NewDynamicOrchestratorWithClient(srURL, caURL, authSysURL, checkAuth, checkIdentity,
		&http.Client{Timeout: 5 * time.Second})
}

// NewDynamicOrchestratorWithClient creates a new orchestrator with a custom
// http.Client.  Use this when the upstream core services are behind TLS and
// the caller must present a client certificate (mutual TLS).
func NewDynamicOrchestratorWithClient(srURL, caURL, authSysURL string, checkAuth, checkIdentity bool, client *http.Client) *DynamicOrchestrator {
	return &DynamicOrchestrator{
		srURL:         srURL,
		caURL:         caURL,
		authSysURL:    authSysURL,
		checkAuth:     checkAuth,
		checkIdentity: checkIdentity,
		httpClient:    client,
		hist:          newHistoryStore(),
	}
}

// QueryHistory returns all recorded orchestration history entries.
func (o *DynamicOrchestrator) QueryHistory() HistoryQueryResponse {
	return o.hist.query()
}

// RecordPushHistory adds a PUSH-type history entry. Used by the trigger handler.
func (o *DynamicOrchestrator) RecordPushHistory(subscriptionID, requester, service, status string) {
	o.hist.add(newHistoryEntryTyped(
		requester, service, status,
		"triggered for subscription "+subscriptionID,
		"PUSH",
	))
}

// Orchestrate performs the pull operation: optionally verify identity, query SR,
// optionally check ConsumerAuthorization, and return results.
//
// token is the Bearer token from the Authorization header (empty string if absent).
// When checkIdentity=true, an empty token returns ErrIdentityRequired; an invalid
// or expired token returns ErrIdentityInvalid. On success, the verified systemName
// from the token replaces req.RequesterSystem.SystemName for all downstream checks.
func (o *DynamicOrchestrator) Orchestrate(req orchmodel.OrchestrationRequest, token string) (orchmodel.OrchestrationResponse, error) {
	// Step 1: Identity verification (beyond AH5 spec — see GAP_ANALYSIS.md D8).
	if o.checkIdentity {
		if token == "" {
			return orchmodel.OrchestrationResponse{}, ErrIdentityRequired
		}
		verifiedName, err := o.verifyIdentity(token)
		if err != nil {
			return orchmodel.OrchestrationResponse{}, ErrIdentityInvalid
		}
		// Override self-reported name with the cryptographically verified identity.
		req.RequesterSystem.SystemName = verifiedName
	}

	// Step 2: Validate request fields.
	if req.RequesterSystem.SystemName == "" {
		return orchmodel.OrchestrationResponse{}, ErrMissingRequester
	}
	if req.RequestedService.ServiceDefinition == "" {
		return orchmodel.OrchestrationResponse{}, ErrMissingService
	}

	// Step 3: Query Service Registry (AH5 service-discovery/lookup endpoint).
	srResp, err := o.querySR(req.RequestedService)
	if err != nil {
		return orchmodel.OrchestrationResponse{}, fmt.Errorf("service registry unreachable: %w", err)
	}

	// Step 4: Filter by ConsumerAuthorization (optional).
	var results []orchmodel.OrchestrationResult
	for _, inst := range srResp.Entries {
		providerName := ""
		if inst.Provider != nil {
			providerName = inst.Provider.Name
		}
		if o.checkAuth {
			ok, err := o.checkAuthorized(req.RequesterSystem.SystemName, providerName, inst.ServiceDefinitionName)
			if err != nil || !ok {
				continue
			}
		}
		// Extract interface template names.
		ifaceNames := make([]string, 0, len(inst.Interfaces))
		for _, ifc := range inst.Interfaces {
			ifaceNames = append(ifaceNames, ifc.TemplateName)
		}
		results = append(results, orchmodel.OrchestrationResult{
			ProviderName:      providerName,
			ServiceDefinition: inst.ServiceDefinitionName,
			ServiceInstanceId: inst.InstanceID,
			Interfaces:        ifaceNames,
			AliveUntil:        inst.ExpiresAt,
		})
	}
	if results == nil {
		results = []orchmodel.OrchestrationResult{}
	}

	// Step 5: Apply orchestration flags.
	flags := req.OrchestrationFlags
	if flags.OnlyPreferred && len(req.PreferredProviders) > 0 {
		preferred := make(map[string]bool, len(req.PreferredProviders))
		for _, p := range req.PreferredProviders {
			preferred[p.SystemName] = true
		}
		filtered := results[:0]
		for _, r := range results {
			if preferred[r.ProviderName] {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}
	if flags.Matchmaking && len(results) > 1 {
		results = results[:1]
	}

	resp := orchmodel.OrchestrationResponse{Results: results}
	o.hist.add(newHistoryEntry(
		req.RequesterSystem.SystemName,
		req.RequestedService.ServiceDefinition,
		"DONE", "",
	))
	return resp, nil
}

// verifyIdentity calls the Authentication system to validate the token.
// Returns the verified systemName on success, or an error (fail-closed on network errors).
func (o *DynamicOrchestrator) verifyIdentity(token string) (string, error) {
	req, err := http.NewRequest(http.MethodGet,
		o.authSysURL+"/authentication/identity/verify/"+url.PathEscape(token), nil)
	if err != nil {
		return "", ErrIdentityInvalid
	}
	resp, err := o.httpClient.Do(req)
	if err != nil {
		// Auth system unreachable — fail-closed.
		return "", ErrIdentityInvalid
	}
	defer resp.Body.Close()
	var result authSysVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", ErrIdentityInvalid
	}
	if !result.Verified {
		return "", ErrIdentityInvalid
	}
	return result.SystemName, nil
}

func (o *DynamicOrchestrator) querySR(filter orchmodel.ServiceRequirement) (*srAH5LookupResponse, error) {
	body := srLookupRequest{
		ServiceDefinitionNames: []string{filter.ServiceDefinition},
	}
	data, _ := json.Marshal(body)
	resp, err := o.httpClient.Post(o.srURL+"/serviceregistry/service-discovery/lookup", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result srAH5LookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (o *DynamicOrchestrator) checkAuthorized(consumer, provider, target string) (bool, error) {
	body := caVerifyRequest{
		Consumer:   consumer,
		Provider:   provider,
		Target:     target,
		TargetType: "SERVICE_DEF",
	}
	data, _ := json.Marshal(body)
	resp, err := o.httpClient.Post(o.caURL+"/consumerauthorization/authorization/verify", "application/json", bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	// Verify now returns a plain JSON Boolean (true or false).
	var authorized bool
	if err := json.NewDecoder(resp.Body).Decode(&authorized); err != nil {
		return false, err
	}
	return authorized, nil
}
