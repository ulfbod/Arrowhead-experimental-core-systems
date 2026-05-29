// Package service — AH5 registry service business logic.
//
// DO NOT MODIFY FOR EXPERIMENTS.
package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"arrowhead/core/internal/model"
	"arrowhead/core/internal/repository"
)

// AH5 validation and operation errors.
var (
	ErrMissingDeviceName            = errors.New("device name is required")
	ErrAH5SystemNameRequired         = errors.New("system name is required")
	ErrLocked                       = errors.New("entity has dependents and cannot be deleted")
	ErrMissingServiceSystemName     = errors.New("systemName is required")
	ErrMissingServiceDefinitionName = errors.New("serviceDefinitionName is required")
	ErrDeviceAlreadyExists          = errors.New("device already exists")
	ErrDeviceNotFound               = errors.New("device not found")
	ErrSystemAlreadyExists          = errors.New("system already exists")
	ErrSystemNotFound               = errors.New("system not found")
	ErrServiceDefAlreadyExists      = errors.New("service definition already exists")
	ErrInterfaceTemplateExists      = errors.New("interface template already exists")
	ErrServiceInstanceExists        = errors.New("service instance already exists")
	ErrServiceInstanceNotFound      = errors.New("service instance not found")
)

// AH5RegistryService implements the AH5 discovery and management business logic.
type AH5RegistryService struct {
	store repository.AH5StoreInterface
}

// NewAH5RegistryService returns a new AH5RegistryService backed by the given store.
func NewAH5RegistryService(store repository.AH5StoreInterface) *AH5RegistryService {
	return &AH5RegistryService{store: store}
}

// ─── Device Discovery ─────────────────────────────────────────────────────────

// RegisterDevice validates and upserts a device.
// Returns the device, true if newly created, and any validation error.
func (s *AH5RegistryService) RegisterDevice(req model.DeviceRegistrationRequest) (*model.Device, bool, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, false, ErrMissingDeviceName
	}
	d, created := s.store.SaveDevice(&req)
	return d, created, nil
}

// LookupDevices returns devices matching the optional filter criteria.
// All non-empty filters are ANDed. An empty request returns all devices.
func (s *AH5RegistryService) LookupDevices(req model.DeviceLookupRequest) model.DeviceLookupResponse {
	all := s.store.AllDevices()
	nameSet := toSet(req.DeviceNames)
	addrSet := toSet(req.Addresses)

	var matched []*model.Device
	for _, d := range all {
		if len(nameSet) > 0 {
			if _, ok := nameSet[d.Name]; !ok {
				continue
			}
		}
		if len(addrSet) > 0 && !deviceHasAddress(d, addrSet) {
			continue
		}
		if req.AddressType != "" && !deviceHasAddressType(d, req.AddressType) {
			continue
		}
		if len(req.MetadataRequirements) > 0 && !matchesMetadata(d.Metadata, req.MetadataRequirements) {
			continue
		}
		matched = append(matched, d)
	}
	if matched == nil {
		matched = []*model.Device{}
	}
	return model.DeviceLookupResponse{Entries: matched, Count: len(matched)}
}

// RevokeDevice removes the named device. Returns (false, nil) if not found,
// (false, ErrLocked) if a system still references this device.
func (s *AH5RegistryService) RevokeDevice(name string) (bool, error) {
	if s.store.HasDependentSystems(name) {
		return false, ErrLocked
	}
	return s.store.DeleteDevice(name), nil
}

// ─── System Discovery ─────────────────────────────────────────────────────────

// RegisterSystem validates and upserts a system.
func (s *AH5RegistryService) RegisterSystem(req model.SystemRegistrationRequest) (*model.AH5System, bool, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, false, ErrAH5SystemNameRequired
	}
	req.Version = normaliseVersion(req.Version)
	sys, created := s.store.SaveSystem(&req)
	return sys, created, nil
}

