// Package api — AH5 HTTP handlers for the extended ServiceRegistry.
//
// Endpoints registered here implement the AH5 discovery and management surfaces.
// They are mounted on more-specific path prefixes than the legacy handler, so
// Go's ServeMux routes them first:
//
//	/serviceregistry/device-discovery/*
//	/serviceregistry/system-discovery/*
//	/serviceregistry/service-discovery/*
//	/serviceregistry/mgmt/*
//
// DO NOT MODIFY FOR EXPERIMENTS.
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"arrowhead/core/internal/model"
	"arrowhead/core/internal/service"
)

// AH5Handler wires HTTP routes to AH5RegistryService.
type AH5Handler struct {
	svc *service.AH5RegistryService
}

// NewAH5Handler returns an http.Handler that handles all AH5 discovery and
// management routes.
func NewAH5Handler(svc *service.AH5RegistryService) http.Handler {
	h := &AH5Handler{svc: svc}
	mux := http.NewServeMux()

	// Device discovery
	mux.HandleFunc("/serviceregistry/device-discovery/register", h.handleDeviceRegister)
	mux.HandleFunc("/serviceregistry/device-discovery/lookup", h.handleDeviceLookup)
	mux.HandleFunc("/serviceregistry/device-discovery/revoke/", h.handleDeviceRevoke)

	// System discovery
	mux.HandleFunc("/serviceregistry/system-discovery/register", h.handleSystemRegister)
	mux.HandleFunc("/serviceregistry/system-discovery/lookup", h.handleSystemLookup)
	mux.HandleFunc("/serviceregistry/system-discovery/revoke", h.handleSystemRevoke)

	// Service discovery
	mux.HandleFunc("/serviceregistry/service-discovery/register", h.handleServiceRegister)
	mux.HandleFunc("/serviceregistry/service-discovery/lookup", h.handleServiceLookup)
	mux.HandleFunc("/serviceregistry/service-discovery/revoke/", h.handleServiceRevoke)

	// Management — devices
	mux.HandleFunc("/serviceregistry/mgmt/devices/query", h.handleMgmtDevicesQuery)
	mux.HandleFunc("/serviceregistry/mgmt/devices", h.handleMgmtDevices)

	// Management — systems
	mux.HandleFunc("/serviceregistry/mgmt/systems/query", h.handleMgmtSystemsQuery)
	mux.HandleFunc("/serviceregistry/mgmt/systems", h.handleMgmtSystems)

	// Management — service definitions
	mux.HandleFunc("/serviceregistry/mgmt/service-definitions/query", h.handleMgmtServiceDefsQuery)
	mux.HandleFunc("/serviceregistry/mgmt/service-definitions", h.handleMgmtServiceDefs)

	// Management — service instances
	mux.HandleFunc("/serviceregistry/mgmt/service-instances/query", h.handleMgmtServiceInstancesQuery)
	mux.HandleFunc("/serviceregistry/mgmt/service-instances", h.handleMgmtServiceInstances)

	// Management — interface templates
	mux.HandleFunc("/serviceregistry/mgmt/interface-templates/query", h.handleMgmtInterfaceTemplatesQuery)
	mux.HandleFunc("/serviceregistry/mgmt/interface-templates", h.handleMgmtInterfaceTemplates)

	return mux
}

// ─── Device Discovery ─────────────────────────────────────────────────────────

// POST /serviceregistry/device-discovery/register
func (h *AH5Handler) handleDeviceRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.DeviceRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	dev, created, err := h.svc.RegisterDevice(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if created {
		writeJSON(w, http.StatusCreated, dev)
	} else {
		writeJSON(w, http.StatusOK, dev)
	}
}

// POST /serviceregistry/device-discovery/lookup
func (h *AH5Handler) handleDeviceLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.DeviceLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	writeJSON(w, http.StatusOK, h.svc.LookupDevices(req))
}

// DELETE /serviceregistry/device-discovery/revoke/{name}
func (h *AH5Handler) handleDeviceRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/serviceregistry/device-discovery/revoke/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "device name required in path")
		return
	}
	if h.svc.RevokeDevice(name) {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── System Discovery ─────────────────────────────────────────────────────────

// POST /serviceregistry/system-discovery/register
func (h *AH5Handler) handleSystemRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.SystemRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	sys, created, err := h.svc.RegisterSystem(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if created {
		writeJSON(w, http.StatusCreated, sys)
	} else {
		writeJSON(w, http.StatusOK, sys)
	}
}

// POST /serviceregistry/system-discovery/lookup
func (h *AH5Handler) handleSystemLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.SystemLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	writeJSON(w, http.StatusOK, h.svc.LookupSystems(req))
}

