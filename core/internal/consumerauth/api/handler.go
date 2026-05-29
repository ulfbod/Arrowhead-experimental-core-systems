// Package api implements the HTTP interface for the ConsumerAuthorization core system.
// AH5 services: authorization (grant, revoke, lookup, verify) + management
package api

import (
	"encoding/json"
	"net/http"
	"net/url"
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
	mux.HandleFunc("/consumerauthorization/authorization/grant", h.handleGrant)
	mux.HandleFunc("/consumerauthorization/authorization/revoke/", h.handleRevoke)
	mux.HandleFunc("/consumerauthorization/authorization/lookup", h.handleLookup)
	mux.HandleFunc("/consumerauthorization/authorization/verify", h.handleVerify)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/grant", h.handleMgmtGrant)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/revoke", h.handleMgmtRevoke)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/query", h.handleMgmtQuery)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/check", h.handleMgmtCheck)
	mux.HandleFunc("/consumerauthorization/authorization/health", h.handleHealth)
	mux.HandleFunc("/health", h.handleHealth)
	// authorization-token sub-service
	mux.HandleFunc("/consumerauthorization/authorization-token/generate", h.handleTokenGenerate)
	mux.HandleFunc("/consumerauthorization/authorization-token/verify/", h.handleTokenVerify)
	mux.HandleFunc("/consumerauthorization/authorization-token/public-key", h.handleTokenPublicKey)
	mux.HandleFunc("/consumerauthorization/authorization-token/encryption-key", h.handleEncryptionKey)
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
	policy, err := h.svc.Grant(req)
	if err != nil {
		if err == service.ErrDuplicateRule {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, policy)
}

// DELETE /authorization/revoke/{instanceId}
// instanceId may be URL-encoded (pipes as %7C)
func (h *Handler) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/consumerauthorization/authorization/revoke/")
	instanceID, err := url.PathUnescape(raw)
	if err != nil || instanceID == "" {
		writeError(w, http.StatusBadRequest, "invalid instanceId")
		return
	}
	if err := h.svc.Revoke(instanceID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// POST /authorization/lookup
// Requires at least one of: instanceIds, cloudIdentifiers, targetNames.
func (h *Handler) handleLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.LookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !hasFilter(req) {
		writeError(w, http.StatusBadRequest, "at least one of instanceIds, cloudIdentifiers, targetNames must be provided")
		return
	}
	resp := h.svc.Lookup(req)
	writeJSON(w, http.StatusOK, resp)
}

// POST /authorization/verify — returns plain JSON Boolean
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
	authorized := h.svc.Verify(req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(authorized)
}

// POST /authorization/mgmt/grant
func (h *Handler) handleMgmtGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.GrantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	policy, err := h.svc.Grant(req)
	if err != nil {
		if err == service.ErrDuplicateRule {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, policy)
}

// DELETE /authorization/mgmt/revoke?instanceIds=...
func (h *Handler) handleMgmtRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	idsParam := r.URL.Query().Get("instanceIds")
	if idsParam == "" {
		writeError(w, http.StatusBadRequest, "instanceIds query parameter required")
		return
	}
	h.svc.BulkRevoke(strings.Split(idsParam, ","))
	w.WriteHeader(http.StatusOK)
}

// POST /authorization/mgmt/query — returns all policies (no filter required)
func (h *Handler) handleMgmtQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.LookupRequest
	json.NewDecoder(r.Body).Decode(&req)
	var policies []model.AuthPolicy
	if hasFilter(req) {
		resp := h.svc.Lookup(req)
		policies = resp.Policies
	} else {
		policies = h.svc.AllPolicies()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"policies":   policies,
		"count":      len(policies),
		"totalCount": len(policies),
	})
}

// POST /authorization/mgmt/check — bulk verify
func (h *Handler) handleMgmtCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var reqs []model.VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	results := h.svc.BulkVerify(reqs)
	writeJSON(w, http.StatusOK, results)
}

// POST /authorization-token/generate
func (h *Handler) handleTokenGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.TokenGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	desc, err := h.svc.GenerateAuthToken(req)
	if err != nil {
		writeError(w, http.StatusNotImplemented, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, desc)
}

// GET /authorization-token/verify/{accessToken}
func (h *Handler) handleTokenVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	accessToken := strings.TrimPrefix(r.URL.Path, "/consumerauthorization/authorization-token/verify/")
	if accessToken == "" {
		writeError(w, http.StatusBadRequest, "accessToken is required")
		return
	}
	resp, ok := h.svc.VerifyAuthToken(accessToken)
	if !ok {
		writeError(w, http.StatusNotFound, service.ErrTokenNotFound.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /authorization-token/public-key — not implemented (returns 404)
func (h *Handler) handleTokenPublicKey(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "public-key endpoint not implemented")
}

// POST /authorization-token/encryption-key — register key
// DELETE /authorization-token/encryption-key — remove key
func (h *Handler) handleEncryptionKey(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req model.EncryptionKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		h.svc.RegisterEncryptionKey(req)
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		systemName := r.URL.Query().Get("systemName")
		if systemName == "" {
			writeError(w, http.StatusBadRequest, "systemName query parameter required")
			return
		}
		h.svc.RemoveEncryptionKey(systemName)
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "POST or DELETE required")
	}
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "consumerauthorization"})
}

func hasFilter(req model.LookupRequest) bool {
	return len(req.InstanceIDs) > 0 || len(req.CloudIdentifiers) > 0 || len(req.TargetNames) > 0
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	exType := errTypeForStatus(status)
	type errBody struct {
		ErrorMessage  string `json:"errorMessage"`
		ErrorCode     int    `json:"errorCode"`
		ExceptionType string `json:"exceptionType"`
		Origin        string `json:"origin"`
	}
	writeJSON(w, status, errBody{ErrorMessage: msg, ErrorCode: status, ExceptionType: exType, Origin: "consumerauthorization.authorization"})
}

func errTypeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusMethodNotAllowed:
		return "INVALID_PARAMETER"
	case http.StatusUnauthorized:
		return "AUTH"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "DATA_NOT_FOUND"
	case http.StatusConflict:
		return "ALREADY_EXISTS"
	case http.StatusLocked:
		return "LOCKED"
	case http.StatusNotImplemented:
		return "NOT_IMPLEMENTED"
	default:
		return "INTERNAL_SERVER_ERROR"
	}
}
