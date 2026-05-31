// Package model defines shared types for all orchestration systems.
package model

import "encoding/json"

// System identifies a participating system (consumer or provider).
type System struct {
	SystemName string `json:"systemName"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
}

// ServiceRequirement describes what service a consumer is looking for.
// AH5 request field name: serviceRequirement.
type ServiceRequirement struct {
	ServiceDefinition string            `json:"serviceDefinition"`
	Interfaces        []string          `json:"interfaces,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// ServiceFilter is a backward-compatible alias for ServiceRequirement.
type ServiceFilter = ServiceRequirement

// OrchestrationFlags controls optional orchestration behaviours (AH5 §G25).
// MATCHMAKING and ONLY_PREFERRED are implemented; others are accepted but ignored.
type OrchestrationFlags struct {
	Matchmaking      bool `json:"MATCHMAKING"`
	OnlyPreferred    bool `json:"ONLY_PREFERRED"`
	AllowTranslation bool `json:"ALLOW_TRANSLATION"` // stub
	OnlyExclusive    bool `json:"ONLY_EXCLUSIVE"`    // stub
	AllowIntercloud  bool `json:"ALLOW_INTERCLOUD"`  // stub
	OnlyIntercloud   bool `json:"ONLY_INTERCLOUD"`   // stub
}

// QoSRequirement specifies quality thresholds for a provider.
// Note on MaxBandwidthBps: despite the "max" prefix (per AH5 spec naming), this field
// is a *minimum* threshold — providers must have BandwidthBps >= MaxBandwidthBps to be included.
type QoSRequirement struct {
	MaxLatencyMs    int64   `json:"maxLatencyMs"`
	MaxBandwidthBps int64   `json:"maxBandwidthBps"` // minimum acceptable bandwidth (bytes/sec)
	MaxJitterMs     int64   `json:"maxJitterMs"`     // maximum acceptable jitter (ms)
	MaxPacketLoss   float64 `json:"maxPacketLoss"`   // maximum acceptable packet loss (%)
}

// OrchestrationRequest is the pull request from a consumer to any orchestration system.
// The JSON field for RequestedService is "serviceRequirement" (AH5 spec); for backward
// compatibility "requestedService" is also accepted on decode.
type OrchestrationRequest struct {
	RequesterSystem     System             `json:"requesterSystem"`
	RequestedService    ServiceRequirement `json:"-"`
	OrchestrationFlags  OrchestrationFlags `json:"orchestrationFlags,omitempty"`
	PreferredProviders  []System           `json:"preferredProviders,omitempty"`
	QualityRequirements []QoSRequirement   `json:"qualityRequirements,omitempty"`
}

type orchestrationRequestWire struct {
	RequesterSystem     System              `json:"requesterSystem"`
	ServiceRequirement  *ServiceRequirement `json:"serviceRequirement"`
	RequestedService    *ServiceRequirement `json:"requestedService"`
	OrchestrationFlags  OrchestrationFlags  `json:"orchestrationFlags"`
	PreferredProviders  []System            `json:"preferredProviders"`
	QualityRequirements []QoSRequirement    `json:"qualityRequirements"`
}

// UnmarshalJSON accepts both "serviceRequirement" (AH5 spec) and "requestedService"
// (backward-compat) and populates RequestedService from whichever is present.
func (r *OrchestrationRequest) UnmarshalJSON(data []byte) error {
	var wire orchestrationRequestWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	r.RequesterSystem = wire.RequesterSystem
	r.OrchestrationFlags = wire.OrchestrationFlags
	r.PreferredProviders = wire.PreferredProviders
	r.QualityRequirements = wire.QualityRequirements
	if wire.ServiceRequirement != nil {
		r.RequestedService = *wire.ServiceRequirement
	} else if wire.RequestedService != nil {
		r.RequestedService = *wire.RequestedService
	}
	return nil
}

// MarshalJSON encodes RequestedService as "serviceRequirement" (AH5 spec canonical name).
func (r OrchestrationRequest) MarshalJSON() ([]byte, error) {
	type alias struct {
		RequesterSystem     System             `json:"requesterSystem"`
		ServiceRequirement  ServiceRequirement `json:"serviceRequirement"`
		OrchestrationFlags  OrchestrationFlags `json:"orchestrationFlags,omitempty"`
		PreferredProviders  []System           `json:"preferredProviders,omitempty"`
		QualityRequirements []QoSRequirement   `json:"qualityRequirements,omitempty"`
	}
	return json.Marshal(alias{
		RequesterSystem:     r.RequesterSystem,
		ServiceRequirement:  r.RequestedService,
		OrchestrationFlags:  r.OrchestrationFlags,
		PreferredProviders:  r.PreferredProviders,
		QualityRequirements: r.QualityRequirements,
	})
}

// OrchestrationResult is one matched provider + service pair.
//
// Field names with typos (serviceDefinitition, cloudIdentitifer) are intentional:
// they are mandated by the AH5 wire format and must match exactly.
type OrchestrationResult struct {
	// ProviderName is the system name of the matched provider.
	ProviderName string `json:"providerName"`
	// ProviderAddress and ProviderPort carry the provider's network coordinates.
	// Used by QoS filtering (G40); not emitted in the AH5 wire response.
	ProviderAddress string `json:"-"`
	ProviderPort    int    `json:"-"`
	// ServiceDefinitition — spec typo (double 't') is intentional, must match AH5 wire format.
	ServiceDefinition string `json:"serviceDefinitition"`
	// CloudIdentitifer — spec typo (missing 'n') is intentional, must match AH5 wire format.
	CloudIdentifier   string            `json:"cloudIdentitifer,omitempty"`
	ServiceInstanceId string            `json:"serviceInstanceId,omitempty"`
	ServiceUri        string            `json:"serviceUri,omitempty"`
	Interfaces        []string          `json:"interfaces,omitempty"`
	Version           int               `json:"version,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	AliveUntil        string            `json:"aliveUntil,omitempty"`
	// Priority is set by store-based orchestrators; 0 = not applicable.
	Priority int `json:"priority,omitempty"`
	// ExclusiveUntil is set when the provider has an active lock; RFC3339 timestamp.
	ExclusiveUntil string `json:"exclusiveUntil,omitempty"`
}

// OrchestrationResponse wraps the list of results.
type OrchestrationResponse struct {
	Results  []OrchestrationResult `json:"results"`
	Warnings []string              `json:"warnings,omitempty"`
}
