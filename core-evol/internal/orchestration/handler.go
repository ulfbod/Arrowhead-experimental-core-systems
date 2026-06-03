package orchestration

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"
)

// Orchestrator is the interface the HTTP handler depends on.
type Orchestrator interface {
	Orchestrate(req OrchestrationRequest) (OrchestrationResponse, error)
}

type handler struct {
	orch        Orchestrator
	locks       *lockStore
	subs        *subscriptionStore
	hist        *historyStore
	mgmtAuthURL string
	pushClient  *http.Client // used for push notification delivery; nil → http.DefaultClient
}

// requireMgmtAuth checks sysop Bearer token when mgmtAuthURL is set.
// Returns true if the request is allowed to proceed.
func (h *handler) requireMgmtAuth(w http.ResponseWriter, r *http.Request) bool {
	if h.mgmtAuthURL == "" {
		return true
	}
	authHeader := r.Header.Get("Authorization")
	token := ""
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	}
	if token == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"errorMessage":  "Authorization: Bearer token required for management access",
			"errorCode":     http.StatusUnauthorized,
			"exceptionType": "AUTH_EXCEPTION",
			"origin":        "dynamicorch-xacml",
		})
		return false
	}
	resp, err := http.Get(h.mgmtAuthURL + "/authentication/identity/verify/" + token) //nolint:noctx
	if err != nil || resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"errorMessage":  "management access denied: token invalid or auth system unreachable",
			"errorCode":     http.StatusUnauthorized,
			"exceptionType": "AUTH_EXCEPTION",
			"origin":        "dynamicorch-xacml",
		})
		return false
	}
	defer resp.Body.Close() //nolint:errcheck
	var body struct {
		Sysop bool `json:"sysop"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || !body.Sysop {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"errorMessage":  "management access denied: sysop privilege required",
			"errorCode":     http.StatusForbidden,
			"exceptionType": "FORBIDDEN",
			"origin":        "dynamicorch-xacml",
		})
		return false
	}
	return true
}

// NewHandler returns an HTTP handler wrapping the given Orchestrator.
// mgmtAuthURL is the Authentication system base URL for sysop management access control.
// Pass "" for open management access (development mode).
func NewHandler(orch Orchestrator, mgmtAuthURL string) http.Handler {
	pushTimeoutSec := 5
	if v := os.Getenv("PUSH_DELIVERY_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pushTimeoutSec = n
		}
	}
	h := &handler{
		orch:        orch,
		locks:       newLockStore(),
		subs:        newSubscriptionStore(),
		hist:        newHistoryStore(),
		mgmtAuthURL: mgmtAuthURL,
		pushClient:  &http.Client{Timeout: time.Duration(pushTimeoutSec) * time.Second},
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
	if !h.requireMgmtAuth(w, r) {
		return
	}
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

func (h *handler) handlePushMgmtQuery(w http.ResponseWriter, r *http.Request) {
	if !h.requireMgmtAuth(w, r) {
		return
	}
	resp := h.subs.queryAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func (h *handler) handlePushMgmtTrigger(w http.ResponseWriter, r *http.Request) {
	if !h.requireMgmtAuth(w, r) {
		return
	}
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

	entryID := h.hist.add(newHistoryEntry(requester, service, "PENDING", "PUSH"))
	go h.deliverPush(sub, entryID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "triggered"}) //nolint:errcheck
}

// deliverPush posts the orchestration trigger notification to the subscriber's
// notifyInterface URL and updates the history entry to DELIVERED or FAILED.
func (h *handler) deliverPush(sub Subscription, entryID string) {
	notifyURL := extractNotifyURL(sub.NotifyInterface)
	if notifyURL == "" {
		h.hist.updateStatus(entryID, "FAILED")
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"subscriptionId":   sub.ID,
		"ownerSystemName":  sub.OwnerSystemName,
		"targetSystemName": sub.TargetSystemName,
	})
	httpClient := h.pushClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Post(notifyURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		h.hist.updateStatus(entryID, "FAILED")
		return
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.hist.updateStatus(entryID, "FAILED")
		return
	}
	h.hist.updateStatus(entryID, "DELIVERED")
}

// extractNotifyURL returns the delivery URL from a notifyInterface map.
func extractNotifyURL(ni map[string]any) string {
	for _, key := range []string{"notifyUri", "uri"} {
		if v, ok := ni[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	addr, _ := ni["address"].(string)
	path, _ := ni["path"].(string)
	if addr == "" {
		return ""
	}
	port := ""
	switch p := ni["port"].(type) {
	case float64:
		port = strconv.Itoa(int(p))
	case string:
		port = p
	}
	if port == "" {
		return "http://" + addr + path
	}
	return "http://" + addr + ":" + port + path
}


func (h *handler) handleLockCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireMgmtAuth(w, r) {
		return
	}
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

func (h *handler) handleLockQuery(w http.ResponseWriter, r *http.Request) {
	if !h.requireMgmtAuth(w, r) {
		return
	}
	resp := h.locks.query()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func (h *handler) handleLockRemove(w http.ResponseWriter, r *http.Request) {
	if !h.requireMgmtAuth(w, r) {
		return
	}
	owner := r.PathValue("owner")
	h.locks.removeByOwner(owner)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) handleHistoryQuery(w http.ResponseWriter, r *http.Request) {
	if !h.requireMgmtAuth(w, r) {
		return
	}
	resp := h.hist.query()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// RegisterRoutes wires all routes onto mux (backward compat — also adds /status).
// mgmtAuthURL is passed to NewHandler for sysop management access control.
func RegisterRoutes(mux *http.ServeMux, orch Orchestrator, domainID string, enabled bool, mgmtAuthURL string) {
	mux.Handle("/serviceorchestration/", NewHandler(orch, mgmtAuthURL))
	mux.Handle("/health", NewHandler(orch, mgmtAuthURL))
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"status":   "UP",
			"domainID": domainID,
			"xacml":    enabled,
		})
	})
}
