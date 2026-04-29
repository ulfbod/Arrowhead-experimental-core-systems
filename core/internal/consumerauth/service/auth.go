// Package service implements ConsumerAuthorization business logic.
// AH5 responsibility: manage and authorize connections between systems via rules.
package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"arrowhead/core/internal/consumerauth/model"
	"arrowhead/core/internal/consumerauth/repository"
)

var (
	ErrMissingConsumer  = errors.New("consumerSystemName is required")
	ErrMissingProvider  = errors.New("providerSystemName is required")
	ErrMissingService   = errors.New("serviceDefinition is required")
	ErrRuleNotFound     = errors.New("authorization rule not found")
)

type AuthService struct {
	repo repository.Repository
}

func NewAuthService(repo repository.Repository) *AuthService {
	return &AuthService{repo: repo}
}

// Grant adds an authorization rule.
func (s *AuthService) Grant(req model.GrantRequest) (model.AuthRule, error) {
	if strings.TrimSpace(req.ConsumerSystemName) == "" {
		return model.AuthRule{}, ErrMissingConsumer
	}
	if strings.TrimSpace(req.ProviderSystemName) == "" {
		return model.AuthRule{}, ErrMissingProvider
	}
	if strings.TrimSpace(req.ServiceDefinition) == "" {
		return model.AuthRule{}, ErrMissingService
	}
	rule := model.AuthRule{
		ConsumerSystemName: req.ConsumerSystemName,
		ProviderSystemName: req.ProviderSystemName,
		ServiceDefinition:  req.ServiceDefinition,
	}
	return s.repo.Save(rule), nil
}

// Revoke removes an authorization rule by ID.
func (s *AuthService) Revoke(id int64) error {
	if !s.repo.Delete(id) {
		return ErrRuleNotFound
	}
	return nil
}

// Lookup returns all rules matching the optional filters.
func (s *AuthService) Lookup(consumer, provider, servicedef string) model.LookupResponse {
	all := s.repo.All()
	var result []model.AuthRule
	for _, r := range all {
		if consumer != "" && r.ConsumerSystemName != consumer {
			continue
		}
		if provider != "" && r.ProviderSystemName != provider {
			continue
		}
		if servicedef != "" && r.ServiceDefinition != servicedef {
			continue
		}
		result = append(result, r)
	}
	if result == nil {
		result = []model.AuthRule{}
	}
	return model.LookupResponse{Rules: result, Count: len(result)}
}

// Verify checks whether an authorization rule exists for the given triple.
func (s *AuthService) Verify(req model.VerifyRequest) model.VerifyResponse {
	for _, r := range s.repo.All() {
		if r.ConsumerSystemName == req.ConsumerSystemName &&
			r.ProviderSystemName == req.ProviderSystemName &&
			r.ServiceDefinition == req.ServiceDefinition {
			id := r.ID
			return model.VerifyResponse{Authorized: true, RuleID: &id}
		}
	}
	return model.VerifyResponse{Authorized: false}
}

// GenerateToken creates a simple authorization token for an authorized consumer.
func (s *AuthService) GenerateToken(req model.TokenRequest) (model.TokenResponse, error) {
	vr := s.Verify(model.VerifyRequest{
		ConsumerSystemName: req.ConsumerSystemName,
		ProviderSystemName: req.ProviderSystemName,
		ServiceDefinition:  req.ServiceDefinition,
	})
	if !vr.Authorized {
		return model.TokenResponse{}, errors.New("consumer is not authorized")
	}
	token := fmt.Sprintf("%x-%s-%s", time.Now().UnixNano(), req.ConsumerSystemName, req.ServiceDefinition)
	return model.TokenResponse{
		Token:              token,
		ConsumerSystemName: req.ConsumerSystemName,
		ServiceDefinition:  req.ServiceDefinition,
	}, nil
}
