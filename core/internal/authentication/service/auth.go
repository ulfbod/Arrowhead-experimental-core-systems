// Package service implements Authentication business logic.
// AH5 responsibility: provide, manage, and validate system identities.
package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"arrowhead/core/internal/authentication/model"
	"arrowhead/core/internal/authentication/repository"
)

var (
	ErrMissingSystemName  = errors.New("systemName is required")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

// AuthService manages identity tokens.
type AuthService struct {
	repo          repository.Repository
	tokenDuration time.Duration
}

func NewAuthService(repo repository.Repository, tokenDuration time.Duration) *AuthService {
	return &AuthService{repo: repo, tokenDuration: tokenDuration}
}

// Login creates and stores an identity token for the given system.
// In this experimental implementation, credentials are not verified —
// any non-empty systemName receives a token (see GAP_ANALYSIS.md).
func (s *AuthService) Login(req model.LoginRequest) (*model.LoginResponse, error) {
	if strings.TrimSpace(req.SystemName) == "" {
		return nil, ErrMissingSystemName
	}
	token := &model.IdentityToken{
		Token:      generateToken(),
		SystemName: req.SystemName,
		ExpiresAt:  time.Now().Add(s.tokenDuration),
	}
	s.repo.Save(token)
	return &model.LoginResponse{
		Token:      token.Token,
		SystemName: token.SystemName,
		ExpiresAt:  token.ExpiresAt,
	}, nil
}

// Logout invalidates an identity token.
func (s *AuthService) Logout(token string) error {
	if !s.repo.Delete(token) {
		return ErrInvalidToken
	}
	return nil
}

// Verify checks whether a token is valid and not expired.
func (s *AuthService) Verify(token string) (*model.VerifyResponse, error) {
	t, ok := s.repo.FindByToken(token)
	if !ok {
		return &model.VerifyResponse{Valid: false}, nil
	}
	if time.Now().After(t.ExpiresAt) {
		s.repo.Delete(token)
		return &model.VerifyResponse{Valid: false}, nil
	}
	return &model.VerifyResponse{
		Valid:      true,
		SystemName: t.SystemName,
		ExpiresAt:  t.ExpiresAt,
	}, nil
}

func generateToken() string {
	// Simple pseudo-unique token (not cryptographically secure).
	// Replace with crypto/rand UUID in production.
	return fmt.Sprintf("%x", time.Now().UnixNano())
}
