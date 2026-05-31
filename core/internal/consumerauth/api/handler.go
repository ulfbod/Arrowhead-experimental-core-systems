// Package api implements the HTTP interface for the ConsumerAuthorization core system.
// AH5 services: authorization (grant, revoke, lookup, verify) + management
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	blclient "arrowhead/core/internal/blacklist/client"
	"arrowhead/core/internal/consumerauth/model"
	"arrowhead/core/internal/consumerauth/service"
	"arrowhead/core/internal/httputil"
	coremodel "arrowhead/core/internal/model"
)

const cauthOrigin = "consumerauthorization"

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
	blClient    blclient.BlacklistClient
}

// NewHandler creates a ConsumerAuthorization HTTP handler.
// mgmtAuthURL guards management endpoints; blClient checks blacklist on grant/verify.
// Pass blclient.NopClient{} to disable blacklist enforcement.
func NewHandler(svc *service.AuthService, mgmtAuthURL string, blClient blclient.BlacklistClient) http.Handler {
	h := &Handler{svc: svc, mgmtAuthURL: mgmtAuthURL, blClient: blClient}
	mux := http.NewServeMux()
	mux.HandleFunc("/consumerauthorization/authorization/grant", h.handleGrant)
	mux.HandleFunc("/consumerauthorization/authorization/revoke/", h.handleRevoke)
	mux.HandleFunc("/consumerauthorization/authorization/lookup", h.handleLookup)
	mux.HandleFunc("/consumerauthorization/authorization/verify", h.handleVerify)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/grant", h.handleMgmtGrant)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/revoke", h.handleMgmtRevoke)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/query", h.handleMgmtQuery)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/check", h.handleMgmtCheck)
	// bulk management endpoints (G39)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/grant-policies", h.handleMgmtGrantPolicies)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/revoke-policies", h.handleMgmtRevokePolicies)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/query-policies", h.handleMgmtQueryPolicies)
	mux.HandleFunc("/consumerauthorization/authorization/mgmt/check-policies", h.handleMgmtCheckPolicies)
	mux.HandleFunc("/consumerauthorization/authorization/health", h.handleHealth)
	mux.HandleFunc("/health", h.handleHealth)
	// authorization-token sub-service
	mux.HandleFunc("/consumerauthorization/authorization-token/generate", h.handleTokenGenerate)
	mux.HandleFunc("/consumerauthorization/authorization-token/verify/", h.handleTokenVerify)
	mux.HandleFunc("/consumerauthorization/authorization-token/public-key", h.handleTokenPublicKey)
	mux.HandleFunc("/consumerauthorization/authorization-token/encryption-key", h.handleEncryptionKey)
	// token bulk management endpoints (G38)
	mux.HandleFunc("/consumerauthorization/authorization-token/mgmt/generate-tokens", h.handleMgmtGenerateTokens)
	mux.HandleFunc("/consumerauthorization/authorization-token/mgmt/revoke-tokens", h.handleMgmtRevokeTokens)
	mux.HandleFunc("/consumerauthorization/authorization-token/mgmt/query-tokens", h.handleMgmtQueryTokens)
	mux.HandleFunc("/consumerauthorization/authorization-token/mgmt/add-encryption-keys", h.handleMgmtAddEncryptionKeys)
	mux.HandleFunc("/consumerauthorization/authorization-token/mgmt/remove-encryption-keys", h.handleMgmtRemoveEncryptionKeys)
	return mux
}

// statusFor maps sentinel errors to HTTP status codes.
func statusFor(err error) int {
	switch {
	case errors.Is(err, service.ErrDuplicateRule):
		return http.StatusConflict
	case errors.Is(err, service.ErrUnsupportedVariant):
		return http.StatusNotImplemented
	case errors.Is(err, service.ErrRuleNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrTokenNotFound):
		return http.StatusNotFound
	default:
		return http.StatusBadRequest
	}
}

// POST /authorization/grant
func (h *Handler) handleGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var req model.GrantRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	ctx := context.Background()
	if req.Provider != "" {
		if blacklisted, _ := h.blClient.IsBlacklisted(ctx, req.Provider); blacklisted {
			httputil.WriteError(w, http.StatusForbidden, "provider system is blacklisted", cauthOrigin)
			return
		}
	}
	policy, err := h.svc.Grant(req)
	if err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), cauthOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, policy, cauthOrigin)
}

// DELETE /authorization/revoke/{instanceId}
// instanceId may be URL-encoded (pipes as %7C)
func (h *Handler) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", cauthOrigin)
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/consumerauthorization/authorization/revoke/")
	instanceID, err := url.PathUnescape(raw)
	if err != nil || instanceID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "invalid instanceId", cauthOrigin)
		return
	}
	if err := h.svc.Revoke(instanceID); err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), cauthOrigin)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// POST /authorization/lookup
