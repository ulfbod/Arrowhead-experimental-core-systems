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

// srQueryRequest mirrors the ServiceRegistry query body.
type srQueryRequest struct {
	ServiceDefinition string            `json:"serviceDefinition,omitempty"`
	Interfaces        []string          `json:"interfaces,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type srSystem struct {
	SystemName string `json:"systemName"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
}

type srServiceInstance struct {
	ServiceDefinition string            `json:"serviceDefinition"`
	ProviderSystem    srSystem          `json:"providerSystem"`
	ServiceUri        string            `json:"serviceUri"`
	Interfaces        []string          `json:"interfaces"`
	Version           int               `json:"version"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type srQueryResponse struct {
	ServiceQueryData []srServiceInstance `json:"serviceQueryData"`
	UnfilteredHits   int                 `json:"unfilteredHits"`
}

// caVerifyRequest mirrors the ConsumerAuthorization verify body.
type caVerifyRequest struct {
	ConsumerSystemName string `json:"consumerSystemName"`
	ProviderSystemName string `json:"providerSystemName"`
	ServiceDefinition  string `json:"serviceDefinition"`
}

type caVerifyResponse struct {
	Authorized bool `json:"authorized"`
}

// authSysVerifyResponse mirrors the Authentication identity/verify response.
type authSysVerifyResponse struct {
	Valid       bool   `json:"valid"`
	SystemName  string `json:"systemName"`
}

// DynamicOrchestrator performs real-time orchestration.
type DynamicOrchestrator struct {
	srURL         string
	caURL         string // ConsumerAuthorization base URL
	authSysURL    string // Authentication system base URL
	checkAuth     bool
	checkIdentity bool
	httpClient    *http.Client
}

// NewDynamicOrchestrator creates a new orchestrator.
//
//   - srURL:         ServiceRegistry base URL
//   - caURL:         ConsumerAuthorization base URL (used when checkAuth=true)
//   - authSysURL:    Authentication system base URL (used when checkIdentity=true)
//   - checkAuth:     when true, filters providers through ConsumerAuthorization
//   - checkIdentity: when true, requires a valid Bearer token and uses the verified
//     systemName from the token instead of the self-reported requesterSystem.systemName
func NewDynamicOrchestrator(srURL, caURL, authSysURL string, checkAuth, checkIdentity bool) *DynamicOrchestrator {
	return &DynamicOrchestrator{
		srURL:         srURL,
		caURL:         caURL,
		authSysURL:    authSysURL,
		checkAuth:     checkAuth,
		checkIdentity: checkIdentity,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}
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

	// Step 3: Query Service Registry.
	srResp, err := o.querySR(req.RequestedService)
	if err != nil {
		return orchmodel.OrchestrationResponse{}, fmt.Errorf("service registry unreachable: %w", err)
	}

	// Step 4: Filter by ConsumerAuthorization (optional).
	var results []orchmodel.OrchestrationResult
	for _, svc := range srResp.ServiceQueryData {
		if o.checkAuth {
			ok, err := o.checkAuthorized(req.RequesterSystem.SystemName, svc.ProviderSystem.SystemName, svc.ServiceDefinition)
			if err != nil || !ok {
				continue
			}
		}
		results = append(results, orchmodel.OrchestrationResult{
			Provider: orchmodel.System{
				SystemName: svc.ProviderSystem.SystemName,
				Address:    svc.ProviderSystem.Address,
				Port:       svc.ProviderSystem.Port,
			},
			Service: orchmodel.ServiceInfo{
				ServiceDefinition: svc.ServiceDefinition,
				ServiceUri:        svc.ServiceUri,
				Interfaces:        svc.Interfaces,
				Version:           svc.Version,
				Metadata:          svc.Metadata,
			},
		})
	}
	if results == nil {
		results = []orchmodel.OrchestrationResult{}
	}
	return orchmodel.OrchestrationResponse{Response: results}, nil
}

// verifyIdentity calls the Authentication system to validate the token.
// Returns the verified systemName on success, or an error (fail-closed on network errors).
func (o *DynamicOrchestrator) verifyIdentity(token string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, o.authSysURL+"/authentication/identity/verify", nil)
	if err != nil {
		return "", ErrIdentityInvalid
	}
	req.Header.Set("Authorization", "Bearer "+token)
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
	if !result.Valid {
		return "", ErrIdentityInvalid
	}
	return result.SystemName, nil
}

func (o *DynamicOrchestrator) querySR(filter orchmodel.ServiceFilter) (*srQueryResponse, error) {
	body := srQueryRequest{
		ServiceDefinition: filter.ServiceDefinition,
		Interfaces:        filter.Interfaces,
		Metadata:          filter.Metadata,
	}
	data, _ := json.Marshal(body)
	resp, err := o.httpClient.Post(o.srURL+"/serviceregistry/query", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result srQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (o *DynamicOrchestrator) checkAuthorized(consumer, provider, service string) (bool, error) {
	body := caVerifyRequest{
		ConsumerSystemName: consumer,
		ProviderSystemName: provider,
		ServiceDefinition:  service,
	}
	data, _ := json.Marshal(body)
	resp, err := o.httpClient.Post(o.caURL+"/authorization/verify", "application/json", bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var result caVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.Authorized, nil
}
