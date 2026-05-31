package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

// identityVerifyResponse mirrors GET /authentication/identity/verify/<token> response.
type identityVerifyResponse struct {
	Verified   bool   `json:"verified"`
	SystemName string `json:"systemName"`
}

// IdentityHTTPClient is a concrete IdentityClient that calls the Authentication system over HTTP.
type IdentityHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewIdentityHTTPClient creates a new IdentityHTTPClient.
func NewIdentityHTTPClient(baseURL string, httpClient *http.Client) *IdentityHTTPClient {
	return &IdentityHTTPClient{baseURL: baseURL, http: httpClient}
}

// VerifyToken calls GET /authentication/identity/verify/<token> and returns the systemName.
// Returns an error when the token is invalid, expired, or the auth system is unreachable (fail-closed).
func (c *IdentityHTTPClient) VerifyToken(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/authentication/identity/verify/"+url.PathEscape(token), nil)
	if err != nil {
		return "", errors.New("invalid auth system URL")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	var result identityVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.Verified {
		return "", errors.New("token not verified")
	}
	return result.SystemName, nil
}
