package main

// cDTa: Inspection and Recovery after Blasting
// Upper-layer composite Digital Twin that orchestrates the post-blast
// inspection and recovery mission by composing cDT1, cDT3, cDT4, and cDT5.

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

// ---- Component URLs ----------------------------------------------------------------

type componentURLs struct {
	CDT1 string // Exploration & Mapping
	CDT3 string // Hazard Detection
	CDT4 string // Material Handling / Clearance
	CDT5 string // Tele-Remote Intervention
}

// ---- Service -----------------------------------------------------------------------

type CDTaService struct {
	mu     sync.RWMutex
	status common.MissionStatus
	comps  componentURLs
	ah     *common.ArrowheadClient
	id     string

	// internal control
	missionActive bool
	phaseEnteredAt time.Time
}

func main() {
	id := envOrDefault("CDT_ID", "cdta")
	name := envOrDefault("CDT_NAME", "cDTa: Inspection & Recovery")
	port := envOrDefault("PORT", "8601")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")

	comps := componentURLs{
		CDT1: envOrDefault("CDT1_URL", "http://localhost:8501"),
		CDT3: envOrDefault("CDT3_URL", "http://localhost:8503"),
		CDT4: envOrDefault("CDT4_URL", "http://localhost:8504"),
		CDT5: envOrDefault("CDT5_URL", "http://localhost:8505"),
	}

	log.Printf("[%s] Starting %s on :%s", id, name, port)
	log.Printf("[%s] Composing: cDT1=%s cDT3=%s cDT4=%s cDT5=%s",
		id, comps.CDT1, comps.CDT3, comps.CDT4, comps.CDT5)

	now := time.Now()
	svc := &CDTaService{
		id:    id,
		comps: comps,
		ah:    common.NewArrowheadClient(ahURL, id),
		status: common.MissionStatus{
			Phase:           common.PhaseIdle,
			Recommendations: []string{"System ready. Issue POST /mission/start to begin post-blast inspection mission."},
			Log:             []string{fmt.Sprintf("[%s] cDTa initialised – awaiting mission start command.", now.Format(time.RFC3339))},
			LastUpdated:     now,
		},
	}

	portInt := 8601
	fmt.Sscanf(port, "%d", &portInt)

	common.RegisterWithRetry(svc.ah, common.RegisterRequest{
		ID:           id,
		Name:         name,
		Address:      host,
		Port:         portInt,
		ServiceType:  "cDT",
		Capabilities: []string{"inspection-recovery", "post-blast-mission", "mission-orchestration"},
		Metadata: map[string]string{
			"layer":    "upper",
			"composes": "cdt1,cdt3,cdt4,cdt5",
		},
	}, 10)

	// Background polling loop
	go svc.pollLoop()

	router := common.NewRouter()
	router.Handle("/health", svc.handleHealth)
	router.Handle("/state", svc.handleState)
	router.Handle("/mission/start", svc.handleMissionStart)
	router.Handle("/mission/abort", svc.handleMissionAbort)
	router.Handle("/mission/reset", svc.handleMissionReset)
	router.Handle("/components", svc.handleComponents)
	router.Handle("/force/phase", svc.handleForcePhase)
	router.Handle("/recommendations", svc.handleRecommendations)
	router.Handle("/connectivity", svc.handleConnectivity)

	log.Printf("[%s] Listening on :%s", id, port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

// ---- Poll loop --------------------------------------------------------------------

func (s *CDTaService) pollLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		if s.missionActive {
			s.mu.Unlock()
			s.evaluateMission()
		} else {
			s.mu.Unlock()
		}
	}
}

