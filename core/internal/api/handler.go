// Package api implements the HTTP interface for the Arrowhead Core Service Registry.
//
// DO NOT MODIFY FOR EXPERIMENTS.
// Experiments must interact with this system via its HTTP API only.
// See experiments/CLAUDE_EXPERIMENTS.md for guidance.
package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	blclient "arrowhead/core/internal/blacklist/client"
	"arrowhead/core/internal/httputil"
	"arrowhead/core/internal/model"
	"arrowhead/core/internal/service"
)

// Handler wires HTTP routes to the RegistryService.
type Handler struct {
	svc      *service.RegistryService
	blClient blclient.BlacklistClient
}

// NewHandler creates a ServiceRegistry HTTP handler.
// blClient is consulted on each register call; pass blclient.NopClient{} to disable.
func NewHandler(svc *service.RegistryService, blClient blclient.BlacklistClient) http.Handler {
	h := &Handler{svc: svc, blClient: blClient}
	mux := http.NewServeMux()
	mux.HandleFunc("/serviceregistry/register", h.handleRegister)
	mux.HandleFunc("/serviceregistry/query", h.handleQuery)
	mux.HandleFunc("/serviceregistry/lookup", h.handleLookup)
	mux.HandleFunc("/serviceregistry/unregister", h.handleUnregister)
	mux.HandleFunc("/health", h.handleHealth)
	return mux
}

// POST /serviceregistry/register
func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", "serviceregistry")
		return
	}
	var req model.RegisterRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	// Reject registrations from blacklisted systems (fail-closed).
	if req.ProviderSystem != nil && req.ProviderSystem.SystemName != "" {
		if blacklisted, _ := h.blClient.IsBlacklisted(context.Background(), req.ProviderSystem.SystemName); blacklisted {
			httputil.WriteError(w, http.StatusForbidden, "system is blacklisted", "serviceregistry")
			return
		}
	}
	svc, err := h.svc.Register(req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error(), "serviceregistry")
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, svc, "serviceregistry")
}

// POST /serviceregistry/query — AH4-compatible, kept for backward compatibility.
func (h *Handler) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", "serviceregistry")
		return
	}
	var req model.QueryRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, h.svc.Query(req), "serviceregistry")
}

// GET /serviceregistry/lookup — AH5-aligned read-only lookup.
// Optional query params: serviceDefinition, version
func (h *Handler) handleLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "GET required", "serviceregistry")
		return
	}
	q := r.URL.Query()
	req := model.QueryRequest{
		ServiceDefinition: strings.TrimSpace(q.Get("serviceDefinition")),
	}
	if v := q.Get("version"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			req.VersionRequirement = n
		}
	}
	httputil.WriteJSON(w, http.StatusOK, h.svc.Query(req), "serviceregistry")
}

// DELETE /serviceregistry/unregister — AH5-aligned revoke.
// Body: { serviceDefinition, providerSystem: {systemName, address, port}, version }
func (h *Handler) handleUnregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", "serviceregistry")
		return
	}
	var req model.UnregisterRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.Unregister(req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error(), "serviceregistry")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// GET /health
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "serviceregistry"}, "serviceregistry")
}
