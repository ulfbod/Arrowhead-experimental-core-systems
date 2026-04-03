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

// ---- QoS configuration for inspection robots ----
// Robot A (idt1a): primary – higher accuracy SLAM, lower latency.
// Robot B (idt1b): fallback – slightly lower accuracy, higher latency.
var robotProviders = []common.ProviderConfig{
	{ID: "idt1a", Primary: true, NominalAccuracy: 0.95, NominalLatencyMs: 12, NominalReliability: 0.97},
	{ID: "idt1b", Primary: false, NominalAccuracy: 0.88, NominalLatencyMs: 18, NominalReliability: 0.93},
}

type CDT1Service struct {
	mu         sync.RWMutex
	mapping    common.MappingResult
	robot1     *common.RobotState
	robot2     *common.RobotState
	robotQoS   common.SourceQoS       // QoS state for robot1 (failover-managed)
	ps         *common.ProviderSelector // idt1a primary → idt1b fallback
	ah         *common.ArrowheadClient
	connected  bool
	serviceLog []string
	streamLog  *common.StreamLogger
}

func main() {
	id := "cdt1"
	name := "Autonomous Exploration and Mapping"
	port := envOrDefault("PORT", "8501")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")
	logDir := envOrDefault("LOG_DIR", "./logs")

	robotProviders[0].URL = envOrDefault("IDT1A_URL", "http://localhost:8101")
	robotProviders[1].URL = envOrDefault("IDT1B_URL", "http://localhost:8102")

	log.Printf("[%s] Starting %s on :%s", id, name, port)

	evLog, err := common.NewFailoverLogger(logDir, "failover_events.csv")
	if err != nil {
		log.Printf("[%s] WARNING: cannot open failover log: %v", id, err)
	}

	streamLog, err := common.NewStreamLogger(logDir, "cdt1_mapping_stream.csv",
		"timestamp_ms,timestamp_iso,active_provider,on_fallback,"+
			"coverage_pct,area_sqm,active_robots,"+
			"accuracy,latency_ms,reliability,freshness_ms,degraded")
	if err != nil {
		log.Printf("[%s] WARNING: cannot open stream log: %v", id, err)
	}

	svc := &CDT1Service{
		ah:        common.NewArrowheadClient(ahURL, id),
		connected: true,
		mapping: common.MappingResult{
			TotalAreaSqm:   5000,
			CoveredAreaSqm: 0,
			CoveragePct:    0,
			ActiveRobots:   0,
			Map:            generateEmptyMap(10, 10),
			Timestamp:      time.Now(),
		},
		serviceLog: []string{},
		streamLog:  streamLog,
		ps:         common.NewProviderSelector(id, "mapping", robotProviders, 3, evLog, common.NewArrowheadClient(ahURL, id)),
	}

	portInt := 8501
	fmt.Sscanf(port, "%d", &portInt)
	common.RegisterWithRetry(svc.ah, common.RegisterRequest{
		ID:           id,
		Name:         name,
		Address:      host,
		Port:         portInt,
		ServiceType:  "cDT",
		Capabilities: []string{"mapping-composite", "area-coverage", "situational-awareness", "mapping"},
		Metadata:     map[string]string{"composes": "idt1a,idt1b", "layer": "lower"},
	}, 10)

	go svc.pollLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, 200, map[string]interface{}{
			"status": "ok", "id": id, "timestamp": time.Now(),
		})
	})
	mux.HandleFunc("/state", svc.handleState)
	mux.HandleFunc("/mapping", svc.handleState)
	mux.HandleFunc("/start", svc.handleStart)
	mux.HandleFunc("/stop", svc.handleStop)
	mux.HandleFunc("/robots", svc.handleRobots)
	mux.HandleFunc("/robot/", svc.handleRobotNavigate)
	mux.HandleFunc("/logs", svc.handleLogs)
	mux.HandleFunc("/connectivity", svc.handleConnectivity)
	// Provider QoS management
	mux.HandleFunc("/providers", svc.handleProviders)
	mux.HandleFunc("/provider/fail", svc.handleProviderFail)
	mux.HandleFunc("/provider/recover", svc.handleProviderRecover)
	mux.HandleFunc("/provider/degrade", svc.handleProviderDegrade)
	// Experiment support: trigger an immediate poll cycle
	mux.HandleFunc("/trigger-poll", svc.handleTriggerPoll)

	log.Printf("[%s] Listening on :%s", id, port)
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}

