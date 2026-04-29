// Package api implements the HTTP interface for FlexibleStoreServiceOrchestration.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/flexiblestore/model"
	"arrowhead/core/internal/orchestration/flexiblestore/service"
)

type Handler struct {
	orch *service.FlexibleStoreOrchestrator
}

func NewHandler(orch *service.FlexibleStoreOrchestrator) http.Handler {
	h := &Handler{orch: orch}
	mux := http.NewServeMux()
	mux.HandleFunc("/orchestration/flexiblestore", h.handleOrchestrate)
	mux.HandleFunc("/orchestration/flexiblestore/rules", h.handleRules)
	mux.HandleFunc("/orchestration/flexiblestore/rules/", h.handleRuleByID)
	mux.HandleFunc("/health", h.handleHealth)
	return mux
}

// POST /orchestration/flexiblestore
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

// GET  /orchestration/flexiblestore/rules
// POST /orchestration/flexiblestore/rules
func (h *Handler) handleRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, h.orch.ListRules())
	case http.MethodPost:
		var req model.CreateFlexibleRuleRequest
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

// DELETE /orchestration/flexiblestore/rules/{id}
func (h *Handler) handleRuleByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/orchestration/flexiblestore/rules/")
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "flexiblestoreorchestration"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
