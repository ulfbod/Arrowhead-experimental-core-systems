// Package model defines types for the Authentication core system.
package model

import "time"

// IdentityToken represents an active login session.
type IdentityToken struct {
	Token      string    `json:"token"`
	SystemName string    `json:"systemName"`
	ExpiresAt  time.Time `json:"expiresAt"`
	LoginTime  time.Time `json:"loginTime"`
}

// LoginRequest is the body for POST /authentication/identity/login.
// CredentialsMap is populated by the handler from the JSON "credentials" field,
// which may be either a plain string (legacy) or {"password":"..."} (AH5).
type LoginRequest struct {
	SystemName     string            `json:"systemName"`
	CredentialsMap map[string]string `json:"-"`
}

// LoginResponse is returned on successful login.
type LoginResponse struct {
	Token          string    `json:"token"`
	SystemName     string    `json:"systemName"`
	ExpirationTime time.Time `json:"expirationTime"`
	Sysop          bool      `json:"sysop"`
}

// VerifyResponse is returned by GET /authentication/identity/verify/{token}.
type VerifyResponse struct {
	Verified       bool   `json:"verified"`
	SystemName     string `json:"systemName,omitempty"`
	LoginTime      string `json:"loginTime,omitempty"`
	ExpirationTime string `json:"expirationTime,omitempty"`
	Sysop          bool   `json:"sysop"`
}
