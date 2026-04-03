package main

// Scenario Runner: Post-Blast Inspection and Recovery
// This service orchestrates the full demo scenario by directly calling all iDT
// and cDT services in a defined sequence. It does not use Arrowhead orchestration
// for its own calls – it reaches services by their configured URLs directly and
// identifies itself via X-Consumer-ID headers.
//
// QoS Experiment endpoints:
//   POST /scenario/config?mode=local|central   – set failover orchestration mode
//   POST /scenario/network-delay?ms=X          – set simulated network delay (0–50 ms)
//   POST /scenario/experiment/run              – run full failover delay experiment
//   GET  /scenario/experiment/results          – return latest experiment results as JSON

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"mineio/internal/common"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// ---- Experiment result types -------------------------------------------------------

// ExperimentRun holds the result of one (networkDelay, mode, run) triple.
type ExperimentRun struct {
	NetworkDelayMs  int     `json:"networkDelayMs"`
	Mode            string  `json:"mode"`
	Run             int     `json:"run"`
	FailoverDelayMs float64 `json:"failoverDelayMs"` // DecisionDelayMs from FailoverEvent
	FailToSwitchMs  float64 `json:"failToSwitchMs"`
	Success         bool    `json:"success"`
	Error           string  `json:"error,omitempty"`
}

// ExperimentResults aggregates all runs and computed averages.
type ExperimentResults struct {
	Runs          []ExperimentRun       `json:"runs"`
	Summary       []ExperimentSummary   `json:"summary"`
	CSVPath       string                `json:"csvPath"`
	CompletedAt   time.Time             `json:"completedAt"`
}

// ExperimentSummary holds the averaged failover delay and confidence intervals
// for one network delay value.
type ExperimentSummary struct {
	NetworkDelayMs           int     `json:"networkDelayMs"`
	AvgLocalDecisionMs       float64 `json:"avgLocalDecisionMs"`
	LocalP10Ms               float64 `json:"localP10Ms"`
	LocalP90Ms               float64 `json:"localP90Ms"`
	AvgCentralDecisionMs     float64 `json:"avgCentralDecisionMs"`
	CentralP10Ms             float64 `json:"centralP10Ms"`
	CentralP90Ms             float64 `json:"centralP90Ms"`
	AvgLocalFailToSwitchMs   float64 `json:"avgLocalFailToSwitchMs"`
	AvgCentralFailToSwitchMs float64 `json:"avgCentralFailToSwitchMs"`
	LocalRuns                int     `json:"localRuns"`
	CentralRuns              int     `json:"centralRuns"`
}

// ---- Service ----------------------------------------------------------------------

type ScenarioService struct {
	mu                sync.RWMutex
	state             ScenarioState
	urls              serviceURLs
	id                string
	ah                *common.ArrowheadClient
	logDir            string
	expMu             sync.Mutex
	expResults        *ExperimentResults
	expRunning        bool
	processingDelayMs int // guarded by s.mu
}

func main() {
	id := envOrDefault("SCENARIO_ID", "scenario")
	name := envOrDefault("SCENARIO_NAME", "Post-Blast Scenario Runner")
	port := envOrDefault("PORT", "8700")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")
	logDir := envOrDefault("LOG_DIR", "./logs")

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
		id:     id,
		ah:     common.NewArrowheadClient(ahURL, id),
		urls:   urls,
		logDir: logDir,
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
	router.Handle("/scenario/sensor-fail", svc.handleSensorFail)
	router.Handle("/scenario/sensor-degrade", svc.handleSensorDegrade)
	router.Handle("/scenario/sensor-recover", svc.handleSensorRecover)
	router.Handle("/scenario/robot-fail", svc.handleRobotFail)
	router.Handle("/scenario/robot-recover", svc.handleRobotRecover)
	// QoS experiment control
	router.Handle("/scenario/config", svc.handleConfig)
	router.Handle("/scenario/network-delay", svc.handleNetworkDelay)
	router.Handle("/scenario/processing-delay", svc.handleProcessingDelay)
	router.Handle("/scenario/mapping-speed", svc.handleMappingSpeed)
	router.Handle("/scenario/clearance-speed", svc.handleClearanceSpeed)
	router.Handle("/scenario/experiment/run", svc.handleExperimentRun)
	router.Handle("/scenario/experiment/results", svc.handleExperimentResults)

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

	var access common.SafeAccessDecision
	if err := common.DoRequest("GET", s.urls.CDTb+"/state", "", s.id, nil, &access); err == nil {
		s.logProgress(fmt.Sprintf("  cDTb safe-access: safe=%v gate=%q | %s",
			access.Safe, access.GatingStatus, access.Reason))
	} else {
		s.logProgress(fmt.Sprintf("  cDTb: unreachable (%v)", err))
	}
}

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

