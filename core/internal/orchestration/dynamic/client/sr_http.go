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

// SRHTTPClient is a concrete ServiceRegistryClient that calls the SR over HTTP.
type SRHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewSRHTTPClient creates a new SRHTTPClient using the given base URL and HTTP client.
func NewSRHTTPClient(baseURL string, httpClient *http.Client) *SRHTTPClient {
	return &SRHTTPClient{baseURL: baseURL, http: httpClient}
}

// LookupServices calls POST /serviceregistry/service-discovery/lookup and converts the
// response to a slice of OrchestrationResult.
func (c *SRHTTPClient) LookupServices(ctx context.Context, req orchmodel.OrchestrationRequest) ([]orchmodel.OrchestrationResult, error) {
	body := srLookupRequest{
		ServiceDefinitionNames: []string{req.RequestedService.ServiceDefinition},
	}
	data, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/serviceregistry/service-discovery/lookup",
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create SR request: %w", err)
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
	for _, inst := range srResp.Entries {
		providerName := ""
		if inst.Provider != nil {
			providerName = inst.Provider.Name
		}
		ifaceNames := make([]string, 0, len(inst.Interfaces))
		for _, ifc := range inst.Interfaces {
			ifaceNames = append(ifaceNames, ifc.TemplateName)
		}
		results = append(results, orchmodel.OrchestrationResult{
			ProviderName:      providerName,
			ServiceDefinition: inst.ServiceDefinitionName,
			ServiceInstanceId: inst.InstanceID,
			Interfaces:        ifaceNames,
			AliveUntil:        inst.ExpiresAt,
			CloudIdentifier:   "LOCAL",
		})
	}
	return results, nil
}
