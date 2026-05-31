// Package service implements ConsumerAuthorization business logic.
// AH5 responsibility: manage and evaluate authorization policies.
package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"arrowhead/core/internal/consumerauth/model"
	"arrowhead/core/internal/consumerauth/repository"
)

var (
	ErrMissingProvider      = errors.New("provider is required")
	ErrMissingTarget        = errors.New("target is required")
	ErrMissingTargetType    = errors.New("targetType is required")
	ErrRuleNotFound         = errors.New("authorization policy not found")
	ErrDuplicateRule        = errors.New("authorization policy already exists")
	ErrUnsupportedVariant   = errors.New("unsupported token variant")
	ErrTokenNotFound        = errors.New("authorization token not found")
)

type authTokenRecord struct {
	Token         string
	TokenType     string
	Provider      string
	TargetType    string
	Target        string
	Scope         string
	Consumer      string
	ExpiresAt     time.Time
	MaxUsageCount int
	UsageCount    int
}

type encryptionKeyRecord struct {
	SystemName string
	Algorithm  string
	Key        string
}

type AuthService struct {
	repo      repository.Repository
	mu        sync.RWMutex
	authTokens map[string]*authTokenRecord
	encKeys    map[string]*encryptionKeyRecord
}

func NewAuthService(repo repository.Repository) *AuthService {
	return &AuthService{
		repo:       repo,
		authTokens: make(map[string]*authTokenRecord),
		encKeys:    make(map[string]*encryptionKeyRecord),
	}
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Grant creates a new authorization policy. Returns ErrDuplicateRule if the
// instanceId (derived from provider+targetType+target) already exists.
func (s *AuthService) Grant(req model.GrantRequest) (model.AuthPolicy, error) {
	if strings.TrimSpace(req.Provider) == "" {
		return model.AuthPolicy{}, ErrMissingProvider
	}
	if strings.TrimSpace(req.Target) == "" {
		return model.AuthPolicy{}, ErrMissingTarget
	}
	if strings.TrimSpace(req.TargetType) == "" {
		return model.AuthPolicy{}, ErrMissingTargetType
	}
	instanceID := model.BuildInstanceID(req.Provider, req.TargetType, req.Target)
	if _, exists := s.repo.FindByInstanceID(instanceID); exists {
		return model.AuthPolicy{}, ErrDuplicateRule
	}
	scoped := req.ScopedPolicies
	if scoped == nil {
		scoped = make(map[string]model.PolicyDef)
	}
	policy := model.AuthPolicy{
		InstanceID:     instanceID,
		AuthLevel:      "PR",
		Cloud:          "LOCAL",
		Provider:       req.Provider,
		TargetType:     req.TargetType,
		Target:         req.Target,
		Description:    req.Description,
		DefaultPolicy:  req.DefaultPolicy,
		ScopedPolicies: scoped,
		CreatedBy:      req.CreatedBy,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	return s.repo.Save(policy), nil
}

// Revoke removes an authorization policy by instanceId.
func (s *AuthService) Revoke(instanceID string) error {
	if !s.repo.Delete(instanceID) {
		return ErrRuleNotFound
	}
	return nil
}

// BulkRevoke removes policies by instanceIds, ignoring ones that don't exist.
func (s *AuthService) BulkRevoke(instanceIDs []string) {
	for _, id := range instanceIDs {
		s.repo.Delete(id)
	}
}

// Lookup returns policies matching the filters. An empty request returns all.
func (s *AuthService) Lookup(req model.LookupRequest) model.LookupResponse {
	all := s.repo.All()
	var result []model.AuthPolicy
	for _, p := range all {
		if matchesLookup(p, req) {
			result = append(result, p)
		}
	}
	if result == nil {
		result = []model.AuthPolicy{}
	}
	return model.LookupResponse{Policies: result, Count: len(result), TotalCount: len(all)}
}

// AllPolicies returns every stored policy.
func (s *AuthService) AllPolicies() []model.AuthPolicy {
	all := s.repo.All()
	if all == nil {
		return []model.AuthPolicy{}
	}
	return all
}

func matchesLookup(p model.AuthPolicy, req model.LookupRequest) bool {
	if len(req.InstanceIDs) > 0 && !containsStr(req.InstanceIDs, p.InstanceID) {
		return false
	}
	if len(req.CloudIdentifiers) > 0 && !containsStr(req.CloudIdentifiers, p.Cloud) {
		return false
	}
	if len(req.TargetNames) > 0 && !containsStr(req.TargetNames, p.Target) {
		return false
	}
	if req.TargetType != "" && p.TargetType != req.TargetType {
		return false
	}
	return true
}

// Verify returns true if the consumer is authorized to access the target.
// When req.Provider is set, only the policy for that specific provider is checked.
func (s *AuthService) Verify(req model.VerifyRequest) bool {
	for _, p := range s.repo.All() {
		if req.Provider != "" && p.Provider != req.Provider {
			continue
		}
		if p.Target != req.Target || p.TargetType != req.TargetType {
			continue
		}
		policy := p.DefaultPolicy
		if req.Scope != "" {
			if sp, ok := p.ScopedPolicies[req.Scope]; ok {
				policy = sp
			}
		}
		if isAuthorized(req.Consumer, policy) {
			return true
		}
	}
	return false
}

// BulkVerify checks authorization for multiple requests.
func (s *AuthService) BulkVerify(reqs []model.VerifyRequest) []bool {
	results := make([]bool, len(reqs))
	for i, req := range reqs {
		results[i] = s.Verify(req)
	}
	return results
}

func isAuthorized(consumer string, policy model.PolicyDef) bool {
	switch policy.PolicyType {
	case model.PolicyAll:
		return true
	case model.PolicyWhitelist:
		return containsStr(policy.PolicyList, consumer)
	case model.PolicyBlacklist:
		return !containsStr(policy.PolicyList, consumer)
	default:
		return false
	}
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// hmacSecret returns the HMAC secret from the environment with a default fallback.
func hmacSecret() string {
	if s := os.Getenv("HMAC_SECRET"); s != "" {
		return s
	}
	return "arrowhead-default-secret"
}

// GenerateAuthToken issues an authorization token.
// Supports TIME_LIMITED_TOKEN, USAGE_LIMITED_TOKEN, and BASE64_SELF_CONTAINED.
// JWT variants return ErrUnsupportedVariant (501).
func (s *AuthService) GenerateAuthToken(req model.TokenGenerateRequest) (model.TokenDescriptor, error) {
	switch req.TokenVariant {
	case model.TokenVariantTimeLimited:
		token := generateToken()
		expiresAt := time.Now().UTC().Add(time.Hour)
		s.mu.Lock()
		s.authTokens[token] = &authTokenRecord{
			Token:      token,
			TokenType:  req.TokenVariant,
			Provider:   req.Provider,
			TargetType: req.TargetType,
			Target:     req.Target,
			Scope:      req.Scope,
			Consumer:   req.Consumer,
			ExpiresAt:  expiresAt,
		}
		s.mu.Unlock()
		return model.TokenDescriptor{
			TokenType:  req.TokenVariant,
			TargetType: req.TargetType,
			Token:      token,
			ExpiresAt:  expiresAt.Format(time.RFC3339),
		}, nil

	case model.TokenVariantUsageLimited:
		token := generateToken()
		expiresAt := time.Now().UTC().Add(time.Hour)
		maxUsage := req.MaxUsageCount
		if maxUsage <= 0 {
			maxUsage = 1
		}
		s.mu.Lock()
		s.authTokens[token] = &authTokenRecord{
			Token:         token,
			TokenType:     req.TokenVariant,
			Provider:      req.Provider,
			TargetType:    req.TargetType,
			Target:        req.Target,
			Scope:         req.Scope,
			Consumer:      req.Consumer,
			ExpiresAt:     expiresAt,
			MaxUsageCount: maxUsage,
			UsageCount:    0,
		}
		s.mu.Unlock()
		return model.TokenDescriptor{
			TokenType:  req.TokenVariant,
			TargetType: req.TargetType,
			Token:      token,
			ExpiresAt:  expiresAt.Format(time.RFC3339),
		}, nil

	case model.TokenVariantBase64SelfContained:
		expiresAt := time.Now().UTC().Add(time.Hour)
		payload := map[string]any{
			"provider":   req.Provider,
			"target":     req.Target,
			"targetType": req.TargetType,
			"consumer":   req.Consumer,
			"scope":      req.Scope,
			"exp":        expiresAt.Unix(),
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return model.TokenDescriptor{}, err
		}
		b64Payload := base64.StdEncoding.EncodeToString(payloadBytes)
		mac := hmac.New(sha256.New, []byte(hmacSecret()))
		mac.Write([]byte(b64Payload)) //nolint:errcheck
		sig := hex.EncodeToString(mac.Sum(nil))
		token := b64Payload + "." + sig
		return model.TokenDescriptor{
			TokenType:  req.TokenVariant,
			TargetType: req.TargetType,
			Token:      token,
			ExpiresAt:  expiresAt.Format(time.RFC3339),
		}, nil

	default:
		return model.TokenDescriptor{}, ErrUnsupportedVariant
	}
}

// VerifyAuthToken checks if the token is valid and not expired.
// Supports TIME_LIMITED_TOKEN (map lookup), USAGE_LIMITED_TOKEN (counter check),
// and BASE64_SELF_CONTAINED (HMAC verify, no map lookup needed).
func (s *AuthService) VerifyAuthToken(accessToken string) (model.TokenVerifyResponse, bool) {
	// Detect BASE64_SELF_CONTAINED tokens: they contain exactly one "." separating
	// the base64 payload from the HMAC signature.
	if parts := strings.SplitN(accessToken, ".", 2); len(parts) == 2 {
		b64Payload := parts[0]
		sig := parts[1]
		// Verify HMAC
		mac := hmac.New(sha256.New, []byte(hmacSecret()))
		mac.Write([]byte(b64Payload)) //nolint:errcheck
		expectedSig := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
			return model.TokenVerifyResponse{}, false
		}
		payloadBytes, err := base64.StdEncoding.DecodeString(b64Payload)
		if err != nil {
			return model.TokenVerifyResponse{}, false
		}
		var payload struct {
			Provider   string `json:"provider"`
			Target     string `json:"target"`
			TargetType string `json:"targetType"`
			Consumer   string `json:"consumer"`
			Scope      string `json:"scope"`
			Exp        int64  `json:"exp"`
		}
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return model.TokenVerifyResponse{}, false
		}
		if time.Now().Unix() > payload.Exp {
			return model.TokenVerifyResponse{}, false
		}
		var scope any = nil
		if payload.Scope != "" {
			scope = payload.Scope
		}
		return model.TokenVerifyResponse{
			Verified:      true,
			ConsumerCloud: "LOCAL",
			Consumer:      payload.Consumer,
			TargetType:    payload.TargetType,
			Target:        payload.Target,
			Scope:         scope,
		}, true
	}

	s.mu.Lock()
	rec, ok := s.authTokens[accessToken]
	if ok && time.Now().After(rec.ExpiresAt) {
		delete(s.authTokens, accessToken)
		ok = false
	}
	if !ok {
		s.mu.Unlock()
		return model.TokenVerifyResponse{}, false
	}
	// Handle USAGE_LIMITED_TOKEN
	if rec.TokenType == model.TokenVariantUsageLimited {
		rec.UsageCount++
		if rec.UsageCount > rec.MaxUsageCount {
			delete(s.authTokens, accessToken)
			s.mu.Unlock()
			return model.TokenVerifyResponse{}, false
		}
	}
	s.mu.Unlock()
	var scope any = nil
	if rec.Scope != "" {
		scope = rec.Scope
	}
	return model.TokenVerifyResponse{
		Verified:      true,
		ConsumerCloud: "LOCAL",
		Consumer:      rec.Consumer,
		TargetType:    rec.TargetType,
		Target:        rec.Target,
		Scope:         scope,
	}, true
}

// BulkGrant creates multiple authorization policies; per-item errors don't abort.
func (s *AuthService) BulkGrant(reqs []model.GrantRequest) []model.BulkGrantResult {
	results := make([]model.BulkGrantResult, len(reqs))
	for i, req := range reqs {
		policy, err := s.Grant(req)
		if err != nil {
			results[i] = model.BulkGrantResult{Error: err.Error()}
		} else {
			results[i] = model.BulkGrantResult{InstanceID: policy.InstanceID, Policy: policy}
		}
	}
	return results
}

// BulkGenerateTokens generates multiple auth tokens; per-item errors don't abort.
func (s *AuthService) BulkGenerateTokens(reqs []model.TokenGenerateRequest) []model.BulkGenerateResult {
	results := make([]model.BulkGenerateResult, len(reqs))
	for i, req := range reqs {
		desc, err := s.GenerateAuthToken(req)
		if err != nil {
			results[i] = model.BulkGenerateResult{Error: err.Error()}
		} else {
			results[i] = model.BulkGenerateResult{Token: desc}
		}
	}
	return results
}

// RevokeTokens removes multiple auth tokens by token string, ignoring missing ones.
func (s *AuthService) RevokeTokens(tokens []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range tokens {
		delete(s.authTokens, t)
	}
}

// ListTokens returns all unexpired auth tokens as TokenRecords.
func (s *AuthService) ListTokens() []model.TokenRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var out []model.TokenRecord
	for token, rec := range s.authTokens {
		if now.After(rec.ExpiresAt) {
			delete(s.authTokens, token)
			continue
		}
		out = append(out, model.TokenRecord{
			Token:      rec.Token,
			TokenType:  rec.TokenType,
			Provider:   rec.Provider,
			TargetType: rec.TargetType,
			Target:     rec.Target,
			Consumer:   rec.Consumer,
			Scope:      rec.Scope,
			ExpiresAt:  rec.ExpiresAt.Format(time.RFC3339),
		})
	}
	return out
}

// BulkAddEncryptionKeys stores multiple encryption keys.
func (s *AuthService) BulkAddEncryptionKeys(keys []model.EncryptionKeyRequest) {
	for _, k := range keys {
		s.RegisterEncryptionKey(k)
	}
}

// BulkRemoveEncryptionKeys removes encryption keys for the given system names.
func (s *AuthService) BulkRemoveEncryptionKeys(systemNames []string) {
	for _, name := range systemNames {
		s.RemoveEncryptionKey(name)
	}
}

// RegisterEncryptionKey stores an encryption key for a system.
func (s *AuthService) RegisterEncryptionKey(req model.EncryptionKeyRequest) {
	s.mu.Lock()
	s.encKeys[req.SystemName] = &encryptionKeyRecord{
		SystemName: req.SystemName,
		Algorithm:  req.Algorithm,
		Key:        req.Key,
	}
	s.mu.Unlock()
}

// RemoveEncryptionKey deletes the encryption key for a system.
func (s *AuthService) RemoveEncryptionKey(systemName string) {
	s.mu.Lock()
	delete(s.encKeys, systemName)
	s.mu.Unlock()
}
