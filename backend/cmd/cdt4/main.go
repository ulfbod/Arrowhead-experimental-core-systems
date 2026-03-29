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

// CDT4Service is the Selective Material Handling and Clearance composite digital twin.
// It composes iDT3a and iDT3b (LHD vehicles) and exposes a unified ClearanceStatus.
type CDT4Service struct {
	mu         sync.RWMutex
	status     common.ClearanceStatus
	lhd1       *common.LHDState
	lhd2       *common.LHDState
	ah         *common.ArrowheadClient
	connected  bool
	serviceLog []string
}

func main() {
	id := "cdt4"
	name := "Selective Material Handling and Clearance"
	port := envOrDefault("PORT", "8504")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")

	log.Printf("[%s] Starting %s on :%s", id, name, port)

	svc := &CDT4Service{
		ah:        common.NewArrowheadClient(ahURL, id),
		connected: true,
		status: common.ClearanceStatus{
			TotalDebrisPct: 0,
			ActiveVehicles: 0,
			EstimatedETA:   0,
			RouteClear:     false,
			Timestamp:      time.Now(),
		},
		serviceLog: []string{},
	}

	portInt := 8504
	fmt.Sscanf(port, "%d", &portInt)
	common.RegisterWithRetry(svc.ah, common.RegisterRequest{
		ID:           id,
		Name:         name,
		Address:      host,
		Port:         portInt,
		ServiceType:  "cDT",
		Capabilities: []string{"clearance", "material-handling", "debris-removal", "route-clearing"},
		Metadata:     map[string]string{"composes": "idt3a,idt3b", "layer": "lower"},
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
	mux.HandleFunc("/clearance/start", svc.handleClearanceStart)
	mux.HandleFunc("/clearance/stop", svc.handleClearanceStop)
	mux.HandleFunc("/vehicles", svc.handleVehicles)
	mux.HandleFunc("/simulate/reset", svc.handleSimulateReset)
	mux.HandleFunc("/logs", svc.handleLogs)
	mux.HandleFunc("/connectivity", svc.handleConnectivity)

	log.Printf("[%s] Listening on :%s", id, port)
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}

// pollLoop fetches LHD states every 3 seconds.
func (s *CDT4Service) pollLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.fetchLHDStates()
	}
}

func (s *CDT4Service) fetchLHDStates() {
	s.mu.RLock()
	connected := s.connected
	s.mu.RUnlock()

	if !connected {
		log.Printf("[cdt4] Connectivity disabled, skipping poll")
		return
	}

	// LHD 1: direct call to iDT3a
	var l1 common.LHDState
	err1 := common.DoRequest(
		"GET",
		envOrDefault("IDT3A_URL", "http://localhost:8301")+"/state",
		"", "cdt4", nil, &l1,
	)
	if err1 != nil {
		log.Printf("[cdt4] LHD1 fetch error: %v", err1)
	}

	// LHD 2: direct call to iDT3b
	var l2 common.LHDState
	err2 := common.DoRequest(
		"GET",
		envOrDefault("IDT3B_URL", "http://localhost:8302")+"/state",
		"", "cdt4", nil, &l2,
	)
	if err2 != nil {
		log.Printf("[cdt4] LHD2 fetch error: %v", err2)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	activeVehicles := 0
	totalDebrisPct := 0.0
	activeCount := 0

	if err1 == nil {
		s.lhd1 = &l1
		if l1.Online && l1.Connected {
			activeVehicles++
			totalDebrisPct += l1.DebrisClearedPct
			activeCount++
		}
		s.addLogLocked(fmt.Sprintf("LHD1 (%s): status=%s debris=%.1f%% fuel=%.0f%%",
			l1.ID, l1.TrammingStatus, l1.DebrisClearedPct, l1.FuelPct))
	}
	if err2 == nil {
		s.lhd2 = &l2
		if l2.Online && l2.Connected {
			activeVehicles++
			totalDebrisPct += l2.DebrisClearedPct
			activeCount++
		}
		s.addLogLocked(fmt.Sprintf("LHD2 (%s): status=%s debris=%.1f%% fuel=%.0f%%",
			l2.ID, l2.TrammingStatus, l2.DebrisClearedPct, l2.FuelPct))
	}

	avgDebrisPct := 0.0
	if activeCount > 0 {
		avgDebrisPct = totalDebrisPct / float64(activeCount)
	}

	eta := estimateETA(avgDebrisPct, activeVehicles)
	routeClear := avgDebrisPct >= 95.0

	s.status = common.ClearanceStatus{
		TotalDebrisPct: avgDebrisPct,
		ActiveVehicles: activeVehicles,
		EstimatedETA:   eta,
		RouteClear:     routeClear,
		Timestamp:      time.Now(),
	}
}

// estimateETA returns a rough ETA in minutes based on debris cleared and active vehicles.
func estimateETA(clearedPct float64, activeVehicles int) int {
	if clearedPct >= 100.0 {
		return 0
	}
	remaining := 100.0 - clearedPct
	// Assume each vehicle clears ~5% per minute
	rate := 5.0
	if activeVehicles > 1 {
		rate = float64(activeVehicles) * 5.0
	}
	if rate <= 0 {
		return 999
	}
	return int(remaining / rate)
}

// handleState returns the composite clearance status and individual LHD states.
func (s *CDT4Service) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"clearance": s.status,
		"lhd1":      s.lhd1,
		"lhd2":      s.lhd2,
		"timestamp": time.Now(),
	})
}

