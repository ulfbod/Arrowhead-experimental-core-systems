package orchestration

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Orchestrator is the interface the HTTP handler depends on.
type Orchestrator interface {
	Orchestrate(req OrchestrationRequest) (OrchestrationResponse, error)
}

// Handler is the HTTP handler for POST /orchestration/dynamic.
type Handler struct {
	orch Orchestrator
}

// NewHandler returns an HTTP handler wrapping the given Orchestrator.
func NewHandler(orch Orchestrator) http.Handler {
	return &Handler{orch: orch}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req OrchestrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := h.orch.Orchestrate(req)
	if err != nil {
		if errors.Is(err, ErrMissingRequester) || errors.Is(err, ErrMissingService) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "orchestration failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// RegisterRoutes wires all routes onto mux.
func RegisterRoutes(mux *http.ServeMux, orch Orchestrator, domainID string, enabled bool) {
	mux.Handle("/orchestration/dynamic", NewHandler(orch))
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "UP",
			"domainID": domainID,
			"xacml":    enabled,
		})
	})
}
