// Package service implements SimpleStoreServiceOrchestration business logic.
//
// AH5 responsibility: find matching service instances using pre-configured
// peer-to-peer orchestration rules (simple-store strategy).
package service

import (
	"errors"
	"strings"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/simplestore/model"
	"arrowhead/core/internal/orchestration/simplestore/repository"
)

var (
	ErrMissingConsumer   = errors.New("consumerSystemName is required")
	ErrMissingService    = errors.New("serviceDefinition is required")
	ErrMissingProvider   = errors.New("provider.systemName is required")
	ErrMissingServiceUri = errors.New("serviceUri is required")
	ErrMissingInterfaces = errors.New("interfaces must contain at least one entry")
	ErrRuleNotFound      = errors.New("rule not found")
)

type SimpleStoreOrchestrator struct {
	repo repository.Repository
}

func NewSimpleStoreOrchestrator(repo repository.Repository) *SimpleStoreOrchestrator {
	return &SimpleStoreOrchestrator{repo: repo}
}

// Orchestrate performs a pull: matches store rules to the consumer's request.
func (o *SimpleStoreOrchestrator) Orchestrate(req orchmodel.OrchestrationRequest) (orchmodel.OrchestrationResponse, error) {
	if req.RequesterSystem.SystemName == "" {
		return orchmodel.OrchestrationResponse{}, errors.New("requesterSystem.systemName is required")
	}
	if req.RequestedService.ServiceDefinition == "" {
		return orchmodel.OrchestrationResponse{}, errors.New("requestedService.serviceDefinition is required")
	}

	var results []orchmodel.OrchestrationResult
	for _, rule := range o.repo.All() {
		if rule.ConsumerSystemName != req.RequesterSystem.SystemName {
			continue
		}
		if rule.ServiceDefinition != req.RequestedService.ServiceDefinition {
			continue
		}
		results = append(results, orchmodel.OrchestrationResult{
			Provider: rule.Provider,
			Service: orchmodel.ServiceInfo{
				ServiceDefinition: rule.ServiceDefinition,
				ServiceUri:        rule.ServiceUri,
				Interfaces:        rule.Interfaces,
				Metadata:          rule.Metadata,
			},
		})
	}
	if results == nil {
		results = []orchmodel.OrchestrationResult{}
	}
	return orchmodel.OrchestrationResponse{Response: results}, nil
}

// CreateRule validates and stores a new orchestration rule.
func (o *SimpleStoreOrchestrator) CreateRule(req model.CreateRuleRequest) (model.StoreRule, error) {
	if strings.TrimSpace(req.ConsumerSystemName) == "" {
		return model.StoreRule{}, ErrMissingConsumer
	}
	if strings.TrimSpace(req.ServiceDefinition) == "" {
		return model.StoreRule{}, ErrMissingService
	}
	if strings.TrimSpace(req.Provider.SystemName) == "" {
		return model.StoreRule{}, ErrMissingProvider
	}
	if strings.TrimSpace(req.ServiceUri) == "" {
		return model.StoreRule{}, ErrMissingServiceUri
	}
	if len(req.Interfaces) == 0 {
		return model.StoreRule{}, ErrMissingInterfaces
	}
	rule := model.StoreRule{
		ConsumerSystemName: req.ConsumerSystemName,
		ServiceDefinition:  req.ServiceDefinition,
		Provider:           req.Provider,
		ServiceUri:         req.ServiceUri,
		Interfaces:         req.Interfaces,
		Metadata:           req.Metadata,
	}
	return o.repo.Save(rule), nil
}

// DeleteRule removes a rule by ID.
func (o *SimpleStoreOrchestrator) DeleteRule(id int64) error {
	if !o.repo.Delete(id) {
		return ErrRuleNotFound
	}
	return nil
}

// ListRules returns all stored rules.
func (o *SimpleStoreOrchestrator) ListRules() model.RulesResponse {
	all := o.repo.All()
	if all == nil {
		all = []model.StoreRule{}
	}
	return model.RulesResponse{Rules: all, Count: len(all)}
}
