package main

import (
	"encoding/json"
	"fmt"
	"log"
	"mineio/internal/common"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Registry struct {
	mu       sync.RWMutex
	services map[string]*common.ServiceRecord
	policies map[string]*common.AuthPolicy
	orchLogs []common.OrchestrationLog
	logMu    sync.Mutex
}

var registry = &Registry{
	services: make(map[string]*common.ServiceRecord),
	policies: make(map[string]*common.AuthPolicy),
	orchLogs: make([]common.OrchestrationLog, 0, 200),
}

func main() {
	port := envOrDefault("PORT", "8000")
	log.Printf("[Arrowhead] Starting Eclipse Arrowhead Core on :%s", port)

	seedPolicies()

	mux := http.NewServeMux()

	// CORS for all
	handler := corsMiddleware(mux)

	// Health
	mux.HandleFunc("/health", common.HealthHandler)

	// Registry
	mux.HandleFunc("/registry/register", handleRegister)
	mux.HandleFunc("/registry", handleListServices)
	mux.HandleFunc("/registry/query", handleQueryServices)

	// Authorization
	mux.HandleFunc("/authorization/check", handleAuthCheck)
	mux.HandleFunc("/authorization/policy", handlePolicy)
	mux.HandleFunc("/authorization/policies", handleListPolicies)

	// Orchestration
	mux.HandleFunc("/orchestration", handleOrchestration)
	mux.HandleFunc("/orchestration/logs", handleOrchLogs)

	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Auth-Token, X-Consumer-ID")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- Registry Handlers ----

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		handleUnregister(w, r)
		return
	}
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST or DELETE required")
		return
	}
	var req common.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	now := time.Now()
	svc := &common.ServiceRecord{
		ID:           req.ID,
		Name:         req.Name,
		Address:      req.Address,
		Port:         req.Port,
		ServiceType:  req.ServiceType,
		Capabilities: req.Capabilities,
		Metadata:     req.Metadata,
		RegisteredAt: now,
		LastSeen:     now,
		Online:       true,
	}
	registry.mu.Lock()
	registry.services[req.ID] = svc
	registry.mu.Unlock()
	log.Printf("[Arrowhead] Registered: %s (%s) at %s:%d", req.ID, req.ServiceType, req.Address, req.Port)
	common.WriteJSON(w, http.StatusCreated, svc)
}

func handleUnregister(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		common.WriteError(w, http.StatusBadRequest, "id required")
		return
	}
	registry.mu.Lock()
	delete(registry.services, id)
	registry.mu.Unlock()
	log.Printf("[Arrowhead] Unregistered: %s", id)
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "unregistered"})
}

func handleListServices(w http.ResponseWriter, r *http.Request) {
	registry.mu.RLock()
	svcs := make([]*common.ServiceRecord, 0, len(registry.services))
	for _, s := range registry.services {
		svcs = append(svcs, s)
	}
	registry.mu.RUnlock()
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"services": svcs,
		"count":    len(svcs),
	})
}

func handleQueryServices(w http.ResponseWriter, r *http.Request) {
	capability := r.URL.Query().Get("capability")
	serviceType := r.URL.Query().Get("type")

	registry.mu.RLock()
	var results []*common.ServiceRecord
	for _, s := range registry.services {
		if serviceType != "" && s.ServiceType != serviceType {
			continue
		}
		if capability != "" {
			found := false
			for _, c := range s.Capabilities {
				if strings.EqualFold(c, capability) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		results = append(results, s)
	}
	registry.mu.RUnlock()
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"services": results,
		"count":    len(results),
	})
}

// ---- Authorization Handlers ----

func handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req common.AuthCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	allowed, policyID := checkAuth(req.ConsumerID, req.ProviderID, req.ServiceName)
	token := ""
	reason := "no matching policy"
	if allowed {
		token = fmt.Sprintf("arrowhead-token:%s:%s:%s:%s", req.ConsumerID, req.ProviderID, req.ServiceName, policyID)
		reason = "policy " + policyID + " allows"
	}
	common.WriteJSON(w, http.StatusOK, common.AuthCheckResponse{
		Allowed:   allowed,
		PolicyID:  policyID,
		AuthToken: token,
		Reason:    reason,
	})
}

