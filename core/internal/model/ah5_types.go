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

import "encoding/json"

// ─── Metadata operators ───────────────────────────────────────────────────────

// MetadataOp names the comparison operator for a MetadataRequirement.
type MetadataOp string

// Supported metadata operators (AH5 spec, G16).
const (
	OpEqualsTo              MetadataOp = "EQUALS_TO"
	OpNotEqualsTo           MetadataOp = "NOT_EQUALS_TO"
	OpLessThanOrEqualsTo    MetadataOp = "LESS_THAN_OR_EQUALS_TO"
	OpGreaterThanOrEqualsTo MetadataOp = "GREATER_THAN_OR_EQUALS_TO"
	OpContains              MetadataOp = "CONTAINS"
	OpNotContains           MetadataOp = "NOT_CONTAINS"
)

// MetadataRequirement is a single metadata filter predicate.
// Two wire forms are accepted:
//
//	structured: {"op":"CONTAINS","value":"world"}
//	shorthand:  "prod"   (treated as EQUALS_TO "prod")
//	            true     (treated as EQUALS_TO true)
type MetadataRequirement struct {
	Op    MetadataOp  `json:"op,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

type metadataRequirementWire struct {
	Op    MetadataOp  `json:"op"`
	Value interface{} `json:"value"`
}

// UnmarshalJSON handles both the structured object form and the bare shorthand form.
func (m *MetadataRequirement) UnmarshalJSON(data []byte) error {
	var wire metadataRequirementWire
	if err := json.Unmarshal(data, &wire); err == nil && wire.Op != "" {
		m.Op = wire.Op
		m.Value = wire.Value
		return nil
	}
	// Shorthand: bare string, number, or bool → implicit EQUALS_TO
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	m.Op = OpEqualsTo
	m.Value = v
	return nil
}

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

// SecurityPolicy enumerates the allowed values for InterfaceInstance.Policy.
type SecurityPolicy string

const (
	PolicyNone                  SecurityPolicy = "NONE"
	PolicyCertAuth              SecurityPolicy = "CERT_AUTH"
	PolicyTimeLimitedTokenAuth  SecurityPolicy = "TIME_LIMITED_TOKEN_AUTH"
	PolicyUsageLimitedTokenAuth SecurityPolicy = "USAGE_LIMITED_TOKEN_AUTH"
	PolicyBase64TokenAuth       SecurityPolicy = "BASE64_SELF_CONTAINED_TOKEN_AUTH"
	PolicyJwtSha256Auth         SecurityPolicy = "RSA_SHA256_JSON_WEB_TOKEN_AUTH"
	PolicyJwtSha512Auth         SecurityPolicy = "RSA_SHA512_JSON_WEB_TOKEN_AUTH"
)

// validSecurityPolicies is the set of accepted SecurityPolicy values.
var validSecurityPolicies = map[string]bool{
	string(PolicyNone):                  true,
	string(PolicyCertAuth):              true,
	string(PolicyTimeLimitedTokenAuth):  true,
	string(PolicyUsageLimitedTokenAuth): true,
	string(PolicyBase64TokenAuth):       true,
	string(PolicyJwtSha256Auth):         true,
	string(PolicyJwtSha512Auth):         true,
}

// IsValidSecurityPolicy returns true if the value is a known SecurityPolicy.
func IsValidSecurityPolicy(s string) bool {
	return validSecurityPolicies[s]
}

// InterfaceInstance is a concrete interface binding on a service instance.
// It accepts two JSON wire formats:
//
//	Structured: {"templateName":"http-json","protocol":"http","policy":"NONE","properties":{...}}
//	Flat string: "HTTP-INSECURE-JSON"  → treated as {templateName, protocol:"http", policy:"NONE"}
type InterfaceInstance struct {
	TemplateName string            `json:"templateName"`
	Protocol     string            `json:"protocol,omitempty"`
	Policy       string            `json:"policy,omitempty"`
	Properties   map[string]string `json:"properties,omitempty"`
}

type interfaceInstanceWire struct {
	TemplateName string            `json:"templateName"`
	Protocol     string            `json:"protocol"`
	Policy       string            `json:"policy"`
	Properties   map[string]string `json:"properties"`
}

// UnmarshalJSON handles both the structured object form and the bare flat-string form.
func (i *InterfaceInstance) UnmarshalJSON(data []byte) error {
	// Try structured object first.
	var wire interfaceInstanceWire
	if err := json.Unmarshal(data, &wire); err == nil && len(data) > 0 && data[0] == '{' {
		i.TemplateName = wire.TemplateName
		i.Protocol = wire.Protocol
		i.Policy = wire.Policy
		i.Properties = wire.Properties
		return nil
	}
	// Try bare string (flat backward-compat format).
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	i.TemplateName = s
	i.Protocol = "http"
	i.Policy = string(PolicyNone)
	return nil
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
	DeviceNames          []string                       `json:"deviceNames,omitempty"`
	Addresses            []string                       `json:"addresses,omitempty"`
	AddressType          string                         `json:"addressType,omitempty"`
	MetadataRequirements map[string]MetadataRequirement `json:"metadataRequirements,omitempty"`
}

// DeviceLookupResponse is returned by device discovery lookup.
type DeviceLookupResponse struct {
	Entries    []*Device `json:"entries"`
	Count      int       `json:"count"`
	TotalCount int       `json:"totalCount"`
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
	SystemNames          []string                       `json:"systemNames,omitempty"`
	Addresses            []string                       `json:"addresses,omitempty"`
	AddressType          string                         `json:"addressType,omitempty"`
	Versions             []string                       `json:"versions,omitempty"`
	DeviceNames          []string                       `json:"deviceNames,omitempty"`
	MetadataRequirements map[string]MetadataRequirement `json:"metadataRequirements,omitempty"`
}

// SystemLookupResponse is returned by system discovery lookup.
type SystemLookupResponse struct {
	Entries    []*AH5System `json:"entries"`
	Count      int          `json:"count"`
	TotalCount int          `json:"totalCount"`
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
	InstanceIDs            []string                       `json:"instanceIds,omitempty"`
	ProviderNames          []string                       `json:"providerNames,omitempty"`
	ServiceDefinitionNames []string                       `json:"serviceDefinitionNames,omitempty"`
	Versions               []string                       `json:"versions,omitempty"`
	InterfaceTemplateNames []string                       `json:"interfaceTemplateNames,omitempty"`
	AlivesAt               string                         `json:"alivesAt,omitempty"`
	MetadataRequirements   map[string]MetadataRequirement `json:"metadataRequirements,omitempty"`
}

// ServiceLookupResponse is returned by service discovery lookup.
type ServiceLookupResponse struct {
	Entries    []*AH5ServiceInstance `json:"entries"`
	Count      int                   `json:"count"`
	TotalCount int                   `json:"totalCount"`
}

// ─── Management — Devices ────────────────────────────────────────────────────

// DeviceListRequest is the body for POST and PUT
// /serviceregistry/mgmt/devices.
type DeviceListRequest struct {
	Devices []*DeviceRegistrationRequest `json:"devices"`
}

// DeviceListResponse is returned by device management endpoints.
type DeviceListResponse struct {
	Devices    []*Device `json:"devices"`
	Count      int       `json:"count"`
	TotalCount int       `json:"totalCount"`
}

// ─── Management — Systems ────────────────────────────────────────────────────

// SystemListRequest is the body for POST and PUT
// /serviceregistry/mgmt/systems.
type SystemListRequest struct {
	Systems []*SystemRegistrationRequest `json:"systems"`
}

// SystemListResponse is returned by system management endpoints.
type SystemListResponse struct {
	Systems    []*AH5System `json:"systems"`
	Count      int          `json:"count"`
	TotalCount int          `json:"totalCount"`
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
	TotalCount         int                  `json:"totalCount"`
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
	Instances  []*AH5ServiceInstance `json:"instances"`
	Count      int                   `json:"count"`
	TotalCount int                   `json:"totalCount"`
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
	TotalCount         int                  `json:"totalCount"`
}
