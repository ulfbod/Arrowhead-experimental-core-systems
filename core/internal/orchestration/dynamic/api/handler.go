// Package api implements the HTTP interface for DynamicServiceOrchestration.
// AH5 service: serviceOrchestration (pull + push)
// Strategy: "dynamic" — real-time SR lookup + optional auth check.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"arrowhead/core/internal/httputil"
	coremodel "arrowhead/core/internal/model"
	orchmodel "arrowhead/core/internal/orchestration/model"
	"arrowhead/core/internal/orchestration/dynamic/service"
)

const dynOrigin = "serviceorchestration.orchestration.pull"

// pageReqOrZero returns the dereferenced PageRequest, or a zero value if p is nil.
func pageReqOrZero(p *coremodel.PageRequest) coremodel.PageRequest {
	if p == nil {
		return coremodel.PageRequest{}
	}
	return *p
}

type Handler struct {
	orch        *service.DynamicOrchestrator
	locks       *service.LockStore
	subs        *service.SubscriptionStore
	mgmtAuthURL string
}

func NewHandler(orch *service.DynamicOrchestrator, mgmtAuthURL string) http.Handler {
	h := &Handler{
		orch:        orch,
		locks:       service.NewLockStore(),
		subs:        service.NewSubscriptionStore(),
		mgmtAuthURL: mgmtAuthURL,
	}
	// Wire the lock store into the orchestrator so ONLY_EXCLUSIVE filtering works (G48).
	orch.SetLockChecker(h.locks)
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

// statusFor maps sentinel errors to HTTP status codes.
func statusFor(err error) int {
	switch {
	case errors.Is(err, service.ErrIdentityRequired):
		return http.StatusUnauthorized
	case errors.Is(err, service.ErrIdentityInvalid):
		return http.StatusUnauthorized
	case errors.Is(err, service.ErrMissingRequester):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrMissingService):
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
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", dynOrigin)
		return
	}
	var req orchmodel.OrchestrationRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	token := httputil.ExtractBearer(r)
	resp, err := h.orch.Orchestrate(req, token)
	if err != nil {
		httputil.WriteError(w, statusFor(err), err.Error(), dynOrigin)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp, dynOrigin)
}

// POST /serviceorchestration/orchestration/subscribe
func (h *Handler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", dynOrigin)
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
	httputil.WriteJSON(w, status, sub, dynOrigin)
}

// DELETE /serviceorchestration/orchestration/unsubscribe/{subscriptionId}
func (h *Handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", dynOrigin)
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
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, dynOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", dynOrigin)
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
	httputil.WriteJSON(w, status, sub, dynOrigin)
}

// DELETE /serviceorchestration/orchestration/mgmt/push/unsubscribe?ids=uuid1,uuid2
func (h *Handler) handlePushMgmtUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, dynOrigin) {
		return
	}
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", dynOrigin)
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
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, dynOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", dynOrigin)
		return
	}
	var body struct {
		SubscriptionID string `json:"subscriptionId"`
	}
	if !httputil.DecodeJSON(w, r, &body) {
		return
	}
	sub, ok := h.subs.Get(body.SubscriptionID)
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, "subscription not found", dynOrigin)
		return
	}
	// Record PENDING and asynchronously deliver to subscriber's notifyInterface.
	h.orch.TriggerPush(sub)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "triggered", "subscriptionId": sub.ID}, dynOrigin)
}

// POST /serviceorchestration/orchestration/mgmt/push/query
func (h *Handler) handlePushMgmtQuery(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, dynOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", dynOrigin)
		return
	}
	var raw struct {
		Pagination *coremodel.PageRequest `json:"pagination"`
	}
	json.NewDecoder(r.Body).Decode(&raw) //nolint:errcheck — empty body OK
	result := h.subs.Query()
	page, total := coremodel.Paginate(result.Subscriptions, pageReqOrZero(raw.Pagination), func(s service.Subscription) string { return s.ID })
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"subscriptions": page,
		"count":         len(page),
		"totalCount":    total,
	}, dynOrigin)
}

// POST /serviceorchestration/orchestration/mgmt/lock/create
func (h *Handler) handleLockCreate(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, dynOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", dynOrigin)
		return
	}
	var req service.CreateLockRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	lock := h.locks.Create(req)
	httputil.WriteJSON(w, http.StatusCreated, lock, dynOrigin)
}

// POST /serviceorchestration/orchestration/mgmt/lock/query
func (h *Handler) handleLockQuery(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, dynOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", dynOrigin)
		return
	}
	var raw struct {
		Pagination *coremodel.PageRequest `json:"pagination"`
	}
	json.NewDecoder(r.Body).Decode(&raw) //nolint:errcheck — empty body OK
	result := h.locks.Query()
	page, total := coremodel.Paginate(result.Locks, pageReqOrZero(raw.Pagination), func(l service.Lock) string { return l.Owner })
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"locks":      page,
		"count":      len(page),
		"totalCount": total,
	}, dynOrigin)
}

// DELETE /serviceorchestration/orchestration/mgmt/lock/remove/{owner}
func (h *Handler) handleLockRemove(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, dynOrigin) {
		return
	}
	if r.Method != http.MethodDelete {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "DELETE required", dynOrigin)
		return
	}
	owner := strings.TrimPrefix(r.URL.Path, "/serviceorchestration/orchestration/mgmt/lock/remove/")
	if owner == "" {
		httputil.WriteError(w, http.StatusBadRequest, "owner is required", dynOrigin)
		return
	}
	h.locks.RemoveByOwner(owner)
	w.WriteHeader(http.StatusNoContent)
}

// POST /serviceorchestration/orchestration/mgmt/history/query
func (h *Handler) handleHistoryQuery(w http.ResponseWriter, r *http.Request) {
	if !httputil.RequireManagementAuth(w, r, h.mgmtAuthURL, dynOrigin) {
		return
	}
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "POST required", dynOrigin)
		return
	}
	var raw struct {
		Pagination          *coremodel.PageRequest `json:"pagination"`
		RequesterSystemName string                 `json:"requesterSystemName"`
		ServiceDefinition   string                 `json:"serviceDefinition"`
		Status              string                 `json:"status"`
		From                string                 `json:"from"`
		To                  string                 `json:"to"`
	}
	json.NewDecoder(r.Body).Decode(&raw) //nolint:errcheck — empty body OK
	filter := service.HistoryQueryFilter{
		RequesterSystemName: raw.RequesterSystemName,
		ServiceDefinition:   raw.ServiceDefinition,
		Status:              raw.Status,
		From:                raw.From,
		To:                  raw.To,
	}
	result := h.orch.QueryHistory(filter)
	page, total := coremodel.Paginate(result.Entries, pageReqOrZero(raw.Pagination), func(e service.HistoryEntry) string { return e.ID })
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"entries":    page,
		"count":      len(page),
		"totalCount": total,
	}, dynOrigin)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "system": "dynamicorchestration"}, dynOrigin)
}
