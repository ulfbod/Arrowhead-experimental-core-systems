// Package service contains the business logic for the Arrowhead Core Service Registry.
//
// DO NOT MODIFY FOR EXPERIMENTS.
// All validation, matching, and registration logic is governed by SPEC.md.
// Changes here require a corresponding SPEC.md update.
package service

import (
	"errors"
	"strings"

	"arrowhead/core/internal/model"
	"arrowhead/core/internal/repository"
)

var (
	ErrMissingServiceDefinition = errors.New("serviceDefinition is required")
	ErrMissingProviderSystem    = errors.New("providerSystem is required")
	ErrMissingSystemName        = errors.New("providerSystem.systemName is required")
	ErrMissingAddress           = errors.New("providerSystem.address is required")
	ErrInvalidPort              = errors.New("providerSystem.port must be > 0")
	ErrMissingServiceUri        = errors.New("serviceUri is required")
	ErrMissingInterfaces        = errors.New("interfaces must contain at least one entry")
	ErrServiceNotFound          = errors.New("service not found")
)

// RegistryService implements the Service Registry business logic.
type RegistryService struct {
	repo repository.Repository
}

func NewRegistryService(repo repository.Repository) *RegistryService {
	return &RegistryService{repo: repo}
}

// Register validates the request and stores the service instance.
// Duplicate registrations (same serviceDefinition + providerSystem + version) overwrite the existing entry.
func (s *RegistryService) Register(req model.RegisterRequest) (*model.ServiceInstance, error) {
	if strings.TrimSpace(req.ServiceDefinition) == "" {
		return nil, ErrMissingServiceDefinition
	}
	if req.ProviderSystem == nil {
		return nil, ErrMissingProviderSystem
	}
	if strings.TrimSpace(req.ProviderSystem.SystemName) == "" {
		return nil, ErrMissingSystemName
	}
	if strings.TrimSpace(req.ProviderSystem.Address) == "" {
		return nil, ErrMissingAddress
	}
	if req.ProviderSystem.Port <= 0 {
		return nil, ErrInvalidPort
	}
	if strings.TrimSpace(req.ServiceUri) == "" {
		return nil, ErrMissingServiceUri
	}
	if len(req.Interfaces) == 0 {
		return nil, ErrMissingInterfaces
	}

	version := req.Version
	if version <= 0 {
		version = 1
	}

	svc := &model.ServiceInstance{
		ServiceDefinition: req.ServiceDefinition,
		ProviderSystem:    *req.ProviderSystem,
		ServiceUri:        req.ServiceUri,
		Interfaces:        req.Interfaces,
		Version:           version,
		Metadata:          req.Metadata,
		Secure:            req.Secure,
	}
	return s.repo.Save(svc), nil
}

// Unregister removes a service instance identified by the natural key.
func (s *RegistryService) Unregister(req model.UnregisterRequest) error {
	if strings.TrimSpace(req.ServiceDefinition) == "" {
		return ErrMissingServiceDefinition
	}
	if req.ProviderSystem == nil {
		return ErrMissingProviderSystem
	}
	if strings.TrimSpace(req.ProviderSystem.SystemName) == "" {
		return ErrMissingSystemName
	}
	if strings.TrimSpace(req.ProviderSystem.Address) == "" {
		return ErrMissingAddress
	}
	if req.ProviderSystem.Port <= 0 {
		return ErrInvalidPort
	}
	version := req.Version
	if version <= 0 {
		version = 1
	}
	if !s.repo.Delete(req.ServiceDefinition, req.ProviderSystem.SystemName, req.ProviderSystem.Address, req.ProviderSystem.Port, version) {
		return ErrServiceNotFound
	}
	return nil
}

// Query returns all service instances matching the filter criteria.
func (s *RegistryService) Query(req model.QueryRequest) model.QueryResponse {
	all := s.repo.All()
	unfilteredHits := len(all)

	var results []*model.ServiceInstance
	for _, svc := range all {
		if req.ServiceDefinition != "" && svc.ServiceDefinition != req.ServiceDefinition {
			continue
		}
		if len(req.Interfaces) > 0 && !hasAllInterfaces(svc.Interfaces, req.Interfaces) {
			continue
		}
		if len(req.Metadata) > 0 && !hasAllMetadata(svc.Metadata, req.Metadata) {
			continue
		}
		if req.VersionRequirement > 0 && svc.Version != req.VersionRequirement {
			continue
		}
		results = append(results, svc)
	}
	if results == nil {
		results = []*model.ServiceInstance{}
	}
	return model.QueryResponse{
		ServiceQueryData: results,
		UnfilteredHits:   unfilteredHits,
	}
}

func hasAllInterfaces(svcIfaces, required []string) bool {
	set := make(map[string]struct{}, len(svcIfaces))
	for _, i := range svcIfaces {
		set[strings.ToUpper(i)] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[strings.ToUpper(r)]; !ok {
			return false
		}
	}
	return true
}

func hasAllMetadata(svcMeta, required map[string]string) bool {
	for k, v := range required {
		if svcMeta[k] != v {
			return false
		}
	}
	return true
}
