package main

// cDTb: Hazard Monitoring, Ventilation, and Gating
// Upper-layer composite Digital Twin responsible for safe-access decisions.
// Composes cDT2 (Gas Monitoring) and cDT3 (Hazard Detection) to determine
// whether the mine environment is safe for personnel re-entry.

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

// ---- Thresholds -------------------------------------------------------------------

const (
	ventCH4Max   = 0.5  // % – methane ventilation limit
	ventCOMax    = 10.0 // ppm – CO ventilation limit
	ventO2Min    = 19.5 // % – minimum oxygen
	ventO2Max    = 23.0 // % – maximum oxygen
)

// ---- Service ----------------------------------------------------------------------

type CDTbService struct {
	mu          sync.RWMutex
	decision    common.SafeAccessDecision
	gasQoS      common.SourceQoS // QoS state propagated from cDT2
	comps       struct {
		CDT2 string
		CDT3 string
	}
	ah             *common.ArrowheadClient
	id             string
	gateManualOpen bool
	streamLog      *common.StreamLogger
}

func main() {
	id := envOrDefault("CDT_ID", "cdtb")
	name := envOrDefault("CDT_NAME", "cDTb: Hazard Monitoring & Gating")
	port := envOrDefault("PORT", "8602")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")

	cdt2URL := envOrDefault("CDT2_URL", "http://localhost:8502")
	cdt3URL := envOrDefault("CDT3_URL", "http://localhost:8503")
	logDir := envOrDefault("LOG_DIR", "./logs")

	log.Printf("[%s] Starting %s on :%s", id, name, port)
	log.Printf("[%s] Composing: cDT2=%s cDT3=%s", id, cdt2URL, cdt3URL)

	streamLog, err := common.NewStreamLogger(logDir, "cdtb_gas_stream.csv",
		"timestamp_ms,timestamp_iso,source_cdt,active_provider,on_fallback,"+
			"ch4_avg,co_avg,o2_avg,accuracy,latency_ms,reliability,"+
			"qos_degraded,safe_for_entry,gate_status")
	if err != nil {
		log.Printf("[%s] WARNING: cannot open stream log: %v", id, err)
	}

	now := time.Now()
	svc := &CDTbService{
		id:        id,
		ah:        common.NewArrowheadClient(ahURL, id),
		streamLog: streamLog,
		decision: common.SafeAccessDecision{
			Safe:            false,
			Reason:          "System initialising – first assessment pending.",
			VentilationOK:   false,
			GatingStatus:    "closed",
			Recommendations: []string{"Awaiting first gas and hazard assessment cycle."},
			LastUpdated:     now,
		},
	}
	svc.comps.CDT2 = cdt2URL
	svc.comps.CDT3 = cdt3URL

	portInt := 8602
	fmt.Sscanf(port, "%d", &portInt)

	common.RegisterWithRetry(svc.ah, common.RegisterRequest{
		ID:           id,
		Name:         name,
		Address:      host,
		Port:         portInt,
		ServiceType:  "cDT",
		Capabilities: []string{"hazard-monitoring", "ventilation-support", "safe-access", "gating-control"},
		Metadata: map[string]string{
			"layer":    "upper",
			"composes": "cdt2,cdt3",
		},
	}, 10)

	go svc.pollLoop()

	router := common.NewRouter()
	router.Handle("/health", svc.handleHealth)
	router.Handle("/state", svc.handleState)
	router.Handle("/gas", svc.handleGas)
	router.Handle("/hazards", svc.handleHazards)
	router.Handle("/gating", svc.handleGating)
	router.Handle("/gate/open", svc.handleGateOpen)
	router.Handle("/gate/close", svc.handleGateClose)
	router.Handle("/recommendations", svc.handleRecommendations)
	router.Handle("/connectivity", svc.handleConnectivity)
	router.Handle("/ventilation/check", svc.handleVentilationCheck)

	log.Printf("[%s] Listening on :%s", id, port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

// ---- Poll loop --------------------------------------------------------------------

func (s *CDTbService) pollLoop() {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.assess()
	}
}

// assess fetches component states, recomputes the safe-access decision, and logs
// any relevant state changes.
func (s *CDTbService) assess() {
	gasResult, gasQoS := s.fetchGas()
	hazardReport := s.fetchHazards()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.decision.GasStatus = gasResult
	s.decision.HazardStatus = hazardReport
	if gasQoS != nil {
		s.gasQoS = *gasQoS
	}

	// ---- Ventilation check ----
	ventOK := false
	ventReason := ""
	if gasResult != nil {
		gl := gasResult.AverageLevels
		switch {
		case gl.CH4 >= ventCH4Max:
			ventReason = fmt.Sprintf("CH4=%.3f%% exceeds ventilation limit of %.1f%%", gl.CH4, ventCH4Max)
		case gl.CO >= ventCOMax:
			ventReason = fmt.Sprintf("CO=%.1fppm exceeds ventilation limit of %.0fppm", gl.CO, ventCOMax)
		case gl.O2 < ventO2Min:
			ventReason = fmt.Sprintf("O2=%.1f%% below minimum %.1f%%", gl.O2, ventO2Min)
		case gl.O2 > ventO2Max:
			ventReason = fmt.Sprintf("O2=%.1f%% exceeds maximum %.1f%%", gl.O2, ventO2Max)
		default:
			ventOK = true
			ventReason = fmt.Sprintf("All gas levels within limits: CH4=%.3f%% CO=%.1fppm O2=%.1f%%", gl.CH4, gl.CO, gl.O2)
		}
	} else {
		ventReason = "Gas monitoring data unavailable – defaulting to unsafe."
	}
	s.decision.VentilationOK = ventOK

	// ---- Hazard analysis ----
	hasCriticalHazard := false
	hasWarningHazard := false
	activeHazardCount := 0
	if hazardReport != nil {
		for _, h := range hazardReport.Hazards {
			if !h.Cleared {
				activeHazardCount++
				if h.Severity == "critical" || hazardReport.OverallRisk == "critical" {
					hasCriticalHazard = true
				} else if h.Severity == "high" || h.Severity == "medium" {
					hasWarningHazard = true
				}
			}
		}
	}

	// ---- Gas alert analysis ----
	hasCriticalGas := false
	hasWarningGas := false
	if gasResult != nil {
		for _, alert := range gasResult.ActiveAlerts {
			if alert.Active {
				if alert.Gas == "CH4" || alert.Gas == "CO" || alert.Gas == "O2-low" {
					hasCriticalGas = true
				} else {
					hasWarningGas = true
				}
			}
		}
	}

	// ---- Gating status ----
	// Manual open overrides logic when conditions are met
	prevGating := s.decision.GatingStatus

	var gatingStatus string
	switch {
	case hasCriticalGas || hasCriticalHazard:
		gatingStatus = "closed"
	case hasWarningGas || hasWarningHazard || !ventOK:
		gatingStatus = "conditional"
	default:
		gatingStatus = "open"
	}

	// Respect manual open only if the logic would allow "open" or "conditional"
	if s.gateManualOpen && gatingStatus != "closed" {
		gatingStatus = "open"
	}
	s.decision.GatingStatus = gatingStatus

	if prevGating != gatingStatus {
		log.Printf("[%s] Gating status changed: %q -> %q", s.id, prevGating, gatingStatus)
	}

	// ---- Safe access decision ----
	noActiveHazards := activeHazardCount == 0
	s.decision.Safe = ventOK && gatingStatus == "open" && noActiveHazards

	// ---- Reason string ----
	switch {
	case hasCriticalGas:
		s.decision.Reason = "UNSAFE: Critical gas levels detected – gate locked closed."
	case hasCriticalHazard:
		s.decision.Reason = "UNSAFE: Critical hazard(s) in zone – gate locked closed."
	case !ventOK:
		s.decision.Reason = "UNSAFE: Ventilation check failed – " + ventReason
	case hasWarningGas || hasWarningHazard:
		s.decision.Reason = "CONDITIONAL: Non-critical warnings present – gate conditional. Investigate before personnel entry."
	case !noActiveHazards:
		s.decision.Reason = fmt.Sprintf("CONDITIONAL: %d active hazard(s) remain – ensure clearance before entry.", activeHazardCount)
	case s.decision.Safe:
		s.decision.Reason = fmt.Sprintf("SAFE: %s", ventReason)
	default:
		s.decision.Reason = "UNSAFE: Combined conditions do not permit safe access."
	}

	s.decision.Recommendations = s.generateRecommendations()
	s.decision.LastUpdated = time.Now()

	// Add QoS degradation advisory if active
	if s.gasQoS.Degraded {
		s.decision.Recommendations = append([]string{
			fmt.Sprintf("QoS WARNING: Gas monitoring on fallback provider %q – accuracy %.0f%%, reliability %.0f%%. Verify readings independently.",
				s.gasQoS.Active.ID,
				s.gasQoS.Active.QoS.Accuracy*100,
				s.gasQoS.Active.QoS.Reliability*100),
		}, s.decision.Recommendations...)
	}

	// Write stream log row
	now := time.Now()
	var ch4, co, o2 float64
	if gasResult != nil {
		ch4 = gasResult.AverageLevels.CH4
		co = gasResult.AverageLevels.CO
		o2 = gasResult.AverageLevels.O2
	}
	onFallback := s.gasQoS.Degraded
	s.streamLog.WriteRow(
		now.UnixMilli(),
		now.Format(time.RFC3339),
		"cdt2",
		s.gasQoS.Active.ID,
		onFallback,
		ch4, co, o2,
		s.gasQoS.Active.QoS.Accuracy,
		s.gasQoS.Active.QoS.LatencyMs,
		s.gasQoS.Active.QoS.Reliability,
		s.gasQoS.Degraded,
		s.decision.Safe,
		s.decision.GatingStatus,
	)
}

// generateRecommendations produces contextual recommendations. Caller must hold s.mu.
func (s *CDTbService) generateRecommendations() []string {
	var recs []string

	if s.decision.GasStatus != nil {
		gl := s.decision.GasStatus.AverageLevels
		if gl.CH4 >= ventCH4Max {
			recs = append(recs,
				fmt.Sprintf("ALERT: CH4 at %.3f%% – activate additional forcing fans and delay all entry.", gl.CH4),
				"Verify ventilation fan operation and check for blasting fume dilution.",
			)
		}
		if gl.CO >= ventCOMax {
			recs = append(recs,
				fmt.Sprintf("ALERT: CO at %.1fppm – allow minimum 30 minutes post-blast ventilation before re-entry.", gl.CO),
			)
		}
		if gl.O2 < ventO2Min {
			recs = append(recs,
				fmt.Sprintf("ALERT: O2 at %.1f%% – oxygen-deficient atmosphere. SCBA required for any entry.", gl.O2),
			)
		}
	}

	if s.decision.HazardStatus != nil {
		misfireCnt, looseCnt := 0, 0
		for _, h := range s.decision.HazardStatus.Hazards {
			if !h.Cleared {
				switch h.Type {
				case "misfire":
					misfireCnt++
				case "loose-rock":
					looseCnt++
				}
			}
		}
		if misfireCnt > 0 {
			recs = append(recs,
				fmt.Sprintf("WARNING: %d uncleared misfire(s) – shotfirer must inspect and clear before any entry or LHD operation.", misfireCnt),
			)
		}
		if looseCnt > 0 {
			recs = append(recs,
				fmt.Sprintf("CAUTION: %d loose-rock hazard(s) detected – barring-down required before personnel entry.", looseCnt),
			)
		}
		if s.decision.HazardStatus.OverallRisk == "critical" {
			recs = append(recs, "Overall hazard risk is CRITICAL – gate will remain closed until hazards are cleared.")
		}
	}

	switch s.decision.GatingStatus {
	case "closed":
		recs = append(recs, "Gate CLOSED – personnel entry prohibited. All hazards must be resolved before gate can open.")
	case "conditional":
		recs = append(recs,
			"Gate CONDITIONAL – limited access for authorised personnel with appropriate PPE only.",
			"Continuous monitoring required during conditional access period.",
		)
	case "open":
		if s.decision.Safe {
			recs = append(recs, "Gate OPEN – environment assessed as safe for normal personnel access.")
		} else {
			recs = append(recs, "Gate OPEN (manual override) – ensure all personnel are aware of prevailing conditions.")
		}
	}

	if !s.decision.VentilationOK {
		recs = append(recs,
			"Trigger POST /ventilation/check for a detailed ventilation assessment.",
			"Coordinate with ventilation engineer to increase airflow to affected zone.",
		)
	}

	if len(recs) == 0 {
		recs = []string{"All conditions nominal – environment is safe for access."}
	}
	return recs
}

// ---- Component fetch helpers -------------------------------------------------------

func (s *CDTbService) fetchGas() (*common.GasMonitorResult, *common.SourceQoS) {
	var resp struct {
		GasMonitor common.GasMonitorResult `json:"gasMonitor"`
		QoS        *common.SourceQoS       `json:"qos"`
	}
	if err := common.DoRequest("GET", s.comps.CDT2+"/state", "", s.id, nil, &resp); err != nil {
		log.Printf("[%s] fetch cDT2 gas state: %v", s.id, err)
		return nil, nil
	}
	return &resp.GasMonitor, resp.QoS
}

func (s *CDTbService) fetchHazards() *common.HazardReport {
	var resp struct {
		HazardReport common.HazardReport `json:"hazardReport"`
	}
	if err := common.DoRequest("GET", s.comps.CDT3+"/state", "", s.id, nil, &resp); err != nil {
		log.Printf("[%s] fetch cDT3 hazard state: %v", s.id, err)
		return nil
	}
	return &resp.HazardReport
}

// ---- HTTP handlers ----------------------------------------------------------------

func (s *CDTbService) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	safe := s.decision.Safe
	gating := s.decision.GatingStatus
	s.mu.RUnlock()
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "ok",
		"id":           s.id,
		"layer":        "upper",
		"safe":         safe,
		"gatingStatus": gating,
		"timestamp":    time.Now(),
	})
}