// Requires at least one of: instanceIds, cloudIdentifiers, targetNames.
func (h *Handler) handleLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var raw struct {
		model.LookupRequest
		Pagination *coremodel.PageRequest `json:"pagination"`
	}
	if !httputil.DecodeJSON(w, r, &raw) {
		return
	}
	if !hasFilter(raw.LookupRequest) {
		httputil.WriteError(w, http.StatusBadRequest, "at least one of instanceIds, cloudIdentifiers, targetNames must be provided", cauthOrigin)
		return
	}
	result := h.svc.Lookup(raw.LookupRequest)
	page, total := coremodel.Paginate(result.Policies, pageReqOrZero(raw.Pagination), func(p model.AuthPolicy) string { return p.InstanceID })
	httputil.WriteJSON(w, http.StatusOK, model.LookupResponse{Policies: page, Count: len(page), TotalCount: total}, cauthOrigin)
}

// POST /authorization/verify — returns plain JSON Boolean
func (h *Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var req model.VerifyRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	ctx := context.Background()
	for _, name := range []string{req.Consumer, req.Provider} {
		if name == "" {
			continue
		}
		if blacklisted, _ := h.blClient.IsBlacklisted(ctx, name); blacklisted {
			// Blacklisted → not authorized; return false without 4xx.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(false) //nolint:errcheck
			return
		}
	}
	authorized := h.svc.Verify(req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(authorized) //nolint:errcheck
}

// POST /authorization/mgmt/grant
func (h *Handler) handleMgmtGrant(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var req model.GrantRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	policy, err := h.svc.Grant(req)
	if err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), cauthOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, policy, cauthOrigin)
}

// DELETE /authorization/mgmt/revoke?instanceIds=...
func (h *Handler) handleMgmtRevoke(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", cauthOrigin)
		return
	}
	idsParam := r.URL.Query().Get("instanceIds")
	if idsParam == "" {
		httputil.WriteError(w, http.StatusBadRequest, "instanceIds query parameter required", cauthOrigin)
		return
	}
	h.svc.BulkRevoke(strings.Split(idsParam, ","))
	w.WriteHeader(http.StatusOK)
}

// POST /authorization/mgmt/query — returns all policies (no filter required)
func (h *Handler) handleMgmtQuery(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var raw struct {
		model.LookupRequest
		Pagination *coremodel.PageRequest `json:"pagination"`
	}
	json.NewDecoder(r.Body).Decode(&raw) //nolint:errcheck
	var policies []model.AuthPolicy
	if hasFilter(raw.LookupRequest) {
		resp := h.svc.Lookup(raw.LookupRequest)
		policies = resp.Policies
	} else {
		policies = h.svc.AllPolicies()
	}
	page, total := coremodel.Paginate(policies, pageReqOrZero(raw.Pagination), func(p model.AuthPolicy) string { return p.InstanceID })
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"policies":   page,
		"count":      len(page),
		"totalCount": total,
	}, cauthOrigin)
}

// POST /authorization/mgmt/check — bulk verify
func (h *Handler) handleMgmtCheck(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var reqs []model.VerifyRequest
	if !httputil.DecodeJSON(w, r, &reqs) {
		return
	}
	results := h.svc.BulkVerify(reqs)
	httputil.WriteJSON(w, http.StatusOK, results, cauthOrigin)
}

// POST /authorization-token/generate
func (h *Handler) handleTokenGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var req model.TokenGenerateRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	desc, err := h.svc.GenerateAuthToken(req)
	if err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), cauthOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, desc, cauthOrigin)
}

// GET /authorization-token/verify/{accessToken}
func (h *Handler) handleTokenVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "GET required", cauthOrigin)
		return
	}
	accessToken := strings.TrimPrefix(r.URL.Path, "/consumerauthorization/authorization-token/verify/")
	if accessToken == "" {
		httputil.WriteError(w, http.StatusBadRequest, "accessToken is required", cauthOrigin)
		return
	}
	resp, ok := h.svc.VerifyAuthToken(accessToken)
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, service.ErrTokenNotFound.Error(), cauthOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp, cauthOrigin)
}

// GET /authorization-token/public-key — not implemented (returns 404)
func (h *Handler) handleTokenPublicKey(w http.ResponseWriter, r *http.Request) {
	httputil.WriteError(w, http.StatusNotFound, "public-key endpoint not implemented", cauthOrigin)
}

