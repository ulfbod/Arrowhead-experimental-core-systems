// Package model defines types for the ConsumerAuthorization core system.
// AH5 responsibility: manage authorization rules between consumer and provider systems.
package model

// AuthRule represents a permission granting a consumer access to a provider's service.
type AuthRule struct {
	ID                   int64  `json:"id"`
	ConsumerSystemName   string `json:"consumerSystemName"`
	ProviderSystemName   string `json:"providerSystemName"`
	ServiceDefinition    string `json:"serviceDefinition"`
}

// GrantRequest is the body for POST /authorization/grant.
type GrantRequest struct {
	ConsumerSystemName string `json:"consumerSystemName"`
	ProviderSystemName string `json:"providerSystemName"`
	ServiceDefinition  string `json:"serviceDefinition"`
}

// VerifyRequest is the body for POST /authorization/verify.
type VerifyRequest struct {
	ConsumerSystemName string `json:"consumerSystemName"`
	ProviderSystemName string `json:"providerSystemName"`
	ServiceDefinition  string `json:"serviceDefinition"`
}

// VerifyResponse is returned by POST /authorization/verify.
type VerifyResponse struct {
	Authorized bool   `json:"authorized"`
	RuleID     *int64 `json:"ruleId,omitempty"`
}

// LookupResponse is returned by GET /authorization/lookup.
type LookupResponse struct {
	Rules []AuthRule `json:"rules"`
	Count int        `json:"count"`
}

// TokenRequest is the body for POST /authorization/token/generate.
type TokenRequest struct {
	ConsumerSystemName string `json:"consumerSystemName"`
	ProviderSystemName string `json:"providerSystemName"`
	ServiceDefinition  string `json:"serviceDefinition"`
}

// TokenResponse is returned by POST /authorization/token/generate.
type TokenResponse struct {
	Token             string `json:"token"`
	ConsumerSystemName string `json:"consumerSystemName"`
	ServiceDefinition  string `json:"serviceDefinition"`
}
