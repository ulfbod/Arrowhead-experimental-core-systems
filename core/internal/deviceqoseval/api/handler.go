// Package api provides HTTP handlers for the Device QoS Evaluator system.
package api

import (
	"encoding/json"
	"net/http"

	"arrowhead/core/internal/deviceqoseval/model"
	"arrowhead/core/internal/deviceqoseval/service"
	"arrowhead/core/internal/httputil"
)

const qosOrigin = "deviceqosevaluator"

// Handler wires HTTP routes to the Evaluator service.
type Handler struct {
	eval *service.Evaluator
}

// NewHandler returns an http.Handler for all Device QoS Evaluator routes.
func NewHandler(eval *service.Evaluator) http.Handler {
	h := &Handler{eval: eval}
	mux := http.NewServeMux()
	mux.HandleFunc("/deviceqosevaluator/quality-evaluation/measure", h.handleMeasure)
	mux.HandleFunc("/deviceqosevaluator/quality-evaluation/mgmt/query", h.handleQuery)
	mux.HandleFunc("/deviceqosevaluator/health", h.handleHealth)
	return mux
}

// POST /deviceqosevaluator/quality-evaluation/measure
func (h *Handler) handleMeasure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", qosOrigin)
		return
	}
	var req model.MeasurementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", qosOrigin)
		return
	}
	if req.Host == "" || req.Port == "" {
		httputil.WriteError(w, http.StatusBadRequest, "host and port are required", qosOrigin)
		return
	}
	rec := h.eval.Measure(req.Host, req.Port)
	httputil.WriteJSON(w, http.StatusOK, rec, qosOrigin)
}

// POST /deviceqosevaluator/quality-evaluation/mgmt/query
func (h *Handler) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", qosOrigin)
		return
	}
	var req model.QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid JSON", qosOrigin)
		return
	}
	records := h.eval.Query(req)
	if records == nil {
		records = []*model.QoSRecord{}
	}
	httputil.WriteJSON(w, http.StatusOK, model.QueryResponse{
		Records:    records,
		Count:      len(records),
		TotalCount: len(records),
	}, qosOrigin)
}

// GET /deviceqosevaluator/health
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "UP"}, qosOrigin)
}
