// Package client provides a BlacklistClient interface and implementations for
// checking whether a system is on the Blacklist core system's active list.
package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// BlacklistClient checks whether a named system is currently blacklisted.
// Implementations MUST be fail-closed: return (true, nil) on any error
// so that callers treat an unreachable Blacklist as a rejection.
type BlacklistClient interface {
	IsBlacklisted(ctx context.Context, name string) (bool, error)
}

// HTTPClient calls GET {baseURL}/blacklist/check/{name} on the Blacklist service.
// Fail-closed: network errors, non-200 responses, or parse failures all return true.
type HTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewHTTPClient returns a new HTTPClient using the given base URL and http.Client.
func NewHTTPClient(baseURL string, httpClient *http.Client) *HTTPClient {
	return &HTTPClient{baseURL: baseURL, http: httpClient}
}

// IsBlacklisted calls the Blacklist service and returns whether name is blacklisted.
// Returns true (fail-closed) on any communication or decode error.
func (c *HTTPClient) IsBlacklisted(ctx context.Context, name string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/blacklist/check/"+name, nil)
	if err != nil {
		return true, nil // fail-closed
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return true, nil // fail-closed: unreachable → treat as blacklisted
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return true, nil // fail-closed
	}
	var blacklisted bool
	if err := json.NewDecoder(resp.Body).Decode(&blacklisted); err != nil {
		return true, nil // fail-closed
	}
	return blacklisted, nil
}

// NopClient is used when no Blacklist URL is configured.
// It always reports systems as not blacklisted (blacklist enforcement disabled).
type NopClient struct{}

// IsBlacklisted always returns false (no-op).
func (NopClient) IsBlacklisted(_ context.Context, _ string) (bool, error) { return false, nil }
