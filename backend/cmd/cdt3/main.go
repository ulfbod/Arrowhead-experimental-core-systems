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

// CDT3Service is the Hazard Detection and Classification composite digital twin.
// It orchestrates over cDT1 (mapping), cDT2 (gas), and the two explorer robots
// to produce a unified HazardReport with risk classification.
type CDT3Service struct {
	mu            sync.RWMutex
	report        common.HazardReport
	lastMapping   *common.MappingResult
	lastGas       *common.GasMonitorResult
	robot1Hazards []common.Hazard
	robot2Hazards []common.Hazard
	ah            *common.ArrowheadClient
	connected     bool
	overrideCleared bool
	serviceLog    []string
}

func main() {
	id := "cdt3"
	name := "Hazard Detection and Classification"
	port := envOrDefault("PORT", "8503")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")

	log.Printf("[%s] Starting %s on :%s", id, name, port)

	svc := &CDT3Service{
		ah:        common.NewArrowheadClient(ahURL, id),
		connected: true,
		report: common.HazardReport{
			Hazards:           []common.Hazard{},
			OverallRisk:       "low",
			SafeForEntry:      true,
			RecommendedAction: "Monitor continuously",
			Timestamp:         time.Now(),
		},
		serviceLog: []string{},
	}

	portInt := 8503
	fmt.Sscanf(port, "%d", &portInt)
	common.RegisterWithRetry(svc.ah, common.RegisterRequest{
		ID:           id,
		Name:         name,
		Address:      host,
		Port:         portInt,
		ServiceType:  "cDT",
		Capabilities: []string{"hazard-detection", "hazard-classification", "risk-assessment", "hazard-report"},
		Metadata:     map[string]string{"composes": "cdt1,cdt2,idt1a,idt1b", "layer": "lower"},
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
	mux.HandleFunc("/hazards", svc.handleHazards)
	mux.HandleFunc("/risk", svc.handleRisk)
	mux.HandleFunc("/override/clear", svc.handleOverrideClear)
	mux.HandleFunc("/logs", svc.handleLogs)
	mux.HandleFunc("/connectivity", svc.handleConnectivity)

	log.Printf("[%s] Listening on :%s", id, port)
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}

// pollLoop aggregates hazard data every 4 seconds.
func (s *CDT3Service) pollLoop() {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.aggregate()
	}
}

func (s *CDT3Service) aggregate() {
	s.mu.RLock()
	connected := s.connected
	overrideCleared := s.overrideCleared
	s.mu.RUnlock()

	if !connected {
		log.Printf("[cdt3] Connectivity disabled, skipping poll")
		return
	}

	// Fetch mapping composite from cDT1
	var mappingResp struct {
		Mapping common.MappingResult `json:"mapping"`
	}
	err := common.DoRequest(
		"GET",
		envOrDefault("CDT1_URL", "http://localhost:8501")+"/state",
		"", "cdt3", nil, &mappingResp,
	)
	var mapping *common.MappingResult
	if err == nil {
		m := mappingResp.Mapping
		mapping = &m
	} else {
		log.Printf("[cdt3] cDT1 fetch error: %v", err)
	}

	// Fetch gas composite from cDT2
	var gasResp struct {
		GasMonitor common.GasMonitorResult `json:"gasMonitor"`
	}
	err = common.DoRequest(
		"GET",
		envOrDefault("CDT2_URL", "http://localhost:8502")+"/state",
		"", "cdt3", nil, &gasResp,
	)
	var gas *common.GasMonitorResult
	if err == nil {
		g := gasResp.GasMonitor
		gas = &g
	} else {
		log.Printf("[cdt3] cDT2 fetch error: %v", err)
	}

	// Fetch hazards from robot 1 via Arrowhead
	var r1 common.RobotState
	err1 := s.ah.CallService("mapping", "GET", "/state", nil, &r1)
	if err1 != nil {
		log.Printf("[cdt3] Robot1 fetch error: %v", err1)
	}

	// Fetch hazards from robot 2 directly
	var r2 common.RobotState
	err2 := common.DoRequest(
		"GET",
		envOrDefault("IDT1B_URL", "http://localhost:8102")+"/state",
		"", "cdt3", nil, &r2,
	)
	if err2 != nil {
		log.Printf("[cdt3] Robot2 fetch error: %v", err2)
	}

	// Collect all hazards
	var allHazards []common.Hazard
	if err1 == nil {
		for _, h := range r1.HazardsDetected {
			if !h.Cleared {
				allHazards = append(allHazards, h)
			}
		}
	}
	if err2 == nil {
		for _, h := range r2.HazardsDetected {
			if !h.Cleared {
				allHazards = append(allHazards, h)
			}
		}
	}

	// Apply override: treat all as cleared for demo if requested
	if overrideCleared {
		allHazards = []common.Hazard{}
	}
	if allHazards == nil {
		allHazards = []common.Hazard{}
	}

	risk, action := classifyRisk(gas, allHazards)
	safeForEntry := risk == "low" && (gas == nil || len(gas.ActiveAlerts) == 0) && len(allHazards) == 0

	report := common.HazardReport{
		Hazards:           allHazards,
		OverallRisk:       risk,
		SafeForEntry:      safeForEntry,
		RecommendedAction: action,
		Timestamp:         time.Now(),
	}

	s.mu.Lock()
	s.report = report
	s.lastMapping = mapping
	s.lastGas = gas
	if err1 == nil {
		s.robot1Hazards = r1.HazardsDetected
	}
	if err2 == nil {
		s.robot2Hazards = r2.HazardsDetected
	}
	s.addLogLocked(fmt.Sprintf("Risk=%s SafeForEntry=%v Hazards=%d", risk, safeForEntry, len(allHazards)))
	s.mu.Unlock()
}

// classifyRisk applies the risk matrix rules and returns a risk level with a recommended action.
func classifyRisk(gas *common.GasMonitorResult, hazards []common.Hazard) (string, string) {
	// Check for misfire in hazard list
	hasMisfire := false
	hasHighSeverity := false
	hasMediumSeverity := false
	hasLowSeverity := false
	for _, h := range hazards {
		if h.Type == "misfire" {
			hasMisfire = true
		}
		switch h.Severity {
		case "critical", "high":
			hasHighSeverity = true
		case "medium":
			hasMediumSeverity = true
		case "low":
			hasLowSeverity = true
		}
	}

	ch4Max := 0.0
	coMax := 0.0
	gasWarning := false
	if gas != nil {
		ch4Max = gas.MaxLevels.CH4
		coMax = gas.MaxLevels.CO
		gasWarning = !gas.EnvironmentSafe
	}

	// Critical: misfire OR CH4>2% OR CO>50ppm OR high-severity hazard
	if hasMisfire || ch4Max > 2.0 || coMax > 50.0 || hasHighSeverity {
		return "critical", "Evacuate immediately. Do not re-enter without clearance."
	}
	// High: CH4>1% OR CO>25ppm OR medium-severity hazard
	if ch4Max > 1.0 || coMax > 25.0 || hasMediumSeverity {
		return "high", "Restrict access. Increase ventilation. Notify safety officer."
	}
	// Medium: gas warning OR low-severity hazard
	if gasWarning || hasLowSeverity {
		return "medium", "Proceed with caution. Monitor gas levels. Keep personnel informed."
	}
	return "low", "Monitor continuously. Normal operations permitted."
}

// handleState returns the full HazardReport together with upstream data.
func (s *CDT3Service) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"hazardReport": s.report,
		"mapping":      s.lastMapping,
		"gas":          s.lastGas,
		"timestamp":    time.Now(),
	})
}

