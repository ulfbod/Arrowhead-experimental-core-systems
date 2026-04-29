// Package api implements the HTTP interface for the Certificate Authority core system.
package api

import (
	"encoding/json"
	"net/http"

	"arrowhead/core/internal/ca/model"
	"arrowhead/core/internal/ca/service"
)

type Handler struct {
	svc *service.CAService
}

func NewHandler(svc *service.CAService) http.Handler {
	h := &Handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("/ca/certificate/issue", h.handleIssue)
	mux.HandleFunc("/ca/certificate/verify", h.handleVerify)
	mux.HandleFunc("/ca/info", h.handleInfo)
	mux.HandleFunc("/ca/health", h.handleHealth)
	mux.HandleFunc("/health", h.handleHealth)
	return mux
}

// POST /ca/certificate/issue
func (h *Handler) handleIssue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.IssueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	issued, err := h.svc.Issue(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, issued)
}

// POST /ca/certificate/verify
func (h *Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		Certificate string `json:"certificate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	systemName, valid, reason := h.svc.VerifyCert(req.Certificate)
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":      valid,
		"systemName": systemName,
		"reason":     reason,
	})
}

// GET /ca/info
func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	writeJSON(w, http.StatusOK, h.svc.CAInfo())
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "ca"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
