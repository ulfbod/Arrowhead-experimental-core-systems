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
	"arrowhead/core/internal/httputil"
	coremodel "arrowhead/core/internal/model"
)

const authOrigin = "authentication"

// pageReqOrZero returns the dereferenced PageRequest, or a zero value if p is nil.
func pageReqOrZero(p *coremodel.PageRequest) coremodel.PageRequest {
	if p == nil {
		return coremodel.PageRequest{}
	}
	return *p
}

type Handler struct {
	svc         *service.AuthService
	mgmtAuthURL string
}

func NewHandler(svc *service.AuthService, mgmtAuthURL string) http.Handler {
	h := &Handler{svc: svc, mgmtAuthURL: mgmtAuthURL}
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

// statusFor maps sentinel errors to HTTP status codes.
func statusFor(err error) int {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, service.ErrInvalidToken):
		return http.StatusUnauthorized
	case errors.Is(err, service.ErrMissingSystemName):
		return http.StatusBadRequest
	default:
		return http.StatusBadRequest
	}
}

// POST /authentication/identity/login
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", authOrigin)
		return
	}
	var raw struct {
		SystemName  string          `json:"systemName"`
		Credentials json.RawMessage `json:"credentials,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", authOrigin)
		return
	}
	req := model.LoginRequest{
		SystemName:     raw.SystemName,
		CredentialsMap: make(map[string]string),
	}
	// When an identity repo is configured, credentials must be a JSON object
	// with a "password" field. Plain strings and null are rejected (G43).
	if h.svc.HasIdentityRepo() {
		if len(raw.Credentials) == 0 || string(raw.Credentials) == "null" {
			httputil.WriteError(w, http.StatusBadRequest, "credentials must be a JSON object with a 'password' field", authOrigin)
			return
		}
		var m map[string]string
		if err := json.Unmarshal(raw.Credentials, &m); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "credentials must be a JSON object with a 'password' field", authOrigin)
			return
		}
		if _, ok := m["password"]; !ok {
			httputil.WriteError(w, http.StatusBadRequest, "credentials object must contain a 'password' field", authOrigin)
			return
		}
		req.CredentialsMap = m
	} else if len(raw.Credentials) > 0 {
		// No identity repo — accept object or string for backward compat.
		var m map[string]string
		if err := json.Unmarshal(raw.Credentials, &m); err == nil {
			req.CredentialsMap = m
		} else {
			var s string
			if err := json.Unmarshal(raw.Credentials, &s); err == nil {
				req.CredentialsMap["password"] = s
			}
		}
	}
	resp, err := h.svc.Login(req)
	if err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), authOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp, authOrigin)
}

// POST /authentication/identity/logout
// Authorization: Bearer <token>
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", authOrigin)
		return
	}
	token := httputil.ExtractBearer(r)
	if token == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "missing Authorization: Bearer token", authOrigin)
		return
	}
	if err := h.svc.Logout(token); err != nil {
		httputil.WriteError(w, http.StatusUnauthorized, err.Error(), authOrigin)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// GET /authentication/identity/verify/{token}
func (h *Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "GET required", authOrigin)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/authentication/identity/verify/")
	if token == "" {
		httputil.WriteJSON(w, http.StatusOK, model.VerifyResponse{Verified: false}, authOrigin)
		return
	}
	resp, err := h.svc.Verify(token)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error(), authOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp, authOrigin)
}

// POST /authentication/identity/change
func (h *Handler) handleChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", authOrigin)
		return
	}
	var req struct {
		SystemName     string         `json:"systemName"`
		Credentials    map[string]any `json:"credentials"`
		NewCredentials map[string]any `json:"newCredentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", authOrigin)
		return
	}
	if err := h.svc.ChangeCredentials(req.SystemName); err != nil {
		httputil.WriteError(w, http.StatusUnauthorized, err.Error(), authOrigin)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// POST /authentication/mgmt/identities/query
func (h *Handler) handleMgmtIdentitiesQuery(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, authOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", authOrigin)
		return
	}
	var raw struct {
		Pagination *coremodel.PageRequest `json:"pagination"`
	}
	json.NewDecoder(r.Body).Decode(&raw) //nolint:errcheck — empty body OK
	records := h.svc.QueryIdentities()
	page, total := coremodel.Paginate(records, pageReqOrZero(raw.Pagination), func(r service.IdentityRecord) string { return r.SystemName })
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"identities": page,
		"count":      len(page),
		"totalCount": total,
	}, authOrigin)
}

// /authentication/mgmt/identities — dispatches on method
func (h *Handler) handleMgmtIdentities(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, authOrigin) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		h.mgmtIdentitiesCreate(w, r)
	case http.MethodPut:
		h.mgmtIdentitiesUpdate(w, r)
	case http.MethodDelete:
		h.mgmtIdentitiesDelete(w, r)
	default:
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST, PUT, or DELETE required", authOrigin)
	}
}

func (h *Handler) mgmtIdentitiesCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AuthenticationMethod string                          `json:"authenticationMethod"`
		Identities           []service.CreateIdentityRequest `json:"identities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", authOrigin)
		return
	}
	// Validate all systemNames against PascalCase before creating any identity (atomic rejection).
	for _, id := range req.Identities {
		if msg := httputil.ValidatePascalCase(id.SystemName); msg != "" {
			httputil.WriteError(w, http.StatusBadRequest, msg, authOrigin)
			return
		}
	}
	records, err := h.svc.CreateIdentities(req.Identities)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error(), authOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"identities": records}, authOrigin)
}

func (h *Handler) mgmtIdentitiesUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AuthenticationMethod string                          `json:"authenticationMethod"`
		Identities           []service.CreateIdentityRequest `json:"identities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", authOrigin)
		return
	}
	records, err := h.svc.UpdateIdentities(req.Identities)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error(), authOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"identities": records}, authOrigin)
}

func (h *Handler) mgmtIdentitiesDelete(w http.ResponseWriter, r *http.Request) {
	namesParam := r.URL.Query().Get("names")
	if namesParam == "" {
		httputil.WriteError(w, http.StatusBadRequest, "names query parameter required", authOrigin)
		return
	}
	names := strings.Split(namesParam, ",")
	h.svc.DeleteIdentities(names)
	w.WriteHeader(http.StatusOK)
}

// /authentication/mgmt/sessions — dispatches on method
func (h *Handler) handleMgmtSessions(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, authOrigin) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var raw struct {
			Pagination *coremodel.PageRequest `json:"pagination"`
		}
		json.NewDecoder(r.Body).Decode(&raw) //nolint:errcheck — empty body OK
		sessions := h.svc.QuerySessions()
		page, total := coremodel.Paginate(sessions, pageReqOrZero(raw.Pagination), func(s service.SessionRecord) string { return s.SystemName })
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"sessions":   page,
			"count":      len(page),
			"totalCount": total,
		}, authOrigin)
	case http.MethodDelete:
		namesParam := r.URL.Query().Get("names")
		if namesParam == "" {
			httputil.WriteError(w, http.StatusBadRequest, "names query parameter required", authOrigin)
			return
		}
		names := strings.Split(namesParam, ",")
		h.svc.RevokeSessions(names)
		w.WriteHeader(http.StatusOK)
	default:
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST or DELETE required", authOrigin)
	}
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "authentication"}, authOrigin)
}
