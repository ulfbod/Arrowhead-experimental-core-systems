// Package repository — AH5 in-memory store for the extended ServiceRegistry.
package repository

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"arrowhead/core/internal/model"
)

// AH5Store is a thread-safe in-memory store for AH5 entities:
// devices, systems, service definitions, interface templates, and service instances.
type AH5Store struct {
	mu                 sync.RWMutex
	devices            map[string]*model.Device
	systems            map[string]*model.AH5System
	serviceDefinitions map[string]*model.ServiceDefinition
	interfaceTemplates map[string]*model.InterfaceTemplate
	// serviceInstances keyed by instanceId (string counter).
	serviceInstances map[string]*model.AH5ServiceInstance
	counter          atomic.Int64
}

// NewAH5Store returns an empty AH5Store.
func NewAH5Store() *AH5Store {
	return &AH5Store{
		devices:            make(map[string]*model.Device),
		systems:            make(map[string]*model.AH5System),
		serviceDefinitions: make(map[string]*model.ServiceDefinition),
		interfaceTemplates: make(map[string]*model.InterfaceTemplate),
		serviceInstances:   make(map[string]*model.AH5ServiceInstance),
	}
}

func ah5Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ─── Devices ─────────────────────────────────────────────────────────────────

// SaveDevice upserts a device. Returns the stored device and true if newly created.
func (s *AH5Store) SaveDevice(req *model.DeviceRegistrationRequest) (*model.Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := ah5Now()
	if d, ok := s.devices[req.Name]; ok {
		d.Metadata = req.Metadata
		d.Addresses = req.Addresses
		d.UpdatedAt = t
		return d, false
	}
	d := &model.Device{
		Name:      req.Name,
		Metadata:  req.Metadata,
		Addresses: req.Addresses,
		CreatedAt: t,
		UpdatedAt: t,
	}
	s.devices[req.Name] = d
	return d, true
}

// GetDevice returns the device with the given name, or nil.
func (s *AH5Store) GetDevice(name string) *model.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.devices[name]
}

// AllDevices returns all stored devices.
func (s *AH5Store) AllDevices() []*model.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, d)
	}
	return out
}

// DeleteDevice removes the named device. Returns false if not found.
func (s *AH5Store) DeleteDevice(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[name]; !ok {
		return false
	}
	delete(s.devices, name)
	return true
}

// CreateDevice creates a new device, failing if one already exists.
// Returns the device and true on success; nil and false if already present.
func (s *AH5Store) CreateDevice(req *model.DeviceRegistrationRequest) (*model.Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[req.Name]; ok {
		return nil, false
	}
	t := ah5Now()
	d := &model.Device{
		Name:      req.Name,
		Metadata:  req.Metadata,
		Addresses: req.Addresses,
		CreatedAt: t,
		UpdatedAt: t,
	}
	s.devices[req.Name] = d
	return d, true
}

// UpdateDevice updates an existing device. Returns the device and true on success;
// nil and false if not found.
func (s *AH5Store) UpdateDevice(req *model.DeviceRegistrationRequest) (*model.Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[req.Name]
	if !ok {
		return nil, false
	}
	d.Metadata = req.Metadata
	d.Addresses = req.Addresses
	d.UpdatedAt = ah5Now()
	return d, true
}

// ─── Systems ──────────────────────────────────────────────────────────────────

// SaveSystem upserts a system. Returns the stored system and true if newly created.
// If DeviceName is set, the device is looked up and embedded (nil if not found).
func (s *AH5Store) SaveSystem(req *model.SystemRegistrationRequest) (*model.AH5System, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := ah5Now()
	var dev *model.Device
	if req.DeviceName != "" {
		dev = s.devices[req.DeviceName]
	}
	if sys, ok := s.systems[req.Name]; ok {
		sys.Metadata = req.Metadata
		sys.Version = req.Version
		sys.Addresses = req.Addresses
		sys.Device = dev
		sys.UpdatedAt = t
		return sys, false
	}
	sys := &model.AH5System{
		Name:      req.Name,
		Metadata:  req.Metadata,
		Version:   req.Version,
		Addresses: req.Addresses,
		Device:    dev,
		CreatedAt: t,
		UpdatedAt: t,
	}
	s.systems[req.Name] = sys
	return sys, true
}

// GetSystem returns the named system, or nil.
func (s *AH5Store) GetSystem(name string) *model.AH5System {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.systems[name]
}

// AllSystems returns all stored systems.
func (s *AH5Store) AllSystems() []*model.AH5System {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.AH5System, 0, len(s.systems))
	for _, sys := range s.systems {
		out = append(out, sys)
	}
	return out
}