// LookupSystems returns systems matching the optional filter criteria.
func (s *AH5RegistryService) LookupSystems(req model.SystemLookupRequest) model.SystemLookupResponse {
	all := s.store.AllSystems()
	nameSet := toSet(req.SystemNames)
	versionSet := toSet(req.Versions)
	deviceNameSet := toSet(req.DeviceNames)
	addrSet := toSet(req.Addresses)

	var matched []*model.AH5System
	for _, sys := range all {
		if len(nameSet) > 0 {
			if _, ok := nameSet[sys.Name]; !ok {
				continue
			}
		}
		if len(versionSet) > 0 {
			if _, ok := versionSet[sys.Version]; !ok {
				continue
			}
		}
		if len(deviceNameSet) > 0 {
			devName := ""
			if sys.Device != nil {
				devName = sys.Device.Name
			}
			if _, ok := deviceNameSet[devName]; !ok {
				continue
			}
		}
		if len(addrSet) > 0 && !systemHasAddress(sys, addrSet) {
			continue
		}
		if req.AddressType != "" && !systemHasAddressType(sys, req.AddressType) {
			continue
		}
		if len(req.MetadataRequirements) > 0 && !matchesMetadata(sys.Metadata, req.MetadataRequirements) {
			continue
		}
		matched = append(matched, sys)
	}
	if matched == nil {
		matched = []*model.AH5System{}
	}
	return model.SystemLookupResponse{Entries: matched, Count: len(matched)}
}

// RevokeSystem removes the named system. Returns false if not found.
func (s *AH5RegistryService) RevokeSystem(name string) bool {
	return s.store.DeleteSystem(name)
}

// ─── Service Discovery ────────────────────────────────────────────────────────

// RegisterService validates and upserts a service instance.
func (s *AH5RegistryService) RegisterService(req model.ServiceRegistrationRequest) (*model.AH5ServiceInstance, bool, error) {
	if strings.TrimSpace(req.SystemName) == "" {
		return nil, false, ErrMissingServiceSystemName
	}
	if strings.TrimSpace(req.ServiceDefinitionName) == "" {
		return nil, false, ErrMissingServiceDefinitionName
	}
	req.Version = normaliseVersion(req.Version)
	inst, created := s.store.SaveServiceInstance(&req)
	return inst, created, nil
}

// LookupServices returns service instances matching the optional filter criteria.
func (s *AH5RegistryService) LookupServices(req model.ServiceLookupRequest) model.ServiceLookupResponse {
	all := s.store.AllServiceInstances()
	idSet := toSet(req.InstanceIDs)
	providerSet := toSet(req.ProviderNames)
	defSet := toSet(req.ServiceDefinitionNames)
	versionSet := toSet(req.Versions)
	tmplSet := toSet(req.InterfaceTemplateNames)

	var matched []*model.AH5ServiceInstance
	for _, inst := range all {
		if len(idSet) > 0 {
			if _, ok := idSet[inst.InstanceID]; !ok {
				continue
			}
		}
		if len(providerSet) > 0 {
			provName := ""
			if inst.Provider != nil {
				provName = inst.Provider.Name
			}
			if _, ok := providerSet[provName]; !ok {
				continue
			}
		}
		if len(defSet) > 0 {
			if _, ok := defSet[inst.ServiceDefinitionName]; !ok {
				continue
			}
		}
		if len(versionSet) > 0 {
			if _, ok := versionSet[inst.Version]; !ok {
				continue
			}
		}
		if len(tmplSet) > 0 && !instanceHasTemplates(inst, tmplSet) {
			continue
		}
		if len(req.MetadataRequirements) > 0 && !matchesMetadata(inst.Metadata, req.MetadataRequirements) {
			continue
		}
		matched = append(matched, inst)
	}
	if req.AlivesAt != "" {
		alivesAt, err := time.Parse(time.RFC3339, req.AlivesAt)
		if err == nil {
			var alive []*model.AH5ServiceInstance
			for _, inst := range matched {
				if inst.ExpiresAt == "" {
					alive = append(alive, inst)
					continue
				}
				exp, perr := time.Parse(time.RFC3339, inst.ExpiresAt)
				if perr != nil || !exp.Before(alivesAt) {
					alive = append(alive, inst)
				}
			}
			matched = alive
		}
	}
	if matched == nil {
		matched = []*model.AH5ServiceInstance{}
	}
	return model.ServiceLookupResponse{Entries: matched, Count: len(matched)}
}

