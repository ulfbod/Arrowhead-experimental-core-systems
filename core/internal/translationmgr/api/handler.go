// Package api provides HTTP handlers for the Translation Manager system.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"arrowhead/core/internal/httputil"
	"arrowhead/core/internal/translationmgr/model"
	"arrowhead/core/internal/translationmgr/service"
)

const tmOrigin = "translationmanager"

// Handler wires HTTP routes to the TranslationService.
type Handler struct {
	svc *service.TranslationService
}

// NewHandler returns an http.Handler for all Translation Manager routes.
func NewHandler(svc *service.TranslationService) http.Handler {
	h := &Handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("/translationmanager/translation/translate", h.handleTranslate)
	mux.HandleFunc("/translationmanager/translation/status/", h.handleStatus)
	mux.HandleFunc("/translationmanager/translation/mgmt/bridges", h.handleBridges)
	mux.HandleFunc("/translationmanager/translation/mgmt/bridges/", h.handleBridgeByID)
	mux.HandleFunc("/translationmanager/health", h.handleHealth)
	return mux
}

// POST /translationmanager/translation/translate
func (h *Handler) handleTranslate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", tmOrigin)
		return
	}
	var req model.TranslateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", tmOrigin)
		return
	}
	if strings.TrimSpace(req.BridgeID) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "bridgeId is required", tmOrigin)
		return
	}
	resp, err := h.svc.Translate(req.BridgeID, req.Payload)
	if errors.Is(err, service.ErrBridgeNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "bridge not found", tmOrigin)
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error(), tmOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp, tmOrigin)
}

// GET /translationmanager/translation/status/{bridgeId}
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "GET required", tmOrigin)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/translationmanager/translation/status/")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "bridgeId required in path", tmOrigin)
		return
	}
	bridge, err := h.svc.GetBridge(id)
	if errors.Is(err, service.ErrBridgeNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "bridge not found", tmOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, bridge, tmOrigin)
}

// POST /translationmanager/translation/mgmt/bridges — create
// GET  /translationmanager/translation/mgmt/bridges — list
func (h *Handler) handleBridges(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req model.Bridge
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", tmOrigin)
			return
		}
		b, err := h.svc.CreateBridge(req)
		if errors.Is(err, service.ErrDuplicateBridge) {
			httputil.WriteError(w, http.StatusConflict, "bridge already exists", tmOrigin)
			return
		}
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error(), tmOrigin)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, b, tmOrigin)
	case http.MethodGet:
		bridges := h.svc.ListBridges()
		httputil.WriteJSON(w, http.StatusOK, bridges, tmOrigin)
	default:
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST or GET required", tmOrigin)
	}
}

// DELETE /translationmanager/translation/mgmt/bridges/{id}
func (h *Handler) handleBridgeByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", tmOrigin)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/translationmanager/translation/mgmt/bridges/")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "id required in path", tmOrigin)
		return
	}
	if err := h.svc.DeleteBridge(id); errors.Is(err, service.ErrBridgeNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "bridge not found", tmOrigin)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// GET /translationmanager/health
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "UP"}, tmOrigin)
}