// handleClearanceStart sends a start command to both LHDs.
func (s *CDT4Service) handleClearanceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	err1 := common.DoRequest(
		"POST",
		envOrDefault("IDT3A_URL", "http://localhost:8301")+"/clearance/start",
		"", "cdt4", nil, nil,
	)
	err2 := common.DoRequest(
		"POST",
		envOrDefault("IDT3B_URL", "http://localhost:8302")+"/clearance/start",
		"", "cdt4", nil, nil,
	)
	s.mu.Lock()
	s.addLogLocked("Clearance started on both LHDs")
	s.mu.Unlock()
	log.Printf("[cdt4] /clearance/start: lhd1_ok=%v lhd2_ok=%v", err1 == nil, err2 == nil)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status": "clearance started",
		"lhd1":   err1 == nil,
		"lhd2":   err2 == nil,
	})
}

// handleClearanceStop sends a stop command to both LHDs.
func (s *CDT4Service) handleClearanceStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	err1 := common.DoRequest(
		"POST",
		envOrDefault("IDT3A_URL", "http://localhost:8301")+"/clearance/stop",
		"", "cdt4", nil, nil,
	)
	err2 := common.DoRequest(
		"POST",
		envOrDefault("IDT3B_URL", "http://localhost:8302")+"/clearance/stop",
		"", "cdt4", nil, nil,
	)
	s.mu.Lock()
	s.addLogLocked("Clearance stopped on both LHDs")
	s.mu.Unlock()
	log.Printf("[cdt4] /clearance/stop: lhd1_ok=%v lhd2_ok=%v", err1 == nil, err2 == nil)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status": "clearance stopped",
		"lhd1":   err1 == nil,
		"lhd2":   err2 == nil,
	})
}

// handleVehicles returns the last-known state of both LHD vehicles.
func (s *CDT4Service) handleVehicles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"lhd1": s.lhd1,
		"lhd2": s.lhd2,
	})
}

// handleSimulateReset resets the debris scenario on both LHDs.
func (s *CDT4Service) handleSimulateReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	err1 := common.DoRequest(
		"POST",
		envOrDefault("IDT3A_URL", "http://localhost:8301")+"/simulate/reset",
		"", "cdt4", nil, nil,
	)
	err2 := common.DoRequest(
		"POST",
		envOrDefault("IDT3B_URL", "http://localhost:8302")+"/simulate/reset",
		"", "cdt4", nil, nil,
	)
	s.mu.Lock()
	s.addLogLocked("Debris scenario reset on both LHDs")
	s.mu.Unlock()
	log.Printf("[cdt4] /simulate/reset: lhd1_ok=%v lhd2_ok=%v", err1 == nil, err2 == nil)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status": "reset triggered",
		"lhd1":   err1 == nil,
		"lhd2":   err2 == nil,
	})
}

// handleLogs returns recent service log entries.
func (s *CDT4Service) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{"logs": s.serviceLog})
}

// handleConnectivity toggles simulated connectivity.
func (s *CDT4Service) handleConnectivity(w http.ResponseWriter, r *http.Request) {
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
	log.Printf("[cdt4] Connectivity toggled: %v", body.Connected)
	common.WriteJSON(w, 200, map[string]bool{"connected": body.Connected})
}

// addLogLocked appends a timestamped log entry. Caller must hold s.mu (write).
func (s *CDT4Service) addLogLocked(msg string) {
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