// RevokeService removes the service instance with the given ID.
func (s *AH5RegistryService) RevokeService(instanceID string) bool {
	return s.store.DeleteServiceInstance(instanceID)
}

// ─── Management — Devices ─────────────────────────────────────────────────────

// QueryDevices returns devices matching the optional filter (reuses LookupDevices).
func (s *AH5RegistryService) QueryDevices(req model.DeviceLookupRequest) model.DeviceListResponse {
	resp := s.LookupDevices(req)
	return model.DeviceListResponse{Devices: resp.Entries, Count: resp.Count}
}

// CreateDevices creates new devices, returning an error if any already exist.
func (s *AH5RegistryService) CreateDevices(req model.DeviceListRequest) (model.DeviceListResponse, error) {
	var result []*model.Device
	for _, d := range req.Devices {
		if strings.TrimSpace(d.Name) == "" {
			return model.DeviceListResponse{}, ErrMissingDeviceName
		}
		created, ok := s.store.CreateDevice(d)
		if !ok {
			return model.DeviceListResponse{}, ErrDeviceAlreadyExists
		}
		result = append(result, created)
	}
	if result == nil {
		result = []*model.Device{}
	}
	return model.DeviceListResponse{Devices: result, Count: len(result)}, nil
}

// UpdateDevices updates existing devices, returning an error if any are not found.
func (s *AH5RegistryService) UpdateDevices(req model.DeviceListRequest) (model.DeviceListResponse, error) {
	var result []*model.Device
	for _, d := range req.Devices {
		updated, ok := s.store.UpdateDevice(d)
		if !ok {
			return model.DeviceListResponse{}, ErrDeviceNotFound
		}
		result = append(result, updated)
	}
	if result == nil {
		result = []*model.Device{}
	}
	return model.DeviceListResponse{Devices: result, Count: len(result)}, nil
}

// RemoveDevices removes the named devices (silent if not found).
func (s *AH5RegistryService) RemoveDevices(names []string) {
	for _, name := range names {
		s.store.DeleteDevice(name)
	}
}

// ─── Management — Systems ─────────────────────────────────────────────────────

// QuerySystems returns systems matching the optional filter (reuses LookupSystems).
func (s *AH5RegistryService) QuerySystems(req model.SystemLookupRequest) model.SystemListResponse {
	resp := s.LookupSystems(req)
	return model.SystemListResponse{Systems: resp.Entries, Count: resp.Count}
}

// CreateSystems creates new systems, returning an error if any already exist.
func (s *AH5RegistryService) CreateSystems(req model.SystemListRequest) (model.SystemListResponse, error) {
	var result []*model.AH5System
	for _, sys := range req.Systems {
		if strings.TrimSpace(sys.Name) == "" {
			return model.SystemListResponse{}, ErrAH5SystemNameRequired
		}
		created, ok := s.store.CreateSystem(sys)
		if !ok {
			return model.SystemListResponse{}, ErrSystemAlreadyExists
		}
		result = append(result, created)
	}
	if result == nil {
		result = []*model.AH5System{}
	}
	return model.SystemListResponse{Systems: result, Count: len(result)}, nil
}

// UpdateSystems updates existing systems, returning an error if any are not found.
func (s *AH5RegistryService) UpdateSystems(req model.SystemListRequest) (model.SystemListResponse, error) {
	var result []*model.AH5System
	for _, sys := range req.Systems {
		updated, ok := s.store.UpdateSystem(sys)
		if !ok {
			return model.SystemListResponse{}, ErrSystemNotFound
		}
		result = append(result, updated)
	}
	if result == nil {
		result = []*model.AH5System{}
	}
	return model.SystemListResponse{Systems: result, Count: len(result)}, nil
}

