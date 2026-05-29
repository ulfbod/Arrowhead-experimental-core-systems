// Package model defines shared types for the Blacklist core system.
package model

import "time"

// Entry represents a blacklist record.
type Entry struct {
	SystemName string    `json:"systemName"`
	Reason     string    `json:"reason"`
	ExpiresAt  time.Time `json:"-"` // zero = never expires
	Active     bool      `json:"active"`
	CreatedBy  string    `json:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
