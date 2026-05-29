// Package repository provides persistence for the Certificate Authority.
// The CA persists its revocation list and next-serial counter so that
// revocations survive service restarts.
package repository

import "time"

// Revocation records a revoked certificate.
type Revocation struct {
	Serial     string
	SystemName string
	RevokedAt  time.Time
}

// Repository is the storage contract for the CA.
type Repository interface {
	// NextSerial returns the current next-serial counter value.
	NextSerial() int64
	// IncrementSerial atomically increments and returns the new serial.
	IncrementSerial() int64
	// AddRevocation records a new revocation (idempotent on duplicate serial).
	AddRevocation(serial, systemName string, revokedAt time.Time)
	// IsRevoked returns true if the serial has been revoked.
	IsRevoked(serial string) bool
	// AllRevocations returns all recorded revocations ordered by revokedAt.
	AllRevocations() []Revocation
}
