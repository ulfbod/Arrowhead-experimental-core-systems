package main

// Scenario Runner: Post-Blast Inspection and Recovery
// This service orchestrates the full demo scenario by directly calling all iDT
// and cDT services in a defined sequence. It does not use Arrowhead orchestration
// for its own calls – it reaches services by their configured URLs directly and
// identifies itself via X-Consumer-ID headers.

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

// ---- Service URL registry ---------------------------------------------------------

type serviceURLs struct {
	IDT1a string
	IDT1b string
	IDT2a string
	IDT2b string
	IDT3a string
	IDT3b string
	IDT4  string
	CDT1  string
	CDT2  string
	CDT3  string
	CDT4  string
	CDT5  string
	CDTa  string
	CDTb  string
}

// ---- Scenario state ---------------------------------------------------------------

type ScenarioPhase string

const (
	ScenarioIdle      ScenarioPhase = "idle"
	ScenarioRunning   ScenarioPhase = "running"
	ScenarioCompleted ScenarioPhase = "completed"
	ScenarioFailed    ScenarioPhase = "failed"
)

type ScenarioState struct {
	Phase       ScenarioPhase `json:"phase"`
	StartedAt   *time.Time    `json:"startedAt,omitempty"`
	CompletedAt *time.Time    `json:"completedAt,omitempty"`
	Progress    []string      `json:"progress"`
	LastUpdated time.Time     `json:"lastUpdated"`
}

// ---- Service ----------------------------------------------------------------------

type ScenarioService struct {
	mu    sync.RWMutex
	state ScenarioState
	urls  serviceURLs
	id    string
	ah    *common.ArrowheadClient
}

