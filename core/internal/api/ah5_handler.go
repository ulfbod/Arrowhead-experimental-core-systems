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
	"errors"
	"net/http"
	"strings"

	"arrowhead/core/internal/httputil"
	"arrowhead/core/internal/model"
	"arrowhead/core/internal/service"
)

const srOrigin = "serviceregistry"

// pageReqOrZero returns the dereferenced PageRequest, or a zero value if p is nil.
func pageReqOrZero(p *model.PageRequest) model.PageRequest {
	if p == nil {
		return model.PageRequest{}
	}
	return *p
}

// AH5Handler wires HTTP routes to AH5RegistryService.
type AH5Handler struct {
	svc             *service.AH5RegistryService
	authURL         string // optional: Authentication URL for token-based system revoke
	mgmtAuthURL     string // optional: Authentication URL for management endpoint access control
	registerAuthURL string // optional: Authentication URL for registration identity enforcement (G10)
}

// NewAH5Handler returns an http.Handler that handles all AH5 discovery and
// management routes. authURL is the base URL of the Authentication system
// (e.g. "http://localhost:8081"). Pass "" to disable token-based revoke.
// mgmtAuthURL is the Authentication URL for mgmt access control (MGMT_AUTH_URL env var);
// pass "" for open management access (development mode).
// registerAuthURL is the Authentication URL for registration identity enforcement (REGISTER_AUTH_URL env var);
// pass "" for open registration (development/PoC mode).
func NewAH5Handler(svc *service.AH5RegistryService, authURL, mgmtAuthURL, registerAuthURL string) http.Handler {
	h := &AH5Handler{svc: svc, authURL: authURL, mgmtAuthURL: mgmtAuthURL, registerAuthURL: registerAuthURL}
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
	mux.HandleFunc("/serviceregistry/mgmt/systems/revoke", h.handleMgmtSystemsRevoke)

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

// statusFor maps sentinel errors to HTTP status codes for this handler.
func (h *AH5Handler) statusFor(err error) int {
	if errors.Is(err, service.ErrLocked) {
		return http.StatusLocked
	}
	return http.StatusBadRequest
}

// ─── Device Discovery ─────────────────────────────────────────────────────────

// POST /serviceregistry/device-discovery/register
func (h *AH5Handler) handleDeviceRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", srOrigin)
		return
	}
	var req model.DeviceRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
		return
	}
	if msg := validateDeviceName(req.Name); msg != "" {
		httputil.WriteError(w, http.StatusBadRequest, msg, srOrigin)
		return
	}
	dev, created, err := h.svc.RegisterDevice(req)
	if err != nil {
		httputil.WriteError(w, h.statusFor(err), err.Error(), srOrigin)
		return
	}
	if created {
		httputil.WriteJSON(w, http.StatusCreated, dev, srOrigin)
	} else {
		httputil.WriteJSON(w, http.StatusOK, dev, srOrigin)
	}
}

// POST /serviceregistry/device-discovery/lookup
func (h *AH5Handler) handleDeviceLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", srOrigin)
		return
	}
	var raw struct {
		model.DeviceLookupRequest
		Pagination *model.PageRequest `json:"pagination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
		return
	}
	full := h.svc.LookupDevices(raw.DeviceLookupRequest)
	page, total := model.Paginate(full.Entries, pageReqOrZero(raw.Pagination), func(d *model.Device) string { return d.Name })
	httputil.WriteJSON(w, http.StatusOK, model.DeviceLookupResponse{Entries: page, Count: len(page), TotalCount: total}, srOrigin)
}

// DELETE /serviceregistry/device-discovery/revoke/{name}
func (h *AH5Handler) handleDeviceRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", srOrigin)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/serviceregistry/device-discovery/revoke/")
	if name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "device name required in path", srOrigin)
		return
	}
	ok, err := h.svc.RevokeDevice(name)
	if errors.Is(err, service.ErrLocked) {
		httputil.WriteError(w, http.StatusLocked, err.Error(), srOrigin)
		return
	}
	if ok {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── System Discovery ─────────────────────────────────────────────────────────

// POST /serviceregistry/system-discovery/register
func (h *AH5Handler) handleSystemRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", srOrigin)
		return
	}
	var req model.SystemRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
		return
	}
	if ok, status := httputil.VerifyTokenIdentity(r, h.registerAuthURL, req.Name); !ok {
		httputil.WriteError(w, status, "registration identity check failed", srOrigin)
		return
	}
	if msg := validateSystemName(req.Name); msg != "" {
		httputil.WriteError(w, http.StatusBadRequest, msg, srOrigin)
		return
	}
	sys, created, err := h.svc.RegisterSystem(req)
	if err != nil {
		httputil.WriteError(w, h.statusFor(err), err.Error(), srOrigin)
		return
	}
	if created {
		httputil.WriteJSON(w, http.StatusCreated, sys, srOrigin)
	} else {
		httputil.WriteJSON(w, http.StatusOK, sys, srOrigin)
	}
}

// POST /serviceregistry/system-discovery/lookup
func (h *AH5Handler) handleSystemLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", srOrigin)
		return
	}
	var raw struct {
		model.SystemLookupRequest
		Pagination *model.PageRequest `json:"pagination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
		return
	}
	full := h.svc.LookupSystems(raw.SystemLookupRequest)
	page, total := model.Paginate(full.Entries, pageReqOrZero(raw.Pagination), func(s *model.AH5System) string { return s.Name })
	httputil.WriteJSON(w, http.StatusOK, model.SystemLookupResponse{Entries: page, Count: len(page), TotalCount: total}, srOrigin)
}