func (s *ScenarioService) post(url string, body interface{}) error {
	return common.DoRequest("POST", url, "", s.id, body, nil)
}

// ---- Experiment automation ---------------------------------------------------------

// runFailoverExperiment benchmarks the failover decision delay at each network latency.
//
// For each networkDelay in 0..50ms (step 5ms) and each repetition:
//   - POST cdt2/benchmark-decision: cdt2 measures local (list lookup) and central
//     (2×networkDelay sleep + Arrowhead call) decision times in isolation, free from
//     any interference by the background poll loop or stale events.
//
// Results are written to failover_delay_vs_network_delay.csv.
func (s *ScenarioService) runFailoverExperiment(runsPerPoint int) {
	log.Printf("[%s] === FAILOVER EXPERIMENT STARTED (runsPerPoint=%d) ===", s.id, runsPerPoint)

	delays := []int{0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50}

	var allRuns []ExperimentRun

	for _, netDelay := range delays {
		for run := 1; run <= runsPerPoint; run++ {
			localRun, centralRun := s.runBenchmark(netDelay, run)
			allRuns = append(allRuns, localRun, centralRun)
			log.Printf("[%s] exp delay=%dms run=%d → local=%.2fms central=%.2fms",
				s.id, netDelay, run, localRun.FailoverDelayMs, centralRun.FailoverDelayMs)
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Ensure global state is clean after experiment.
	common.SetNetworkDelayMs(0)
	common.SetOrchestrationMode("local")

	summary := computeSummary(delays, allRuns)
	csvPath, _ := s.writeAggregatedCSV(summary)

	s.expMu.Lock()
	s.expResults = &ExperimentResults{
		Runs:        allRuns,
		Summary:     summary,
		CSVPath:     csvPath,
		CompletedAt: time.Now(),
	}
	s.expRunning = false
	s.expMu.Unlock()

	log.Printf("[%s] === FAILOVER EXPERIMENT COMPLETE – CSV: %s ===", s.id, csvPath)
}

// runBenchmark calls cdt2's /benchmark-decision endpoint for one (netDelay, run) pair.
// The scenario's own HTTP delay is reset to 0 so the HTTP call to cdt2 is not delayed;
// cdt2 applies the simulated delay internally during the central decision path.
// Returns one ExperimentRun per mode (local and central).
func (s *ScenarioService) runBenchmark(netDelay int, run int) (localRun, centralRun ExperimentRun) {
	localRun = ExperimentRun{NetworkDelayMs: netDelay, Mode: "local", Run: run}
	centralRun = ExperimentRun{NetworkDelayMs: netDelay, Mode: "central", Run: run}

	// Reset scenario-side delay so the HTTP call to cdt2 is not artificially delayed.
	common.SetNetworkDelayMs(0)

	var resp struct {
		LocalDecisionMs   float64 `json:"localDecisionMs"`
		CentralDecisionMs float64 `json:"centralDecisionMs"`
	}
	s.mu.RLock()
	procDelay := s.processingDelayMs
	s.mu.RUnlock()

	err := common.DoRequest("POST", s.urls.CDT2+"/benchmark-decision", "", s.id,
		map[string]int{"networkDelayMs": netDelay, "processingDelayMs": procDelay}, &resp)
	if err != nil {
		localRun.Error = fmt.Sprintf("benchmark: %v", err)
		centralRun.Error = localRun.Error
		return
	}

	localRun.FailoverDelayMs = resp.LocalDecisionMs
	localRun.Success = true
	centralRun.FailoverDelayMs = resp.CentralDecisionMs
	centralRun.Success = true
	return
}

// percentile returns the p-th percentile (0–100) of a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := p / 100.0 * float64(len(sorted)-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// computeSummary computes mean, p10, p90 of decision delay per (delay, mode).
func computeSummary(delays []int, runs []ExperimentRun) []ExperimentSummary {
	type key struct {
		delay int
		mode  string
	}
	type accum struct {
		decisions     []float64
		sumFailSwitch float64
		count         int
	}
	acc := map[key]*accum{}

	for _, r := range runs {
		if !r.Success {
			continue
		}
		k := key{r.NetworkDelayMs, r.Mode}
		if acc[k] == nil {
			acc[k] = &accum{}
		}
		acc[k].decisions = append(acc[k].decisions, r.FailoverDelayMs)
		acc[k].sumFailSwitch += r.FailToSwitchMs
		acc[k].count++
	}

	summary := make([]ExperimentSummary, len(delays))
	for i, d := range delays {
		s := ExperimentSummary{NetworkDelayMs: d}
		if a := acc[key{d, "local"}]; a != nil && a.count > 0 {
			sort.Float64s(a.decisions)
			sum := 0.0
			for _, v := range a.decisions { sum += v }
			s.AvgLocalDecisionMs = sum / float64(a.count)
			s.LocalP10Ms = percentile(a.decisions, 10)
			s.LocalP90Ms = percentile(a.decisions, 90)
			s.AvgLocalFailToSwitchMs = a.sumFailSwitch / float64(a.count)
			s.LocalRuns = a.count
		}
		if a := acc[key{d, "central"}]; a != nil && a.count > 0 {
			sort.Float64s(a.decisions)
			sum := 0.0
			for _, v := range a.decisions { sum += v }
			s.AvgCentralDecisionMs = sum / float64(a.count)
			s.CentralP10Ms = percentile(a.decisions, 10)
			s.CentralP90Ms = percentile(a.decisions, 90)
			s.AvgCentralFailToSwitchMs = a.sumFailSwitch / float64(a.count)
			s.CentralRuns = a.count
		}
		summary[i] = s
	}
	return summary
}

// writeAggregatedCSV writes failover_delay_vs_network_delay.csv.
//
// Columns:
//
//	network_delay_ms  local_avg_ms  local_p10_ms  local_p90_ms  central_avg_ms  central_p10_ms  central_p90_ms
//
// The file is directly usable with gnuplot errorbars:
//
//	plot "file.csv" using 1:2:3:4 with yerrorbars title "Local", \
//	     ""          using 1:5:6:7 with yerrorbars title "Centralized"
func (s *ScenarioService) writeAggregatedCSV(summary []ExperimentSummary) (string, error) {
	if err := os.MkdirAll(s.logDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(s.logDir, "failover_delay_vs_network_delay.csv")
	f, err := os.Create(path)
	if err != nil {
		return path, err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	_ = w.Write([]string{
		"network_delay_ms",
		"local_avg_ms", "local_p10_ms", "local_p90_ms",
		"central_avg_ms", "central_p10_ms", "central_p90_ms",
	})
	for _, s := range summary {
		_ = w.Write([]string{
			strconv.Itoa(s.NetworkDelayMs),
			fmt.Sprintf("%.3f", s.AvgLocalDecisionMs),
			fmt.Sprintf("%.3f", s.LocalP10Ms),
			fmt.Sprintf("%.3f", s.LocalP90Ms),
			fmt.Sprintf("%.3f", s.AvgCentralDecisionMs),
			fmt.Sprintf("%.3f", s.CentralP10Ms),
			fmt.Sprintf("%.3f", s.CentralP90Ms),
		})
	}
	w.Flush()
	return path, w.Error()
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

	for _, target := range []struct{ id, url string }{
		{"idt1a", s.urls.IDT1a}, {"idt1b", s.urls.IDT1b},
	} {
		if err := s.post(target.url+"/simulate/reset", nil); err != nil {
			log.Printf("[%s] WARNING: reset %s failed: %v", s.id, target.id, err)
		}
	}
	normalGas := map[string]float64{"ch4": 0.1, "co": 5.0, "co2": 0.04, "o2": 20.9, "no2": 0.5}
	for _, target := range []struct{ id, url string }{
		{"idt2a", s.urls.IDT2a}, {"idt2b", s.urls.IDT2b},
	} {
		s.post(target.url+"/simulate/gas", normalGas)
	}
	for _, target := range []struct{ id, url string }{
		{"idt3a", s.urls.IDT3a}, {"idt3b", s.urls.IDT3b},
	} {
		s.post(target.url+"/simulate/reset", nil)
	}
	s.post(s.urls.CDTa+"/mission/reset", nil)

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
		"phase":     phase,
		"log":       progress,
		"count":     len(progress),
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
		"status":    "clear-all complete",
		"results":   results,
		"message":   "All hazards cleared and gas normalised. Monitor cDTb /state for updated gate status.",
		"timestamp": time.Now(),
	})
}

// ---- QoS failure injection handlers -----------------------------------------------

func (s *ScenarioService) handleSensorFail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Sensor string `json:"sensor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Sensor == "" {
		common.WriteError(w, http.StatusBadRequest, `body must be {"sensor":"idt2a"}`)
		return
	}
	sensorURLs := map[string]string{"idt2a": s.urls.IDT2a, "idt2b": s.urls.IDT2b}
	url, ok := sensorURLs[body.Sensor]
	if !ok {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown sensor %q – valid: idt2a, idt2b", body.Sensor))
		return
	}
	if err := s.post(url+"/simulate/fail", nil); err != nil {
		common.WriteError(w, http.StatusBadGateway, fmt.Sprintf("sensor fail injection failed: %v", err))
		return
	}
	msg := fmt.Sprintf("Sensor %s set to FAIL mode (HTTP 503 on /state).", body.Sensor)
	s.logProgress(msg)
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "failure mode activated", "sensor": body.Sensor})
}

func (s *ScenarioService) handleSensorDegrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Sensor      string  `json:"sensor"`
		NoiseFactor float64 `json:"noiseFactor"`
		LatencyMs   int     `json:"latencyMs"`
	}
	body.NoiseFactor = 5.0
	body.LatencyMs = 100
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Sensor == "" {
		common.WriteError(w, http.StatusBadRequest, `body must be {"sensor":"idt2a","noiseFactor":5.0,"latencyMs":100}`)
		return
	}
	sensorURLs := map[string]string{"idt2a": s.urls.IDT2a, "idt2b": s.urls.IDT2b}
	url, ok := sensorURLs[body.Sensor]
	if !ok {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown sensor %q – valid: idt2a, idt2b", body.Sensor))
		return
	}
	payload := map[string]interface{}{"noiseFactor": body.NoiseFactor, "latencyMs": body.LatencyMs}
	if err := s.post(url+"/simulate/degrade", payload); err != nil {
		common.WriteError(w, http.StatusBadGateway, fmt.Sprintf("sensor degrade injection failed: %v", err))
		return
	}
	msg := fmt.Sprintf("Sensor %s set to DEGRADE mode (noiseFactor=%.1f latencyMs=%d).", body.Sensor, body.NoiseFactor, body.LatencyMs)
	s.logProgress(msg)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "degraded", "sensor": body.Sensor, "noiseFactor": body.NoiseFactor, "latencyMs": body.LatencyMs,
	})
}

func (s *ScenarioService) handleSensorRecover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Sensor string `json:"sensor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Sensor == "" {
		common.WriteError(w, http.StatusBadRequest, `body must be {"sensor":"idt2a"}`)
		return
	}
	sensorURLs := map[string]string{"idt2a": s.urls.IDT2a, "idt2b": s.urls.IDT2b}
	url, ok := sensorURLs[body.Sensor]
	if !ok {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown sensor %q – valid: idt2a, idt2b", body.Sensor))
		return
	}
	if err := s.post(url+"/simulate/recover", nil); err != nil {
		common.WriteError(w, http.StatusBadGateway, fmt.Sprintf("sensor recover failed: %v", err))
		return
	}
	msg := fmt.Sprintf("Sensor %s RECOVERED – normal operation restored.", body.Sensor)
	s.logProgress(msg)
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "recovered", "sensor": body.Sensor})
}

