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

type CDT1Service struct {
	mu         sync.RWMutex
	mapping    common.MappingResult
	robot1     *common.RobotState
	robot2     *common.RobotState
	ah         *common.ArrowheadClient
	connected  bool
	serviceLog []string
}

func main() {
	id := "cdt1"
	name := "Autonomous Exploration and Mapping"
	port := envOrDefault("PORT", "8501")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")

	log.Printf("[%s] Starting %s on :%s", id, name, port)

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
			"status":    "ok",
			"id":        id,
			"timestamp": time.Now(),
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

	log.Printf("[%s] Listening on :%s", id, port)
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}

// pollLoop runs fetchRobotStates every 3 seconds.
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

	// Robot 1: discovered via Arrowhead orchestration for "mapping" capability
	var r1 common.RobotState
	err1 := s.ah.CallService("mapping", "GET", "/state", nil, &r1)
	if err1 != nil {
		log.Printf("[cdt1] Robot1 fetch error: %v", err1)
	}

	// Robot 2: direct call to iDT1b (Arrowhead returns first authorized provider;
	// second robot reached via known sidecar URL for demo purposes)
	var r2 common.RobotState
	err2 := common.DoRequest(
		"GET",
		envOrDefault("IDT1B_URL", "http://localhost:8102")+"/state",
		"", "cdt1", nil, &r2,
	)
	if err2 != nil {
		log.Printf("[cdt1] Robot2 fetch error: %v", err2)
	}

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
			r1.ID, r1.MappingProgress, r1.BatteryPct))
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
		CoveredAreaSqm: totalArea,
		CoveragePct:    avgCoverage,
		ActiveRobots:   active,
		Map:            generateMap(int(avgCoverage)),
		Timestamp:      time.Now(),
	}
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
		"timestamp": time.Now(),
	})
}

// handleStart tells both robots to begin SLAM.
func (s *CDT1Service) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	err1 := s.ah.CallService("slam", "POST", "/slam/start", nil, nil)
	err2 := common.DoRequest(
		"POST",
		envOrDefault("IDT1B_URL", "http://localhost:8102")+"/slam/start",
		"", "cdt1", nil, nil,
	)
	s.mu.Lock()
	s.addLogLocked("Started SLAM on both robots")
	s.mu.Unlock()
	log.Printf("[cdt1] /start: robot1_ok=%v robot2_ok=%v", err1 == nil, err2 == nil)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status": "started",
		"robot1": err1 == nil,
		"robot2": err2 == nil,
	})
}

// handleStop tells both robots to halt SLAM.
func (s *CDT1Service) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	err1 := s.ah.CallService("slam", "POST", "/slam/stop", nil, nil)
	err2 := common.DoRequest(
		"POST",
		envOrDefault("IDT1B_URL", "http://localhost:8102")+"/slam/stop",
		"", "cdt1", nil, nil,
	)
	s.mu.Lock()
	s.addLogLocked("Stopped SLAM on both robots")
	s.mu.Unlock()
	log.Printf("[cdt1] /stop: robot1_ok=%v robot2_ok=%v", err1 == nil, err2 == nil)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status": "stopped",
		"robot1": err1 == nil,
		"robot2": err2 == nil,
	})
}

// handleRobots returns the last-known state of both constituent robots.
func (s *CDT1Service) handleRobots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"robot1": s.robot1,
		"robot2": s.robot2,
	})
}

// handleRobotNavigate proxies a navigate command to the addressed robot.
// URL format: POST /robot/{id}/navigate
func (s *CDT1Service) handleRobotNavigate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	// Extract robot id from path: /robot/<id>/navigate
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["robot", "<id>", "navigate"]
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
		orch, err := s.ah.Discover("mapping")
		if err != nil {
			common.WriteError(w, 502, "orchestration failed: "+err.Error())
			return
		}
		targetURL = orch.Endpoint + "/navigate"
		err = common.DoRequest("POST", targetURL, orch.AuthToken, "cdt1", body, nil)
		if err != nil {
			common.WriteError(w, 502, "robot1 navigate error: "+err.Error())
			return
		}
	case "idt1b", "robot2":
		targetURL = envOrDefault("IDT1B_URL", "http://localhost:8102") + "/navigate"
		err := common.DoRequest("POST", targetURL, "", "cdt1", body, nil)
		if err != nil {
			common.WriteError(w, 502, "robot2 navigate error: "+err.Error())
			return
		}
	default:
		common.WriteError(w, 404, "unknown robot id: "+robotID)
		return
	}

	s.mu.Lock()
	s.addLogLocked(fmt.Sprintf("Navigate command proxied to %s", robotID))
	s.mu.Unlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"status":  "command sent",
		"robotId": robotID,
	})
}

// handleLogs returns recent service log entries.
func (s *CDT1Service) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{"logs": s.serviceLog})
}

// handleConnectivity toggles a simulated connectivity issue.
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
	log.Printf("[cdt1] Connectivity toggled: %v", body.Connected)
	common.WriteJSON(w, 200, map[string]bool{"connected": body.Connected})
}

// addLogLocked appends a timestamped log entry. Caller must hold s.mu (write).
func (s *CDT1Service) addLogLocked(msg string) {
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	if len(s.serviceLog) >= 50 {
		s.serviceLog = s.serviceLog[1:]
	}
	s.serviceLog = append(s.serviceLog, entry)
}

// generateEmptyMap returns a rows×cols grid of zeros.
func generateEmptyMap(rows, cols int) [][]int {
	m := make([][]int, rows)
	for i := range m {
		m[i] = make([]int, cols)
	}
	return m
}

// generateMap returns a 10×10 grid with `progress` cells marked as explored.
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

// corsMiddleware sets permissive CORS headers and handles preflight requests.
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