func (s *CDTbService) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	d := s.decision
	gasQoS := s.gasQoS
	s.mu.RUnlock()
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"safe":            d.Safe,
		"reason":          d.Reason,
		"gasStatus":       d.GasStatus,
		"hazardStatus":    d.HazardStatus,
		"ventilationOk":   d.VentilationOK,
		"gatingStatus":    d.GatingStatus,
		"recommendations": d.Recommendations,
		"gasQoS":          gasQoS,
		"lastUpdated":     d.LastUpdated,
	})
}

func (s *CDTbService) handleGas(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	gas := s.decision.GasStatus
	ventOK := s.decision.VentilationOK
	s.mu.RUnlock()

	if gas == nil {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"available":     false,
			"ventilationOK": false,
			"timestamp":     time.Now(),
		})
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"available":      true,
		"ventilationOK":  ventOK,
		"gasMonitor":     gas,
		"thresholds": map[string]interface{}{
			"ch4MaxPct": ventCH4Max,
			"coMaxPpm":  ventCOMax,
			"o2MinPct":  ventO2Min,
			"o2MaxPct":  ventO2Max,
		},
		"timestamp": time.Now(),
	})
}

func (s *CDTbService) handleHazards(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	hr := s.decision.HazardStatus
	s.mu.RUnlock()

	if hr == nil {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"timestamp": time.Now(),
		})
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"available":    true,
		"hazardReport": hr,
		"timestamp":    time.Now(),
	})
}

