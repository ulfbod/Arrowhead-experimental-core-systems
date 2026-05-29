// Package api implements the HTTP interface for the Authentication core system.
// AH5 service: identity (login, logout, verify) + management
package api

import (
	"encoding/json"
	"errors"
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
	mux.HandleFunc("/authentication/identity/verify/", h.handleVerify)
	mux.HandleFunc("/authentication/identity/change", h.handleChange)
	mux.HandleFunc("/authentication/mgmt/identities/query", h.handleMgmtIdentitiesQuery)
	mux.HandleFunc("/authentication/mgmt/identities", h.handleMgmtIdentities)
	mux.HandleFunc("/authentication/mgmt/sessions", h.handleMgmtSessions)
	mux.HandleFunc("/authentication/health", h.handleHealth)
	mux.HandleFunc("/health", h.handleHealth)
	return mux
}

// POST /authentication/identity/login
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var raw struct {
		SystemName  string          `json:"systemName"`
		Credentials json.RawMessage `json:"credentials,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req := model.LoginRequest{
		SystemName:     raw.SystemName,
		CredentialsMap: make(map[string]string),
	}
	if len(raw.Credentials) > 0 {
		// Try as object: {"password": "..."}
		var m map[string]string
		if err := json.Unmarshal(raw.Credentials, &m); err == nil {
			req.CredentialsMap = m
		} else {
			// Try as plain string (legacy)
			var s string
			if err := json.Unmarshal(raw.Credentials, &s); err == nil {
				req.CredentialsMap["password"] = s
			}
		}
	}
	resp, err := h.svc.Login(req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// POST /authentication/identity/logout
// Authorization: Bearer <token>
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
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

// GET /authentication/identity/verify/{token}
func (h *Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/authentication/identity/verify/")
	if token == "" {
		writeJSON(w, http.StatusOK, model.VerifyResponse{Verified: false})
		return
	}
	resp, err := h.svc.Verify(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /authentication/identity/change
func (h *Handler) handleChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		SystemName     string         `json:"systemName"`
		Credentials    map[string]any `json:"credentials"`
		NewCredentials map[string]any `json:"newCredentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.svc.ChangeCredentials(req.SystemName); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// POST /authentication/mgmt/identities/query
func (h *Handler) handleMgmtIdentitiesQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	records := h.svc.QueryIdentities()
	writeJSON(w, http.StatusOK, map[string]any{
		"identities": records,
		"totalCount": len(records),
	})
}

// /authentication/mgmt/identities — dispatches on method
func (h *Handler) handleMgmtIdentities(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.mgmtIdentitiesCreate(w, r)
	case http.MethodPut:
		h.mgmtIdentitiesUpdate(w, r)
	case http.MethodDelete:
		h.mgmtIdentitiesDelete(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "POST, PUT, or DELETE required")
	}
}

func (h *Handler) mgmtIdentitiesCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AuthenticationMethod string                          `json:"authenticationMethod"`
		Identities           []service.CreateIdentityRequest `json:"identities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	records, err := h.svc.CreateIdentities(req.Identities)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"identities": records})
}

func (h *Handler) mgmtIdentitiesUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AuthenticationMethod string                          `json:"authenticationMethod"`
		Identities           []service.CreateIdentityRequest `json:"identities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	records, err := h.svc.UpdateIdentities(req.Identities)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"identities": records})
}

func (h *Handler) mgmtIdentitiesDelete(w http.ResponseWriter, r *http.Request) {
	namesParam := r.URL.Query().Get("names")
	if namesParam == "" {
		writeError(w, http.StatusBadRequest, "names query parameter required")
		return
	}
	names := strings.Split(namesParam, ",")
	h.svc.DeleteIdentities(names)
	w.WriteHeader(http.StatusOK)
}

// /authentication/mgmt/sessions — dispatches on method
func (h *Handler) handleMgmtSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		sessions := h.svc.QuerySessions()
		writeJSON(w, http.StatusOK, map[string]any{
			"sessions":   sessions,
			"totalCount": len(sessions),
		})
	case http.MethodDelete:
		namesParam := r.URL.Query().Get("names")
		if namesParam == "" {
			writeError(w, http.StatusBadRequest, "names query parameter required")
			return
		}
		names := strings.Split(namesParam, ",")
		h.svc.RevokeSessions(names)
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "POST or DELETE required")
	}
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
	exType := errTypeForStatus(status)
	type errBody struct {
		ErrorMessage  string `json:"errorMessage"`
		ErrorCode     int    `json:"errorCode"`
		ExceptionType string `json:"exceptionType"`
		Origin        string `json:"origin"`
	}
	writeJSON(w, status, errBody{ErrorMessage: msg, ErrorCode: status, ExceptionType: exType, Origin: "authentication.identity"})
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
	case http.StatusLocked:
		return "LOCKED"
	default:
		return "INTERNAL_SERVER_ERROR"
	}
}