// handleHazards returns the flat list of all active hazards from all sources.
func (s *CDT3Service) handleHazards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"hazards":       s.report.Hazards,
		"robot1Hazards": s.robot1Hazards,
		"robot2Hazards": s.robot2Hazards,
		"count":         len(s.report.Hazards),
		"timestamp":     time.Now(),
	})
}

// handleRisk returns a concise risk summary.
func (s *CDT3Service) handleRisk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"overallRisk":       s.report.OverallRisk,
		"safeForEntry":      s.report.SafeForEntry,
		"recommendedAction": s.report.RecommendedAction,
		"timestamp":         s.report.Timestamp,
	})
}

// handleOverrideClear manually marks all hazards as cleared (demo use).
func (s *CDT3Service) handleOverrideClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	s.mu.Lock()
	s.overrideCleared = true
	s.report.Hazards = []common.Hazard{}
	s.report.OverallRisk = "low"
	s.report.SafeForEntry = true
	s.report.RecommendedAction = "All hazards manually cleared. Monitor continuously."
	s.report.Timestamp = time.Now()
	s.addLogLocked("Override: all hazards manually cleared")
	s.mu.Unlock()
	log.Printf("[cdt3] Override clear applied")
	common.WriteJSON(w, 200, map[string]interface{}{
		"status":    "cleared",
		"timestamp": time.Now(),
	})
}

// handleLogs returns recent service log entries.
func (s *CDT3Service) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{"logs": s.serviceLog})
}

// handleConnectivity toggles simulated connectivity.
func (s *CDT3Service) handleConnectivity(w http.ResponseWriter, r *http.Request) {
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
	log.Printf("[cdt3] Connectivity toggled: %v", body.Connected)
	common.WriteJSON(w, 200, map[string]bool{"connected": body.Connected})
}

// addLogLocked appends a timestamped log entry. Caller must hold s.mu (write).
func (s *CDT3Service) addLogLocked(msg string) {
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
