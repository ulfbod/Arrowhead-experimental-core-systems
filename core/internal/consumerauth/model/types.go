// Package model defines types for the ConsumerAuthorization core system.
// AH5 responsibility: manage authorization policies between consumer and provider systems.
package model

import "strings"

// PolicyDef describes a single authorization policy entry.
type PolicyDef struct {
	PolicyType string   `json:"policyType"`
	PolicyList []string `json:"policyList,omitempty"`
}

// AuthPolicy is the central authorization policy record (provider-centric).
type AuthPolicy struct {
	InstanceID     string               `json:"instanceId"`
	AuthLevel      string               `json:"authorizationLevel"`
	Cloud          string               `json:"cloud"`
	Provider       string               `json:"provider"`
	TargetType     string               `json:"targetType"`
	Target         string               `json:"target"`
	Description    string               `json:"description,omitempty"`
	DefaultPolicy  PolicyDef            `json:"defaultPolicy"`
	ScopedPolicies map[string]PolicyDef `json:"scopedPolicies"`
	CreatedBy      string               `json:"createdBy,omitempty"`
	CreatedAt      string               `json:"createdAt,omitempty"`
}

// GrantRequest is the body for POST /authorization/grant.
type GrantRequest struct {
	Provider       string               `json:"provider"`
	TargetType     string               `json:"targetType"`
	Target         string               `json:"target"`
	Description    string               `json:"description,omitempty"`
	DefaultPolicy  PolicyDef            `json:"defaultPolicy"`
	ScopedPolicies map[string]PolicyDef `json:"scopedPolicies,omitempty"`
	CreatedBy      string               `json:"createdBy,omitempty"`
}

// LookupRequest is the body for POST /authorization/lookup.
type LookupRequest struct {
	InstanceIDs      []string `json:"instanceIds,omitempty"`
	CloudIdentifiers []string `json:"cloudIdentifiers,omitempty"`
	TargetNames      []string `json:"targetNames,omitempty"`
	TargetType       string   `json:"targetType,omitempty"`
}

// LookupResponse is returned by POST /authorization/lookup.
type LookupResponse struct {
	Policies   []AuthPolicy `json:"policies"`
	Count      int          `json:"count"`
	TotalCount int          `json:"totalCount"`
}

// VerifyRequest is the body for POST /authorization/verify.
// Provider is optional: when set, only the policy for that specific provider is checked.
type VerifyRequest struct {
	Consumer   string `json:"consumer"`
	Provider   string `json:"provider,omitempty"`
	Target     string `json:"target"`
	TargetType string `json:"targetType"`
	Scope      string `json:"scope,omitempty"`
}

// Policy type constants.
const (
	PolicyAll       = "ALL"
	PolicyWhitelist = "WHITELIST"
	PolicyBlacklist = "BLACKLIST"
)

// Target type constants.
const (
	TargetServiceDef = "SERVICE_DEF"
	TargetEventType  = "EVENT_TYPE"
)

// BuildInstanceID constructs the composite instance ID: PR|LOCAL|<provider>|<targetType>|<target>.
func BuildInstanceID(provider, targetType, target string) string {
	return "PR|LOCAL|" + provider + "|" + targetType + "|" + target
}

// EncodeInstanceID percent-encodes pipe characters for use in URL paths.
func EncodeInstanceID(id string) string {
	return strings.ReplaceAll(id, "|", "%7C")
}

// ─── Authorization token types ─────────────────────────────────────────────────

// TokenVariantTimeLimited is the only fully implemented token variant.
const TokenVariantTimeLimited = "TIME_LIMITED_TOKEN"

// TokenGenerateRequest is the body for POST /authorization-token/generate.
type TokenGenerateRequest struct {
	TokenVariant string `json:"tokenVariant"`
	Provider     string `json:"provider"`
	TargetType   string `json:"targetType"`
	Target       string `json:"target"`
	Scope        string `json:"scope,omitempty"`
	Consumer     string `json:"consumer,omitempty"`
}

// TokenDescriptor is returned on successful token generation (AuthorizationTokenDescriptor).
type TokenDescriptor struct {
	TokenType  string `json:"tokenType"`
	TargetType string `json:"targetType"`
	Token      string `json:"token"`
	ExpiresAt  string `json:"expiresAt"`
}

// TokenVerifyResponse is returned by GET /authorization-token/verify/{accessToken}.
type TokenVerifyResponse struct {
	Verified      bool   `json:"verified"`
	ConsumerCloud string `json:"consumerCloud"`
	Consumer      string `json:"consumer"`
	TargetType    string `json:"targetType"`
	Target        string `json:"target"`
	Scope         any    `json:"scope"`
}

// EncryptionKeyRequest is the body for POST /authorization-token/encryption-key.
type EncryptionKeyRequest struct {
	SystemName string `json:"systemName"`
	Algorithm  string `json:"algorithm"`
	Key        string `json:"key"`
}