// DeleteSystem removes the named system. Returns false if not found.
func (s *AH5Store) DeleteSystem(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.systems[name]; !ok {
		return false
	}
	delete(s.systems, name)
	return true
}

// CreateSystem creates a new system, failing if one already exists.
func (s *AH5Store) CreateSystem(req *model.SystemRegistrationRequest) (*model.AH5System, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.systems[req.Name]; ok {
		return nil, false
	}
	t := ah5Now()
	var dev *model.Device
	if req.DeviceName != "" {
		dev = s.devices[req.DeviceName]
	}
	sys := &model.AH5System{
		Name:      req.Name,
		Metadata:  req.Metadata,
		Version:   req.Version,
		Addresses: req.Addresses,
		Device:    dev,
		CreatedAt: t,
		UpdatedAt: t,
	}
	s.systems[req.Name] = sys
	return sys, true
}

// UpdateSystem updates an existing system. Returns false if not found.
func (s *AH5Store) UpdateSystem(req *model.SystemRegistrationRequest) (*model.AH5System, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sys, ok := s.systems[req.Name]
	if !ok {
		return nil, false
	}
	var dev *model.Device
	if req.DeviceName != "" {
		dev = s.devices[req.DeviceName]
	}
	sys.Metadata = req.Metadata
	sys.Version = req.Version
	sys.Addresses = req.Addresses
	sys.Device = dev
	sys.UpdatedAt = ah5Now()
	return sys, true
}

// ─── Service Definitions ──────────────────────────────────────────────────────

// SaveServiceDefinitions upserts service definitions (no-op if already present).
// Returns all definitions by name (existing or newly created).
func (s *AH5Store) SaveServiceDefinitions(names []string) []*model.ServiceDefinition {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := ah5Now()
	out := make([]*model.ServiceDefinition, 0, len(names))
	for _, name := range names {
		if _, ok := s.serviceDefinitions[name]; !ok {
			s.serviceDefinitions[name] = &model.ServiceDefinition{
				Name:      name,
				CreatedAt: t,
				UpdatedAt: t,
			}
		}
		out = append(out, s.serviceDefinitions[name])
	}
	return out
}

// CreateServiceDefinitions creates service definitions, failing if any already exist.
// Returns nil and the conflicting name on failure.
func (s *AH5Store) CreateServiceDefinitions(names []string) ([]*model.ServiceDefinition, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range names {
		if _, ok := s.serviceDefinitions[name]; ok {
			return nil, name
		}
	}
	t := ah5Now()
	out := make([]*model.ServiceDefinition, 0, len(names))
	for _, name := range names {
		def := &model.ServiceDefinition{Name: name, CreatedAt: t, UpdatedAt: t}
		s.serviceDefinitions[name] = def
		out = append(out, def)
	}
	return out, ""
}

// AllServiceDefinitions returns all stored service definitions.
func (s *AH5Store) AllServiceDefinitions() []*model.ServiceDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.ServiceDefinition, 0, len(s.serviceDefinitions))
	for _, def := range s.serviceDefinitions {
		out = append(out, def)
	}
	return out
}

// DeleteServiceDefinitions removes the named service definitions.
func (s *AH5Store) DeleteServiceDefinitions(names []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range names {
		delete(s.serviceDefinitions, name)
	}
}

// ─── Interface Templates ──────────────────────────────────────────────────────

// CreateInterfaceTemplates creates interface templates, failing if any already exist.
// Returns nil and the conflicting name on failure.
func (s *AH5Store) CreateInterfaceTemplates(templates []*model.InterfaceTemplate) ([]*model.InterfaceTemplate, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tmpl := range templates {
		if _, ok := s.interfaceTemplates[tmpl.Name]; ok {
			return nil, tmpl.Name
		}
	}
	t := ah5Now()
	out := make([]*model.InterfaceTemplate, 0, len(templates))
	for _, tmpl := range templates {
		stored := &model.InterfaceTemplate{
			Name:                 tmpl.Name,
			Protocol:             tmpl.Protocol,
			PropertyRequirements: tmpl.PropertyRequirements,
			CreatedAt:            t,
			UpdatedAt:            t,
		}
		s.interfaceTemplates[tmpl.Name] = stored
		out = append(out, stored)
	}
	return out, ""
}

// AllInterfaceTemplates returns all stored interface templates.
func (s *AH5Store) AllInterfaceTemplates() []*model.InterfaceTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.InterfaceTemplate, 0, len(s.interfaceTemplates))
	for _, tmpl := range s.interfaceTemplates {
		out = append(out, tmpl)
	}
	return out
}

