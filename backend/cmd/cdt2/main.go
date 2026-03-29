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

// ---- QoS configuration for gas sensors ----
// Primary (idt2a): higher accuracy, lower latency.
// Fallback (idt2b): slightly lower accuracy, higher latency.
var gasProviders = []common.ProviderConfig{
	{ID: "idt2a", Primary: true, NominalAccuracy: 0.95, NominalLatencyMs: 10, NominalReliability: 0.98},
	{ID: "idt2b", Primary: false, NominalAccuracy: 0.85, NominalLatencyMs: 15, NominalReliability: 0.95},
}

type CDT2Service struct {
	mu         sync.RWMutex
	gasResult  common.GasMonitorResult
	sensor1    *common.GasSensorState
	sensor2    *common.GasSensorState
	sensorQoS  common.SourceQoS       // QoS state for the primary (failover-managed) sensor
	ps         *common.ProviderSelector // manages sensor1: idt2a (primary) → idt2b (fallback)
	ah         *common.ArrowheadClient
	connected  bool
	serviceLog []string
	streamLog  *common.StreamLogger
}

func main() {
	id := "cdt2"
	name := "Gas Concentration Monitoring"
	port := envOrDefault("PORT", "8502")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")
	logDir := envOrDefault("LOG_DIR", "./logs")

	// Inject URLs into provider configs from environment
	gasProviders[0].URL = envOrDefault("IDT2A_URL", "http://localhost:8201")
	gasProviders[1].URL = envOrDefault("IDT2B_URL", "http://localhost:8202")

	log.Printf("[%s] Starting %s on :%s", id, name, port)

	evLog, err := common.NewFailoverLogger(logDir, "failover_events.csv")
	if err != nil {
		log.Printf("[%s] WARNING: cannot open failover log: %v", id, err)
	}

	streamLog, err := common.NewStreamLogger(logDir, "cdt2_gas_stream.csv",
		"timestamp_ms,timestamp_iso,active_provider,on_fallback,"+
			"ch4_pct,co_ppm,co2_pct,o2_pct,no2_ppm,"+
			"accuracy,latency_ms,reliability,freshness_ms,degraded")
	if err != nil {
		log.Printf("[%s] WARNING: cannot open stream log: %v", id, err)
	}

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
		streamLog:  streamLog,
		ps: common.NewProviderSelector(id, "gas-measurement", gasProviders, 3, evLog, common.NewArrowheadClient(ahURL, id)),
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
			"status": "ok", "id": id, "timestamp": time.Now(),
		})
	})
	mux.HandleFunc("/state", svc.handleState)
	mux.HandleFunc("/alerts", svc.handleAlerts)
	mux.HandleFunc("/threshold", svc.handleThreshold)
	mux.HandleFunc("/sensors", svc.handleSensors)
	mux.HandleFunc("/simulate/spike", svc.handleSimulateSpike)
	mux.HandleFunc("/logs", svc.handleLogs)
	mux.HandleFunc("/connectivity", svc.handleConnectivity)
	// Provider QoS management endpoints (used by scenario runner)
	mux.HandleFunc("/providers", svc.handleProviders)
	mux.HandleFunc("/provider/fail", svc.handleProviderFail)
	mux.HandleFunc("/provider/recover", svc.handleProviderRecover)
	mux.HandleFunc("/provider/degrade", svc.handleProviderDegrade)
	// Experiment support: trigger an immediate poll cycle
	mux.HandleFunc("/trigger-poll", svc.handleTriggerPoll)

	log.Printf("[%s] Listening on :%s", id, port)
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}

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

	// Sensor 1: managed by ProviderSelector (idt2a primary → idt2b fallback).
	var s1 common.GasSensorState
	qos1, err1 := s.ps.Do("GET", "/state", nil, &s1)
	if err1 != nil {
		log.Printf("[cdt2] Sensor1 fetch error (provider=%s): %v", s.ps.ActiveProviderID(), err1)
	}

	// Sensor 2: always direct to idt2b (independent reading).
	var s2 common.GasSensorState
	err2 := common.DoRequest("GET", gasProviders[1].URL+"/state", "", "cdt2", nil, &s2)
	if err2 != nil {
		log.Printf("[cdt2] Sensor2 (idt2b) fetch error: %v", err2)
	}

	psState := s.ps.State()

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
			psState.Active.ID, s1.GasLevels.CH4, s1.GasLevels.CO, s1.EnvironmentStatus))
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
	s.sensorQoS = psState

	// Write stream log row
	now := time.Now()
	onFallback := psState.Active.ID != gasProviders[0].ID
	s.streamLog.WriteRow(
		now.UnixMilli(),
		now.Format(time.RFC3339),
		psState.Active.ID,
		onFallback,
		avg.CH4, avg.CO, avg.CO2, avg.O2, avg.NO2,
		qos1.Accuracy, qos1.LatencyMs, qos1.Reliability, qos1.FreshnessMs,
		psState.Degraded,
	)
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

