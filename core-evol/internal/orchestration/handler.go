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

type handler struct {
	orch  Orchestrator
	locks *lockStore
	subs  *subscriptionStore
	hist  *historyStore
}

// NewHandler returns an HTTP handler wrapping the given Orchestrator.
func NewHandler(orch Orchestrator) http.Handler {
	h := &handler{
		orch:  orch,
		locks: newLockStore(),
		subs:  newSubscriptionStore(),
		hist:  newHistoryStore(),
	}
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.HandleFunc("GET /serviceorchestration/orchestration/pull/health", h.handleHealth)

	// Pull orchestration
	mux.HandleFunc("POST /serviceorchestration/orchestration/pull", h.handleOrchestrate)

	// Subscribe / unsubscribe
	mux.HandleFunc("POST /serviceorchestration/orchestration/subscribe", h.handleSubscribe)
	mux.HandleFunc("DELETE /serviceorchestration/orchestration/unsubscribe/{id}", h.handleUnsubscribe)

	// Push mgmt
	mux.HandleFunc("POST /serviceorchestration/orchestration/mgmt/push/subscribe", h.handlePushMgmtSubscribe)
	mux.HandleFunc("POST /serviceorchestration/orchestration/mgmt/push/trigger", h.handlePushMgmtTrigger)
	mux.HandleFunc("POST /serviceorchestration/orchestration/mgmt/push/query", h.handlePushMgmtQuery)

	// Lock mgmt
	mux.HandleFunc("POST /serviceorchestration/orchestration/mgmt/lock/create", h.handleLockCreate)
	mux.HandleFunc("POST /serviceorchestration/orchestration/mgmt/lock/query", h.handleLockQuery)
	mux.HandleFunc("DELETE /serviceorchestration/orchestration/mgmt/lock/remove/{owner}", h.handleLockRemove)

	// History
	mux.HandleFunc("POST /serviceorchestration/orchestration/mgmt/history/query", h.handleHistoryQuery)

	return mux
}

func (h *handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "UP"}) //nolint:errcheck
}

func (h *handler) handleOrchestrate(w http.ResponseWriter, r *http.Request) {
	var req OrchestrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	requester := req.RequesterSystem.SystemName
	service := req.RequestedService.ServiceDefinition

	resp, err := h.orch.Orchestrate(req)
	if err != nil {
		if errors.Is(err, ErrMissingRequester) || errors.Is(err, ErrMissingService) {
			h.hist.add(newHistoryEntry(requester, service, "ERROR", "PULL"))
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.hist.add(newHistoryEntry(requester, service, "ERROR", "PULL"))
		http.Error(w, "orchestration failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.hist.add(newHistoryEntry(requester, service, "DONE", "PULL"))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func (h *handler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	var req CreateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	sub, created := h.subs.subscribe(req)
	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusCreated)
	}
	json.NewEncoder(w).Encode(sub) //nolint:errcheck
}

func (h *handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if h.subs.unsubscribe(id) {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) handlePushMgmtSubscribe(w http.ResponseWriter, r *http.Request) {
	var req CreateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	sub, created := h.subs.subscribe(req)
	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusCreated)
	}
	json.NewEncoder(w).Encode(sub) //nolint:errcheck
}

func (h *handler) handlePushMgmtQuery(w http.ResponseWriter, _ *http.Request) {
	resp := h.subs.queryAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func (h *handler) handlePushMgmtTrigger(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SubscriptionId string `json:"subscriptionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	sub, ok := h.subs.get(body.SubscriptionId)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Extract requester/service from the stored orchestration request map.
	requester, service := "", ""
	if sys, ok := sub.OrchestrationRequest["requesterSystem"].(map[string]any); ok {
		requester, _ = sys["systemName"].(string)
	}
	if svc, ok := sub.OrchestrationRequest["requestedService"].(map[string]any); ok {
		service, _ = svc["serviceDefinition"].(string)
	}

	h.hist.add(newHistoryEntry(requester, service, "PENDING", "PUSH"))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "triggered"}) //nolint:errcheck
}

func (h *handler) handleLockCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateLockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	lock := h.locks.create(req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(lock) //nolint:errcheck
}

func (h *handler) handleLockQuery(w http.ResponseWriter, _ *http.Request) {
	resp := h.locks.query()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func (h *handler) handleLockRemove(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	h.locks.removeByOwner(owner)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) handleHistoryQuery(w http.ResponseWriter, _ *http.Request) {
	resp := h.hist.query()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// RegisterRoutes wires all routes onto mux (backward compat — also adds /status).
func RegisterRoutes(mux *http.ServeMux, orch Orchestrator, domainID string, enabled bool) {
	mux.Handle("/serviceorchestration/", NewHandler(orch))
	mux.Handle("/health", NewHandler(orch))
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"status":   "UP",
			"domainID": domainID,
			"xacml":    enabled,
		})
	})
}