// evaluateMission fetches component states and drives the phase state machine.
func (s *CDTaService) evaluateMission() {
	// Fetch component data (outside the lock so HTTP calls don't stall readers).
	mapping := s.fetchMapping()
	hazardReport := s.fetchHazards()
	clearance := s.fetchClearance()
	intervention := s.fetchIntervention()

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.missionActive {
		return
	}

	// Update sub-results
	s.status.Mapping = mapping
	s.status.Hazards = hazardReport
	s.status.Clearance = clearance
	s.status.Intervention = intervention
	s.status.LastUpdated = time.Now()

	currentPhase := s.status.Phase
	timeInPhase := time.Since(s.phaseEnteredAt)

	switch currentPhase {
	case common.PhaseExploring:
		// Advance when mapping progress exceeds 30%, or critical hazards are detected, or after 90s
		criticalDetected := hazardReport != nil && (hazardReport.OverallRisk == "critical" || hazardReport.OverallRisk == "high")
		coverageReached := mapping != nil && mapping.CoveragePct > 30
		timeout := timeInPhase > 90*time.Second
		if coverageReached || criticalDetected || timeout {
			reason := fmt.Sprintf("Mapping coverage %.1f%%", func() float64 {
				if mapping != nil {
					return mapping.CoveragePct
				}
				return 0
			}())
			if criticalDetected {
				reason += fmt.Sprintf(" – %s hazards detected, triggering hazard scan", hazardReport.OverallRisk)
			} else if timeout {
				reason += " – exploration timeout, advancing to hazard scan"
			} else {
				reason += " (threshold 30%%) – transitioning to hazard scan"
			}
			s.transitionTo(common.PhaseHazardScan, reason)
		}

	case common.PhaseHazardScan:
		// Advance when hazard report shows risk is not "critical", or after 30s timeout
		canAdvance := false
		reason := ""
		if hazardReport != nil && hazardReport.OverallRisk != "critical" {
			canAdvance = true
			reason = fmt.Sprintf("Hazard scan complete – overall risk level: %q. Proceeding to clearance.", hazardReport.OverallRisk)
		} else if timeInPhase > 30*time.Second {
			canAdvance = true
			reason = "Hazard scan timeout (30s) – proceeding to clearance phase regardless of risk level."
		}
		if canAdvance {
			s.transitionTo(common.PhaseClearance, reason)
		}

	case common.PhaseClearance:
		// Advance when combined debris cleared exceeds 80%, all hazards cleared, or after 60s timeout
		debrisDone := clearance != nil && clearance.TotalDebrisPct > 80
		hazardsDone := hazardReport != nil && hazardReport.SafeForEntry
		clearanceTimeout := timeInPhase > 60*time.Second
		if debrisDone {
			s.transitionTo(common.PhaseVerifying,
				fmt.Sprintf("Debris clearance at %.1f%% (threshold 80%%) – entering verification phase.", clearance.TotalDebrisPct))
		} else if hazardsDone && clearanceTimeout {
			s.transitionTo(common.PhaseVerifying,
				"Clearance timeout (60s) – all hazards resolved and site assessed safe. Entering verification phase.")
		} else if clearanceTimeout {
			s.transitionTo(common.PhaseVerifying,
				"Clearance timeout (60s) – proceeding to verification phase.")
		}

	case common.PhaseVerifying:
		// Advance when no active hazards and route is clear, or after 30s timeout
		noActiveHazards := true
		if hazardReport != nil {
			for _, h := range hazardReport.Hazards {
				if !h.Cleared {
					noActiveHazards = false
					break
				}
			}
		}
		routeClear := clearance != nil && clearance.RouteClear
		verifyTimeout := timeInPhase > 30*time.Second
		if (noActiveHazards && routeClear) || verifyTimeout {
			reason := "Verification complete – no active hazards detected and haul route confirmed clear. Mission success."
			if verifyTimeout && !(noActiveHazards && routeClear) {
				reason = "Verification timeout (30s) – final sweep complete. Mission concluded."
			}
			s.transitionTo(common.PhaseComplete, reason)
			now := time.Now()
			s.status.CompletedAt = &now
			s.missionActive = false
		}
	}

	// Regenerate recommendations every poll cycle
	s.status.Recommendations = s.generateRecommendations()
}

// transitionTo moves to a new phase, emits a log entry, and records the phase entry time.
// Caller must hold s.mu (write lock).
func (s *CDTaService) transitionTo(phase common.MissionPhase, reason string) {
	from := s.status.Phase
	s.status.Phase = phase
	s.phaseEnteredAt = time.Now()
	entry := fmt.Sprintf("[%s] PHASE TRANSITION: %s -> %s | %s",
		time.Now().Format(time.RFC3339), from, phase, reason)
	s.status.Log = append(s.status.Log, entry)
	log.Printf("[%s] %s", s.id, entry)
}