// DELETE /serviceregistry/system-discovery/revoke?name={name}
//
// Note (G10): AH5 spec identifies the system from the auth token. This
// implementation uses a ?name= query parameter.
func (h *AH5Handler) handleSystemRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", srOrigin)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "name query parameter required", srOrigin)
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
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", srOrigin)
		return
	}
	var req model.ServiceRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
		return
	}
	if ok, status := httputil.VerifyTokenIdentity(r, h.registerAuthURL, req.SystemName); !ok {
		httputil.WriteError(w, status, "registration identity check failed", srOrigin)
		return
	}
	if msg := validateSystemName(req.SystemName); msg != "" {
		httputil.WriteError(w, http.StatusBadRequest, msg, srOrigin)
		return
	}
	if msg := validateServiceDefinitionName(req.ServiceDefinitionName); msg != "" {
		httputil.WriteError(w, http.StatusBadRequest, msg, srOrigin)
		return
	}
	if msg := validateInterfaces(req.Interfaces); msg != "" {
		httputil.WriteError(w, http.StatusBadRequest, msg, srOrigin)
		return
	}
	inst, created, err := h.svc.RegisterService(req)
	if err != nil {
		httputil.WriteError(w, h.statusFor(err), err.Error(), srOrigin)
		return
	}
	if created {
		httputil.WriteJSON(w, http.StatusCreated, inst, srOrigin)
	} else {
		httputil.WriteJSON(w, http.StatusOK, inst, srOrigin)
	}
}

// POST /serviceregistry/service-discovery/lookup
// Requires at least one of: instanceIds, providerNames, serviceDefinitionNames.
func (h *AH5Handler) handleServiceLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", srOrigin)
		return
	}
	var raw struct {
		model.ServiceLookupRequest
		Pagination *model.PageRequest `json:"pagination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
		return
	}
	if !hasServiceLookupFilter(raw.ServiceLookupRequest) {
		httputil.WriteError(w, http.StatusBadRequest, "at least one of instanceIds, providerNames, serviceDefinitionNames must be provided", srOrigin)
		return
	}
	full := h.svc.LookupServices(raw.ServiceLookupRequest)
	page, total := model.Paginate(full.Entries, pageReqOrZero(raw.Pagination), func(si *model.AH5ServiceInstance) string { return si.InstanceID })
	httputil.WriteJSON(w, http.StatusOK, model.ServiceLookupResponse{Entries: page, Count: len(page), TotalCount: total}, srOrigin)
}

// DELETE /serviceregistry/service-discovery/revoke/{instanceId}
func (h *AH5Handler) handleServiceRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", srOrigin)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/serviceregistry/service-discovery/revoke/")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "instanceId required in path", srOrigin)
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
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, srOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", srOrigin)
		return
	}
	var raw struct {
		model.DeviceLookupRequest
		Pagination *model.PageRequest `json:"pagination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
		return
	}
	full := h.svc.QueryDevices(raw.DeviceLookupRequest)
	page, total := model.Paginate(full.Devices, pageReqOrZero(raw.Pagination), func(d *model.Device) string { return d.Name })
	httputil.WriteJSON(w, http.StatusOK, model.DeviceListResponse{Devices: page, Count: len(page), TotalCount: total}, srOrigin)
}

