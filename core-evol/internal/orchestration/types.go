// Package orchestration implements the XACML-aware DynamicOrchestration service.
//
// This is an AH5-evolved variant of the core DynamicOrchestration system that
// replaces per-provider ConsumerAuthorization.verify with a single AuthzForce
// XACML decision. See AH5_EVOL.md for the full rationale.
package orchestration

// System identifies a participating system (consumer or provider).
// Mirrors core/internal/orchestration/model.System — duplicated because
// core-evol must not import core/internal/.
type System struct {
	SystemName string `json:"systemName"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
}

// ServiceFilter describes what service a consumer is looking for.
type ServiceFilter struct {
	ServiceDefinition string            `json:"serviceDefinition"`
	Interfaces        []string          `json:"interfaces,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// OrchestrationRequest is the pull request from a consumer.
type OrchestrationRequest struct {
	RequesterSystem  System        `json:"requesterSystem"`
	RequestedService ServiceFilter `json:"requestedService"`
}

// ServiceInfo holds the discoverable details of a matched service.
type ServiceInfo struct {
	ServiceDefinition string            `json:"serviceDefinition"`
	ServiceUri        string            `json:"serviceUri"`
	Interfaces        []string          `json:"interfaces"`
	Version           int               `json:"version"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// OrchestrationResult is one matched provider + service pair.
type OrchestrationResult struct {
	Provider System      `json:"provider"`
	Service  ServiceInfo `json:"service"`
}

// OrchestrationResponse wraps the list of results.
type OrchestrationResponse struct {
	Response []OrchestrationResult `json:"response"`
}

// ServiceInstance is an internal representation of a service entry returned
// by the ServiceRegistry query.
type ServiceInstance struct {
	ServiceDefinition string
	Provider          System
	ServiceUri        string
	Interfaces        []string
	Version           int
	Metadata          map[string]string
}