func isEnvironmentSafe(max common.GasLevels, alerts []common.GasAlert) bool {
	if max.CH4 >= 1.0 || max.CO >= 25.0 {
		return false
	}
	if max.O2 != 0 && max.O2 < 19.5 {
		return false
	}
	return len(alerts) == 0
}

// ---- HTTP handlers ----

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
		"qos":        s.sensorQoS,
		"timestamp":  time.Now(),
	})
}

func (s *CDT2Service) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"alerts": s.gasResult.ActiveAlerts, "count": len(s.gasResult.ActiveAlerts), "timestamp": time.Now(),
	})
}

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
	err1 := common.DoRequest("POST", gasProviders[0].URL+"/threshold", "", "cdt2", body, nil)
	err2 := common.DoRequest("POST", gasProviders[1].URL+"/threshold", "", "cdt2", body, nil)
	s.mu.Lock()
	s.addLogLocked("Updated gas thresholds on both sensors")
	s.mu.Unlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"status": "thresholds updated", "sensor1": err1 == nil, "sensor2": err2 == nil,
	})
}

func (s *CDT2Service) handleSensors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{"sensor1": s.sensor1, "sensor2": s.sensor2})
}

func (s *CDT2Service) handleSimulateSpike(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	err1 := common.DoRequest("POST", gasProviders[0].URL+"/simulate/spike", "", "cdt2", body, nil)
	err2 := common.DoRequest("POST", gasProviders[1].URL+"/simulate/spike", "", "cdt2", body, nil)
	s.mu.Lock()
	s.addLogLocked("Triggered gas spike simulation on both sensors")
	s.mu.Unlock()
	common.WriteJSON(w, 200, map[string]interface{}{
		"status": "spike triggered", "sensor1": err1 == nil, "sensor2": err2 == nil,
	})
}

func (s *CDT2Service) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	common.WriteJSON(w, 200, map[string]interface{}{"logs": s.serviceLog})
}

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
	common.WriteJSON(w, 200, map[string]bool{"connected": body.Connected})
}

// handleProviders returns the QoS state of all managed providers.
func (s *CDT2Service) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		common.WriteError(w, 405, "GET required")
		return
	}
	common.WriteJSON(w, 200, s.ps.State())
}

// handleProviderFail forces the named provider into failed state.
// Body: {"providerId": "idt2a"}
func (s *CDT2Service) handleProviderFail(w http.ResponseWriter, r *http.Request) {
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

// handleProviderRecover recovers the named provider.
func (s *CDT2Service) handleProviderRecover(w http.ResponseWriter, r *http.Request) {
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

// handleProviderDegrade degrades the named provider's QoS.
// Body: {"providerId": "idt2a", "accuracyFactor": 0.6, "latencyMs": 100}
func (s *CDT2Service) handleProviderDegrade(w http.ResponseWriter, r *http.Request) {
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
func (s *CDT2Service) handleTriggerPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	s.fetchSensorStates()
	ev := s.ps.LatestFailoverEvent()
	common.WriteJSON(w, 200, map[string]interface{}{
		"status":         "polled",
		"activeProvider": s.ps.ActiveProviderID(),
		"latestFailover": ev,
	})
}

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
