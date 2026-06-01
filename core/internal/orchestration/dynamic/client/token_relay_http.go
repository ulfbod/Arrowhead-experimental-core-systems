package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	orchmodel "arrowhead/core/internal/orchestration/model"
)

// caTokenGenerateRequest is the body for POST /consumerauthorization/authorization-token/generate.
type caTokenGenerateRequest struct {
	TokenVariant string `json:"tokenVariant"`
	Provider     string `json:"provider"`
	TargetType   string `json:"targetType"`
	Target       string `json:"target"`
	Consumer     string `json:"consumer,omitempty"`
}

// caTokenDescriptorResponse mirrors the response from ConsumerAuth token generate.
type caTokenDescriptorResponse struct {
	TokenType  string `json:"tokenType"`
	TargetType string `json:"targetType"`
	Token      string `json:"token"`
	UsageLimit *int   `json:"usageLimit,omitempty"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
}

// CATokenRelayHTTPClient is a concrete TokenRelayClient that calls ConsumerAuthorization over HTTP.
type CATokenRelayHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewCATokenRelayHTTPClient creates a new CATokenRelayHTTPClient.
func NewCATokenRelayHTTPClient(baseURL string, httpClient *http.Client) *CATokenRelayHTTPClient {
	return &CATokenRelayHTTPClient{baseURL: baseURL, http: httpClient}
}

// GenerateToken calls POST /consumerauthorization/authorization-token/generate and returns
// the token descriptor for embedding in OrchestrationResult.AuthorizationTokens.
// Fail-open: returns (nil, err) on network or decode errors so the orchestrator can proceed
// without the token rather than returning an error to the consumer.
func (c *CATokenRelayHTTPClient) GenerateToken(
	ctx context.Context,
	consumer, provider, serviceDefinition, tokenVariant string,
) (*orchmodel.AuthorizationTokenDescriptor, error) {
	body := caTokenGenerateRequest{
		TokenVariant: tokenVariant,
		Provider:     provider,
		TargetType:   "SERVICE_DEF",
		Target:       serviceDefinition,
		Consumer:     consumer,
	}
	data, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/consumerauthorization/authorization-token/generate",
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("build token relay request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ConsumerAuth returned %d for token generate", resp.StatusCode)
	}
	var desc caTokenDescriptorResponse
	if err := json.NewDecoder(resp.Body).Decode(&desc); err != nil {
		return nil, err
	}
	return &orchmodel.AuthorizationTokenDescriptor{
		TokenType:  desc.TokenType,
		TargetType: desc.TargetType,
		Token:      desc.Token,
		UsageLimit: desc.UsageLimit,
		ExpiresAt:  desc.ExpiresAt,
	}, nil
}
