package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ---- Arrowhead Types ----

type ServiceRecord struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Port         int               `json:"port"`
	ServiceType  string            `json:"serviceType"` // "iDT" or "cDT" or "core"
	Capabilities []string          `json:"capabilities"`
	Metadata     map[string]string `json:"metadata"`
	RegisteredAt time.Time         `json:"registeredAt"`
	LastSeen     time.Time         `json:"lastSeen"`
	Online       bool              `json:"online"`
}

type RegisterRequest struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Port         int               `json:"port"`
	ServiceType  string            `json:"serviceType"`
	Capabilities []string          `json:"capabilities"`
	Metadata     map[string]string `json:"metadata"`
}

type AuthPolicy struct {
	ID          string    `json:"id"`
	ConsumerID  string    `json:"consumerId"`
	ProviderID  string    `json:"providerId"`
	ServiceName string    `json:"serviceName"`
	Allowed     bool      `json:"allowed"`
	CreatedAt   time.Time `json:"createdAt"`
}

type AuthCheckRequest struct {
	ConsumerID  string `json:"consumerId"`
	ProviderID  string `json:"providerId"`
	ServiceName string `json:"serviceName"`
}

type AuthCheckResponse struct {
	Allowed   bool   `json:"allowed"`
	PolicyID  string `json:"policyId"`
	AuthToken string `json:"authToken"`
	Reason    string `json:"reason"`
}

type OrchestrationRequest struct {
	ConsumerID  string `json:"consumerId"`
	ServiceName string `json:"serviceName"`
}

type OrchestrationResponse struct {
	Provider  *ServiceRecord `json:"provider"`
	AuthToken string         `json:"authToken"`
	Endpoint  string         `json:"endpoint"`
	Allowed   bool           `json:"allowed"`
	Reason    string         `json:"reason"`
}

type OrchestrationLog struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	ConsumerID  string    `json:"consumerId"`
	ServiceName string    `json:"serviceName"`
	ProviderID  string    `json:"providerId"`
	AuthToken   string    `json:"authToken"`
	Allowed     bool      `json:"allowed"`
	Message     string    `json:"message"`
}

// ---- Shared Machine Types ----

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type Hazard struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // "loose-rock", "misfire", "gas", "structural"
	Severity   string    `json:"severity"` // "low", "medium", "high", "critical"
	Position   Position  `json:"position"`
	DetectedAt time.Time `json:"detectedAt"`
	Cleared    bool      `json:"cleared"`
	ClearedAt  *time.Time `json:"clearedAt,omitempty"`
}

type GasLevels struct {
	CH4  float64 `json:"ch4"`  // methane %
	CO   float64 `json:"co"`   // carbon monoxide ppm
	CO2  float64 `json:"co2"`  // carbon dioxide %
	O2   float64 `json:"o2"`   // oxygen %
	NO2  float64 `json:"no2"`  // nitrogen dioxide ppm
}

type GasAlert struct {
	ID        string    `json:"id"`
	Gas       string    `json:"gas"`
	Level     float64   `json:"level"`
	Threshold float64   `json:"threshold"`
	Location  Position  `json:"location"`
	Timestamp time.Time `json:"timestamp"`
	Active    bool      `json:"active"`
}

// ---- iDT Robot Types ----

type RobotState struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Online            bool      `json:"online"`
	Connected         bool      `json:"connected"`
	Position          Position  `json:"position"`
	BatteryPct        float64   `json:"batteryPct"`
	MappingProgress   float64   `json:"mappingProgress"`
	SlamActive        bool      `json:"slamActive"`
	NavigationStatus  string    `json:"navigationStatus"`
	HazardsDetected   []Hazard  `json:"hazardsDetected"`
	AreaCoveredSqm    float64   `json:"areaCoveredSqm"`
	LastUpdated       time.Time `json:"lastUpdated"`
}

// ---- iDT Gas Sensor Types ----

type GasSensorState struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Online           bool       `json:"online"`
	Connected        bool       `json:"connected"`
	Position         Position   `json:"position"`
	GasLevels        GasLevels  `json:"gasLevels"`
	Alerts           []GasAlert `json:"alerts"`
	EnvironmentStatus string    `json:"environmentStatus"` // "safe", "warning", "danger"
	LastUpdated      time.Time  `json:"lastUpdated"`
}

// ---- iDT LHD Vehicle Types ----

type LHDState struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Online           bool      `json:"online"`
	Connected        bool      `json:"connected"`
	Position         Position  `json:"position"`
	PayloadTons      float64   `json:"payloadTons"`
	MaxPayloadTons   float64   `json:"maxPayloadTons"`
	Available        bool      `json:"available"`
	TrammingStatus   string    `json:"trammingStatus"` // "idle","tramming","loading","unloading"
	DebrisClearedPct float64   `json:"debrisClearedPct"`
	FuelPct          float64   `json:"fuelPct"`
	LastUpdated      time.Time `json:"lastUpdated"`
}

// ---- iDT Tele-Remote Types ----

