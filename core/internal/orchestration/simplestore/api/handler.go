// Package api implements the HTTP interface for SimpleStoreServiceOrchestration.
// AH5 services: serviceOrchestration (pull) + serviceOrchestrationSimpleStoreManagement
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/simplestore/model"
	"arrowhead/core/internal/orchestration/simplestore/service"
)

type Handler struct {
	orch *service.SimpleStoreOrchestrator
	subs *service.SubscriptionStore
}

func NewHandler(orch *service.SimpleStoreOrchestrator) http.Handler {
	h := &Handler{orch: orch, subs: service.NewSubscriptionStore()}
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

// POST /serviceorchestration/orchestration/pull
func (h *Handler) handleOrchestrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req orchmodel.OrchestrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	resp, err := h.orch.Orchestrate(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /serviceorchestration/orchestration/mgmt/simple-store/create
func (h *Handler) handleMgmtCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.CreateRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	rule, err := h.orch.CreateRule(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

// POST /serviceorchestration/orchestration/mgmt/simple-store/query
func (h *Handler) handleMgmtQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	// Body is a filter object (ignored for now — returns all rules)
	writeJSON(w, http.StatusOK, h.orch.ListRules())
}

// POST /serviceorchestration/orchestration/mgmt/simple-store/modify-priorities
func (h *Handler) handleMgmtModifyPriorities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req model.ModifyPrioritiesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	h.orch.ModifyPriorities(req)
	writeJSON(w, http.StatusOK, h.orch.ListRules())
}

// Legacy alias: GET/POST /serviceorchestration/orchestration/simplestore/rules
func (h *Handler) handleRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, h.orch.ListRules())
	case http.MethodPost:
		var req model.CreateRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		rule, err := h.orch.CreateRule(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, rule)
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or POST required")
	}
}

// Legacy alias: DELETE /serviceorchestration/orchestration/simplestore/rules/{id}
func (h *Handler) handleRuleByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/serviceorchestration/orchestration/simplestore/rules/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := h.orch.DeleteRule(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /serviceorchestration/orchestration/subscribe
func (h *Handler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req service.CreateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	sub, created := h.subs.Subscribe(req)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, sub)
}

// DELETE /serviceorchestration/orchestration/unsubscribe/{subscriptionId}
func (h *Handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "simplestoreorchestration"})
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
	writeJSON(w, status, errBody{ErrorMessage: msg, ErrorCode: status, ExceptionType: exType, Origin: "serviceorchestration.orchestration.simplestore"})
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
