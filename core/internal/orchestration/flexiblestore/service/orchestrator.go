// Package service implements FlexibleStoreServiceOrchestration business logic.
//
// AH5 note: FlexibleStore docs are "Coming soon" (see GAP_ANALYSIS.md).
// This design extends SimpleStore with:
//   - priority ordering (lower priority value = returned first)
//   - optional metadata filter matching on rules
package service

import (
	"errors"
	"sort"
	"strings"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/flexiblestore/model"
	"arrowhead/core/internal/orchestration/flexiblestore/repository"
)

var (
	ErrMissingConsumer   = errors.New("consumerSystemName is required")
	ErrMissingService    = errors.New("serviceDefinition is required")
	ErrMissingProvider   = errors.New("provider.systemName is required")
	ErrMissingServiceUri = errors.New("serviceUri is required")
	ErrMissingInterfaces = errors.New("interfaces must contain at least one entry")
	ErrRuleNotFound      = errors.New("rule not found")
)

type FlexibleStoreOrchestrator struct {
	repo repository.Repository
}

func NewFlexibleStoreOrchestrator(repo repository.Repository) *FlexibleStoreOrchestrator {
	return &FlexibleStoreOrchestrator{repo: repo}
}

// Orchestrate performs a priority-ordered pull using store rules.
func (o *FlexibleStoreOrchestrator) Orchestrate(req orchmodel.OrchestrationRequest) (orchmodel.OrchestrationResponse, error) {
	if req.RequesterSystem.SystemName == "" {
		return orchmodel.OrchestrationResponse{}, errors.New("requesterSystem.systemName is required")
	}
	if req.RequestedService.ServiceDefinition == "" {
		return orchmodel.OrchestrationResponse{}, errors.New("requestedService.serviceDefinition is required")
	}

	var matched []model.FlexibleRule
	for _, rule := range o.repo.All() {
		if rule.ConsumerSystemName != req.RequesterSystem.SystemName {
			continue
		}
		if rule.ServiceDefinition != req.RequestedService.ServiceDefinition {
			continue
		}
		if !metadataSubset(req.RequestedService.Metadata, rule.MetadataFilter) {
			continue
		}
		matched = append(matched, rule)
	}

	// Sort by priority ascending (lower = higher priority).
	sort.Slice(matched, func(i, j int) bool {
		pi, pj := matched[i].Priority, matched[j].Priority
		if pi == 0 {
			pi = 1<<31 - 1
		}
		if pj == 0 {
			pj = 1<<31 - 1
		}
		return pi < pj
	})

	results := make([]orchmodel.OrchestrationResult, 0, len(matched))
	for _, rule := range matched {
		results = append(results, orchmodel.OrchestrationResult{
			Provider: rule.Provider,
			Service: orchmodel.ServiceInfo{
				ServiceDefinition: rule.ServiceDefinition,
				ServiceUri:        rule.ServiceUri,
				Interfaces:        rule.Interfaces,
			},
			Priority: rule.Priority,
		})
	}
	return orchmodel.OrchestrationResponse{Response: results}, nil
}

// CreateRule validates and stores a flexible orchestration rule.
func (o *FlexibleStoreOrchestrator) CreateRule(req model.CreateFlexibleRuleRequest) (model.FlexibleRule, error) {
	if strings.TrimSpace(req.ConsumerSystemName) == "" {
		return model.FlexibleRule{}, ErrMissingConsumer
	}
	if strings.TrimSpace(req.ServiceDefinition) == "" {
		return model.FlexibleRule{}, ErrMissingService
	}
	if strings.TrimSpace(req.Provider.SystemName) == "" {
		return model.FlexibleRule{}, ErrMissingProvider
	}
	if strings.TrimSpace(req.ServiceUri) == "" {
		return model.FlexibleRule{}, ErrMissingServiceUri
	}
	if len(req.Interfaces) == 0 {
		return model.FlexibleRule{}, ErrMissingInterfaces
	}
	rule := model.FlexibleRule{
		ConsumerSystemName: req.ConsumerSystemName,
		ServiceDefinition:  req.ServiceDefinition,
		Provider:           req.Provider,
		ServiceUri:         req.ServiceUri,
		Interfaces:         req.Interfaces,
		Priority:           req.Priority,
		MetadataFilter:     req.MetadataFilter,
	}
	return o.repo.Save(rule), nil
}

func (o *FlexibleStoreOrchestrator) DeleteRule(id int64) error {
	if !o.repo.Delete(id) {
		return ErrRuleNotFound
	}
	return nil
}

func (o *FlexibleStoreOrchestrator) ListRules() model.RulesResponse {
	all := o.repo.All()
	if all == nil {
		all = []model.FlexibleRule{}
	}
	return model.RulesResponse{Rules: all, Count: len(all)}
}

// metadataSubset returns true if requestMeta contains all key-value pairs in filter.
// An empty or nil filter always matches.
func metadataSubset(requestMeta, filter map[string]string) bool {
	for k, v := range filter {
		if requestMeta[k] != v {
			return false
		}
	}
	return true
}