func (s *CDT1Service) pollLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.fetchRobotStates()
	}
}

func (s *CDT1Service) fetchRobotStates() {
	s.mu.RLock()
	connected := s.connected
	s.mu.RUnlock()
	if !connected {
		log.Printf("[cdt1] Connectivity disabled, skipping poll")
		return
	}

	// Robot 1: managed by ProviderSelector (idt1a primary → idt1b fallback).
	var r1 common.RobotState
	qos1, err1 := s.ps.Do("GET", "/state", nil, &r1)
	if err1 != nil {
		log.Printf("[cdt1] Robot1 fetch error (provider=%s): %v", s.ps.ActiveProviderID(), err1)
	}

	// Robot 2: always direct to idt1b (independent contribution).
	var r2 common.RobotState
	err2 := common.DoRequest("GET", robotProviders[1].URL+"/state", "", "cdt1", nil, &r2)
	if err2 != nil {
		log.Printf("[cdt1] Robot2 (idt1b) fetch error: %v", err2)
	}

	psState := s.ps.State()

	s.mu.Lock()
	defer s.mu.Unlock()

	active := 0
	totalCoverage := 0.0
	totalArea := 0.0

	if err1 == nil {
		s.robot1 = &r1
		if r1.Online && r1.Connected {
			active++
			totalCoverage += r1.MappingProgress
			totalArea += r1.AreaCoveredSqm
		}
		s.addLogLocked(fmt.Sprintf("Robot1 (%s): progress=%.1f%% battery=%.0f%%",
			psState.Active.ID, r1.MappingProgress, r1.BatteryPct))
	}
	if err2 == nil {
		s.robot2 = &r2
		if r2.Online && r2.Connected {
			active++
			totalCoverage += r2.MappingProgress
			totalArea += r2.AreaCoveredSqm
		}
		s.addLogLocked(fmt.Sprintf("Robot2 (%s): progress=%.1f%% battery=%.0f%%",
			r2.ID, r2.MappingProgress, r2.BatteryPct))
	}

	avgCoverage := 0.0
	if active > 0 {
		avgCoverage = totalCoverage / float64(active)
	}

	s.mapping = common.MappingResult{
		TotalAreaSqm:   5000,
		CoveredAreaSqm: avgCoverage / 100.0 * 5000.0, // consistent with displayed percentage
		CoveragePct:    avgCoverage,
		ActiveRobots:   active,
		Map:            generateMap(int(avgCoverage)),
		Timestamp:      time.Now(),
	}
	s.robotQoS = psState

	// Write stream log row
	now := time.Now()
	onFallback := psState.Active.ID != robotProviders[0].ID
	s.streamLog.WriteRow(
		now.UnixMilli(),
		now.Format(time.RFC3339),
		psState.Active.ID,
		onFallback,
		avgCoverage, totalArea, active,
		qos1.Accuracy, qos1.LatencyMs, qos1.Reliability, qos1.FreshnessMs,
		psState.Degraded,
	)
}

// handleState returns the combined mapping result plus individual robot states.
func (s *CDT1Service) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"mapping":   s.mapping,
		"robot1":    s.robot1,
		"robot2":    s.robot2,
		"qos":       s.robotQoS,
		"timestamp": time.Now(),
	})
}

// handleStart tells both robots to begin SLAM.
func (s *CDT1Service) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	err1 := common.DoRequest("POST", robotProviders[0].URL+"/slam/start", "", "cdt1", nil, nil)
	err2 := common.DoRequest("POST", robotProviders[1].URL+"/slam/start", "", "cdt1", nil, nil)
	s.mu.Lock()
	s.addLogLocked("Started SLAM on both robots")
	s.mu.Unlock()
	log.Printf("[cdt1] /start: robot1_ok=%v robot2_ok=%v", err1 == nil, err2 == nil)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status": "started", "robot1": err1 == nil, "robot2": err2 == nil,
	})
}

// handleStop tells both robots to halt SLAM.
func (s *CDT1Service) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	err1 := common.DoRequest("POST", robotProviders[0].URL+"/slam/stop", "", "cdt1", nil, nil)
	err2 := common.DoRequest("POST", robotProviders[1].URL+"/slam/stop", "", "cdt1", nil, nil)
	s.mu.Lock()
	s.addLogLocked("Stopped SLAM on both robots")
	s.mu.Unlock()
	log.Printf("[cdt1] /stop: robot1_ok=%v robot2_ok=%v", err1 == nil, err2 == nil)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status": "stopped", "robot1": err1 == nil, "robot2": err2 == nil,
	})
}