func checkAuth(consumerID, providerID, serviceName string) (bool, string) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	for _, p := range registry.policies {
		consumerMatch := p.ConsumerID == "*" || p.ConsumerID == consumerID
		providerMatch := p.ProviderID == "*" || p.ProviderID == providerID
		serviceMatch := p.ServiceName == "*" || p.ServiceName == serviceName
		if consumerMatch && providerMatch && serviceMatch {
			return p.Allowed, p.ID
		}
	}
	return false, ""
}

func handlePolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost, http.MethodPut:
		var policy common.AuthPolicy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			common.WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if policy.ID == "" {
			policy.ID = fmt.Sprintf("policy-%d", time.Now().UnixNano())
		}
		policy.CreatedAt = time.Now()
		registry.mu.Lock()
		registry.policies[policy.ID] = &policy
		registry.mu.Unlock()
		log.Printf("[Arrowhead] Policy %s: %s->%s [%s] allowed=%v", policy.ID, policy.ConsumerID, policy.ProviderID, policy.ServiceName, policy.Allowed)
		common.WriteJSON(w, http.StatusCreated, policy)
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		registry.mu.Lock()
		delete(registry.policies, id)
		registry.mu.Unlock()
		common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		common.WriteError(w, http.StatusMethodNotAllowed, "POST, PUT or DELETE required")
	}
}

func handleListPolicies(w http.ResponseWriter, r *http.Request) {
	registry.mu.RLock()
	policies := make([]*common.AuthPolicy, 0, len(registry.policies))
	for _, p := range registry.policies {
		policies = append(policies, p)
	}
	registry.mu.RUnlock()
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"policies": policies,
		"count":    len(policies),
	})
}

// ---- Orchestration ----

func handleOrchestration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req common.OrchestrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	registry.mu.RLock()
	// Find all providers with the requested capability
	var candidates []*common.ServiceRecord
	for _, svc := range registry.services {
		if !svc.Online {
			continue
		}
		for _, cap := range svc.Capabilities {
			if strings.EqualFold(cap, req.ServiceName) {
				candidates = append(candidates, svc)
				break
			}
		}
	}
	registry.mu.RUnlock()

	logEntry := common.OrchestrationLog{
		ID:          fmt.Sprintf("orch-%d", time.Now().UnixNano()),
		Timestamp:   time.Now(),
		ConsumerID:  req.ConsumerID,
		ServiceName: req.ServiceName,
	}

	// Check authorization for each candidate
	for _, svc := range candidates {
		allowed, policyID := checkAuth(req.ConsumerID, svc.ID, req.ServiceName)
		if allowed {
			token := fmt.Sprintf("arrowhead-token:%s:%s:%s:%s", req.ConsumerID, svc.ID, req.ServiceName, policyID)
			endpoint := fmt.Sprintf("http://%s:%d", svc.Address, svc.Port)
			logEntry.ProviderID = svc.ID
			logEntry.AuthToken = token
			logEntry.Allowed = true
			logEntry.Message = fmt.Sprintf("Authorized: %s -> %s for %s via policy %s", req.ConsumerID, svc.ID, req.ServiceName, policyID)
			addOrchLog(logEntry)
			log.Printf("[Arrowhead] Orchestration: %s -> %s for %s (allowed)", req.ConsumerID, svc.ID, req.ServiceName)
			common.WriteJSON(w, http.StatusOK, common.OrchestrationResponse{
				Provider:  svc,
				AuthToken: token,
				Endpoint:  endpoint,
				Allowed:   true,
				Reason:    logEntry.Message,
			})
			return
		}
	}

	// Not authorized or no provider
	reason := "no authorized provider found"
	if len(candidates) == 0 {
		reason = fmt.Sprintf("no provider registered for capability '%s'", req.ServiceName)
	}
	logEntry.Allowed = false
	logEntry.Message = reason
	addOrchLog(logEntry)
	log.Printf("[Arrowhead] Orchestration DENIED: %s -> %s: %s", req.ConsumerID, req.ServiceName, reason)
	common.WriteJSON(w, http.StatusOK, common.OrchestrationResponse{
		Allowed: false,
		Reason:  reason,
	})
}

