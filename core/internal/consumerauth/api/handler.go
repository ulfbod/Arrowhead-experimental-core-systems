// Package api implements the HTTP interface for the ConsumerAuthorization core system.
// AH5 services: authorization (grant, revoke, lookup, verify) + authorizationToken (generate)
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"arrowhead/core/internal/consumerauth/model"
	"arrowhead/core/internal/consumerauth/service"
)

type Handler struct {
	svc *service.AuthService
}

func NewHandler(svc *service.AuthService) http.Handler {
	h := &Handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("/authorization/grant", h.handleGrant)
	mux.HandleFunc("/authorization/revoke/", h.handleRevoke)
	mux.HandleFunc("/authorization/lookup", h.handleLookup)
	mux.HandleFunc("/authorization/verify", h.handleVerify)
	mux.HandleFunc("/authorization/token/generate", h.handleGenerateToken)
	mux.HandleFunc("/health", h.handleHealth)
	return mux
}

// POST /authorization/grant
func (h *Handler) handleGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.GrantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	rule, err := h.svc.Grant(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

// DELETE /authorization/revoke/{id}
func (h *Handler) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/authorization/revoke/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	if err := h.svc.Revoke(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// GET /authorization/lookup?consumer=...&provider=...&service=...
func (h *Handler) handleLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	q := r.URL.Query()
	resp := h.svc.Lookup(q.Get("consumer"), q.Get("provider"), q.Get("service"))
	writeJSON(w, http.StatusOK, resp)
}

// POST /authorization/verify
func (h *Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	writeJSON(w, http.StatusOK, h.svc.Verify(req))
}

// POST /authorization/token/generate
func (h *Handler) handleGenerateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.TokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	resp, err := h.svc.GenerateToken(req)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "consumerauthorization"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
