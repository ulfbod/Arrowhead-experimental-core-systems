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

type CDT2Service struct {
	mu         sync.RWMutex
	gasResult  common.GasMonitorResult
	sensor1    *common.GasSensorState
	sensor2    *common.GasSensorState
	ah         *common.ArrowheadClient
	connected  bool
	serviceLog []string
}

func main() {
	id := "cdt2"
	name := "Gas Concentration Monitoring"
	port := envOrDefault("PORT", "8502")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")

	log.Printf("[%s] Starting %s on :%s", id, name, port)

	svc := &CDT2Service{
		ah:        common.NewArrowheadClient(ahURL, id),
		connected: true,
		gasResult: common.GasMonitorResult{
			AverageLevels:   common.GasLevels{O2: 20.9},
			MaxLevels:       common.GasLevels{O2: 20.9},
			ActiveAlerts:    []common.GasAlert{},
			EnvironmentSafe: true,
			ActiveSensors:   0,
			Timestamp:       time.Now(),
		},
		serviceLog: []string{},
	}

	portInt := 8502
	fmt.Sscanf(port, "%d", &portInt)
	common.RegisterWithRetry(svc.ah, common.RegisterRequest{
		ID:           id,
		Name:         name,
		Address:      host,
		Port:         portInt,
		ServiceType:  "cDT",
		Capabilities: []string{"gas-monitoring", "gas-concentration", "environment-status", "gas-measurement"},
		Metadata:     map[string]string{"composes": "idt2a,idt2b", "layer": "lower"},
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
	mux.HandleFunc("/alerts", svc.handleAlerts)
	mux.HandleFunc("/threshold", svc.handleThreshold)
	mux.HandleFunc("/sensors", svc.handleSensors)
	mux.HandleFunc("/simulate/spike", svc.handleSimulateSpike)
	mux.HandleFunc("/logs", svc.handleLogs)
	mux.HandleFunc("/connectivity", svc.handleConnectivity)

	log.Printf("[%s] Listening on :%s", id, port)
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}

// pollLoop fetches sensor states every 3 seconds.
func (s *CDT2Service) pollLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.fetchSensorStates()
	}
}

func (s *CDT2Service) fetchSensorStates() {
	s.mu.RLock()
	connected := s.connected
	s.mu.RUnlock()

	if !connected {
		log.Printf("[cdt2] Connectivity disabled, skipping poll")
		return
	}

	// Sensor 1: direct call to iDT2a
	var s1 common.GasSensorState
	err1 := common.DoRequest(
		"GET",
		envOrDefault("IDT2A_URL", "http://localhost:8201")+"/state",
		"", "cdt2", nil, &s1,
	)
	if err1 != nil {
		log.Printf("[cdt2] Sensor1 (idt2a) fetch error: %v", err1)
	}

	// Sensor 2: direct call to iDT2b
	var s2 common.GasSensorState
	err2 := common.DoRequest(
		"GET",
		envOrDefault("IDT2B_URL", "http://localhost:8202")+"/state",
		"", "cdt2", nil, &s2,
	)
	if err2 != nil {
		log.Printf("[cdt2] Sensor2 (idt2b) fetch error: %v", err2)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	activeSensors := 0
	var activeSensorStates []common.GasSensorState

	if err1 == nil {
		s.sensor1 = &s1
		if s1.Online && s1.Connected {
			activeSensors++
			activeSensorStates = append(activeSensorStates, s1)
		}
		s.addLogLocked(fmt.Sprintf("Sensor1 (%s): CH4=%.2f%% CO=%.1fppm env=%s",
			s1.ID, s1.GasLevels.CH4, s1.GasLevels.CO, s1.EnvironmentStatus))
	}
	if err2 == nil {
		s.sensor2 = &s2
		if s2.Online && s2.Connected {
			activeSensors++
			activeSensorStates = append(activeSensorStates, s2)
		}
		s.addLogLocked(fmt.Sprintf("Sensor2 (%s): CH4=%.2f%% CO=%.1fppm env=%s",
			s2.ID, s2.GasLevels.CH4, s2.GasLevels.CO, s2.EnvironmentStatus))
	}

	avg, max := aggregateGasLevels(activeSensorStates)
	allAlerts := collectAlerts(activeSensorStates)
	safe := isEnvironmentSafe(max, allAlerts)

	s.gasResult = common.GasMonitorResult{
		AverageLevels:   avg,
		MaxLevels:       max,
		ActiveAlerts:    allAlerts,
		EnvironmentSafe: safe,
		ActiveSensors:   activeSensors,
		Timestamp:       time.Now(),
	}
}

// aggregateGasLevels computes per-gas average and max across active sensors.
func aggregateGasLevels(sensors []common.GasSensorState) (avg, max common.GasLevels) {
	if len(sensors) == 0 {
		avg = common.GasLevels{O2: 20.9}
		max = common.GasLevels{O2: 20.9}
		return
	}
	n := float64(len(sensors))
	for _, s := range sensors {
		avg.CH4 += s.GasLevels.CH4
		avg.CO += s.GasLevels.CO
		avg.CO2 += s.GasLevels.CO2
		avg.O2 += s.GasLevels.O2
		avg.NO2 += s.GasLevels.NO2

		if s.GasLevels.CH4 > max.CH4 {
			max.CH4 = s.GasLevels.CH4
		}
		if s.GasLevels.CO > max.CO {
			max.CO = s.GasLevels.CO
		}
		if s.GasLevels.CO2 > max.CO2 {
			max.CO2 = s.GasLevels.CO2
		}
		// O2: lower is worse; track minimum as "max hazard"
		if max.O2 == 0 || s.GasLevels.O2 < max.O2 {
			max.O2 = s.GasLevels.O2
		}
		if s.GasLevels.NO2 > max.NO2 {
			max.NO2 = s.GasLevels.NO2
		}
	}
	avg.CH4 /= n
	avg.CO /= n
	avg.CO2 /= n
	avg.O2 /= n
	avg.NO2 /= n
	return
}

// collectAlerts merges active alerts from all sensors, deduplicating by ID.
func collectAlerts(sensors []common.GasSensorState) []common.GasAlert {
	seen := map[string]bool{}
	var alerts []common.GasAlert
	for _, s := range sensors {
		for _, a := range s.Alerts {
			if a.Active && !seen[a.ID] {
				seen[a.ID] = true
				alerts = append(alerts, a)
			}
		}
	}
	if alerts == nil {
		alerts = []common.GasAlert{}
	}
	return alerts
}

// isEnvironmentSafe returns false if any threshold is exceeded or active alerts exist.
func isEnvironmentSafe(max common.GasLevels, alerts []common.GasAlert) bool {
	if max.CH4 >= 1.0 {
		return false
	}
	if max.CO >= 25.0 {
		return false
	}
	if max.O2 != 0 && max.O2 < 19.5 {
		return false
	}
	if len(alerts) > 0 {
		return false
	}
	return true
}

// handleState returns the full composite gas monitoring result.
func (s *CDT2Service) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"gasMonitor": s.gasResult,
		"sensor1":    s.sensor1,
		"sensor2":    s.sensor2,
		"timestamp":  time.Now(),
	})
}