func (s *ScenarioService) handleRobotFail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Robot string `json:"robot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Robot == "" {
		common.WriteError(w, http.StatusBadRequest, `body must be {"robot":"idt1a"}`)
		return
	}
	robotURLs := map[string]string{"idt1a": s.urls.IDT1a, "idt1b": s.urls.IDT1b}
	url, ok := robotURLs[body.Robot]
	if !ok {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown robot %q – valid: idt1a, idt1b", body.Robot))
		return
	}
	if err := s.post(url+"/simulate/fail", nil); err != nil {
		common.WriteError(w, http.StatusBadGateway, fmt.Sprintf("robot fail injection failed: %v", err))
		return
	}
	msg := fmt.Sprintf("Robot %s set to FAIL mode (HTTP 503 on /state).", body.Robot)
	s.logProgress(msg)
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "failure mode activated", "robot": body.Robot})
}

func (s *ScenarioService) handleRobotRecover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Robot string `json:"robot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Robot == "" {
		common.WriteError(w, http.StatusBadRequest, `body must be {"robot":"idt1a"}`)
		return
	}
	robotURLs := map[string]string{"idt1a": s.urls.IDT1a, "idt1b": s.urls.IDT1b}
	url, ok := robotURLs[body.Robot]
	if !ok {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown robot %q – valid: idt1a, idt1b", body.Robot))
		return
	}
	if err := s.post(url+"/simulate/recover", nil); err != nil {
		common.WriteError(w, http.StatusBadGateway, fmt.Sprintf("robot recover failed: %v", err))
		return
	}
	msg := fmt.Sprintf("Robot %s RECOVERED – normal operation restored.", body.Robot)
	s.logProgress(msg)
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "recovered", "robot": body.Robot})
}