// POST /serviceregistry/mgmt/devices — create
// PUT  /serviceregistry/mgmt/devices — update
// DELETE /serviceregistry/mgmt/devices?names=... — remove
func (h *AH5Handler) handleMgmtDevices(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, srOrigin) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req model.DeviceListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
			return
		}
		resp, err := h.svc.CreateDevices(req)
		if err != nil {
			httputil.WriteError(w, h.statusFor(err), err.Error(), srOrigin)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, resp, srOrigin)
	case http.MethodPut:
		var req model.DeviceListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
			return
		}
		resp, err := h.svc.UpdateDevices(req)
		if err != nil {
			httputil.WriteError(w, h.statusFor(err), err.Error(), srOrigin)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, resp, srOrigin)
	case http.MethodDelete:
		names := r.URL.Query()["names"]
		h.svc.RemoveDevices(names)
		w.WriteHeader(http.StatusOK)
	default:
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST, PUT, or DELETE required", srOrigin)
	}
}

// ─── Management — Systems ─────────────────────────────────────────────────────

// POST /serviceregistry/mgmt/systems/query
func (h *AH5Handler) handleMgmtSystemsQuery(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, srOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", srOrigin)
		return
	}
	var raw struct {
		model.SystemLookupRequest
		Pagination *model.PageRequest `json:"pagination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
		return
	}
	full := h.svc.QuerySystems(raw.SystemLookupRequest)
	page, total := model.Paginate(full.Systems, pageReqOrZero(raw.Pagination), func(s *model.AH5System) string { return s.Name })
	httputil.WriteJSON(w, http.StatusOK, model.SystemListResponse{Systems: page, Count: len(page), TotalCount: total}, srOrigin)
}

// POST /serviceregistry/mgmt/systems — create
// PUT  /serviceregistry/mgmt/systems — update
// DELETE /serviceregistry/mgmt/systems?names=... — remove
func (h *AH5Handler) handleMgmtSystems(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, srOrigin) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req model.SystemListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
			return
		}
		resp, err := h.svc.CreateSystems(req)
		if err != nil {
			httputil.WriteError(w, h.statusFor(err), err.Error(), srOrigin)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, resp, srOrigin)
	case http.MethodPut:
		var req model.SystemListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
			return
		}
		resp, err := h.svc.UpdateSystems(req)
		if err != nil {
			httputil.WriteError(w, h.statusFor(err), err.Error(), srOrigin)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, resp, srOrigin)
	case http.MethodDelete:
		names := r.URL.Query()["names"]
		h.svc.RemoveSystems(names)
		w.WriteHeader(http.StatusOK)
	default:
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST, PUT, or DELETE required", srOrigin)
	}
}

// DELETE /serviceregistry/mgmt/systems/revoke
// Revokes (removes) the system identified by the Bearer token from the Authentication system.
// Falls back to ?name= parameter when authURL is not configured.
func (h *AH5Handler) handleMgmtSystemsRevoke(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, srOrigin) {
		return
	}
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", srOrigin)
		return
	}

	var systemName string
	if h.authURL != "" {
		// Token-based: resolve system name via Authentication system.
		token := httputil.ExtractBearer(r)
		if token == "" {
			httputil.WriteError(w, http.StatusUnauthorized, "Authorization: Bearer token required", srOrigin)
			return
		}
		name, err := h.resolveSystemNameFromToken(token)
		if err != nil {
			httputil.WriteError(w, http.StatusUnauthorized, "identity token is invalid or unreachable", srOrigin)
			return
		}
		systemName = name
	} else {
		// Fallback: name from query param.
		systemName = r.URL.Query().Get("name")
		if systemName == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name query parameter required", srOrigin)
			return
		}
	}

	if h.svc.RevokeSystem(systemName) {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// resolveSystemNameFromToken calls GET /authentication/identity/verify/<token>
// and returns the systemName from the response.
func (h *AH5Handler) resolveSystemNameFromToken(token string) (string, error) {
	url := h.authURL + "/authentication/identity/verify/" + token
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	var body struct {
		SystemName string `json:"systemName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.SystemName == "" {
		return "", errors.New("empty systemName in verify response")
	}
	return body.SystemName, nil
}

// ─── Management — Service Definitions ────────────────────────────────────────

