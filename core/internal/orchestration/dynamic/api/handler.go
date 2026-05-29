// Package api implements the HTTP interface for DynamicServiceOrchestration.
// AH5 service: serviceOrchestration (pull + push)
// Strategy: "dynamic" — real-time SR lookup + optional auth check.
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/dynamic/service"
)

type Handler struct {
	orch  *service.DynamicOrchestrator
	locks *service.LockStore
	subs  *service.SubscriptionStore
}

func NewHandler(orch *service.DynamicOrchestrator) http.Handler {
	h := &Handler{
		orch:  orch,
		locks: service.NewLockStore(),
		subs:  service.NewSubscriptionStore(),
	}
	mux := http.NewServeMux()
	// Pull orchestration
	mux.HandleFunc("/serviceorchestration/orchestration/pull", h.handleOrchestrate)
	// Push orchestration — discovery endpoints (Step 19.1)
	mux.HandleFunc("/serviceorchestration/orchestration/subscribe", h.handleSubscribe)
	mux.HandleFunc("/serviceorchestration/orchestration/unsubscribe/", h.handleUnsubscribe)
	// Push management endpoints (Step 19.2)
	mux.HandleFunc("/serviceorchestration/orchestration/mgmt/push/subscribe", h.handlePushMgmtSubscribe)
	mux.HandleFunc("/serviceorchestration/orchestration/mgmt/push/unsubscribe", h.handlePushMgmtUnsubscribe)
	mux.HandleFunc("/serviceorchestration/orchestration/mgmt/push/trigger", h.handlePushMgmtTrigger)
	mux.HandleFunc("/serviceorchestration/orchestration/mgmt/push/query", h.handlePushMgmtQuery)
	// Lock management (Step 18.2)
	mux.HandleFunc("/serviceorchestration/orchestration/mgmt/lock/create", h.handleLockCreate)
	mux.HandleFunc("/serviceorchestration/orchestration/mgmt/lock/query", h.handleLockQuery)
	mux.HandleFunc("/serviceorchestration/orchestration/mgmt/lock/remove/", h.handleLockRemove)
	// Orchestration history (Step 18.3)
	mux.HandleFunc("/serviceorchestration/orchestration/mgmt/history/query", h.handleHistoryQuery)
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
	token := extractBearer(r)
	resp, err := h.orch.Orchestrate(req, token)
	if err != nil {
		if err == service.ErrIdentityRequired || err == service.ErrIdentityInvalid {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
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

// POST /serviceorchestration/orchestration/mgmt/push/subscribe
func (h *Handler) handlePushMgmtSubscribe(w http.ResponseWriter, r *http.Request) {
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

// DELETE /serviceorchestration/orchestration/mgmt/push/unsubscribe?ids=uuid1,uuid2
func (h *Handler) handlePushMgmtUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	idsParam := r.URL.Query().Get("ids")
	if idsParam != "" {
		ids := strings.Split(idsParam, ",")
		h.subs.UnsubscribeMany(ids)
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /serviceorchestration/orchestration/mgmt/push/trigger
func (h *Handler) handlePushMgmtTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		SubscriptionID string `json:"subscriptionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	sub, ok := h.subs.Get(body.SubscriptionID)
	if !ok {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	// Record a PENDING history entry for this push trigger.
	// Delivery is a stub — notification is not actually sent.
	h.orch.RecordPushHistory(sub.ID, sub.OwnerSystemName, "", "PENDING")
	writeJSON(w, http.StatusOK, map[string]string{"status": "triggered", "subscriptionId": sub.ID})
}

// POST /serviceorchestration/orchestration/mgmt/push/query
func (h *Handler) handlePushMgmtQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	writeJSON(w, http.StatusOK, h.subs.Query())
}

// POST /serviceorchestration/orchestration/mgmt/lock/create
func (h *Handler) handleLockCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req service.CreateLockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	lock := h.locks.Create(req)
	writeJSON(w, http.StatusCreated, lock)
}

// POST /serviceorchestration/orchestration/mgmt/lock/query
func (h *Handler) handleLockQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	writeJSON(w, http.StatusOK, h.locks.Query())
}

// DELETE /serviceorchestration/orchestration/mgmt/lock/remove/{owner}
func (h *Handler) handleLockRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE required")
		return
	}
	owner := strings.TrimPrefix(r.URL.Path, "/serviceorchestration/orchestration/mgmt/lock/remove/")
	if owner == "" {
		writeError(w, http.StatusBadRequest, "owner is required")
		return
	}
	h.locks.RemoveByOwner(owner)
	w.WriteHeader(http.StatusNoContent)
}

// POST /serviceorchestration/orchestration/mgmt/history/query
func (h *Handler) handleHistoryQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	writeJSON(w, http.StatusOK, h.orch.QueryHistory())
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "dynamicorchestration"})
}

func extractBearer(r *http.Request) string {
	hdr := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(hdr, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
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
	writeJSON(w, status, errBody{ErrorMessage: msg, ErrorCode: status, ExceptionType: exType, Origin: "serviceorchestration.orchestration.pull"})
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