// RemoveSystems removes the named systems (silent if not found).
func (s *AH5RegistryService) RemoveSystems(names []string) {
	for _, name := range names {
		s.store.DeleteSystem(name)
	}
}

// ─── Management — Service Definitions ────────────────────────────────────────

// QueryServiceDefinitions returns all stored service definitions.
func (s *AH5RegistryService) QueryServiceDefinitions() model.ServiceDefinitionListResponse {
	all := s.store.AllServiceDefinitions()
	if all == nil {
		all = []*model.ServiceDefinition{}
	}
	return model.ServiceDefinitionListResponse{ServiceDefinitions: all, Count: len(all)}
}

// CreateServiceDefinitions creates new service definitions, returning an error if any exist.
func (s *AH5RegistryService) CreateServiceDefinitions(req model.ServiceDefinitionListRequest) (model.ServiceDefinitionListResponse, error) {
	defs, conflict := s.store.CreateServiceDefinitions(req.ServiceDefinitionNames)
	if conflict != "" {
		return model.ServiceDefinitionListResponse{}, ErrServiceDefAlreadyExists
	}
	if defs == nil {
		defs = []*model.ServiceDefinition{}
	}
	return model.ServiceDefinitionListResponse{ServiceDefinitions: defs, Count: len(defs)}, nil
}

// RemoveServiceDefinitions removes the named service definitions.
func (s *AH5RegistryService) RemoveServiceDefinitions(names []string) {
	s.store.DeleteServiceDefinitions(names)
}

// ─── Management — Interface Templates ────────────────────────────────────────

// QueryInterfaceTemplates returns all stored interface templates.
func (s *AH5RegistryService) QueryInterfaceTemplates() model.InterfaceTemplateListResponse {
	all := s.store.AllInterfaceTemplates()
	if all == nil {
		all = []*model.InterfaceTemplate{}
	}
	return model.InterfaceTemplateListResponse{InterfaceTemplates: all, Count: len(all)}
}

// CreateInterfaceTemplates creates new interface templates, returning an error if any exist.
func (s *AH5RegistryService) CreateInterfaceTemplates(req model.InterfaceTemplateListRequest) (model.InterfaceTemplateListResponse, error) {
	tmpls, conflict := s.store.CreateInterfaceTemplates(req.InterfaceTemplates)
	if conflict != "" {
		return model.InterfaceTemplateListResponse{}, ErrInterfaceTemplateExists
	}
	if tmpls == nil {
		tmpls = []*model.InterfaceTemplate{}
	}
	return model.InterfaceTemplateListResponse{InterfaceTemplates: tmpls, Count: len(tmpls)}, nil
}

// RemoveInterfaceTemplates removes the named interface templates.
func (s *AH5RegistryService) RemoveInterfaceTemplates(names []string) {
	s.store.DeleteInterfaceTemplates(names)
}

// ─── Management — Service Instances ──────────────────────────────────────────

// QueryServiceInstances returns service instances matching the optional filter.
func (s *AH5RegistryService) QueryServiceInstances(req model.ServiceLookupRequest) model.ServiceListResponse {
	resp := s.LookupServices(req)
	return model.ServiceListResponse{Instances: resp.Entries, Count: resp.Count}
}

// CreateServiceInstances creates new service instances, returning an error if any exist.
func (s *AH5RegistryService) CreateServiceInstances(req model.ServiceCreateListRequest) (model.ServiceListResponse, error) {
	var result []*model.AH5ServiceInstance
	for _, r := range req.Instances {
		if strings.TrimSpace(r.SystemName) == "" {
			return model.ServiceListResponse{}, ErrMissingServiceSystemName
		}
		if strings.TrimSpace(r.ServiceDefinitionName) == "" {
			return model.ServiceListResponse{}, ErrMissingServiceDefinitionName
		}
		inst, ok := s.store.CreateServiceInstance(r)
		if !ok {
			return model.ServiceListResponse{}, ErrServiceInstanceExists
		}
		result = append(result, inst)
	}
	if result == nil {
		result = []*model.AH5ServiceInstance{}
	}
	return model.ServiceListResponse{Instances: result, Count: len(result)}, nil
}