// DeleteInterfaceTemplates removes the named interface templates.
func (s *AH5Store) DeleteInterfaceTemplates(names []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range names {
		delete(s.interfaceTemplates, name)
	}
}

// ─── Service Instances ────────────────────────────────────────────────────────

// SaveServiceInstance upserts a service instance keyed by
// (systemName, serviceDefinitionName, version). Returns the instance and true if
// newly created (false if updated).
func (s *AH5Store) SaveServiceInstance(req *model.ServiceRegistrationRequest) (*model.AH5ServiceInstance, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := ah5Now()

	provider := s.providerFor(req.SystemName, t)

	// Linear scan to find by natural key (small scale — in-memory research system).
	for _, inst := range s.serviceInstances {
		if inst.Provider != nil && inst.Provider.Name == req.SystemName &&
			inst.ServiceDefinitionName == req.ServiceDefinitionName &&
			inst.Version == req.Version {
			inst.ExpiresAt = req.ExpiresAt
			inst.Metadata = req.Metadata
			inst.Interfaces = req.Interfaces
			inst.Provider = provider
			inst.UpdatedAt = t
			return inst, false
		}
	}
	id := fmt.Sprintf("%d", s.counter.Add(1))
	inst := &model.AH5ServiceInstance{
		InstanceID:            id,
		Provider:              provider,
		ServiceDefinitionName: req.ServiceDefinitionName,
		Version:               req.Version,
		ExpiresAt:             req.ExpiresAt,
		Metadata:              req.Metadata,
		Interfaces:            req.Interfaces,
		CreatedAt:             t,
		UpdatedAt:             t,
	}
	s.serviceInstances[id] = inst
	return inst, true
}

// CreateServiceInstance creates a new service instance without upsert behaviour
// (management create). Returns nil and false if the natural key already exists.
func (s *AH5Store) CreateServiceInstance(req *model.ServiceCreateRequest) (*model.AH5ServiceInstance, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := ah5Now()

	for _, inst := range s.serviceInstances {
		if inst.Provider != nil && inst.Provider.Name == req.SystemName &&
			inst.ServiceDefinitionName == req.ServiceDefinitionName &&
			inst.Version == req.Version {
			return nil, false
		}
	}
	provider := s.providerFor(req.SystemName, t)
	id := fmt.Sprintf("%d", s.counter.Add(1))
	inst := &model.AH5ServiceInstance{
		InstanceID:            id,
		Provider:              provider,
		ServiceDefinitionName: req.ServiceDefinitionName,
		Version:               req.Version,
		ExpiresAt:             req.ExpiresAt,
		Metadata:              req.Metadata,
		Interfaces:            req.Interfaces,
		CreatedAt:             t,
		UpdatedAt:             t,
	}
	s.serviceInstances[id] = inst
	return inst, true
}

// UpdateServiceInstance updates an existing instance by instanceId.
// Returns false if not found.
func (s *AH5Store) UpdateServiceInstance(req *model.ServiceUpdateRequest) (*model.AH5ServiceInstance, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inst, ok := s.serviceInstances[req.InstanceID]
	if !ok {
		return nil, false
	}
	inst.ExpiresAt = req.ExpiresAt
	inst.Metadata = req.Metadata
	inst.Interfaces = req.Interfaces
	inst.UpdatedAt = ah5Now()
	return inst, true
}

// AllServiceInstances returns all stored service instances.
func (s *AH5Store) AllServiceInstances() []*model.AH5ServiceInstance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.AH5ServiceInstance, 0, len(s.serviceInstances))
	for _, inst := range s.serviceInstances {
		out = append(out, inst)
	}
	return out
}

// DeleteServiceInstance removes the instance with the given ID.
// Returns false if not found.
func (s *AH5Store) DeleteServiceInstance(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.serviceInstances[id]; !ok {
		return false
	}
	delete(s.serviceInstances, id)
	return true
}

// DeleteServiceInstances removes multiple instances by ID (silent if not found).
func (s *AH5Store) DeleteServiceInstances(ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.serviceInstances, id)
	}
}

// providerFor returns the registered AH5System for systemName, or a minimal
// stub when the system has not been explicitly registered via system-discovery.
// Must be called with s.mu held.
func (s *AH5Store) providerFor(systemName, t string) *model.AH5System {
	if sys, ok := s.systems[systemName]; ok {
		return sys
	}
	return &model.AH5System{Name: systemName, CreatedAt: t, UpdatedAt: t}
}
