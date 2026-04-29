// Package model defines the Certificate Authority domain types.
package model

import "time"

// IssueRequest is the body for POST /ca/certificate/issue.
type IssueRequest struct {
	SystemName string `json:"systemName"`
	ValidDays  int    `json:"validDays,omitempty"` // 0 = service default
}

// IssuedCert is returned by a successful certificate issue.
type IssuedCert struct {
	SystemName  string    `json:"systemName"`
	Certificate string    `json:"certificate"` // PEM-encoded X.509 certificate
	PrivateKey  string    `json:"privateKey"`  // PEM-encoded EC private key
	IssuedAt    time.Time `json:"issuedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// CAInfo is returned by GET /ca/info.
type CAInfo struct {
	CommonName  string `json:"commonName"`
	Certificate string `json:"certificate"` // PEM-encoded CA certificate
}