func main() {
	id := envOrDefault("SCENARIO_ID", "scenario")
	name := envOrDefault("SCENARIO_NAME", "Post-Blast Scenario Runner")
	port := envOrDefault("PORT", "8700")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")

	urls := serviceURLs{
		IDT1a: envOrDefault("IDT1A_URL", "http://localhost:8101"),
		IDT1b: envOrDefault("IDT1B_URL", "http://localhost:8102"),
		IDT2a: envOrDefault("IDT2A_URL", "http://localhost:8201"),
		IDT2b: envOrDefault("IDT2B_URL", "http://localhost:8202"),
		IDT3a: envOrDefault("IDT3A_URL", "http://localhost:8301"),
		IDT3b: envOrDefault("IDT3B_URL", "http://localhost:8302"),
		IDT4:  envOrDefault("IDT4_URL", "http://localhost:8401"),
		CDT1:  envOrDefault("CDT1_URL", "http://localhost:8501"),
		CDT2:  envOrDefault("CDT2_URL", "http://localhost:8502"),
		CDT3:  envOrDefault("CDT3_URL", "http://localhost:8503"),
		CDT4:  envOrDefault("CDT4_URL", "http://localhost:8504"),
		CDT5:  envOrDefault("CDT5_URL", "http://localhost:8505"),
		CDTa:  envOrDefault("CDTA_URL", "http://localhost:8601"),
		CDTb:  envOrDefault("CDTB_URL", "http://localhost:8602"),
	}

	log.Printf("[%s] Starting %s on :%s", id, name, port)

	now := time.Now()
	svc := &ScenarioService{
		id:  id,
		ah:  common.NewArrowheadClient(ahURL, id),
		urls: urls,
		state: ScenarioState{
			Phase:       ScenarioIdle,
			Progress:    []string{fmt.Sprintf("[%s] Scenario runner initialised. Ready for POST /scenario/start.", now.Format(time.RFC3339))},
			LastUpdated: now,
		},
	}

	portInt := 8700
	fmt.Sscanf(port, "%d", &portInt)

	common.RegisterWithRetry(svc.ah, common.RegisterRequest{
		ID:           id,
		Name:         name,
		Address:      host,
		Port:         portInt,
		ServiceType:  "core",
		Capabilities: []string{"scenario-orchestration", "demo-control", "post-blast-scenario"},
		Metadata:     map[string]string{"role": "scenario-runner"},
	}, 10)

	router := common.NewRouter()
	router.Handle("/health", svc.handleHealth)
	router.Handle("/state", svc.handleState)
	router.Handle("/scenario/start", svc.handleScenarioStart)
	router.Handle("/scenario/reset", svc.handleScenarioReset)
	router.Handle("/scenario/log", svc.handleScenarioLog)
	router.Handle("/scenario/inject-hazard", svc.handleInjectHazard)
	router.Handle("/scenario/gas-spike", svc.handleGasSpike)
	router.Handle("/scenario/clear-all", svc.handleClearAll)

	log.Printf("[%s] Listening on :%s", id, port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

// ---- Scenario execution -----------------------------------------------------------

// runPostBlastScenario executes the full post-blast scenario in a goroutine.
func (s *ScenarioService) runPostBlastScenario() {
	s.logProgress("=== POST-BLAST INSPECTION & RECOVERY SCENARIO STARTED ===")
	s.logProgress("Step 1: Resetting all service state to initial conditions...")

	// Step 1a: Reset LHD debris counters
	s.logProgress("  Resetting LHD iDT3a debris state...")
	if err := s.post(s.urls.IDT3a+"/simulate/reset", nil); err != nil {
		s.logProgress(fmt.Sprintf("  WARNING: Could not reset iDT3a: %v", err))
	} else {
		s.logProgress("  iDT3a: debris reset to 0%.")
	}

	s.logProgress("  Resetting LHD iDT3b debris state...")
	if err := s.post(s.urls.IDT3b+"/simulate/reset", nil); err != nil {
		s.logProgress(fmt.Sprintf("  WARNING: Could not reset iDT3b: %v", err))
	} else {
		s.logProgress("  iDT3b: debris reset to 0%.")
	}

	// Step 1b: Clear any existing hazards on robots
	s.logProgress("  Clearing existing hazards on iDT1a...")
	if err := s.post(s.urls.IDT1a+"/hazard/clear", map[string]string{"id": ""}); err != nil {
		s.logProgress(fmt.Sprintf("  WARNING: Could not clear iDT1a hazards: %v", err))
	} else {
		s.logProgress("  iDT1a: all hazards cleared.")
	}

	s.logProgress("  Clearing existing hazards on iDT1b...")
	if err := s.post(s.urls.IDT1b+"/hazard/clear", map[string]string{"id": ""}); err != nil {
		s.logProgress(fmt.Sprintf("  WARNING: Could not clear iDT1b hazards: %v", err))
	} else {
		s.logProgress("  iDT1b: all hazards cleared.")
	}

	// Step 1c: Normalise gas sensors
	s.logProgress("  Normalising gas levels on iDT2a and iDT2b...")
	normalGas := map[string]float64{"ch4": 0.1, "co": 5.0, "co2": 0.04, "o2": 20.9, "no2": 0.5}
	if err := s.post(s.urls.IDT2a+"/simulate/gas", normalGas); err != nil {
		s.logProgress(fmt.Sprintf("  WARNING: Could not normalise iDT2a gas: %v", err))
	} else {
		s.logProgress("  iDT2a: gas normalised to baseline levels.")
	}
	if err := s.post(s.urls.IDT2b+"/simulate/gas", normalGas); err != nil {
		s.logProgress(fmt.Sprintf("  WARNING: Could not normalise iDT2b gas: %v", err))
	} else {
		s.logProgress("  iDT2b: gas normalised to baseline levels.")
	}

	// Step 1d: Reset cDTa mission
	s.logProgress("  Resetting cDTa mission state...")
	if err := s.post(s.urls.CDTa+"/mission/reset", nil); err != nil {
		s.logProgress(fmt.Sprintf("  WARNING: Could not reset cDTa: %v", err))
	} else {
		s.logProgress("  cDTa: mission reset to idle.")
	}

	s.logProgress("Step 1 complete: All services reset to initial conditions.")
	time.Sleep(1 * time.Second)

	// Step 2: Inject post-blast conditions
	s.logProgress("Step 2: Injecting post-blast scenario conditions...")

	// 2a: Inject hazards on iDT1a – 2 loose-rock (medium) + 1 misfire (high)
	s.logProgress("  Injecting 2x loose-rock (medium severity) hazards on iDT1a...")
	for i := 0; i < 2; i++ {
		h := map[string]string{"type": "loose-rock", "severity": "medium"}
		if err := s.post(s.urls.IDT1a+"/hazard/inject", h); err != nil {
			s.logProgress(fmt.Sprintf("  WARNING: loose-rock inject %d failed: %v", i+1, err))
		} else {
			s.logProgress(fmt.Sprintf("  iDT1a: loose-rock hazard %d injected (medium).", i+1))
		}
	}

	s.logProgress("  Injecting 1x misfire (high severity) hazard on iDT1a...")
	if err := s.post(s.urls.IDT1a+"/hazard/inject", map[string]string{"type": "misfire", "severity": "high"}); err != nil {
		s.logProgress(fmt.Sprintf("  WARNING: misfire inject failed: %v", err))
	} else {
		s.logProgress("  iDT1a: misfire hazard injected (high) – shotfirer inspection required.")
	}

	// 2b: Set gas spike on iDT2a – CH4=1.5%, CO=35ppm
	s.logProgress("  Injecting post-blast gas spike on iDT2a (CH4=1.5%, CO=35ppm)...")
	spike := map[string]float64{"ch4": 1.5, "co": 35.0, "co2": 0.8, "o2": 19.0, "no2": 2.0}
	if err := s.post(s.urls.IDT2a+"/simulate/gas", spike); err != nil {
		s.logProgress(fmt.Sprintf("  WARNING: Gas spike inject failed: %v", err))
	} else {
		s.logProgress("  iDT2a: gas spike applied – CH4=1.5% (above 1% threshold), CO=35ppm (above 25ppm threshold).")
		s.logProgress("  cDTb will detect unsafe conditions and lock gate CLOSED.")
	}

	s.logProgress("Step 2 complete: Post-blast conditions injected.")
	time.Sleep(1 * time.Second)

	// Step 3: Start cDTa mission
	s.logProgress("Step 3: Commanding cDTa to start post-blast inspection mission...")
	if err := s.post(s.urls.CDTa+"/mission/start", nil); err != nil {
		s.logProgress(fmt.Sprintf("FAILED: Could not start cDTa mission: %v", err))
		s.markFailed()
		return
	}
	s.logProgress("  cDTa mission started – phase transition: idle -> exploring.")

	// Step 4: Wait for cDTa to reach exploring phase
	s.logProgress("Step 4: Waiting for cDTa to confirm 'exploring' phase...")
	exploring := false
	for attempt := 0; attempt < 12; attempt++ {
		time.Sleep(2 * time.Second)
		var mStatus common.MissionStatus
		if err := common.DoRequest("GET", s.urls.CDTa+"/state", "", s.id, nil, &mStatus); err == nil {
			if mStatus.Phase == common.PhaseExploring {
				exploring = true
				s.logProgress(fmt.Sprintf("  cDTa confirmed in 'exploring' phase after %d seconds.", (attempt+1)*2))
				break
			}
			s.logProgress(fmt.Sprintf("  cDTa phase: %q (waiting for 'exploring')...", mStatus.Phase))
		}
	}
	if !exploring {
		s.logProgress("  WARNING: cDTa did not confirm 'exploring' phase within timeout – continuing anyway.")
	}

	// Step 5: Start SLAM on robots via cDT1
	s.logProgress("Step 5: Activating SLAM mapping on inspection robots via cDT1...")
	if err := s.post(s.urls.CDT1+"/slam/start", nil); err != nil {
		s.logProgress(fmt.Sprintf("  WARNING: Could not start SLAM via cDT1: %v", err))
		// Also try directly on the robots as fallback
		s.logProgress("  Attempting direct SLAM start on iDT1a and iDT1b as fallback...")
		if err2 := s.post(s.urls.IDT1a+"/slam/start", nil); err2 != nil {
			s.logProgress(fmt.Sprintf("  WARNING: iDT1a SLAM start failed: %v", err2))
		} else {
			s.logProgress("  iDT1a: SLAM activated.")
		}
		if err2 := s.post(s.urls.IDT1b+"/slam/start", nil); err2 != nil {
			s.logProgress(fmt.Sprintf("  WARNING: iDT1b SLAM start failed: %v", err2))
		} else {
			s.logProgress("  iDT1b: SLAM activated.")
		}
	} else {
		s.logProgress("  SLAM activated on all inspection robots via cDT1.")
	}

	s.logProgress("Step 5 complete: Robots are now mapping the blast zone.")

	// Step 6: Periodic progress logging every 10s
	s.logProgress("Step 6: Monitoring scenario progress (logging every 10s)...")
	s.logProgress("  Watch cDTa /state for automatic phase transitions.")
	s.logProgress("  Watch cDTb /state for safe-access gate decisions.")
	s.logProgress("  Expected sequence: exploring -> hazard_scan -> clearance -> verifying -> complete")

	for i := 1; i <= 6; i++ {
		time.Sleep(10 * time.Second)
		s.logScenarioProgress(i)
	}

	s.logProgress("=== POST-BLAST SCENARIO EXECUTION COMPLETE ===")
	s.logProgress("The mission will continue automatically in the background.")
	s.logProgress("Monitor /state endpoints on cDTa and cDTb for live status.")

	s.mu.Lock()
	now := time.Now()
	s.state.Phase = ScenarioCompleted
	s.state.CompletedAt = &now
	s.state.LastUpdated = now
	s.mu.Unlock()
}

// logScenarioProgress fetches and logs current state from key services.
func (s *ScenarioService) logScenarioProgress(tick int) {
	s.logProgress(fmt.Sprintf("--- Progress check #%d ---", tick))

	// cDTa mission status
	var mStatus common.MissionStatus
	if err := common.DoRequest("GET", s.urls.CDTa+"/state", "", s.id, nil, &mStatus); err == nil {
		coverage := 0.0
		if mStatus.Mapping != nil {
			coverage = mStatus.Mapping.CoveragePct
		}
		cleared := 0.0
		if mStatus.Clearance != nil {
			cleared = mStatus.Clearance.TotalDebrisPct
		}
		s.logProgress(fmt.Sprintf("  cDTa mission phase: %q | map coverage: %.1f%% | debris cleared: %.1f%%",
			mStatus.Phase, coverage, cleared))
	} else {
		s.logProgress(fmt.Sprintf("  cDTa: unreachable (%v)", err))
	}

	// cDTb safe-access decision
	var access common.SafeAccessDecision
	if err := common.DoRequest("GET", s.urls.CDTb+"/state", "", s.id, nil, &access); err == nil {
		s.logProgress(fmt.Sprintf("  cDTb safe-access: safe=%v gate=%q | %s",
			access.Safe, access.GatingStatus, access.Reason))
	} else {
		s.logProgress(fmt.Sprintf("  cDTb: unreachable (%v)", err))
	}
}

// logProgress appends a timestamped message to the progress log.
func (s *ScenarioService) logProgress(msg string) {
	entry := fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), msg)
	log.Printf("[%s] %s", s.id, msg)
	s.mu.Lock()
	s.state.Progress = append(s.state.Progress, entry)
	s.state.LastUpdated = time.Now()
	s.mu.Unlock()
}