// POST /serviceregistry/mgmt/service-definitions/query
func (h *AH5Handler) handleMgmtServiceDefsQuery(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, srOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", srOrigin)
		return
	}
	var raw struct {
		Pagination *model.PageRequest `json:"pagination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
		return
	}
	full := h.svc.QueryServiceDefinitions()
	page, total := model.Paginate(full.ServiceDefinitions, pageReqOrZero(raw.Pagination), func(sd *model.ServiceDefinition) string { return sd.Name })
	httputil.WriteJSON(w, http.StatusOK, model.ServiceDefinitionListResponse{ServiceDefinitions: page, Count: len(page), TotalCount: total}, srOrigin)
}

// POST /serviceregistry/mgmt/service-definitions — create
// DELETE /serviceregistry/mgmt/service-definitions?names=... — remove
func (h *AH5Handler) handleMgmtServiceDefs(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, srOrigin) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req model.ServiceDefinitionListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
			return
		}
		resp, err := h.svc.CreateServiceDefinitions(req)
		if err != nil {
			httputil.WriteError(w, h.statusFor(err), err.Error(), srOrigin)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, resp, srOrigin)
	case http.MethodDelete:
		names := r.URL.Query()["names"]
		h.svc.RemoveServiceDefinitions(names)
		w.WriteHeader(http.StatusOK)
	default:
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST or DELETE required", srOrigin)
	}
}

// ─── Management — Service Instances ──────────────────────────────────────────

// POST /serviceregistry/mgmt/service-instances/query
func (h *AH5Handler) handleMgmtServiceInstancesQuery(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, srOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", srOrigin)
		return
	}
	var raw struct {
		model.ServiceLookupRequest
		Pagination *model.PageRequest `json:"pagination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
		return
	}
	full := h.svc.QueryServiceInstances(raw.ServiceLookupRequest)
	page, total := model.Paginate(full.Instances, pageReqOrZero(raw.Pagination), func(si *model.AH5ServiceInstance) string { return si.InstanceID })
	httputil.WriteJSON(w, http.StatusOK, model.ServiceListResponse{Instances: page, Count: len(page), TotalCount: total}, srOrigin)
}

// POST   /serviceregistry/mgmt/service-instances — create
// PUT    /serviceregistry/mgmt/service-instances — update
// DELETE /serviceregistry/mgmt/service-instances?serviceInstances=... — remove
func (h *AH5Handler) handleMgmtServiceInstances(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, srOrigin) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req model.ServiceCreateListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
			return
		}
		resp, err := h.svc.CreateServiceInstances(req)
		if err != nil {
			httputil.WriteError(w, h.statusFor(err), err.Error(), srOrigin)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, resp, srOrigin)
	case http.MethodPut:
		var req model.ServiceUpdateListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
			return
		}
		resp, err := h.svc.UpdateServiceInstances(req)
		if err != nil {
			httputil.WriteError(w, h.statusFor(err), err.Error(), srOrigin)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, resp, srOrigin)
	case http.MethodDelete:
		ids := r.URL.Query()["serviceInstances"]
		h.svc.RemoveServiceInstances(ids)
		w.WriteHeader(http.StatusOK)
	default:
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST, PUT, or DELETE required", srOrigin)
	}
}

// ─── Management — Interface Templates ────────────────────────────────────────

// POST /serviceregistry/mgmt/interface-templates/query
func (h *AH5Handler) handleMgmtInterfaceTemplatesQuery(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, srOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", srOrigin)
		return
	}
	var raw struct {
		Pagination *model.PageRequest `json:"pagination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
		return
	}
	full := h.svc.QueryInterfaceTemplates()
	page, total := model.Paginate(full.InterfaceTemplates, pageReqOrZero(raw.Pagination), func(it *model.InterfaceTemplate) string { return it.Name })
	httputil.WriteJSON(w, http.StatusOK, model.InterfaceTemplateListResponse{InterfaceTemplates: page, Count: len(page), TotalCount: total}, srOrigin)
}

// POST   /serviceregistry/mgmt/interface-templates — create
// DELETE /serviceregistry/mgmt/interface-templates?names=... — remove
func (h *AH5Handler) handleMgmtInterfaceTemplates(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, srOrigin) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req model.InterfaceTemplateListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", srOrigin)
			return
		}
		for _, tmpl := range req.InterfaceTemplates {
			if tmpl != nil {
				if msg := validateInterfaceTemplateName(tmpl.Name); msg != "" {
					httputil.WriteError(w, http.StatusBadRequest, msg, srOrigin)
					return
				}
			}
		}
		resp, err := h.svc.CreateInterfaceTemplates(req)
		if err != nil {
			httputil.WriteError(w, h.statusFor(err), err.Error(), srOrigin)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, resp, srOrigin)
	case http.MethodDelete:
		names := r.URL.Query()["names"]
		h.svc.RemoveInterfaceTemplates(names)
		w.WriteHeader(http.StatusOK)
	default:
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST or DELETE required", srOrigin)
	}
}
