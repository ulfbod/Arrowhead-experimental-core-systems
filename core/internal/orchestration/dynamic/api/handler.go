// Package api implements the HTTP interface for DynamicServiceOrchestration.
// AH5 service: serviceOrchestration (pull)
// Strategy: "dynamic" — real-time SR lookup + optional auth check.
package api

import (
	"encoding/json"
	"net/http"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/dynamic/service"
)

type Handler struct {
	orch *service.DynamicOrchestrator
}

func NewHandler(orch *service.DynamicOrchestrator) http.Handler {
	h := &Handler{orch: orch}
	mux := http.NewServeMux()
	mux.HandleFunc("/orchestration/dynamic", h.handleOrchestrate)
	mux.HandleFunc("/orchestration/dynamic/health", h.handleHealth)
	mux.HandleFunc("/health", h.handleHealth)
	return mux
}

// POST /orchestration/dynamic
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

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "dynamicorchestration"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