func (s *CDTbService) handleGating(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	gating := s.decision.GatingStatus
	reason := s.decision.Reason
	safe := s.decision.Safe
	ventOK := s.decision.VentilationOK
	s.mu.RUnlock()

	// Determine conditions that must be met before opening
	var conditions []string
	if !ventOK {
		conditions = append(conditions, "Ventilation check must pass (CH4<0.5%, CO<10ppm, O2 19.5-23%)")
	}
	if s.decision.HazardStatus != nil {
		for _, h := range s.decision.HazardStatus.Hazards {
			if !h.Cleared && (h.Severity == "critical" || h.Severity == "high") {
				conditions = append(conditions, fmt.Sprintf("Clear hazard %q (%s) at zone (%.0f,%.0f)", h.Type, h.Severity, h.Position.X, h.Position.Y))
			}
		}
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":     gating,
		"reason":     reason,
		"canOpen":    safe,
		"conditions": conditions,
		"timestamp":  time.Now(),
	})
}

func (s *CDTbService) handleGateOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Force bool `json:"force"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.decision.GatingStatus == "closed" && !body.Force {
		common.WriteError(w, http.StatusForbidden,
			fmt.Sprintf("gate cannot be opened – current status is %q: %s. Use {\"force\":true} to override.", s.decision.GatingStatus, s.decision.Reason))
		return
	}

	s.gateManualOpen = true
	s.decision.GatingStatus = "open"
	s.decision.LastUpdated = time.Now()

	forced := ""
	if body.Force {
		forced = " (forced override)"
	}
	log.Printf("[%s] Gate opened manually%s.", s.id, forced)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "open",
		"forced":    body.Force,
		"timestamp": time.Now(),
		"warning":   "Manual gate open in effect – automatic assessment will resume on next poll cycle.",
	})
}

func (s *CDTbService) handleGateClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	s.mu.Lock()
	s.gateManualOpen = false
	s.decision.GatingStatus = "closed"
	s.decision.LastUpdated = time.Now()
	s.mu.Unlock()

	log.Printf("[%s] Gate closed manually.", s.id)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "closed",
		"timestamp": time.Now(),
	})
}

func (s *CDTbService) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	recs := s.decision.Recommendations
	safe := s.decision.Safe
	gating := s.decision.GatingStatus
	s.mu.RUnlock()
	if recs == nil {
		recs = []string{}
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"safe":            safe,
		"gatingStatus":    gating,
		"recommendations": recs,
		"count":           len(recs),
		"timestamp":       time.Now(),
	})
}

func (s *CDTbService) handleConnectivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		common.WriteError(w, http.StatusMethodNotAllowed, "PUT required")
		return
	}
	var body struct {
		Connected bool `json:"connected"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	log.Printf("[%s] Connectivity update received: connected=%v", s.id, body.Connected)
	common.WriteJSON(w, http.StatusOK, map[string]bool{"connected": body.Connected})
}