// UpdateServiceInstances updates existing service instances by instanceId.
func (s *AH5RegistryService) UpdateServiceInstances(req model.ServiceUpdateListRequest) (model.ServiceListResponse, error) {
	var result []*model.AH5ServiceInstance
	for _, r := range req.Instances {
		inst, ok := s.store.UpdateServiceInstance(r)
		if !ok {
			return model.ServiceListResponse{}, ErrServiceInstanceNotFound
		}
		result = append(result, inst)
	}
	if result == nil {
		result = []*model.AH5ServiceInstance{}
	}
	return model.ServiceListResponse{Instances: result, Count: len(result)}, nil
}

// RemoveServiceInstances removes service instances by ID (silent if not found).
func (s *AH5RegistryService) RemoveServiceInstances(ids []string) {
	s.store.DeleteServiceInstances(ids)
}

// ─── Version helpers ──────────────────────────────────────────────────────────

// normaliseVersion returns "1.0.0" when v is empty or whitespace-only.
// Used in RegisterService and RegisterSystem (discovery endpoints only).
// Management endpoints (CreateServiceInstances, UpdateServiceInstances) do NOT normalise.
func normaliseVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return "1.0.0"
	}
	return v
}

// ─── Filter helpers ───────────────────────────────────────────────────────────

func toSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(items))
	for _, v := range items {
		m[v] = struct{}{}
	}
	return m
}

func deviceHasAddress(d *model.Device, addrSet map[string]struct{}) bool {
	for _, a := range d.Addresses {
		if _, ok := addrSet[a.Address]; ok {
			return true
		}
	}
	return false
}

func deviceHasAddressType(d *model.Device, typ string) bool {
	for _, a := range d.Addresses {
		if strings.EqualFold(a.Type, typ) {
			return true
		}
	}
	return false
}

func systemHasAddress(sys *model.AH5System, addrSet map[string]struct{}) bool {
	for _, a := range sys.Addresses {
		if _, ok := addrSet[a.Address]; ok {
			return true
		}
	}
	return false
}

func systemHasAddressType(sys *model.AH5System, typ string) bool {
	for _, a := range sys.Addresses {
		if strings.EqualFold(a.Type, typ) {
			return true
		}
	}
	return false
}

func instanceHasTemplates(inst *model.AH5ServiceInstance, tmplSet map[string]struct{}) bool {
	for _, iface := range inst.Interfaces {
		if _, ok := tmplSet[iface.TemplateName]; ok {
			return true
		}
	}
	return false
}

// matchesMetadata returns true if all requirements in reqs are satisfied by
// the metadata map m.  An empty reqs map matches everything.
func matchesMetadata(m map[string]string, reqs map[string]model.MetadataRequirement) bool {
	for key, req := range reqs {
		actual, exists := m[key]
		if !exists {
			return false
		}
		op := req.Op
		if op == "" {
			op = model.OpEqualsTo
		}
		valStr := fmt.Sprintf("%v", req.Value)
		switch op {
		case model.OpEqualsTo:
			if actual != valStr {
				return false
			}
		case model.OpNotEqualsTo:
			if actual == valStr {
				return false
			}
		case model.OpContains:
			if !strings.Contains(actual, valStr) {
				return false
			}
		case model.OpNotContains:
			if strings.Contains(actual, valStr) {
				return false
			}
		case model.OpLessThanOrEqualsTo:
			a, err1 := strconv.ParseFloat(actual, 64)
			v, err2 := strconv.ParseFloat(valStr, 64)
			if err1 != nil || err2 != nil || a > v {
				return false
			}
		case model.OpGreaterThanOrEqualsTo:
			a, err1 := strconv.ParseFloat(actual, 64)
			v, err2 := strconv.ParseFloat(valStr, 64)
			if err1 != nil || err2 != nil || a < v {
				return false
			}
		default:
			return false
		}
	}
	return true
}
