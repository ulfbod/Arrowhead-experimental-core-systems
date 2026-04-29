// Package api implements the HTTP interface for SimpleStoreServiceOrchestration.
// AH5 services: serviceOrchestration (pull) + serviceOrchestrationSimpleStoreManagement
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/simplestore/model"
	"arrowhead/core/internal/orchestration/simplestore/service"
)

type Handler struct {
	orch *service.SimpleStoreOrchestrator
}

func NewHandler(orch *service.SimpleStoreOrchestrator) http.Handler {
	h := &Handler{orch: orch}
	mux := http.NewServeMux()
	mux.HandleFunc("/orchestration/simplestore", h.handleOrchestrate)
	mux.HandleFunc("/orchestration/simplestore/rules", h.handleRules)
	mux.HandleFunc("/orchestration/simplestore/rules/", h.handleRuleByID)
	mux.HandleFunc("/orchestration/simplestore/health", h.handleHealth)
	mux.HandleFunc("/health", h.handleHealth)
	return mux
}

// POST /orchestration/simplestore
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

// GET /orchestration/simplestore/rules  — list all rules
// POST /orchestration/simplestore/rules — create rule
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

// DELETE /orchestration/simplestore/rules/{id}
func (h *Handler) handleRuleByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/orchestration/simplestore/rules/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	if err := h.orch.DeleteRule(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
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
	writeJSON(w, status, map[string]string{"error": msg})
}
