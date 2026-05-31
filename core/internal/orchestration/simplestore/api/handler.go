// Package api implements the HTTP interface for SimpleStoreServiceOrchestration.
// AH5 services: serviceOrchestration (pull) + serviceOrchestrationSimpleStoreManagement
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"arrowhead/core/internal/httputil"
	coremodel "arrowhead/core/internal/model"
	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/simplestore/model"
	"arrowhead/core/internal/orchestration/simplestore/service"
)

const ssOrigin = "serviceorchestration.orchestration.simplestore"

// pageReqOrZero returns the dereferenced PageRequest, or a zero value if p is nil.
func pageReqOrZero(p *coremodel.PageRequest) coremodel.PageRequest {
	if p == nil {
		return coremodel.PageRequest{}
	}
	return *p
}

type Handler struct {
	orch        *service.SimpleStoreOrchestrator
	subs        *service.SubscriptionStore
	mgmtAuthURL string
}

func NewHandler(orch *service.SimpleStoreOrchestrator, mgmtAuthURL string) http.Handler {
	h := &Handler{orch: orch, subs: service.NewSubscriptionStore(), mgmtAuthURL: mgmtAuthURL}
	mux := http.NewServeMux()
	// Pull orchestration
	mux.HandleFunc("/serviceorchestration/orchestration/pull", h.handleOrchestrate)
	// Push orchestration — discovery endpoints (Step 19.1)
	mux.HandleFunc("/serviceorchestration/orchestration/subscribe", h.handleSubscribe)
	mux.HandleFunc("/serviceorchestration/orchestration/unsubscribe/", h.handleUnsubscribe)
	// New AH5-aligned mgmt paths
	mux.HandleFunc("/serviceorchestration/orchestration/mgmt/simple-store/create", h.handleMgmtCreate)
	mux.HandleFunc("/serviceorchestration/orchestration/mgmt/simple-store/query", h.handleMgmtQuery)
	mux.HandleFunc("/serviceorchestration/orchestration/mgmt/simple-store/modify-priorities", h.handleMgmtModifyPriorities)
	// Legacy alias paths (kept during transition)
	mux.HandleFunc("/serviceorchestration/orchestration/simplestore/rules", h.handleRules)
	mux.HandleFunc("/serviceorchestration/orchestration/simplestore/rules/", h.handleRuleByID)
	// Health
	mux.HandleFunc("/serviceorchestration/orchestration/pull/health", h.handleHealth)
	mux.HandleFunc("/health", h.handleHealth)
	return mux
}

// statusFor maps sentinel errors to HTTP status codes.
func statusFor(err error) int {
	switch {
	case errors.Is(err, service.ErrRuleNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrMissingConsumer):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrMissingService):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrMissingProvider):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrMissingServiceUri):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrMissingInterfaces):
		return http.StatusBadRequest
	case errors.Is(err, orchmodel.ErrInterclouNotSupported):
		return http.StatusNotImplemented
	default:
		return http.StatusBadRequest
	}
}

// POST /serviceorchestration/orchestration/pull
func (h *Handler) handleOrchestrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", ssOrigin)
		return
	}
	var req orchmodel.OrchestrationRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	resp, err := h.orch.Orchestrate(req)
	if err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), ssOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp, ssOrigin)
}

// POST /serviceorchestration/orchestration/mgmt/simple-store/create
func (h *Handler) handleMgmtCreate(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, ssOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", ssOrigin)
		return
	}
	var req model.CreateRuleRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	rule, err := h.orch.CreateRule(req)
	if err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), ssOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, rule, ssOrigin)
}

// POST /serviceorchestration/orchestration/mgmt/simple-store/query
func (h *Handler) handleMgmtQuery(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, ssOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", ssOrigin)
		return
	}
	var raw struct {
		Pagination *coremodel.PageRequest `json:"pagination"`
	}
	json.NewDecoder(r.Body).Decode(&raw) //nolint:errcheck — empty body OK
	all := h.orch.ListRules()
	page, total := coremodel.Paginate(all.Rules, pageReqOrZero(raw.Pagination), func(r model.StoreRule) string { return r.ID })
	httputil.WriteJSON(w, http.StatusOK, model.RulesResponse{Rules: page, Count: len(page), TotalCount: total}, ssOrigin)
}

// POST /serviceorchestration/orchestration/mgmt/simple-store/modify-priorities
func (h *Handler) handleMgmtModifyPriorities(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, ssOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", ssOrigin)
		return
	}
	var req model.ModifyPrioritiesRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	h.orch.ModifyPriorities(req)
	httputil.WriteJSON(w, http.StatusOK, h.orch.ListRules(), ssOrigin)
}

// Legacy alias: GET/POST /serviceorchestration/orchestration/simplestore/rules
func (h *Handler) handleRules(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, ssOrigin) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		all := h.orch.ListRules()
		httputil.WriteJSON(w, http.StatusOK, model.RulesResponse{Rules: all.Rules, Count: len(all.Rules), TotalCount: all.TotalCount}, ssOrigin)
	case http.MethodPost:
		var req model.CreateRuleRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		rule, err := h.orch.CreateRule(req)
		if err != nil {
			httputil.WriteError(w, statusFor(err), err.Error(), ssOrigin)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, rule, ssOrigin)
	default:
		httputil.WriteError(w, http.StatusMethodNotAllowed, "GET or POST required", ssOrigin)
	}
}

// Legacy alias: DELETE /serviceorchestration/orchestration/simplestore/rules/{id}
func (h *Handler) handleRuleByID(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, ssOrigin) {
		return
	}
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", ssOrigin)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/serviceorchestration/orchestration/simplestore/rules/")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "id is required", ssOrigin)
		return
	}
	if err := h.orch.DeleteRule(id); err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), ssOrigin)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /serviceorchestration/orchestration/subscribe
func (h *Handler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", ssOrigin)
		return
	}
	var req service.CreateSubscriptionRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	sub, created := h.subs.Subscribe(req)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httputil.WriteJSON(w, status, sub, ssOrigin)
}

// DELETE /serviceorchestration/orchestration/unsubscribe/{subscriptionId}
func (h *Handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", ssOrigin)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/serviceorchestration/orchestration/unsubscribe/")
	if h.subs.Unsubscribe(id) {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "simplestoreorchestration"}, ssOrigin)
}
