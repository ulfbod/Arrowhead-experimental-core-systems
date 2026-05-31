// Package api implements the HTTP interface for the Certificate Authority core system.
package api

import (
	"errors"
	"net/http"

	"arrowhead/core/internal/ca/model"
	"arrowhead/core/internal/ca/service"
	"arrowhead/core/internal/httputil"
)

const caOrigin = "ca"

type Handler struct {
	svc *service.CAService
}

func NewHandler(svc *service.CAService) http.Handler {
	h := &Handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("/ca/certificate/issue", h.handleIssue)
	mux.HandleFunc("/ca/certificate/verify", h.handleVerify)
	mux.HandleFunc("/ca/certificate/revoke", h.handleRevoke)
	mux.HandleFunc("/ca/crl", h.handleCRL)
	mux.HandleFunc("/ca/info", h.handleInfo)
	mux.HandleFunc("/ca/health", h.handleHealth)
	mux.HandleFunc("/health", h.handleHealth)
	return mux
}

// statusFor maps sentinel errors to HTTP status codes.
func statusFor(err error) int {
	switch {
	case errors.Is(err, service.ErrMissingSystemName):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrMissingCertificate):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrCertNotIssuedByCA):
		return http.StatusBadRequest
	default:
		return http.StatusBadRequest
	}
}

// POST /ca/certificate/issue
func (h *Handler) handleIssue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", caOrigin)
		return
	}
	var req model.IssueRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	issued, err := h.svc.Issue(req)
	if err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), caOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, issued, caOrigin)
}

// POST /ca/certificate/verify
func (h *Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", caOrigin)
		return
	}
	var req struct {
		Certificate string `json:"certificate"`
	}
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	systemName, valid, reason := h.svc.VerifyCert(req.Certificate)
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"valid":      valid,
		"systemName": systemName,
		"reason":     reason,
	}, caOrigin)
}

// POST /ca/certificate/revoke
func (h *Handler) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", caOrigin)
		return
	}
	var req model.RevokeRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	resp, err := h.svc.Revoke(req.Certificate)
	if err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), caOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp, caOrigin)
}

// GET /ca/crl
func (h *Handler) handleCRL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "GET required", caOrigin)
		return
	}
	crlPEM, err := h.svc.CRL()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to generate CRL", caOrigin)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.WriteHeader(http.StatusOK)
	w.Write(crlPEM) //nolint:errcheck
}

// GET /ca/info
func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "GET required", caOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, h.svc.CAInfo(), caOrigin)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "ca"}, caOrigin)
}
