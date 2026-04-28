// Package api implements the HTTP interface for the Arrowhead Core Service Registry.
//
// DO NOT MODIFY FOR EXPERIMENTS.
// Experiments must interact with this system via its HTTP API only.
// See experiments/CLAUDE_EXPERIMENTS.md for guidance.
package api

import (
	"encoding/json"
	"arrowhead/serviceregistry/internal/model"
	"arrowhead/serviceregistry/internal/service"
	"net/http"
)

// Handler wires HTTP routes to the RegistryService.
type Handler struct {
	svc *service.RegistryService
}

func NewHandler(svc *service.RegistryService) http.Handler {
	h := &Handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("/serviceregistry/register", h.handleRegister)
	mux.HandleFunc("/serviceregistry/query", h.handleQuery)
	mux.HandleFunc("/health", h.handleHealth)
	return mux
}

// POST /serviceregistry/register
func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	svc, err := h.svc.Register(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, svc)
}

// POST /serviceregistry/query
func (h *Handler) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	writeJSON(w, http.StatusOK, h.svc.Query(req))
}

// GET /health
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