// generateRecommendations produces contextual recommendations for the current phase.
// Caller must hold s.mu (at least read lock).
func (s *CDTaService) generateRecommendations() []string {
	var recs []string
	switch s.status.Phase {
	case common.PhaseIdle:
		recs = append(recs, "System ready. Issue POST /mission/start to begin post-blast inspection mission.")

	case common.PhaseExploring:
		coverage := 0.0
		if s.status.Mapping != nil {
			coverage = s.status.Mapping.CoveragePct
		}
		recs = append(recs,
			fmt.Sprintf("Exploration in progress – current map coverage: %.1f%%. Mission will advance at 70%%.", coverage),
			"Ensure inspection robots (iDT1a, iDT1b) have SLAM active and sufficient battery.",
			"Maintain communication links to all iDTs during tunnel exploration.",
		)

	case common.PhaseHazardScan:
		recs = append(recs,
			"Hazard scan active – do not permit personnel entry until scan is complete.",
			"cDT3 is correlating robot data and gas readings to identify risk zones.",
		)
		if s.status.Hazards != nil {
			looseCnt, misfireCnt := 0, 0
			var highRiskZones []string
			for _, h := range s.status.Hazards.Hazards {
				if !h.Cleared {
					switch h.Type {
					case "loose-rock":
						looseCnt++
					case "misfire":
						misfireCnt++
					}
					if h.Severity == "high" || h.Severity == "critical" {
						highRiskZones = append(highRiskZones, fmt.Sprintf("zone (%.0f,%.0f)", h.Position.X, h.Position.Y))
					}
				}
			}
			if looseCnt > 0 {
				recs = append(recs, fmt.Sprintf("Hazard scan detected %d loose-rock hazard(s) – recommend LHD clearance of fall zones before personnel entry.", looseCnt))
			}
			if misfireCnt > 0 {
				recs = append(recs, fmt.Sprintf("WARNING: %d misfire hazard(s) detected – shotfirer inspection required before any LHD operation.", misfireCnt))
			}
			if len(highRiskZones) > 0 {
				recs = append(recs, fmt.Sprintf("High-risk areas identified at: %v – keep all personnel clear.", highRiskZones))
			}
			if s.status.Hazards.OverallRisk == "critical" {
				recs = append(recs, "CRITICAL risk level – mission will hold in hazard_scan until risk degrades or timeout. Consider manual intervention via cDT5.")
			}
		}

	case common.PhaseClearance:
		cleared := 0.0
		if s.status.Clearance != nil {
			cleared = s.status.Clearance.TotalDebrisPct
		}
		recs = append(recs,
			fmt.Sprintf("Debris clearance underway – %.1f%% cleared (target 80%%).", cleared),
			"LHD vehicles (iDT3a, iDT3b) are tramming debris to the tip. Monitor fuel levels.",
			"Ensure haul route is kept clear of personnel during active tramming cycles.",
		)
		if s.status.Clearance != nil && s.status.Clearance.EstimatedETA > 0 {
			recs = append(recs, fmt.Sprintf("Estimated clearance completion: %d minutes.", s.status.Clearance.EstimatedETA))
		}
		if s.status.Hazards != nil {
			for _, h := range s.status.Hazards.Hazards {
				if !h.Cleared && h.Type == "misfire" {
					recs = append(recs, "CAUTION: Uncleared misfire hazard in zone – LHD approach restricted. Shotfirer sign-off required.")
					break
				}
			}
		}

	case common.PhaseVerifying:
		recs = append(recs,
			"Verification phase – robots conducting final sweep to confirm route clearance.",
			"Gas sensors (iDT2a, iDT2b) confirming atmosphere via cDTb before re-entry authorisation.",
			"Do not issue re-entry permit until cDTb confirms gating status 'open'.",
		)
		if s.status.Clearance != nil && !s.status.Clearance.RouteClear {
			recs = append(recs, "Haul route not yet confirmed clear – awaiting final LHD pass.")
		}

	case common.PhaseComplete:
		recs = append(recs,
			"Mission complete – post-blast inspection and recovery successful.",
			"Await re-entry authorisation from cDTb (safe-access decision required).",
			"Submit blasting report and update geotechnical records before next cycle.",
			"Schedule robot battery charging before next mission.",
		)

	case common.PhaseFailed:
		recs = append(recs,
			"Mission failed – review event log for root cause.",
			"Initiate manual inspection protocol and notify mine manager.",
			"Issue POST /mission/reset to clear state after manual review.",
		)
	}
	return recs
}

