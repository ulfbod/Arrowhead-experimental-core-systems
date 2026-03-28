package main

import (
	"encoding/json"
	"fmt"
	"log"
	"mineio/internal/common"
	"net/http"
	"os"
	"sync"
	"time"
)

// interveneRequest is the body schema for POST /intervene.
type interveneRequest struct {
	TargetMachineID string `json:"targetMachineId"`
	Command         string `json:"command"`
}

// overrideRequest is the body schema for POST /override/start.
type overrideRequest struct {
	TargetMachineID string `json:"targetMachineId"`
}

// CDT5Service is the Tele-Remote Intervention composite digital twin.
// It composes iDT4 (operator station) and can relay commands to any constituent iDT.
type CDT5Service struct {
	mu           sync.RWMutex
	intervention common.InterventionStatus
	teleRemote   *common.TeleRemoteState
	ah           *common.ArrowheadClient
	connected    bool
	serviceLog   []string
}

func main() {
	id := "cdt5"
	name := "Tele-Remote Intervention"
	port := envOrDefault("PORT", "8505")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")

	log.Printf("[%s] Starting %s on :%s", id, name, port)

	svc := &CDT5Service{
		ah:        common.NewArrowheadClient(ahURL, id),
		connected: true,
		intervention: common.InterventionStatus{
			Active:          false,
			OperatorPresent: false,
			TargetMachine:   "",
			LastCommand:     "",
			Timestamp:       time.Now(),
		},
		serviceLog: []string{},
	}

	portInt := 8505
	fmt.Sscanf(port, "%d", &portInt)
	common.RegisterWithRetry(svc.ah, common.RegisterRequest{
		ID:           id,
		Name:         name,
		Address:      host,
		Port:         portInt,
		ServiceType:  "cDT",
		Capabilities: []string{"tele-remote", "operator-override", "intervention", "tele-operation"},
		Metadata:     map[string]string{"composes": "idt4", "relays": "idt1a,idt1b,idt3a,idt3b", "layer": "lower"},
	}, 10)

	go svc.pollLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, 200, map[string]interface{}{
			"status":    "ok",
			"id":        id,
			"timestamp": time.Now(),
		})
	})
	mux.HandleFunc("/state", svc.handleState)
	mux.HandleFunc("/intervene", svc.handleIntervene)
	mux.HandleFunc("/override/start", svc.handleOverrideStart)
	mux.HandleFunc("/override/stop", svc.handleOverrideStop)
	mux.HandleFunc("/operator", svc.handleOperator)
	mux.HandleFunc("/logs", svc.handleLogs)
	mux.HandleFunc("/connectivity", svc.handleConnectivity)

	log.Printf("[%s] Listening on :%s", id, port)
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}

// pollLoop polls iDT4 every 3 seconds for operator/override status.
func (s *CDT5Service) pollLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.fetchTeleRemoteState()
	}
}

func (s *CDT5Service) fetchTeleRemoteState() {
	s.mu.RLock()
	connected := s.connected
	s.mu.RUnlock()

	if !connected {
		log.Printf("[cdt5] Connectivity disabled, skipping poll")
		return
	}

	// iDT4 discovered via Arrowhead for "tele-operation" capability
	var tr common.TeleRemoteState
	err := s.ah.CallService("tele-operation", "GET", "/state", nil, &tr)
	if err != nil {
		// Fallback: direct call
		err = common.DoRequest(
			"GET",
			envOrDefault("IDT4_URL", "http://localhost:8401")+"/state",
			"", "cdt5", nil, &tr,
		)
	}
	if err != nil {
		log.Printf("[cdt5] iDT4 fetch error: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.teleRemote = &tr
	// Sync intervention status from tele-remote state
	s.intervention.OperatorPresent = tr.OperatorPresent
	s.intervention.Active = tr.OverrideActive
	if tr.OverrideActive {
		s.intervention.TargetMachine = tr.TargetMachineID
	}
	s.intervention.LastCommand = tr.LastCommand
	s.intervention.Timestamp = tr.LastUpdated

	s.addLogLocked(fmt.Sprintf("iDT4 (%s): operator=%v override=%v target=%s lastCmd=%s",
		tr.ID, tr.OperatorPresent, tr.OverrideActive, tr.TargetMachineID, tr.LastCommand))
}

// machineURL returns the base URL for a known machine ID.
func machineURL(id string) (string, bool) {
	switch id {
	case "idt1a", "robot1":
		return envOrDefault("IDT1A_URL", "http://localhost:8101"), true
	case "idt1b", "robot2":
		return envOrDefault("IDT1B_URL", "http://localhost:8102"), true
	case "idt3a", "lhd1":
		return envOrDefault("IDT3A_URL", "http://localhost:8301"), true
	case "idt3b", "lhd2":
		return envOrDefault("IDT3B_URL", "http://localhost:8302"), true
	case "idt4":
		return envOrDefault("IDT4_URL", "http://localhost:8401"), true
	}
	return "", false
}

// handleState returns the current InterventionStatus plus the raw tele-remote state.
func (s *CDT5Service) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"intervention": s.intervention,
		"teleRemote":   s.teleRemote,
		"timestamp":    time.Now(),
	})
}