// handleAlerts returns only the combined active alert list.
func (s *CDT2Service) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"alerts":    s.gasResult.ActiveAlerts,
		"count":     len(s.gasResult.ActiveAlerts),
		"timestamp": time.Now(),
	})
}

// handleThreshold accepts a POST body with new threshold values and forwards to both sensors.
func (s *CDT2Service) handleThreshold(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, 400, "invalid JSON body")
		return
	}

	err1 := common.DoRequest("POST", envOrDefault("IDT2A_URL", "http://localhost:8201")+"/threshold", "", "cdt2", body, nil)
	err2 := common.DoRequest(
		"POST",
		envOrDefault("IDT2B_URL", "http://localhost:8202")+"/threshold",
		"", "cdt2", body, nil,
	)
	s.mu.Lock()
	s.addLogLocked("Updated gas thresholds on both sensors")
	s.mu.Unlock()
	log.Printf("[cdt2] /threshold: sensor1_ok=%v sensor2_ok=%v", err1 == nil, err2 == nil)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status":  "thresholds updated",
		"sensor1": err1 == nil,
		"sensor2": err2 == nil,
	})
}

// handleSensors returns the last-known individual sensor states.
func (s *CDT2Service) handleSensors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"sensor1": s.sensor1,
		"sensor2": s.sensor2,
	})
}

// handleSimulateSpike triggers a gas spike simulation on both sensors.
func (s *CDT2Service) handleSimulateSpike(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)

	err1 := common.DoRequest("POST", envOrDefault("IDT2A_URL", "http://localhost:8201")+"/simulate/spike", "", "cdt2", body, nil)
	err2 := common.DoRequest(
		"POST",
		envOrDefault("IDT2B_URL", "http://localhost:8202")+"/simulate/spike",
		"", "cdt2", body, nil,
	)
	s.mu.Lock()
	s.addLogLocked("Triggered gas spike simulation on both sensors")
	s.mu.Unlock()
	log.Printf("[cdt2] /simulate/spike: sensor1_ok=%v sensor2_ok=%v", err1 == nil, err2 == nil)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status":  "spike triggered",
		"sensor1": err1 == nil,
		"sensor2": err2 == nil,
	})
}

// handleLogs returns recent service log entries.
func (s *CDT2Service) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{"logs": s.serviceLog})
}

// handleConnectivity toggles a simulated connectivity issue.
func (s *CDT2Service) handleConnectivity(w http.ResponseWriter, r *http.Request) {
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
	log.Printf("[cdt2] Connectivity toggled: %v", body.Connected)
	common.WriteJSON(w, 200, map[string]bool{"connected": body.Connected})
}

// addLogLocked appends a timestamped log entry. Caller must hold s.mu (write).
func (s *CDT2Service) addLogLocked(msg string) {
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