// ---- Component fetch helpers -------------------------------------------------------

func (s *CDTaService) fetchMapping() *common.MappingResult {
	var resp struct {
		Mapping common.MappingResult `json:"mapping"`
	}
	if err := common.DoRequest("GET", s.comps.CDT1+"/state", "", s.id, nil, &resp); err != nil {
		log.Printf("[%s] fetch cDT1 mapping state: %v", s.id, err)
		return nil
	}
	return &resp.Mapping
}

func (s *CDTaService) fetchHazards() *common.HazardReport {
	var resp struct {
		HazardReport common.HazardReport `json:"hazardReport"`
	}
	if err := common.DoRequest("GET", s.comps.CDT3+"/state", "", s.id, nil, &resp); err != nil {
		log.Printf("[%s] fetch cDT3 hazard state: %v", s.id, err)
		return nil
	}
	return &resp.HazardReport
}

func (s *CDTaService) fetchClearance() *common.ClearanceStatus {
	var resp struct {
		Clearance common.ClearanceStatus `json:"clearance"`
	}
	if err := common.DoRequest("GET", s.comps.CDT4+"/state", "", s.id, nil, &resp); err != nil {
		log.Printf("[%s] fetch cDT4 clearance state: %v", s.id, err)
		return nil
	}
	return &resp.Clearance
}

func (s *CDTaService) fetchIntervention() *common.InterventionStatus {
	var resp struct {
		Intervention common.InterventionStatus `json:"intervention"`
	}
	if err := common.DoRequest("GET", s.comps.CDT5+"/state", "", s.id, nil, &resp); err != nil {
		log.Printf("[%s] fetch cDT5 intervention state: %v", s.id, err)
		return nil
	}
	return &resp.Intervention
}

// ---- HTTP Handlers ----------------------------------------------------------------

func (s *CDTaService) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	phase := s.status.Phase
	s.mu.RUnlock()
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"id":        s.id,
		"layer":     "upper",
		"phase":     phase,
		"timestamp": time.Now(),
	})
}

func (s *CDTaService) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	status := s.status
	s.mu.RUnlock()
	common.WriteJSON(w, http.StatusOK, status)
}

func (s *CDTaService) handleMissionStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status.Phase != common.PhaseIdle {
		common.WriteError(w, http.StatusConflict,
			fmt.Sprintf("mission already active in phase %q – abort or reset first", s.status.Phase))
		return
	}

	now := time.Now()
	s.missionActive = true
	s.phaseEnteredAt = now
	s.status.StartedAt = &now
	s.status.CompletedAt = nil
	s.status.Mapping = nil
	s.status.Hazards = nil
	s.status.Clearance = nil
	s.status.Intervention = nil

	entry := fmt.Sprintf("[%s] Mission started – initiating post-blast inspection and recovery sequence.", now.Format(time.RFC3339))
	s.status.Log = append(s.status.Log, entry)
	log.Printf("[%s] %s", s.id, entry)

	s.status.Phase = common.PhaseExploring
	s.phaseEnteredAt = now
	phaseEntry := fmt.Sprintf("[%s] PHASE TRANSITION: idle -> exploring | Mission start command received. Deploying inspection robots to blast zone.", now.Format(time.RFC3339))
	s.status.Log = append(s.status.Log, phaseEntry)
	log.Printf("[%s] %s", s.id, phaseEntry)

	// Activate SLAM mapping on robots via cDT1
	go func() {
		if err := common.DoRequest("POST", s.comps.CDT1+"/start", "", s.id, nil, nil); err != nil {
			log.Printf("[%s] WARNING: could not start SLAM via cDT1: %v", s.id, err)
		} else {
			log.Printf("[%s] SLAM activated on inspection robots via cDT1.", s.id)
		}
	}()

	s.status.Recommendations = s.generateRecommendations()
	s.status.LastUpdated = now

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "mission started",
		"phase":   s.status.Phase,
		"startedAt": now,
	})
}

