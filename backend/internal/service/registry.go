package service

import (
	"errors"
	"mineio/internal/model"
	"mineio/internal/repository"
	"strings"
)

var (
	ErrMissingServiceDefinition = errors.New("serviceDefinition is required")
	ErrMissingProviderSystem    = errors.New("providerSystem is required")
	ErrMissingSystemName        = errors.New("providerSystem.systemName is required")
	ErrMissingAddress           = errors.New("providerSystem.address is required")
	ErrInvalidPort              = errors.New("providerSystem.port must be > 0")
	ErrMissingServiceUri        = errors.New("serviceUri is required")
	ErrMissingInterfaces        = errors.New("interfaces must contain at least one entry")
)

// RegistryService implements the Service Registry business logic.
type RegistryService struct {
	repo repository.Repository
}

func NewRegistryService(repo repository.Repository) *RegistryService {
	return &RegistryService{repo: repo}
}

// Register validates the request and stores the service instance.
// Duplicate registrations (same definition + provider) overwrite the existing entry.
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

// Query returns all service instances matching the filter criteria.
// All non-zero fields in the request are applied as AND filters:
//   - ServiceDefinition: exact match
//   - Interfaces:        service must provide ALL requested interfaces (case-insensitive)
//   - Metadata:          service must contain ALL requested key-value pairs
//   - VersionRequirement: service version must equal the requested value
//
// An empty request returns all registered services.
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

// hasAllInterfaces returns true if svcIfaces contains every interface in required (case-insensitive).
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

// hasAllMetadata returns true if svcMeta contains every key-value pair in required.
func hasAllMetadata(svcMeta, required map[string]string) bool {
	for k, v := range required {
		if svcMeta[k] != v {
			return false
		}
	}
	return true
}