// ---- QoS experiment handlers -------------------------------------------------------

// handleConfig sets the orchestration mode.
// POST /scenario/config?mode=local  (or mode=central)
// Also accepts JSON body: {"mode":"local"}
func (s *ScenarioService) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		var body struct {
			Mode string `json:"mode"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		mode = body.Mode
	}
	if mode != "local" && mode != "central" {
		common.WriteError(w, http.StatusBadRequest, `mode must be "local" or "central"`)
		return
	}

	common.SetOrchestrationMode(mode)
	s.logProgress(fmt.Sprintf("Orchestration mode set to %q.", mode))
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": mode})
}

// handleNetworkDelay sets the simulated network delay.
// POST /scenario/network-delay?ms=20
// Also accepts JSON body: {"ms":20}
func (s *ScenarioService) handleNetworkDelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	msStr := r.URL.Query().Get("ms")
	ms := 0
	if msStr != "" {
		fmt.Sscanf(msStr, "%d", &ms)
	} else {
		var body struct {
			Ms int `json:"ms"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		ms = body.Ms
	}
	if ms < 0 {
		ms = 0
	}

	common.SetNetworkDelayMs(ms)
	s.logProgress(fmt.Sprintf("Network delay set to %dms.", ms))
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "networkDelayMs": ms})
}

