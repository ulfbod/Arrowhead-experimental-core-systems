package orchestration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	// ErrMissingRequester is returned when requesterSystem.systemName is empty.
	ErrMissingRequester = errors.New("requesterSystem.systemName is required")
	// ErrMissingService is returned when requestedService.serviceDefinition is empty.
	ErrMissingService = errors.New("requestedService.serviceDefinition is required")
)

// AuthDecider abstracts the XACML PDP access-control decision.
// Implemented by *authzforce.Client (support/authzforce).
type AuthDecider interface {
	Decide(domainID, subject, resource, action string) (bool, error)
}

// RegistryQuerier abstracts the ServiceRegistry query.
type RegistryQuerier interface {
	QuerySR(filter ServiceFilter) ([]ServiceInstance, error)
}

// XACMLOrchestrator performs AH5-evolved orchestration using a single XACML
// decision instead of per-provider ConsumerAuthorization checks.
//
// Decision semantics:
//   - XACML Permit → return all SR results for the requested service
//   - XACML Deny   → return empty list (consumer not authorised for this service)
//   - XACML error  → fail-closed: return empty list (treat as Deny)
//   - enabled=false → bypass XACML, return all SR results (passthrough mode)
type XACMLOrchestrator struct {
	sr       RegistryQuerier
	af       AuthDecider
	domainID string
	enabled  bool
}

// NewXACMLOrchestrator creates a new XACMLOrchestrator.
//
//   - sr:       ServiceRegistry client
//   - af:       AuthzForce PDP client
//   - domainID: XACML domain to query
//   - enabled:  when false, XACML check is skipped (ENABLE_AUTH=false)
func NewXACMLOrchestrator(sr RegistryQuerier, af AuthDecider, domainID string, enabled bool) *XACMLOrchestrator {
	return &XACMLOrchestrator{sr: sr, af: af, domainID: domainID, enabled: enabled}
}

// Orchestrate processes an orchestration request:
//  1. Validate requesterSystem.systemName and requestedService.serviceDefinition.
//  2. Query ServiceRegistry for providers of the requested service.
//  3. If enabled: call AuthzForce.Decide(domainID, consumer, service, "consume").
//     Permit → return all providers; Deny or error → return empty (fail-closed).
//  4. If disabled: return all providers.
func (o *XACMLOrchestrator) Orchestrate(req OrchestrationRequest) (OrchestrationResponse, error) {
	if req.RequesterSystem.SystemName == "" {
		return OrchestrationResponse{}, ErrMissingRequester
	}
	if req.RequestedService.ServiceDefinition == "" {
		return OrchestrationResponse{}, ErrMissingService
	}

	instances, err := o.sr.QuerySR(req.RequestedService)
	if err != nil {
		return OrchestrationResponse{}, fmt.Errorf("service registry: %w", err)
	}

	if !o.enabled {
		return buildResponse(instances), nil
	}

	permitted, err := o.af.Decide(o.domainID, req.RequesterSystem.SystemName, req.RequestedService.ServiceDefinition, "consume")
	if err != nil || !permitted {
		return OrchestrationResponse{Response: []OrchestrationResult{}}, nil
	}

	return buildResponse(instances), nil
}

func buildResponse(instances []ServiceInstance) OrchestrationResponse {
	results := make([]OrchestrationResult, 0, len(instances))
	for _, inst := range instances {
		results = append(results, OrchestrationResult{
			Provider: inst.Provider,
			Service: ServiceInfo{
				ServiceDefinition: inst.ServiceDefinition,
				ServiceUri:        inst.ServiceUri,
				Interfaces:        inst.Interfaces,
				Version:           inst.Version,
				Metadata:          inst.Metadata,
			},
		})
	}
	return OrchestrationResponse{Response: results}
}

// --- ServiceRegistry HTTP client ---

// srClient implements RegistryQuerier against a live ServiceRegistry.
type srClient struct {
	baseURL string
	http    *http.Client
}

// NewSRClient returns a RegistryQuerier backed by a real ServiceRegistry.
func NewSRClient(baseURL string) RegistryQuerier {
	return &srClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// internal SR wire types
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

func (c *srClient) QuerySR(filter ServiceFilter) ([]ServiceInstance, error) {
	body := srQueryRequest{
		ServiceDefinition: filter.ServiceDefinition,
		Interfaces:        filter.Interfaces,
		Metadata:          filter.Metadata,
	}
	data, _ := json.Marshal(body)
	resp, err := c.http.Post(c.baseURL+"/serviceregistry/query", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result srQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	instances := make([]ServiceInstance, 0, len(result.ServiceQueryData))
	for _, svc := range result.ServiceQueryData {
		instances = append(instances, ServiceInstance{
			ServiceDefinition: svc.ServiceDefinition,
			Provider: System{
				SystemName: svc.ProviderSystem.SystemName,
				Address:    svc.ProviderSystem.Address,
				Port:       svc.ProviderSystem.Port,
			},
			ServiceUri: svc.ServiceUri,
			Interfaces: svc.Interfaces,
			Version:    svc.Version,
			Metadata:   svc.Metadata,
		})
	}
	return instances, nil
}
