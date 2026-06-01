package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	orchmodel "arrowhead/core/internal/orchestration/model"
)

// srLookupRequest is the body for POST /serviceregistry/service-discovery/lookup (AH5).
type srLookupRequest struct {
	ServiceDefinitionNames []string `json:"serviceDefinitionNames"`
	Versions               []string `json:"versions,omitempty"`
}

// srAH5System mirrors the provider system in an AH5 service instance response.
type srAH5System struct {
	Name string `json:"name"`
}

// srAH5Interface mirrors the interface entry in an AH5 service instance response.
type srAH5Interface struct {
	TemplateName string `json:"templateName"`
}

// srAH5ServiceInstance mirrors the AH5 service instance response shape.
type srAH5ServiceInstance struct {
	InstanceID            string           `json:"instanceId"`
	Provider              *srAH5System     `json:"provider,omitempty"`
	ServiceDefinitionName string           `json:"serviceDefinitionName"`
	Version               string           `json:"version,omitempty"`
	ExpiresAt             string           `json:"expiresAt,omitempty"`
	Interfaces            []srAH5Interface `json:"interfaces,omitempty"`
}

// srAH5LookupResponse mirrors POST /serviceregistry/service-discovery/lookup response.
type srAH5LookupResponse struct {
	Entries []*srAH5ServiceInstance `json:"entries"`
	Count   int                     `json:"count"`
}

// ─── Legacy SR bridge types (G58) ────────────────────────────────────────────

// srLegacyQueryRequest is the body for POST /serviceregistry/query (legacy/AH4 endpoint).
type srLegacyQueryRequest struct {
	ServiceDefinition  string   `json:"serviceDefinition,omitempty"`
	Interfaces         []string `json:"interfaces,omitempty"`
	VersionRequirement int      `json:"versionRequirement,omitempty"`
}

// srLegacySystem mirrors the providerSystem in a legacy ServiceInstance.
type srLegacySystem struct {
	SystemName string `json:"systemName"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
}

// srLegacyServiceInstance mirrors the legacy ServiceInstance model.
type srLegacyServiceInstance struct {
	ServiceDefinition string         `json:"serviceDefinition"`
	ProviderSystem    srLegacySystem `json:"providerSystem"`
	ServiceUri        string         `json:"serviceUri"`
	Interfaces        []string       `json:"interfaces"`
	Version           int            `json:"version"`
}

// srLegacyQueryResponse mirrors POST /serviceregistry/query response.
type srLegacyQueryResponse struct {
	ServiceQueryData []*srLegacyServiceInstance `json:"serviceQueryData"`
	UnfilteredHits   int                        `json:"unfilteredHits"`
}

// SRHTTPClient is a concrete ServiceRegistryClient that calls the SR over HTTP.
type SRHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewSRHTTPClient creates a new SRHTTPClient using the given base URL and HTTP client.
func NewSRHTTPClient(baseURL string, httpClient *http.Client) *SRHTTPClient {
	return &SRHTTPClient{baseURL: baseURL, http: httpClient}
}

// LookupServices calls POST /serviceregistry/service-discovery/lookup (AH5 endpoint) and
// also bridges the legacy POST /serviceregistry/query endpoint (G58). Results from both
// stores are merged; AH5 results take priority (no duplicate providerName|serviceDefinition).
//
// Fail-open for legacy: if the legacy call fails, AH5 results are returned as-is.
func (c *SRHTTPClient) LookupServices(ctx context.Context, req orchmodel.OrchestrationRequest) ([]orchmodel.OrchestrationResult, error) {
	// ── Step 1: query AH5 service-discovery endpoint ──────────────────────────
	body := srLookupRequest{
		ServiceDefinitionNames: []string{req.RequestedService.ServiceDefinition},
	}
	if req.RequestedService.VersionRequirement != "" {
		body.Versions = []string{req.RequestedService.VersionRequirement}
	}
	data, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/serviceregistry/service-discovery/lookup",
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create SR AH5 request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	var srResp srAH5LookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&srResp); err != nil {
		return nil, err
	}
	results := make([]orchmodel.OrchestrationResult, 0, len(srResp.Entries))
	seen := make(map[string]bool) // key = providerName|serviceDefinition
	for _, inst := range srResp.Entries {
		providerName := ""
		if inst.Provider != nil {
			providerName = inst.Provider.Name
		}
		ifaceNames := make([]string, 0, len(inst.Interfaces))
		for _, ifc := range inst.Interfaces {
			ifaceNames = append(ifaceNames, ifc.TemplateName)
		}
		r := orchmodel.OrchestrationResult{
			ProviderName:      providerName,
			ServiceDefinition: inst.ServiceDefinitionName,
			ServiceInstanceId: inst.InstanceID,
			Interfaces:        ifaceNames,
			AliveUntil:        inst.ExpiresAt,
			CloudIdentifier:   "LOCAL",
		}
		results = append(results, r)
		seen[providerName+"|"+inst.ServiceDefinitionName] = true
	}

	// ── Step 2: bridge legacy store (fail-open) ───────────────────────────────
	legacyBody := srLegacyQueryRequest{ServiceDefinition: req.RequestedService.ServiceDefinition}
	legacyData, _ := json.Marshal(legacyBody)
	legacyReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/serviceregistry/query",
		bytes.NewReader(legacyData))
	if err == nil {
		legacyReq.Header.Set("Content-Type", "application/json")
		legacyResp, legacyErr := c.http.Do(legacyReq)
		if legacyErr == nil {
			defer legacyResp.Body.Close() //nolint:errcheck
			var lResp srLegacyQueryResponse
			if decodeErr := json.NewDecoder(legacyResp.Body).Decode(&lResp); decodeErr == nil {
				for _, inst := range lResp.ServiceQueryData {
					key := inst.ProviderSystem.SystemName + "|" + inst.ServiceDefinition
					if seen[key] {
						continue // AH5 result takes priority
					}
					seen[key] = true
					results = append(results, orchmodel.OrchestrationResult{
						ProviderName:      inst.ProviderSystem.SystemName,
						ProviderAddress:   inst.ProviderSystem.Address,
						ProviderPort:      inst.ProviderSystem.Port,
						ServiceDefinition: inst.ServiceDefinition,
						ServiceUri:        inst.ServiceUri,
						Interfaces:        inst.Interfaces,
						CloudIdentifier:   "LOCAL",
					})
				}
			}
		}
	}

	return results, nil
}
