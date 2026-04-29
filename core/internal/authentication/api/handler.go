// Package api implements the HTTP interface for the Authentication core system.
// AH5 service: identity (login, logout, verify)
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"arrowhead/core/internal/authentication/model"
	"arrowhead/core/internal/authentication/service"
)

type Handler struct {
	svc *service.AuthService
}

func NewHandler(svc *service.AuthService) http.Handler {
	h := &Handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("/authentication/identity/login", h.handleLogin)
	mux.HandleFunc("/authentication/identity/logout", h.handleLogout)
	mux.HandleFunc("/authentication/identity/verify", h.handleVerify)
	mux.HandleFunc("/health", h.handleHealth)
	return mux
}

// POST /authentication/identity/login
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	resp, err := h.svc.Login(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// DELETE /authentication/identity/logout
// Authorization: Bearer <token>
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	token := extractBearer(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing Authorization: Bearer token")
		return
	}
	if err := h.svc.Logout(token); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// GET /authentication/identity/verify
// Authorization: Bearer <token>
func (h *Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	token := extractBearer(r)
	if token == "" {
		writeJSON(w, http.StatusOK, model.VerifyResponse{Valid: false})
		return
	}
	resp, err := h.svc.Verify(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "authentication"})
}

func extractBearer(r *http.Request) string {
	hdr := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(hdr, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