func (s *ScenarioService) markFailed() {
	s.mu.Lock()
	s.state.Phase = ScenarioFailed
	s.state.LastUpdated = time.Now()
	s.mu.Unlock()
}

// post is a convenience wrapper for POST calls to scenario-managed services.
func (s *ScenarioService) post(url string, body interface{}) error {
	return common.DoRequest("POST", url, "", s.id, body, nil)
}

// ---- HTTP Handlers ----------------------------------------------------------------

func (s *ScenarioService) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	phase := s.state.Phase
	s.mu.RUnlock()
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"id":        s.id,
		"phase":     phase,
		"timestamp": time.Now(),
	})
}

func (s *ScenarioService) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()
	common.WriteJSON(w, http.StatusOK, state)
}

func (s *ScenarioService) handleScenarioStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	s.mu.Lock()
	if s.state.Phase == ScenarioRunning {
		s.mu.Unlock()
		common.WriteError(w, http.StatusConflict, "scenario already running – reset first")
		return
	}
	now := time.Now()
	s.state.Phase = ScenarioRunning
	s.state.StartedAt = &now
	s.state.CompletedAt = nil
	s.state.Progress = []string{fmt.Sprintf("[%s] Scenario start requested by operator.", now.Format(time.RFC3339))}
	s.state.LastUpdated = now
	s.mu.Unlock()

	go s.runPostBlastScenario()

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "started",
		"phase":     ScenarioRunning,
		"startedAt": now,
		"message":   "Post-blast scenario is running. Monitor GET /state or GET /scenario/log for progress.",
	})
}

