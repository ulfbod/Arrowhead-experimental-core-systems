// Package service implements the business logic for the Blacklist core system.
package service

import (
	"time"

	"arrowhead/core/internal/blacklist/model"
	"arrowhead/core/internal/blacklist/repository"
)

// BlacklistService provides blacklist operations over a Repository.
type BlacklistService struct {
	repo repository.Repository
}

func NewBlacklistService(repo repository.Repository) *BlacklistService {
	return &BlacklistService{repo: repo}
}

// Add creates a new blacklist entry for systemName.
// expiresAt zero value means the entry never expires.
func (s *BlacklistService) Add(systemName, reason string, expiresAt time.Time, createdBy string) model.Entry {
	return s.repo.Save(model.Entry{
		SystemName: systemName,
		Reason:     reason,
		ExpiresAt:  expiresAt,
		Active:     true,
		CreatedBy:  createdBy,
	})
}

// Remove inactivates all entries for systemName (does not delete records).
func (s *BlacklistService) Remove(systemName string) bool {
	return s.repo.SetActive(systemName, false)
}

// IsBlacklisted returns true if systemName has at least one active, non-expired entry.
func (s *BlacklistService) IsBlacklisted(systemName string) bool {
	for _, e := range s.repo.All() {
		if e.SystemName != systemName || !e.Active {
			continue
		}
		if !e.ExpiresAt.IsZero() && e.ExpiresAt.Before(time.Now()) {
			continue
		}
		return true
	}
	return false
}

// QueryFilter holds optional filter parameters for Query.
type QueryFilter struct {
	SystemNames []string
	Active      *bool
}

// Query returns all entries, applying filter if non-nil.
func (s *BlacklistService) Query(filter *QueryFilter) []model.Entry {
	all := s.repo.All()
	if filter == nil {
		return all
	}
	var out []model.Entry
	for _, e := range all {
		if filter.Active != nil && e.Active != *filter.Active {
			continue
		}
		if len(filter.SystemNames) > 0 && !contains(filter.SystemNames, e.SystemName) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