func (s *CDTaService) handleMissionAbort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.missionActive {
		common.WriteError(w, http.StatusConflict, "no active mission to abort")
		return
	}

	now := time.Now()
	s.missionActive = false
	entry := fmt.Sprintf("[%s] MISSION ABORTED by operator command from phase %q.", now.Format(time.RFC3339), s.status.Phase)
	s.status.Log = append(s.status.Log, entry)
	log.Printf("[%s] %s", s.id, entry)

	s.status.Phase = common.PhaseFailed
	s.status.Recommendations = []string{
		"Mission aborted – initiate manual inspection protocol.",
		"Review event log and notify mine manager before resetting.",
		"Issue POST /mission/reset when ready to restart.",
	}
	s.status.LastUpdated = now

	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "aborted", "phase": string(common.PhaseFailed)})
}

func (s *CDTaService) handleMissionReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.missionActive = false
	s.status = common.MissionStatus{
		Phase:           common.PhaseIdle,
		Recommendations: []string{"System reset. Issue POST /mission/start to begin a new mission."},
		Log: []string{
			fmt.Sprintf("[%s] cDTa reset to idle state by operator.", now.Format(time.RFC3339)),
		},
		LastUpdated: now,
	}
	log.Printf("[%s] Mission reset to idle by operator.", s.id)
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "reset", "phase": string(common.PhaseIdle)})
}

func (s *CDTaService) handleComponents(w http.ResponseWriter, r *http.Request) {
	type compStatus struct {
		ID      string `json:"id"`
		URL     string `json:"url"`
		Online  bool   `json:"online"`
		Details interface{} `json:"details,omitempty"`
	}

	check := func(id, url string) compStatus {
		var result map[string]interface{}
		err := common.DoRequest("GET", url+"/health", "", s.id, nil, &result)
		return compStatus{
			ID:      id,
			URL:     url,
			Online:  err == nil,
			Details: result,
		}
	}

	components := []compStatus{
		check("cdt1", s.comps.CDT1),
		check("cdt3", s.comps.CDT3),
		check("cdt4", s.comps.CDT4),
		check("cdt5", s.comps.CDT5),
	}

	allOnline := true
	for _, c := range components {
		if !c.Online {
			allOnline = false
		}
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"components": components,
		"allOnline":  allOnline,
		"timestamp":  time.Now(),
	})
}

func (s *CDTaService) handleForcePhase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Phase string `json:"phase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Phase == "" {
		common.WriteError(w, http.StatusBadRequest, "body must be {\"phase\":\"<phase_name>\"}")
		return
	}

	validPhases := map[string]common.MissionPhase{
		"idle":        common.PhaseIdle,
		"exploring":   common.PhaseExploring,
		"hazard_scan": common.PhaseHazardScan,
		"clearance":   common.PhaseClearance,
		"verifying":   common.PhaseVerifying,
		"complete":    common.PhaseComplete,
		"failed":      common.PhaseFailed,
	}
	phase, ok := validPhases[body.Phase]
	if !ok {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown phase %q", body.Phase))
		return
	}

	s.mu.Lock()
	prev := s.status.Phase
	now := time.Now()
	s.status.Phase = phase
	s.phaseEnteredAt = now

	if phase == common.PhaseIdle || phase == common.PhaseComplete || phase == common.PhaseFailed {
		s.missionActive = false
	} else {
		s.missionActive = true
		if s.status.StartedAt == nil {
			s.status.StartedAt = &now
		}
	}

	entry := fmt.Sprintf("[%s] FORCED PHASE TRANSITION: %s -> %s | Demo/manual override by operator.", now.Format(time.RFC3339), prev, phase)
	s.status.Log = append(s.status.Log, entry)
	log.Printf("[%s] %s", s.id, entry)
	s.status.Recommendations = s.generateRecommendations()
	s.status.LastUpdated = now
	s.mu.Unlock()

	log.Printf("[%s] Phase forced to %q for demo.", s.id, phase)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "phase forced",
		"previous":  prev,
		"current":   phase,
		"timestamp": now,
	})
}

func (s *CDTaService) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	recs := s.status.Recommendations
	phase := s.status.Phase
	s.mu.RUnlock()
	if recs == nil {
		recs = []string{}
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"phase":           phase,
		"recommendations": recs,
		"count":           len(recs),
		"timestamp":       time.Now(),
	})
}

func (s *CDTaService) handleConnectivity(w http.ResponseWriter, r *http.Request) {
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

// ---- Helpers -----------------------------------------------------------------------

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