type TeleRemoteState struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Online            bool      `json:"online"`
	OperatorPresent   bool      `json:"operatorPresent"`
	OverrideActive    bool      `json:"overrideActive"`
	TargetMachineID   string    `json:"targetMachineId"`
	AuthorizationLevel string   `json:"authorizationLevel"`
	LastCommand       string    `json:"lastCommand"`
	LastCommandTime   time.Time `json:"lastCommandTime"`
	LastUpdated       time.Time `json:"lastUpdated"`
}

// ---- cDT Composite Types ----

type MappingResult struct {
	TotalAreaSqm     float64   `json:"totalAreaSqm"`
	CoveredAreaSqm   float64   `json:"coveredAreaSqm"`
	CoveragePct      float64   `json:"coveragePct"`
	ActiveRobots     int       `json:"activeRobots"`
	Map              [][]int   `json:"map"` // simple grid: 0=unknown,1=clear,2=obstacle
	Timestamp        time.Time `json:"timestamp"`
}

type GasMonitorResult struct {
	AverageLevels    GasLevels  `json:"averageLevels"`
	MaxLevels        GasLevels  `json:"maxLevels"`
	ActiveAlerts     []GasAlert `json:"activeAlerts"`
	EnvironmentSafe  bool       `json:"environmentSafe"`
	ActiveSensors    int        `json:"activeSensors"`
	Timestamp        time.Time  `json:"timestamp"`
}

type HazardReport struct {
	Hazards          []Hazard  `json:"hazards"`
	OverallRisk      string    `json:"overallRisk"` // "low","medium","high","critical"
	SafeForEntry     bool      `json:"safeForEntry"`
	RecommendedAction string   `json:"recommendedAction"`
	Timestamp        time.Time `json:"timestamp"`
}

type ClearanceStatus struct {
	TotalDebrisPct   float64   `json:"totalDebrisPct"`
	ActiveVehicles   int       `json:"activeVehicles"`
	EstimatedETA     int       `json:"estimatedEtaMinutes"`
	RouteClear       bool      `json:"routeClear"`
	Timestamp        time.Time `json:"timestamp"`
}

type InterventionStatus struct {
	Active          bool      `json:"active"`
	OperatorPresent bool      `json:"operatorPresent"`
	TargetMachine   string    `json:"targetMachine"`
	LastCommand     string    `json:"lastCommand"`
	Timestamp       time.Time `json:"timestamp"`
}

// ---- Upper cDT Mission Types ----

type MissionPhase string
const (
	PhaseIdle        MissionPhase = "idle"
	PhaseExploring   MissionPhase = "exploring"
	PhaseGasCheck    MissionPhase = "gas_check"
	PhaseHazardScan  MissionPhase = "hazard_scan"
	PhaseClearance   MissionPhase = "clearance"
	PhaseVerifying   MissionPhase = "verifying"
	PhaseComplete    MissionPhase = "complete"
	PhaseFailed      MissionPhase = "failed"
)

type MissionStatus struct {
	Phase            MissionPhase `json:"phase"`
	StartedAt        *time.Time   `json:"startedAt,omitempty"`
	CompletedAt      *time.Time   `json:"completedAt,omitempty"`
	Mapping          *MappingResult    `json:"mapping,omitempty"`
	Hazards          *HazardReport     `json:"hazards,omitempty"`
	Clearance        *ClearanceStatus  `json:"clearance,omitempty"`
	Intervention     *InterventionStatus `json:"intervention,omitempty"`
	Recommendations  []string     `json:"recommendations"`
	Log              []string     `json:"log"`
	LastUpdated      time.Time    `json:"lastUpdated"`
}

type SafeAccessDecision struct {
	Safe             bool       `json:"safe"`
	Reason           string     `json:"reason"`
	GasStatus        *GasMonitorResult `json:"gasStatus,omitempty"`
	HazardStatus     *HazardReport     `json:"hazardStatus,omitempty"`
	VentilationOK    bool       `json:"ventilationOk"`
	GatingStatus     string     `json:"gatingStatus"` // "closed","open","conditional"
	Recommendations  []string   `json:"recommendations"`
	LastUpdated      time.Time  `json:"lastUpdated"`
}

// ---- HTTP Helpers ----

func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Auth-Token, X-Consumer-ID")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Auth-Token, X-Consumer-ID")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now(),
	})
}

// SimpleRouter is a lightweight HTTP mux
type SimpleRouter struct {
	mux *http.ServeMux
}

func NewRouter() *SimpleRouter {
	return &SimpleRouter{mux: http.NewServeMux()}
}

func (sr *SimpleRouter) Handle(pattern string, handler http.HandlerFunc) {
	sr.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Auth-Token, X-Consumer-ID")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler(w, r)
	})
}

func (sr *SimpleRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sr.mux.ServeHTTP(w, r)
}

// Endpoint returns the full URL for a service
func Endpoint(host string, port int, path string) string {
	return fmt.Sprintf("http://%s:%d%s", host, port, path)
}
