// Package model defines the data types for the Arrowhead Core Service Registry.
//
// DO NOT MODIFY FOR EXPERIMENTS.
// All fields are defined by SPEC.md. Do not add, remove, or rename fields
// without a corresponding SPEC.md update.
package model

// System represents a service provider or consumer.
// Uniqueness key: (SystemName, Address, Port).
type System struct {
	SystemName         string `json:"systemName"`
	Address            string `json:"address"`
	Port               int    `json:"port"`
	AuthenticationInfo string `json:"authenticationInfo,omitempty"`
}

// ServiceInstance is a registered service entry in the registry.
type ServiceInstance struct {
	ID                int64             `json:"id"`
	ServiceDefinition string            `json:"serviceDefinition"`
	ProviderSystem    System            `json:"providerSystem"`
	ServiceUri        string            `json:"serviceUri"`
	Interfaces        []string          `json:"interfaces"`
	Version           int               `json:"version"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Secure            string            `json:"secure,omitempty"`
}

// RegisterRequest is the body for POST /serviceregistry/register.
type RegisterRequest struct {
	ServiceDefinition string            `json:"serviceDefinition"`
	ProviderSystem    *System           `json:"providerSystem"`
	ServiceUri        string            `json:"serviceUri"`
	Interfaces        []string          `json:"interfaces"`
	Version           int               `json:"version"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Secure            string            `json:"secure,omitempty"`
}

// UnregisterRequest is the body for DELETE /serviceregistry/unregister.
// Identifies a service instance by its natural key.
type UnregisterRequest struct {
	ServiceDefinition string  `json:"serviceDefinition"`
	ProviderSystem    *System `json:"providerSystem"`
	Version           int     `json:"version"`
}

// QueryRequest is the body for POST /serviceregistry/query.
// All non-zero fields are ANDed together as filters.
type QueryRequest struct {
	ServiceDefinition  string            `json:"serviceDefinition,omitempty"`
	Interfaces         []string          `json:"interfaces,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	VersionRequirement int               `json:"versionRequirement,omitempty"`
}

// QueryResponse is returned by POST /serviceregistry/query and GET /serviceregistry/lookup.
type QueryResponse struct {
	ServiceQueryData []*ServiceInstance `json:"serviceQueryData"`
	UnfilteredHits   int                `json:"unfilteredHits"`
}
