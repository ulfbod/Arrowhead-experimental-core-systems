// Package model defines types for the Authentication core system.
package model

import "time"

// IdentityToken represents an active login session.
type IdentityToken struct {
	Token      string    `json:"token"`
	SystemName string    `json:"systemName"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// LoginRequest is the body for POST /authentication/identity/login.
type LoginRequest struct {
	SystemName  string `json:"systemName"`
	Credentials string `json:"credentials"`
}

// LoginResponse is returned on successful login.
type LoginResponse struct {
	Token      string    `json:"token"`
	SystemName string    `json:"systemName"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// VerifyResponse is returned by GET /authentication/identity/verify.
type VerifyResponse struct {
	Valid       bool      `json:"valid"`
	SystemName  string    `json:"systemName,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt,omitempty"`
}