func addOrchLog(entry common.OrchestrationLog) {
	registry.logMu.Lock()
	registry.orchLogs = append(registry.orchLogs, entry)
	if len(registry.orchLogs) > 200 {
		registry.orchLogs = registry.orchLogs[len(registry.orchLogs)-200:]
	}
	registry.logMu.Unlock()
}

func handleOrchLogs(w http.ResponseWriter, r *http.Request) {
	registry.logMu.Lock()
	logs := make([]common.OrchestrationLog, len(registry.orchLogs))
	copy(logs, registry.orchLogs)
	registry.logMu.Unlock()
	// Return most recent first
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"logs":  logs,
		"count": len(logs),
	})
}

// ---- Policy Seeding ----

func addPolicy(id, consumer, provider, service string, allowed bool) {
	registry.policies[id] = &common.AuthPolicy{
		ID:          id,
		ConsumerID:  consumer,
		ProviderID:  provider,
		ServiceName: service,
		Allowed:     allowed,
		CreatedAt:   time.Now(),
	}
}

func seedPolicies() {
	log.Println("[Arrowhead] Seeding default authorization policies...")

	// cDT1 (Exploration & Mapping) -> inspection robots
	addPolicy("p-cdt1-idt1a", "cdt1", "idt1a", "*", true)
	addPolicy("p-cdt1-idt1b", "cdt1", "idt1b", "*", true)

	// cDT2 (Gas Monitoring) -> gas sensors
	addPolicy("p-cdt2-idt2a", "cdt2", "idt2a", "*", true)
	addPolicy("p-cdt2-idt2b", "cdt2", "idt2b", "*", true)

	// cDT3 (Hazard Detection) -> mapping + gas + robots
	addPolicy("p-cdt3-cdt1", "cdt3", "cdt1", "*", true)
	addPolicy("p-cdt3-cdt2", "cdt3", "cdt2", "*", true)
	addPolicy("p-cdt3-idt1a", "cdt3", "idt1a", "*", true)
	addPolicy("p-cdt3-idt1b", "cdt3", "idt1b", "*", true)

	// cDT4 (Material Handling) -> LHD vehicles
	addPolicy("p-cdt4-idt3a", "cdt4", "idt3a", "*", true)
	addPolicy("p-cdt4-idt3b", "cdt4", "idt3b", "*", true)

	// cDT5 (Tele-Remote Intervention) -> tele-remote + machines
	addPolicy("p-cdt5-idt4", "cdt5", "idt4", "*", true)
	addPolicy("p-cdt5-idt1a", "cdt5", "idt1a", "*", true)
	addPolicy("p-cdt5-idt1b", "cdt5", "idt1b", "*", true)
	addPolicy("p-cdt5-idt3a", "cdt5", "idt3a", "*", true)
	addPolicy("p-cdt5-idt3b", "cdt5", "idt3b", "*", true)

	// cDTa (Inspection & Recovery) -> lower cDTs
	addPolicy("p-cdta-cdt1", "cdta", "cdt1", "*", true)
	addPolicy("p-cdta-cdt3", "cdta", "cdt3", "*", true)
	addPolicy("p-cdta-cdt4", "cdta", "cdt4", "*", true)
	addPolicy("p-cdta-cdt5", "cdta", "cdt5", "*", true)

	// cDTb (Hazard Monitoring) -> lower cDTs
	addPolicy("p-cdtb-cdt2", "cdtb", "cdt2", "*", true)
	addPolicy("p-cdtb-cdt3", "cdtb", "cdt3", "*", true)

	// Scenario runner -> all
	addPolicy("p-scenario-all", "scenario", "*", "*", true)

	// All services -> arrowhead (for registration and discovery)
	addPolicy("p-all-arrowhead", "*", "arrowhead", "*", true)

	log.Printf("[Arrowhead] Seeded %d authorization policies", len(registry.policies))
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