// DELETE /serviceregistry/system-discovery/revoke?name={name}
//
// Note (G10): AH5 spec identifies the system from the auth token. This
// implementation uses a ?name= query parameter.
func (h *AH5Handler) handleSystemRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name query parameter required")
		return
	}
	if h.svc.RevokeSystem(name) {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── Service Discovery ────────────────────────────────────────────────────────

// POST /serviceregistry/service-discovery/register
func (h *AH5Handler) handleServiceRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.ServiceRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	inst, created, err := h.svc.RegisterService(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if created {
		writeJSON(w, http.StatusCreated, inst)
	} else {
		writeJSON(w, http.StatusOK, inst)
	}
}

// POST /serviceregistry/service-discovery/lookup
func (h *AH5Handler) handleServiceLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.ServiceLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	writeJSON(w, http.StatusOK, h.svc.LookupServices(req))
}

// DELETE /serviceregistry/service-discovery/revoke/{instanceId}
func (h *AH5Handler) handleServiceRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/serviceregistry/service-discovery/revoke/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "instanceId required in path")
		return
	}
	if h.svc.RevokeService(id) {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── Management — Devices ─────────────────────────────────────────────────────

// POST /serviceregistry/mgmt/devices/query
func (h *AH5Handler) handleMgmtDevicesQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.DeviceLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	writeJSON(w, http.StatusOK, h.svc.QueryDevices(req))
}

// POST /serviceregistry/mgmt/devices — create
// PUT  /serviceregistry/mgmt/devices — update
// DELETE /serviceregistry/mgmt/devices?names=... — remove
func (h *AH5Handler) handleMgmtDevices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req model.DeviceListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		resp, err := h.svc.CreateDevices(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, resp)
	case http.MethodPut:
		var req model.DeviceListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		resp, err := h.svc.UpdateDevices(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodDelete:
		names := r.URL.Query()["names"]
		h.svc.RemoveDevices(names)
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "POST, PUT, or DELETE required")
	}
}

// ─── Management — Systems ─────────────────────────────────────────────────────

// POST /serviceregistry/mgmt/systems/query
func (h *AH5Handler) handleMgmtSystemsQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.SystemLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	writeJSON(w, http.StatusOK, h.svc.QuerySystems(req))
}

// POST /serviceregistry/mgmt/systems — create
// PUT  /serviceregistry/mgmt/systems — update
// DELETE /serviceregistry/mgmt/systems?names=... — remove
func (h *AH5Handler) handleMgmtSystems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req model.SystemListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		resp, err := h.svc.CreateSystems(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, resp)
	case http.MethodPut:
		var req model.SystemListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		resp, err := h.svc.UpdateSystems(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodDelete:
		names := r.URL.Query()["names"]
		h.svc.RemoveSystems(names)
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "POST, PUT, or DELETE required")
	}
}

// ─── Management — Service Definitions ────────────────────────────────────────

// POST /serviceregistry/mgmt/service-definitions/query
func (h *AH5Handler) handleMgmtServiceDefsQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	// No filter supported at this time — return all.
	writeJSON(w, http.StatusOK, h.svc.QueryServiceDefinitions())
}

// POST /serviceregistry/mgmt/service-definitions — create
// DELETE /serviceregistry/mgmt/service-definitions?names=... — remove
func (h *AH5Handler) handleMgmtServiceDefs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req model.ServiceDefinitionListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		resp, err := h.svc.CreateServiceDefinitions(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, resp)
	case http.MethodDelete:
		names := r.URL.Query()["names"]
		h.svc.RemoveServiceDefinitions(names)
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "POST or DELETE required")
	}
}

// ─── Management — Service Instances ──────────────────────────────────────────

// POST /serviceregistry/mgmt/service-instances/query
func (h *AH5Handler) handleMgmtServiceInstancesQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.ServiceLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	writeJSON(w, http.StatusOK, h.svc.QueryServiceInstances(req))
}

// POST   /serviceregistry/mgmt/service-instances — create
// PUT    /serviceregistry/mgmt/service-instances — update
// DELETE /serviceregistry/mgmt/service-instances?serviceInstances=... — remove
func (h *AH5Handler) handleMgmtServiceInstances(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req model.ServiceCreateListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		resp, err := h.svc.CreateServiceInstances(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, resp)
	case http.MethodPut:
		var req model.ServiceUpdateListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		resp, err := h.svc.UpdateServiceInstances(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodDelete:
		ids := r.URL.Query()["serviceInstances"]
		h.svc.RemoveServiceInstances(ids)
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "POST, PUT, or DELETE required")
	}
}

// ─── Management — Interface Templates ────────────────────────────────────────

// POST /serviceregistry/mgmt/interface-templates/query
func (h *AH5Handler) handleMgmtInterfaceTemplatesQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	writeJSON(w, http.StatusOK, h.svc.QueryInterfaceTemplates())
}

// POST   /serviceregistry/mgmt/interface-templates — create
// DELETE /serviceregistry/mgmt/interface-templates?names=... — remove
func (h *AH5Handler) handleMgmtInterfaceTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req model.InterfaceTemplateListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		resp, err := h.svc.CreateInterfaceTemplates(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, resp)
	case http.MethodDelete:
		names := r.URL.Query()["names"]
		h.svc.RemoveInterfaceTemplates(names)
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "POST or DELETE required")
	}
}