func (s *ScenarioService) handleScenarioReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	now := time.Now()
	s.mu.Lock()
	s.state = ScenarioState{
		Phase:       ScenarioIdle,
		Progress:    []string{fmt.Sprintf("[%s] Scenario reset to idle by operator.", now.Format(time.RFC3339))},
		LastUpdated: now,
	}
	s.mu.Unlock()

	log.Printf("[%s] Scenario reset to idle.", s.id)
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "reset", "phase": string(ScenarioIdle)})
}

func (s *ScenarioService) handleScenarioLog(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	progress := s.state.Progress
	phase := s.state.Phase
	s.mu.RUnlock()
	if progress == nil {
		progress = []string{}
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"phase":   phase,
		"log":     progress,
		"count":   len(progress),
		"timestamp": time.Now(),
	})
}

func (s *ScenarioService) handleInjectHazard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		RobotID  string `json:"robotId"`
		Type     string `json:"type"`
		Severity string `json:"severity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Map robotId to URL
	robotURLs := map[string]string{
		"idt1a": s.urls.IDT1a,
		"idt1b": s.urls.IDT1b,
	}
	robotURL, ok := robotURLs[body.RobotID]
	if !ok {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown robotId %q – valid: idt1a, idt1b", body.RobotID))
		return
	}
	if body.Type == "" {
		body.Type = "loose-rock"
	}
	if body.Severity == "" {
		body.Severity = "medium"
	}

	payload := map[string]string{"type": body.Type, "severity": body.Severity}
	if err := s.post(robotURL+"/hazard/inject", payload); err != nil {
		common.WriteError(w, http.StatusBadGateway, fmt.Sprintf("inject failed: %v", err))
		return
	}

	msg := fmt.Sprintf("Hazard injected on %s: type=%q severity=%q", body.RobotID, body.Type, body.Severity)
	s.logProgress(msg)
	common.WriteJSON(w, http.StatusCreated, map[string]string{
		"status":   "injected",
		"robotId":  body.RobotID,
		"type":     body.Type,
		"severity": body.Severity,
	})
}

func (s *ScenarioService) handleGasSpike(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	s.logProgress("Gas spike triggered on iDT2a via /scenario/gas-spike.")

	if err := s.post(s.urls.IDT2a+"/simulate/spike", nil); err != nil {
		common.WriteError(w, http.StatusBadGateway, fmt.Sprintf("gas spike failed on iDT2a: %v", err))
		return
	}
	s.logProgress("iDT2a: dangerous gas spike applied (CH4=2.5%, CO=80ppm, O2=17%).")
	s.logProgress("cDTb will detect critical gas condition and lock gate CLOSED.")

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "spike triggered",
		"targets": []string{"idt2a"},
		"levels":  map[string]interface{}{"ch4Pct": 2.5, "coPpm": 80.0, "o2Pct": 17.0},
		"message": "Dangerous gas levels injected. Monitor cDTb /state for gating response.",
	})
}

func (s *ScenarioService) handleClearAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	s.logProgress("Clear-all command received – resetting hazards and normalising gas.")

	results := map[string]string{}

	// Clear all hazards on both robots
	for _, target := range []struct{ id, url string }{
		{"idt1a", s.urls.IDT1a},
		{"idt1b", s.urls.IDT1b},
	} {
		if err := s.post(target.url+"/hazard/clear", map[string]string{"id": ""}); err != nil {
			results[target.id] = fmt.Sprintf("hazard clear FAILED: %v", err)
			s.logProgress(fmt.Sprintf("  WARNING: %s hazard clear failed: %v", target.id, err))
		} else {
			results[target.id] = "hazards cleared"
			s.logProgress(fmt.Sprintf("  %s: all hazards cleared.", target.id))
		}
	}

	// Normalise gas on both sensors
	normalGas := map[string]float64{"ch4": 0.1, "co": 5.0, "co2": 0.04, "o2": 20.9, "no2": 0.5}
	for _, target := range []struct{ id, url string }{
		{"idt2a", s.urls.IDT2a},
		{"idt2b", s.urls.IDT2b},
	} {
		if err := s.post(target.url+"/simulate/gas", normalGas); err != nil {
			results[target.id] = fmt.Sprintf("gas normalise FAILED: %v", err)
			s.logProgress(fmt.Sprintf("  WARNING: %s gas normalise failed: %v", target.id, err))
		} else {
			results[target.id] = "gas normalised"
			s.logProgress(fmt.Sprintf("  %s: gas levels normalised to baseline.", target.id))
		}
	}

	s.logProgress("Clear-all complete. cDTb should reassess gate status within 4 seconds.")

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "clear-all complete",
		"results": results,
		"message": "All hazards cleared and gas normalised. Monitor cDTb /state for updated gate status.",
		"timestamp": time.Now(),
	})
}

// ---- Helpers -----------------------------------------------------------------------

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
