// Package model — AH5 extended types for the Arrowhead 5 ServiceRegistry.
//
// These types implement the AH5 discovery and management surfaces:
//   - Device Discovery  (/serviceregistry/device-discovery/*)
//   - System Discovery  (/serviceregistry/system-discovery/*)
//   - Service Discovery (/serviceregistry/service-discovery/*)
//   - Registry Mgmt     (/serviceregistry/mgmt/*)
//
// They are intentionally separate from the legacy types in types.go so that
// the existing AH4-compatible endpoints are unaffected.
//
// DO NOT MODIFY FOR EXPERIMENTS.
package model

// Address is a network endpoint descriptor used in AH5 system and device models.
type Address struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

// Device is an AH5 device entity.
type Device struct {
	Name      string            `json:"name"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Addresses []Address         `json:"addresses,omitempty"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

// AH5System is an AH5 system entity.
// It is distinct from the legacy System type, which uses address+port rather
// than a structured address list.
type AH5System struct {
	Name      string            `json:"name"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Version   string            `json:"version,omitempty"`
	Addresses []Address         `json:"addresses,omitempty"`
	Device    *Device           `json:"device,omitempty"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

// ServiceDefinition is a named service definition stored in the registry.
type ServiceDefinition struct {
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// InterfaceTemplate defines an interface contract that service instances may use.
type InterfaceTemplate struct {
	Name                 string            `json:"name"`
	Protocol             string            `json:"protocol"`
	PropertyRequirements map[string]string `json:"propertyRequirements,omitempty"`
	CreatedAt            string            `json:"createdAt"`
	UpdatedAt            string            `json:"updatedAt"`
}

// InterfaceInstance is a concrete interface binding on a service instance.
type InterfaceInstance struct {
	TemplateName string            `json:"templateName"`
	Protocol     string            `json:"protocol,omitempty"`
	Policy       string            `json:"policy,omitempty"`
	Properties   map[string]string `json:"properties,omitempty"`
}

// AH5ServiceInstance is an AH5-style service instance (as opposed to the
// legacy ServiceInstance which uses numeric IDs and a flat address+port model).
type AH5ServiceInstance struct {
	InstanceID            string              `json:"instanceId"`
	Provider              *AH5System          `json:"provider,omitempty"`
	ServiceDefinitionName string              `json:"serviceDefinitionName"`
	Version               string              `json:"version,omitempty"`
	ExpiresAt             string              `json:"expiresAt,omitempty"`
	Metadata              map[string]string   `json:"metadata,omitempty"`
	Interfaces            []InterfaceInstance `json:"interfaces,omitempty"`
	CreatedAt             string              `json:"createdAt"`
	UpdatedAt             string              `json:"updatedAt"`
}

// ─── Device Discovery ────────────────────────────────────────────────────────

// DeviceRegistrationRequest is the body for
// POST /serviceregistry/device-discovery/register.
type DeviceRegistrationRequest struct {
	Name      string            `json:"name"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Addresses []Address         `json:"addresses,omitempty"`
}

// DeviceLookupRequest is the body for
// POST /serviceregistry/device-discovery/lookup.
type DeviceLookupRequest struct {
	DeviceNames []string `json:"deviceNames,omitempty"`
	Addresses   []string `json:"addresses,omitempty"`
	AddressType string   `json:"addressType,omitempty"`
}

// DeviceLookupResponse is returned by device discovery lookup.
type DeviceLookupResponse struct {
	Entries []*Device `json:"entries"`
	Count   int       `json:"count"`
}

// ─── System Discovery ────────────────────────────────────────────────────────

// SystemRegistrationRequest is the body for
// POST /serviceregistry/system-discovery/register.
//
// Note (G10): The AH5 spec derives the system name from the caller's auth token.
// This implementation requires it explicitly in the request body because credential
// verification is not implemented (see G2).
type SystemRegistrationRequest struct {
	Name       string            `json:"name"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Version    string            `json:"version,omitempty"`
	Addresses  []Address         `json:"addresses,omitempty"`
	DeviceName string            `json:"deviceName,omitempty"`
}

// SystemLookupRequest is the body for
// POST /serviceregistry/system-discovery/lookup.
type SystemLookupRequest struct {
	SystemNames []string `json:"systemNames,omitempty"`
	Addresses   []string `json:"addresses,omitempty"`
	AddressType string   `json:"addressType,omitempty"`
	Versions    []string `json:"versions,omitempty"`
	DeviceNames []string `json:"deviceNames,omitempty"`
}

// SystemLookupResponse is returned by system discovery lookup.
type SystemLookupResponse struct {
	Entries []*AH5System `json:"entries"`
	Count   int          `json:"count"`
}

// ─── Service Discovery ───────────────────────────────────────────────────────

// ServiceRegistrationRequest is the body for
// POST /serviceregistry/service-discovery/register.
//
// Note (G10): The AH5 spec derives the provider system name from the caller's auth
// token. This implementation requires it explicitly in the request body.
type ServiceRegistrationRequest struct {
	SystemName            string              `json:"systemName"`
	ServiceDefinitionName string              `json:"serviceDefinitionName"`
	Version               string              `json:"version,omitempty"`
	ExpiresAt             string              `json:"expiresAt,omitempty"`
	Metadata              map[string]string   `json:"metadata,omitempty"`
	Interfaces            []InterfaceInstance `json:"interfaces,omitempty"`
}

// ServiceLookupRequest is the body for
// POST /serviceregistry/service-discovery/lookup.
type ServiceLookupRequest struct {
	InstanceIDs            []string `json:"instanceIds,omitempty"`
	ProviderNames          []string `json:"providerNames,omitempty"`
	ServiceDefinitionNames []string `json:"serviceDefinitionNames,omitempty"`
	Versions               []string `json:"versions,omitempty"`
	InterfaceTemplateNames []string `json:"interfaceTemplateNames,omitempty"`
}

// ServiceLookupResponse is returned by service discovery lookup.
type ServiceLookupResponse struct {
	Entries []*AH5ServiceInstance `json:"entries"`
	Count   int                   `json:"count"`
}

// ─── Management — Devices ────────────────────────────────────────────────────

// DeviceListRequest is the body for POST and PUT
// /serviceregistry/mgmt/devices.
type DeviceListRequest struct {
	Devices []*DeviceRegistrationRequest `json:"devices"`
}

// DeviceListResponse is returned by device management endpoints.
type DeviceListResponse struct {
	Devices []*Device `json:"devices"`
	Count   int       `json:"count"`
}

// ─── Management — Systems ────────────────────────────────────────────────────

// SystemListRequest is the body for POST and PUT
// /serviceregistry/mgmt/systems.
type SystemListRequest struct {
	Systems []*SystemRegistrationRequest `json:"systems"`
}

// SystemListResponse is returned by system management endpoints.
type SystemListResponse struct {
	Systems []*AH5System `json:"systems"`
	Count   int          `json:"count"`
}

// ─── Management — Service Definitions ───────────────────────────────────────

// ServiceDefinitionListRequest is the body for POST
// /serviceregistry/mgmt/service-definitions.
type ServiceDefinitionListRequest struct {
	ServiceDefinitionNames []string `json:"serviceDefinitionNames"`
}

// ServiceDefinitionListResponse is returned by service definition management.
type ServiceDefinitionListResponse struct {
	ServiceDefinitions []*ServiceDefinition `json:"serviceDefinitions"`
	Count              int                  `json:"count"`
}

// ─── Management — Service Instances ─────────────────────────────────────────

// ServiceCreateRequest describes a single service instance to create.
type ServiceCreateRequest struct {
	SystemName            string              `json:"systemName"`
	ServiceDefinitionName string              `json:"serviceDefinitionName"`
	Version               string              `json:"version,omitempty"`
	ExpiresAt             string              `json:"expiresAt,omitempty"`
	Metadata              map[string]string   `json:"metadata,omitempty"`
	Interfaces            []InterfaceInstance `json:"interfaces,omitempty"`
}

// ServiceCreateListRequest is the body for POST
// /serviceregistry/mgmt/service-instances.
type ServiceCreateListRequest struct {
	Instances []*ServiceCreateRequest `json:"instances"`
}

// ServiceUpdateRequest describes an update to an existing service instance.
type ServiceUpdateRequest struct {
	InstanceID string              `json:"instanceId"`
	ExpiresAt  string              `json:"expiresAt,omitempty"`
	Metadata   map[string]string   `json:"metadata,omitempty"`
	Interfaces []InterfaceInstance `json:"interfaces,omitempty"`
}

// ServiceUpdateListRequest is the body for PUT
// /serviceregistry/mgmt/service-instances.
type ServiceUpdateListRequest struct {
	Instances []*ServiceUpdateRequest `json:"instances"`
}

// ServiceListResponse is returned by service instance management endpoints.
type ServiceListResponse struct {
	Instances []*AH5ServiceInstance `json:"instances"`
	Count     int                   `json:"count"`
}

// ─── Management — Interface Templates ───────────────────────────────────────

// InterfaceTemplateListRequest is the body for POST
// /serviceregistry/mgmt/interface-templates.
type InterfaceTemplateListRequest struct {
	InterfaceTemplates []*InterfaceTemplate `json:"interfaceTemplates"`
}

// InterfaceTemplateListResponse is returned by interface template management.
type InterfaceTemplateListResponse struct {
	InterfaceTemplates []*InterfaceTemplate `json:"interfaceTemplates"`
	Count              int                  `json:"count"`
}
