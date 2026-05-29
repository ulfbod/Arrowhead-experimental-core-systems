// Package service implements ConsumerAuthorization business logic.
// AH5 responsibility: manage and evaluate authorization policies.
package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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
	Token      string
	TokenType  string
	Provider   string
	TargetType string
	Target     string
	Scope      string
	Consumer   string
	ExpiresAt  time.Time
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

// GenerateAuthToken issues an authorization token. Only TIME_LIMITED_TOKEN is supported.
func (s *AuthService) GenerateAuthToken(req model.TokenGenerateRequest) (model.TokenDescriptor, error) {
	if req.TokenVariant != model.TokenVariantTimeLimited {
		return model.TokenDescriptor{}, ErrUnsupportedVariant
	}
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
}

// VerifyAuthToken checks if the token is valid and not expired.
func (s *AuthService) VerifyAuthToken(accessToken string) (model.TokenVerifyResponse, bool) {
	s.mu.Lock()
	rec, ok := s.authTokens[accessToken]
	if ok && time.Now().After(rec.ExpiresAt) {
		delete(s.authTokens, accessToken)
		ok = false
	}
	s.mu.Unlock()
	if !ok {
		return model.TokenVerifyResponse{}, false
	}
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