func (s *CDTbService) handleVentilationCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	log.Printf("[%s] Ventilation check triggered.", s.id)

	// Fetch fresh gas data
	gas, _ := s.fetchGas()

	s.mu.Lock()
	s.decision.GasStatus = gas
	s.mu.Unlock()

	var assessment map[string]interface{}
	if gas == nil {
		assessment = map[string]interface{}{
			"available": false,
			"result":    "FAIL",
			"reason":    "Gas monitoring data unavailable – cannot assess ventilation.",
		}
	} else {
		gl := gas.AverageLevels
		pass := gl.CH4 < ventCH4Max && gl.CO < ventCOMax && gl.O2 >= ventO2Min && gl.O2 <= ventO2Max
		result := "PASS"
		reason := "All gas concentrations within ventilation thresholds."
		if !pass {
			result = "FAIL"
			switch {
			case gl.CH4 >= ventCH4Max:
				reason = fmt.Sprintf("CH4=%.3f%% exceeds ventilation limit %.1f%%", gl.CH4, ventCH4Max)
			case gl.CO >= ventCOMax:
				reason = fmt.Sprintf("CO=%.1fppm exceeds limit %.0fppm", gl.CO, ventCOMax)
			case gl.O2 < ventO2Min:
				reason = fmt.Sprintf("O2=%.1f%% below minimum %.1f%%", gl.O2, ventO2Min)
			case gl.O2 > ventO2Max:
				reason = fmt.Sprintf("O2=%.1f%% exceeds maximum %.1f%%", gl.O2, ventO2Max)
			}
		}
		assessment = map[string]interface{}{
			"available":  true,
			"result":     result,
			"pass":       pass,
			"reason":     reason,
			"gasLevels":  gl,
			"thresholds": map[string]interface{}{"ch4MaxPct": ventCH4Max, "coMaxPpm": ventCOMax, "o2MinPct": ventO2Min, "o2MaxPct": ventO2Max},
		}
		log.Printf("[%s] Ventilation check result: %s – %s", s.id, result, reason)
	}

	assessment["timestamp"] = time.Now()
	common.WriteJSON(w, http.StatusOK, assessment)
}

// ---- Helpers -----------------------------------------------------------------------

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
