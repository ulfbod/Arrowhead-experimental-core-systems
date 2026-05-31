package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// caVerifyRequest mirrors the ConsumerAuthorization verify body (AH5 model).
type caVerifyRequest struct {
	Consumer   string `json:"consumer"`
	Provider   string `json:"provider,omitempty"`
	Target     string `json:"target"`
	TargetType string `json:"targetType"`
}

// CAHTTPClient is a concrete ConsumerAuthClient that calls ConsumerAuthorization over HTTP.
type CAHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewCAHTTPClient creates a new CAHTTPClient using the given base URL and HTTP client.
func NewCAHTTPClient(baseURL string, httpClient *http.Client) *CAHTTPClient {
	return &CAHTTPClient{baseURL: baseURL, http: httpClient}
}

// IsAuthorized calls POST /consumerauthorization/authorization/verify and returns the result.
// Fail-closed: returns (false, err) on network or decode errors.
func (c *CAHTTPClient) IsAuthorized(ctx context.Context, consumer, provider, target string) (bool, error) {
	body := caVerifyRequest{
		Consumer:   consumer,
		Provider:   provider,
		Target:     target,
		TargetType: "SERVICE_DEF",
	}
	data, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/consumerauthorization/authorization/verify",
		bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("create CA request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close() //nolint:errcheck
	var authorized bool
	if err := json.NewDecoder(resp.Body).Decode(&authorized); err != nil {
		return false, err
	}
	return authorized, nil
}