func (s *CDT1Service) handleRobots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{"robot1": s.robot1, "robot2": s.robot2})
}

// handleRobotNavigate proxies a navigate command. URL: POST /robot/{id}/navigate
func (s *CDT1Service) handleRobotNavigate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || parts[2] != "navigate" {
		common.WriteError(w, 404, "not found")
		return
	}
	robotID := parts[1]
	var body interface{}
	json.NewDecoder(r.Body).Decode(&body)

	var targetURL string
	switch robotID {
	case "idt1a", "robot1":
		targetURL = robotProviders[0].URL + "/navigate"
	case "idt1b", "robot2":
		targetURL = robotProviders[1].URL + "/navigate"
	default:
		common.WriteError(w, 404, "unknown robot id: "+robotID)
		return
	}
	if err := common.DoRequest("POST", targetURL, "", "cdt1", body, nil); err != nil {
		common.WriteError(w, 502, "navigate error: "+err.Error())
		return
	}
	s.mu.Lock()
	s.addLogLocked(fmt.Sprintf("Navigate command proxied to %s", robotID))
	s.mu.Unlock()
	common.WriteJSON(w, 200, map[string]interface{}{"status": "command sent", "robotId": robotID})
}

func (s *CDT1Service) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{"logs": s.serviceLog})
}

func (s *CDT1Service) handleConnectivity(w http.ResponseWriter, r *http.Request) {
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
	common.WriteJSON(w, 200, map[string]bool{"connected": body.Connected})
}

func (s *CDT1Service) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	common.WriteJSON(w, 200, s.ps.State())
}

func (s *CDT1Service) handleProviderFail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	var body struct {
		ProviderID string `json:"providerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProviderID == "" {
		common.WriteError(w, 400, "providerId required")
		return
	}
	s.ps.MarkFailed(body.ProviderID)
	common.WriteJSON(w, 200, map[string]string{"status": "failed", "providerId": body.ProviderID})
}

func (s *CDT1Service) handleProviderRecover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	var body struct {
		ProviderID string `json:"providerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProviderID == "" {
		common.WriteError(w, 400, "providerId required")
		return
	}
	s.ps.MarkRecovered(body.ProviderID)
	common.WriteJSON(w, 200, map[string]string{"status": "recovered", "providerId": body.ProviderID})
}

func (s *CDT1Service) handleProviderDegrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	var body struct {
		ProviderID     string  `json:"providerId"`
		AccuracyFactor float64 `json:"accuracyFactor"`
		LatencyMs      float64 `json:"latencyMs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProviderID == "" {
		common.WriteError(w, 400, "providerId required")
		return
	}
	if body.AccuracyFactor <= 0 || body.AccuracyFactor > 1 {
		body.AccuracyFactor = 0.65
	}
	s.ps.MarkDegraded(body.ProviderID, body.AccuracyFactor, body.LatencyMs)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status": "degraded", "providerId": body.ProviderID,
		"accuracyFactor": body.AccuracyFactor, "latencyMs": body.LatencyMs,
	})
}

// handleTriggerPoll forces an immediate poll of iDT providers.
// Used by the experiment runner to bypass the normal ticker interval.
func (s *CDT1Service) handleTriggerPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	s.fetchRobotStates()
	ev := s.ps.LatestFailoverEvent()
	common.WriteJSON(w, 200, map[string]interface{}{
		"status":        "polled",
		"activeProvider": s.ps.ActiveProviderID(),
		"latestFailover": ev,
	})
}

func (s *CDT1Service) addLogLocked(msg string) {
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	if len(s.serviceLog) >= 50 {
		s.serviceLog = s.serviceLog[1:]
	}
	s.serviceLog = append(s.serviceLog, entry)
}

func generateEmptyMap(rows, cols int) [][]int {
	m := make([][]int, rows)
	for i := range m {
		m[i] = make([]int, cols)
	}
	return m
}

func generateMap(progress int) [][]int {
	m := generateEmptyMap(10, 10)
	n := progress / 10
	if n > 100 {
		n = 100
	}
	for i := 0; i < n; i++ {
		row := i / 10
		col := i % 10
		if row < 10 && col < 10 {
			m[row][col] = 1
		}
	}
	return m
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