// POST /authorization-token/encryption-key — register key
// DELETE /authorization-token/encryption-key — remove key
func (h *Handler) handleEncryptionKey(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req model.EncryptionKeyRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		h.svc.RegisterEncryptionKey(req)
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		systemName := r.URL.Query().Get("systemName")
		if systemName == "" {
			httputil.WriteError(w, http.StatusBadRequest, "systemName query parameter required", cauthOrigin)
			return
		}
		h.svc.RemoveEncryptionKey(systemName)
		w.WriteHeader(http.StatusOK)
	default:
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST or DELETE required", cauthOrigin)
	}
}

// POST /authorization/mgmt/grant-policies — bulk grant
func (h *Handler) handleMgmtGrantPolicies(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var req model.BulkGrantRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	results := h.svc.BulkGrant(req.Policies)
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"results": results, "count": len(results)}, cauthOrigin)
}

// DELETE /authorization/mgmt/revoke-policies — bulk revoke via request body
func (h *Handler) handleMgmtRevokePolicies(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", cauthOrigin)
		return
	}
	var req model.BulkRevokeRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	h.svc.BulkRevoke(req.InstanceIDs)
	w.WriteHeader(http.StatusOK)
}

// POST /authorization/mgmt/query-policies — query with filters + pagination
func (h *Handler) handleMgmtQueryPolicies(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var raw struct {
		model.LookupRequest
		Pagination *coremodel.PageRequest `json:"pagination"`
	}
	json.NewDecoder(r.Body).Decode(&raw) //nolint:errcheck
	var policies []model.AuthPolicy
	if hasFilter(raw.LookupRequest) {
		resp := h.svc.Lookup(raw.LookupRequest)
		policies = resp.Policies
	} else {
		policies = h.svc.AllPolicies()
	}
	page, total := coremodel.Paginate(policies, pageReqOrZero(raw.Pagination), func(p model.AuthPolicy) string { return p.InstanceID })
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"policies":   page,
		"count":      len(page),
		"totalCount": total,
	}, cauthOrigin)
}

// POST /authorization/mgmt/check-policies — bulk authorization check
func (h *Handler) handleMgmtCheckPolicies(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var reqs []model.VerifyRequest
	if !httputil.DecodeJSON(w, r, &reqs) {
		return
	}
	results := make([]model.BulkCheckResult, len(reqs))
	for i, req := range reqs {
		results[i] = model.BulkCheckResult{
			Consumer:   req.Consumer,
			Provider:   req.Provider,
			Target:     req.Target,
			TargetType: req.TargetType,
			Scope:      req.Scope,
			Authorized: h.svc.Verify(req),
		}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"results": results, "count": len(results)}, cauthOrigin)
}

// POST /authorization-token/mgmt/generate-tokens — bulk token generation
func (h *Handler) handleMgmtGenerateTokens(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var req model.BulkGenerateRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	results := h.svc.BulkGenerateTokens(req.Requests)
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"results": results, "count": len(results)}, cauthOrigin)
}

// DELETE /authorization-token/mgmt/revoke-tokens — bulk token revocation
func (h *Handler) handleMgmtRevokeTokens(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", cauthOrigin)
		return
	}
	var req model.BulkRevokeTokensRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	h.svc.RevokeTokens(req.Tokens)
	w.WriteHeader(http.StatusOK)
}

// POST /authorization-token/mgmt/query-tokens — list tokens with pagination
func (h *Handler) handleMgmtQueryTokens(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var raw struct {
		Pagination *coremodel.PageRequest `json:"pagination"`
	}
	json.NewDecoder(r.Body).Decode(&raw) //nolint:errcheck
	all := h.svc.ListTokens()
	page, total := coremodel.Paginate(all, pageReqOrZero(raw.Pagination), func(t model.TokenRecord) string { return t.Token })
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"tokens":     page,
		"count":      len(page),
		"totalCount": total,
	}, cauthOrigin)
}

// POST /authorization-token/mgmt/add-encryption-keys — bulk add encryption keys
func (h *Handler) handleMgmtAddEncryptionKeys(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", cauthOrigin)
		return
	}
	var req model.BulkEncryptionKeysRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	h.svc.BulkAddEncryptionKeys(req.Keys)
	w.WriteHeader(http.StatusCreated)
}

// DELETE /authorization-token/mgmt/remove-encryption-keys — bulk remove encryption keys
func (h *Handler) handleMgmtRemoveEncryptionKeys(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, cauthOrigin) {
		return
	}
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", cauthOrigin)
		return
	}
	var req model.BulkRemoveEncryptionKeysRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	h.svc.BulkRemoveEncryptionKeys(req.SystemNames)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "consumerauthorization"}, cauthOrigin)
}

func hasFilter(req model.LookupRequest) bool {
	return len(req.InstanceIDs) > 0 || len(req.CloudIdentifiers) > 0 || len(req.TargetNames) > 0
}
