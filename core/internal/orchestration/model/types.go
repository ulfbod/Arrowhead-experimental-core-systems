// Package model defines shared types for all orchestration systems.
package model

// System identifies a participating system (consumer or provider).
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

// OrchestrationRequest is the pull request from a consumer to any orchestration system.
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
	// Priority is set by store-based orchestrators; 0 = not applicable.
	Priority int `json:"priority,omitempty"`
}

// OrchestrationResponse wraps the list of results.
type OrchestrationResponse struct {
	Response []OrchestrationResult `json:"response"`
}