// handleProcessingDelay sets the simulated per-node processing time.
// POST /scenario/processing-delay?ms=4
// Also accepts JSON body: {"ms":4}
func (s *ScenarioService) handleProcessingDelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	msStr := r.URL.Query().Get("ms")
	ms := 0
	if msStr != "" {
		fmt.Sscanf(msStr, "%d", &ms)
	} else {
		var body struct {
			Ms int `json:"ms"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		ms = body.Ms
	}
	if ms < 0 {
		ms = 0
	}
	if ms > 10 {
		ms = 10
	}

	s.mu.Lock()
	s.processingDelayMs = ms
	s.mu.Unlock()

	common.SetProcessingDelayMs(ms)
	s.logProgress(fmt.Sprintf("Processing delay set to %dms.", ms))
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "processingDelayMs": ms})
}

// handleMappingSpeed broadcasts a SLAM rate to all inspection robots (iDT1a, iDT1b).
// POST /scenario/mapping-speed
// Body: {"durationSec": 30}
func (s *ScenarioService) handleMappingSpeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		DurationSec float64 `json:"durationSec"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.DurationSec <= 0 {
		body.DurationSec = 30
	}
	payload := map[string]float64{"durationSec": body.DurationSec}
	var errs []string
	for _, url := range []string{s.urls.IDT1a, s.urls.IDT1b} {
		if err := s.post(url+"/simulate/slam-rate", payload); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		s.logProgress(fmt.Sprintf("mapping-speed: partial errors: %v", errs))
	}
	s.logProgress(fmt.Sprintf("Mapping speed set: 100%% in %.0fs (rate=%.4f%%/s).", body.DurationSec, 100.0/body.DurationSec))
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok", "durationSec": body.DurationSec,
		"targets": []string{"idt1a", "idt1b"},
	})
}

