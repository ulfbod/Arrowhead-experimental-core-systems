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

var ErrMissingRequester = errors.New("requesterSystem.systemName is required")
var ErrMissingService   = errors.New("requestedService.serviceDefinition is required")

// srQueryRequest mirrors the ServiceRegistry query body.
type srQueryRequest struct {
	ServiceDefinition  string            `json:"serviceDefinition,omitempty"`
	Interfaces         []string          `json:"interfaces,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
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

// authVerifyRequest mirrors the ConsumerAuthorization verify body.
type authVerifyRequest struct {
	ConsumerSystemName string `json:"consumerSystemName"`
	ProviderSystemName string `json:"providerSystemName"`
	ServiceDefinition  string `json:"serviceDefinition"`
}

type authVerifyResponse struct {
	Authorized bool `json:"authorized"`
}

// DynamicOrchestrator performs real-time orchestration.
type DynamicOrchestrator struct {
	srURL      string
	authURL    string
	checkAuth  bool
	httpClient *http.Client
}

func NewDynamicOrchestrator(srURL, authURL string, checkAuth bool) *DynamicOrchestrator {
	return &DynamicOrchestrator{
		srURL:      srURL,
		authURL:    authURL,
		checkAuth:  checkAuth,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Orchestrate performs the pull operation: query SR, optionally check auth, return results.
func (o *DynamicOrchestrator) Orchestrate(req orchmodel.OrchestrationRequest) (orchmodel.OrchestrationResponse, error) {
	if req.RequesterSystem.SystemName == "" {
		return orchmodel.OrchestrationResponse{}, ErrMissingRequester
	}
	if req.RequestedService.ServiceDefinition == "" {
		return orchmodel.OrchestrationResponse{}, ErrMissingService
	}

	// Step 1: Query Service Registry.
	srResp, err := o.querySR(req.RequestedService)
	if err != nil {
		return orchmodel.OrchestrationResponse{}, fmt.Errorf("service registry unreachable: %w", err)
	}

	// Step 2: Filter by authorization (optional).
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
	body := authVerifyRequest{
		ConsumerSystemName: consumer,
		ProviderSystemName: provider,
		ServiceDefinition:  service,
	}
	data, _ := json.Marshal(body)
	resp, err := o.httpClient.Post(o.authURL+"/authorization/verify", "application/json", bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var result authVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.Authorized, nil
}
