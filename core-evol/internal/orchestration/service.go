package orchestration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	pb "arrowhead/core-evol/proto/authorize"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	// ErrMissingRequester is returned when requesterSystem.systemName is empty.
	ErrMissingRequester = errors.New("requesterSystem.systemName is required")
	// ErrMissingService is returned when requestedService.serviceDefinition is empty.
	ErrMissingService = errors.New("requestedService.serviceDefinition is required")
)

// AuthDecider abstracts the PDP access-control decision.
//
// Each field maps to a XACML attribute (see proto/authorize/authorize.proto):
//   - domainID  → XACML domain (AuthzForce UUID or external name; may be empty)
//   - subject   → XACML subject-id (consumer system name)
//   - service   → XACML resource-id (service definition)
//   - provider  → XACML urn:arrowhead:attribute:provider-id (provider system name; may be empty)
//   - action    → XACML action-id ("orchestrate" for orchestration, "consume" for enforcement)
//
// Implementations:
//   - *GRPCDecider   — calls authz-pdp over gRPC (authorize.proto)
//   - *CADecider     — calls AH5 ConsumerAuthorization over HTTP
type AuthDecider interface {
	Decide(domainID, subject, service, provider, action string) (bool, error)
}

// RegistryQuerier abstracts the ServiceRegistry query.
type RegistryQuerier interface {
	QuerySR(filter ServiceFilter) ([]ServiceInstance, error)
}

// XACMLOrchestrator performs AH5-evolved orchestration.
//
// Per-provider decision semantics (action = "orchestrate"):
//   - Permit      → include provider in result
//   - Deny        → exclude provider (fail-closed)
//   - error       → exclude provider (fail-closed; treat as Deny)
//   - enabled=false → bypass AuthDecider, return all SR results (passthrough mode)
type XACMLOrchestrator struct {
	sr       RegistryQuerier
	decider  AuthDecider
	domainID string
	enabled  bool
}

// NewXACMLOrchestrator creates a new XACMLOrchestrator.
//
//   - sr:       ServiceRegistry client
//   - decider:  AuthDecider implementation (GRPCDecider or CADecider)
//   - domainID: policy domain identifier passed to AuthDecider
//   - enabled:  when false, authorization check is skipped (ENABLE_AUTH=false)
func NewXACMLOrchestrator(sr RegistryQuerier, decider AuthDecider, domainID string, enabled bool) *XACMLOrchestrator {
	return &XACMLOrchestrator{sr: sr, decider: decider, domainID: domainID, enabled: enabled}
}

// Orchestrate processes an orchestration request:
//  1. Validate requesterSystem.systemName and requestedService.serviceDefinition.
//  2. Query ServiceRegistry for all providers of the requested service.
//  3. If enabled: for each provider call AuthDecider.Decide with
//     action="orchestrate", service and provider as separate fields.
//     Include only Permit providers; skip on Deny or error (fail-closed).
//  4. If disabled: return all providers without authorization check.
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

	// Per-provider decision: action="orchestrate", service and provider as separate fields.
	// Fail-closed: error or Deny → exclude provider from results.
	results := make([]OrchestrationResult, 0, len(instances))
	for _, inst := range instances {
		permitted, decErr := o.decider.Decide(
			o.domainID,
			req.RequesterSystem.SystemName,
			inst.ServiceDefinition,
			inst.Provider.SystemName,
			"orchestrate",
		)
		if decErr != nil || !permitted {
			continue
		}
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
	return OrchestrationResponse{Response: results}, nil
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

// --- GRPCDecider — calls authz-pdp over gRPC (authorize.proto) ---

// GRPCDecider implements AuthDecider by calling the authz-pdp gRPC service.
// It is the primary AuthDecider in experiment-12.
type GRPCDecider struct {
	client pb.AuthorizationPDPClient
}

// NewGRPCDecider dials the authz-pdp gRPC server at addr and returns a
// GRPCDecider. The caller owns the connection lifecycle.
func NewGRPCDecider(addr string) (*GRPCDecider, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dial authz-pdp %s: %w", addr, err)
	}
	return &GRPCDecider{client: pb.NewAuthorizationPDPClient(conn)}, conn, nil
}

// Decide sends a DecisionRequest to authz-pdp. Returns true iff the response
// Decision is PERMIT. Any gRPC error or non-PERMIT decision → false (fail-closed).
func (g *GRPCDecider) Decide(domainID, subject, service, provider, action string) (bool, error) {
	resp, err := g.client.Decide(context.Background(), &pb.DecisionRequest{
		DomainId: domainID,
		Subject:  subject,
		Service:  service,
		Provider: provider,
		Action:   action,
	})
	if err != nil {
		return false, err
	}
	return resp.Decision == pb.Decision_PERMIT, nil
}

// --- CADecider — calls AH5 ConsumerAuthorization over HTTP ---

// CADecider implements AuthDecider by calling the AH5 ConsumerAuthorization
// service. It is the fallback when AUTH_BACKEND=consumerauth.
//
// Mapping: subject→consumerSystemName, provider→providerSystemName, service→serviceDefinition.
// domainID and action are ignored (CA has no domain/action concept).
type CADecider struct {
	baseURL string
	http    *http.Client
}

// NewCADecider returns a CADecider backed by the ConsumerAuth service at baseURL.
func NewCADecider(baseURL string) *CADecider {
	return &CADecider{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

type caVerifyRequest struct {
	ConsumerSystemName string `json:"consumerSystemName"`
	ProviderSystemName string `json:"providerSystemName"`
	ServiceDefinition  string `json:"serviceDefinition"`
}

type caVerifyResponse struct {
	Authorized bool `json:"authorized"`
}

// Decide calls ConsumerAuthorization.verify(consumer, provider, service).
// domainID and action are ignored — CA is a flat boolean grant store.
func (c *CADecider) Decide(_, subject, service, provider, _ string) (bool, error) {
	body := caVerifyRequest{
		ConsumerSystemName: subject,
		ProviderSystemName: provider,
		ServiceDefinition:  service,
	}
	data, _ := json.Marshal(body)
	resp, err := c.http.Post(c.baseURL+"/consumerauth/verify", "application/json", bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("consumerauth verify: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("consumerauth verify returned %d", resp.StatusCode)
	}
	var result caVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("consumerauth decode: %w", err)
	}
	return result.Authorized, nil
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