// handleClearanceSpeed broadcasts a clearance rate to all LHD vehicles (iDT3a, iDT3b).
// POST /scenario/clearance-speed
// Body: {"durationSec": 30}
func (s *ScenarioService) handleClearanceSpeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		DurationSec float64 `json:"durationSec"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.DurationSec <= 0 {
		body.DurationSec = 30
	}
	payload := map[string]float64{"durationSec": body.DurationSec}
	var errs []string
	for _, url := range []string{s.urls.IDT3a, s.urls.IDT3b} {
		if err := s.post(url+"/simulate/clearance-rate", payload); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		s.logProgress(fmt.Sprintf("clearance-speed: partial errors: %v", errs))
	}
	s.logProgress(fmt.Sprintf("Clearance speed set: 100%% in %.0fs (rate=%.4f%%/s).", body.DurationSec, 100.0/body.DurationSec))
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok", "durationSec": body.DurationSec,
		"targets": []string{"idt3a", "idt3b"},
	})
}

// handleExperimentRun launches the full failover delay experiment asynchronously.
// POST /scenario/experiment/run
// Optional body: {"runsPerPoint": 5}
func (s *ScenarioService) handleExperimentRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	s.expMu.Lock()
	if s.expRunning {
		s.expMu.Unlock()
		common.WriteError(w, http.StatusConflict, "experiment already running – wait for completion or GET /scenario/experiment/results")
		return
	}
	s.expRunning = true
	s.expMu.Unlock()

	var body struct {
		RunsPerPoint int `json:"runsPerPoint"`
	}
	body.RunsPerPoint = 5 // default
	json.NewDecoder(r.Body).Decode(&body)
	if body.RunsPerPoint < 1 {
		body.RunsPerPoint = 1
	}
	if body.RunsPerPoint > 20 {
		body.RunsPerPoint = 20
	}

	go s.runFailoverExperiment(body.RunsPerPoint)

	common.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":       "experiment started",
		"runsPerPoint": body.RunsPerPoint,
		"totalRuns":    11 * 2 * body.RunsPerPoint, // 11 delays × 2 modes × N runs
		"message":      "Experiment running in background. Poll GET /scenario/experiment/results for status.",
	})
}

// handleExperimentResults returns the latest experiment results.
// GET /scenario/experiment/results
func (s *ScenarioService) handleExperimentResults(w http.ResponseWriter, r *http.Request) {
	s.expMu.Lock()
	running := s.expRunning
	results := s.expResults
	s.expMu.Unlock()

	if running {
		common.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":  "running",
			"message": "Experiment in progress. Check back later.",
		})
		return
	}
	if results == nil {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "no results",
			"message": "No experiment has been run yet. POST /scenario/experiment/run to start.",
		})
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "completed",
		"results": results,
	})
}

// ---- Helpers -----------------------------------------------------------------------

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
