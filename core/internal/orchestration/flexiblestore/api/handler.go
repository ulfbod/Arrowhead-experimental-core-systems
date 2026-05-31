// Package api implements the HTTP interface for FlexibleStoreServiceOrchestration.
package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"arrowhead/core/internal/httputil"
	coremodel "arrowhead/core/internal/model"
	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/flexiblestore/model"
	"arrowhead/core/internal/orchestration/flexiblestore/service"
)

const fsOrigin = "serviceorchestration.orchestration.flexiblestore"

// pageReqOrZero returns the dereferenced PageRequest, or a zero value if p is nil.
func pageReqOrZero(p *coremodel.PageRequest) coremodel.PageRequest {
	if p == nil {
		return coremodel.PageRequest{}
	}
	return *p
}

type Handler struct {
	orch        *service.FlexibleStoreOrchestrator
	mgmtAuthURL string
}

func NewHandler(orch *service.FlexibleStoreOrchestrator, mgmtAuthURL string) http.Handler {
	h := &Handler{orch: orch, mgmtAuthURL: mgmtAuthURL}
	mux := http.NewServeMux()
	mux.HandleFunc("/serviceorchestration/orchestration/pull", h.handleOrchestrate)
	mux.HandleFunc("/serviceorchestration/orchestration/flexiblestore/rules", h.handleRules)
	mux.HandleFunc("/serviceorchestration/orchestration/flexiblestore/rules/", h.handleRuleByID)
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
	default:
		return http.StatusBadRequest
	}
}

// POST /orchestration/flexiblestore
func (h *Handler) handleOrchestrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", fsOrigin)
		return
	}
	var req orchmodel.OrchestrationRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	resp, err := h.orch.Orchestrate(req)
	if err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), fsOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp, fsOrigin)
}

// GET  /orchestration/flexiblestore/rules
// POST /orchestration/flexiblestore/rules
func (h *Handler) handleRules(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, fsOrigin) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		all := h.orch.ListRules()
		httputil.WriteJSON(w, http.StatusOK, model.RulesResponse{Rules: all.Rules, Count: len(all.Rules), TotalCount: all.TotalCount}, fsOrigin)
	case http.MethodPost:
		var req model.CreateFlexibleRuleRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		rule, err := h.orch.CreateRule(req)
		if err != nil {
			httputil.WriteError(w, statusFor(err), err.Error(), fsOrigin)
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, rule, fsOrigin)
	default:
		httputil.WriteError(w, http.StatusMethodNotAllowed, "GET or POST required", fsOrigin)
	}
}

// DELETE /orchestration/flexiblestore/rules/{id}
func (h *Handler) handleRuleByID(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, fsOrigin) {
		return
	}
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", fsOrigin)
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/serviceorchestration/orchestration/flexiblestore/rules/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, "invalid rule id", fsOrigin)
		return
	}
	if err := h.orch.DeleteRule(id); err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), fsOrigin)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "flexiblestoreorchestration"}, fsOrigin)
}