// handleIntervene relays an operator command via iDT4 and directly to the target machine.
// POST /intervene  body: {"targetMachineId":"idt1a","command":"stop"}
func (s *CDT5Service) handleIntervene(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	var req interveneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetMachineID == "" || req.Command == "" {
		common.WriteError(w, 400, "body must include targetMachineId and command")
		return
	}

	// Notify iDT4 of the intervention
	idt4Payload := map[string]string{
		"targetMachineId": req.TargetMachineID,
		"command":         req.Command,
	}
	idt4Err := common.DoRequest(
		"POST",
		envOrDefault("IDT4_URL", "http://localhost:8401")+"/intervene",
		"", "cdt5", idt4Payload, nil,
	)
	if idt4Err != nil {
		log.Printf("[cdt5] iDT4 intervene notify error: %v", idt4Err)
	}

	// Relay command to target machine
	targetURL, known := machineURL(req.TargetMachineID)
	if !known {
		common.WriteError(w, 404, "unknown targetMachineId: "+req.TargetMachineID)
		return
	}
	cmdPayload := map[string]string{"command": req.Command}
	targetErr := common.DoRequest("POST", targetURL+"/command", "", "cdt5", cmdPayload, nil)
	if targetErr != nil {
		log.Printf("[cdt5] Target machine command error (%s): %v", req.TargetMachineID, targetErr)
	}

	s.mu.Lock()
	s.intervention.LastCommand = req.Command
	s.intervention.TargetMachine = req.TargetMachineID
	s.intervention.Timestamp = time.Now()
	s.addLogLocked(fmt.Sprintf("Intervene: target=%s cmd=%s idt4_ok=%v target_ok=%v",
		req.TargetMachineID, req.Command, idt4Err == nil, targetErr == nil))
	s.mu.Unlock()

	common.WriteJSON(w, 200, map[string]interface{}{
		"status":          "command relayed",
		"targetMachineId": req.TargetMachineID,
		"command":         req.Command,
		"idt4Notified":    idt4Err == nil,
		"targetReached":   targetErr == nil,
	})
}

// handleOverrideStart activates operator override on iDT4 for the specified target machine.
// POST /override/start  body: {"targetMachineId":"idt1a"}
func (s *CDT5Service) handleOverrideStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	var req overrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetMachineID == "" {
		common.WriteError(w, 400, "body must include targetMachineId")
		return
	}

	payload := map[string]string{"targetMachineId": req.TargetMachineID}
	err := common.DoRequest(
		"POST",
		envOrDefault("IDT4_URL", "http://localhost:8401")+"/override/start",
		"", "cdt5", payload, nil,
	)
	if err != nil {
		log.Printf("[cdt5] iDT4 override/start error: %v", err)
	}

	s.mu.Lock()
	s.intervention.Active = true
	s.intervention.TargetMachine = req.TargetMachineID
	s.intervention.Timestamp = time.Now()
	s.addLogLocked(fmt.Sprintf("Override started: target=%s idt4_ok=%v", req.TargetMachineID, err == nil))
	s.mu.Unlock()

	log.Printf("[cdt5] Override started for %s", req.TargetMachineID)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status":          "override active",
		"targetMachineId": req.TargetMachineID,
		"idt4Ok":          err == nil,
	})
}

// handleOverrideStop deactivates the operator override on iDT4.
func (s *CDT5Service) handleOverrideStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	err := common.DoRequest(
		"POST",
		envOrDefault("IDT4_URL", "http://localhost:8401")+"/override/stop",
		"", "cdt5", nil, nil,
	)
	if err != nil {
		log.Printf("[cdt5] iDT4 override/stop error: %v", err)
	}

	s.mu.Lock()
	prevTarget := s.intervention.TargetMachine
	s.intervention.Active = false
	s.intervention.TargetMachine = ""
	s.intervention.Timestamp = time.Now()
	s.addLogLocked(fmt.Sprintf("Override stopped: was target=%s idt4_ok=%v", prevTarget, err == nil))
	s.mu.Unlock()

	log.Printf("[cdt5] Override stopped")
	common.WriteJSON(w, 200, map[string]interface{}{
		"status": "override deactivated",
		"idt4Ok": err == nil,
	})
}

// handleOperator returns the current operator/tele-remote status from iDT4.
func (s *CDT5Service) handleOperator(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"operatorPresent": s.intervention.OperatorPresent,
		"overrideActive":  s.intervention.Active,
		"targetMachine":   s.intervention.TargetMachine,
		"teleRemote":      s.teleRemote,
		"timestamp":       time.Now(),
	})
}

// handleLogs returns recent service log entries.
func (s *CDT5Service) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{"logs": s.serviceLog})
}

// handleConnectivity toggles simulated connectivity.
func (s *CDT5Service) handleConnectivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		common.WriteError(w, 405, "PUT required")
		return
	}
	var body struct {
		Connected bool `json:"connected"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, 400, "invalid JSON body")
		return
	}
	s.mu.Lock()
	s.connected = body.Connected
	s.addLogLocked(fmt.Sprintf("Connectivity set to %v", body.Connected))
	s.mu.Unlock()
	log.Printf("[cdt5] Connectivity toggled: %v", body.Connected)
	common.WriteJSON(w, 200, map[string]bool{"connected": body.Connected})
}

// addLogLocked appends a timestamped log entry. Caller must hold s.mu (write).
func (s *CDT5Service) addLogLocked(msg string) {
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	if len(s.serviceLog) >= 50 {
		s.serviceLog = s.serviceLog[1:]
	}
	s.serviceLog = append(s.serviceLog, entry)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Auth-Token, X-Consumer-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
