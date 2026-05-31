// Package repository — AH5StoreInterface defines the storage contract for
// AH5 ServiceRegistry entities. Both AH5Store (in-memory) and AH5SQLiteStore
// implement this interface.
package repository

import "arrowhead/core/internal/model"

// AH5StoreInterface is the storage contract for AH5 ServiceRegistry.
type AH5StoreInterface interface {
	// Devices
	SaveDevice(req *model.DeviceRegistrationRequest) (*model.Device, bool)
	GetDevice(name string) *model.Device
	AllDevices() []*model.Device
	DeleteDevice(name string) bool
	CreateDevice(req *model.DeviceRegistrationRequest) (*model.Device, bool)
	UpdateDevice(req *model.DeviceRegistrationRequest) (*model.Device, bool)
	HasDependentSystems(deviceName string) bool

	// Systems
	SaveSystem(req *model.SystemRegistrationRequest) (*model.AH5System, bool)
	GetSystem(name string) *model.AH5System
	AllSystems() []*model.AH5System
	DeleteSystem(name string) bool
	CreateSystem(req *model.SystemRegistrationRequest) (*model.AH5System, bool)
	UpdateSystem(req *model.SystemRegistrationRequest) (*model.AH5System, bool)

	// ServiceDefinitions
	SaveServiceDefinitions(names []string) []*model.ServiceDefinition
	CreateServiceDefinitions(names []string) ([]*model.ServiceDefinition, string)
	UpdateServiceDefinitions(names []string) ([]*model.ServiceDefinition, bool)
	AllServiceDefinitions() []*model.ServiceDefinition
	DeleteServiceDefinitions(names []string)

	// InterfaceTemplates
	CreateInterfaceTemplates(templates []*model.InterfaceTemplate) ([]*model.InterfaceTemplate, string)
	UpdateInterfaceTemplates(templates []*model.InterfaceTemplate) ([]*model.InterfaceTemplate, bool)
	AllInterfaceTemplates() []*model.InterfaceTemplate
	DeleteInterfaceTemplates(names []string)

	// ServiceInstances
	SaveServiceInstance(req *model.ServiceRegistrationRequest) (*model.AH5ServiceInstance, bool)
	CreateServiceInstance(req *model.ServiceCreateRequest) (*model.AH5ServiceInstance, bool)
	UpdateServiceInstance(req *model.ServiceUpdateRequest) (*model.AH5ServiceInstance, bool)
	AllServiceInstances() []*model.AH5ServiceInstance
	DeleteServiceInstance(id string) bool
	DeleteServiceInstances(ids []string)
}
