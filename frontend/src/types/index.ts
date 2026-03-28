// ============================================================
// Arrowhead Core Types
// ============================================================

export interface ServiceRecord {
  id: string
  name: string
  address: string
  port: number
  type: string         // e.g. "iDT", "cDT", "orchestrator"
  status: 'online' | 'offline' | 'warning'
  lastSeen: string     // ISO timestamp
  endpoints: string[]
  metadata?: Record<string, string>
}

export interface AuthPolicy {
  id: string
  consumerId: string
  providerId: string
  serviceDefinition: string
  createdAt: string
  allowed: boolean
}

export interface OrchestrationLog {
  id: string
  timestamp: string
  consumer: string
  provider: string
  serviceDefinition: string
  allowed: boolean
  reason?: string
}

// ============================================================
// Position
// ============================================================

export interface Position {
  x: number
  y: number
  z: number
}

// ============================================================
// iDT1a/b - Robot State
// ============================================================

export interface RobotState {
  id: string
  name: string
  status: 'idle' | 'moving' | 'scanning' | 'error' | 'offline'
  position: Position
  batteryPercent: number
  mappingProgress: number   // 0–100
  hazardCount: number
  speed: number
  heading: number
  lastUpdate: string
  online: boolean
}

// ============================================================
// iDT2a/b - Gas Sensor State
// ============================================================

export interface GasLevels {
  ch4: number    // % LEL
  co: number     // ppm
  co2: number    // % vol
  o2: number     // % vol
  no2?: number   // ppm
}

export interface GasAlert {
  id: string
  gas: string
  level: number
  threshold: number
  severity: 'low' | 'medium' | 'high' | 'critical'
  timestamp: string
  location: string
  acknowledged: boolean
}

export interface GasSensorState {
  id: string
  name: string
  status: 'nominal' | 'alert' | 'fault' | 'offline'
  position: Position
  levels: GasLevels
  alerts: GasAlert[]
  lastCalibration: string
  lastUpdate: string
  online: boolean
}

// ============================================================
// iDT3a/b - LHD (Load-Haul-Dump) State
// ============================================================

export interface LHDState {
  id: string
  name: string
  status: 'idle' | 'loading' | 'hauling' | 'dumping' | 'error' | 'offline'
  position: Position
  debrisPercent: number    // bucket fill 0–100
  fuelPercent: number      // 0–100
  payloadTonnes: number
  speed: number
  heading: number
  cyclesCompleted: number
  lastUpdate: string
  online: boolean
}

// ============================================================
// iDT4 - Tele-Remote Operator State
// ============================================================

export interface TeleRemoteState {
  id: string
  name: string
  status: 'standby' | 'active' | 'takeover' | 'offline'
  operatorId: string
  controlledAsset?: string
  latencyMs: number
  sessionStart?: string
  lastUpdate: string
  online: boolean
}

// ============================================================
// cDT1 - Mapping Result
// ============================================================

export interface MapCell {
  x: number
  y: number
  explored: boolean
  hazard: boolean
  passable: boolean
}

export interface MappingResult {
  coveragePercent: number
  areaCoveredM2: number
  totalAreaM2: number
  activeRobots: string[]
  mapCells: MapCell[]
  startTime: string
  lastUpdate: string
  status: 'idle' | 'mapping' | 'paused' | 'complete'
}

// ============================================================
// cDT2 - Gas Monitor Result
// ============================================================

export interface GasMonitorResult {
  aggregatedLevels: GasLevels
  activeSensors: string[]
  activeAlerts: GasAlert[]
  ventilationStatus: 'nominal' | 'reduced' | 'critical' | 'offline'
  overallStatus: 'safe' | 'caution' | 'danger'
  lastUpdate: string
}

// ============================================================
// cDT3 - Hazard Report
// ============================================================

export type HazardSeverity = 'low' | 'medium' | 'high' | 'critical'

export interface Hazard {
  id: string
  type: string          // e.g. "rock_fall", "gas_pocket", "structural"
  severity: HazardSeverity
  position: Position
  detectedAt: string
  clearedAt?: string
  cleared: boolean
  description: string
  detectedBy: string    // robot/sensor id
}

export interface HazardReport {
  totalHazards: number
  activeHazards: number
  clearedHazards: number
  hazards: Hazard[]
  riskLevel: 'low' | 'medium' | 'high' | 'critical'
  lastUpdate: string
}

// ============================================================
// cDT4 - Clearance Status
// ============================================================

export interface ClearanceStatus {
  status: 'idle' | 'in_progress' | 'complete' | 'failed'
  clearancePercent: number
  activeAssets: string[]
  hazardsCleared: number
  hazardsPending: number
  estimatedCompletionMin: number
  startTime?: string
  lastUpdate: string
}

// ============================================================
// cDT5 - Intervention Status
// ============================================================

export interface InterventionStatus {
  status: 'standby' | 'requested' | 'active' | 'completed' | 'aborted'
  operatorAssigned?: string
  reason?: string
  requestTime?: string
  startTime?: string
  endTime?: string
  notes: string[]
  lastUpdate: string
}

// ============================================================
// cDTa - Mission Phase + Status
// ============================================================

export type MissionPhase =
  | 'idle'
  | 'exploring'
  | 'hazard_scan'
  | 'clearance'
  | 'verifying'
  | 'complete'

export interface MissionEvent {
  id: string
  timestamp: string
  phase: MissionPhase
  event: string
  level: 'info' | 'warning' | 'error' | 'success'
}

export interface Recommendation {
  id: string
  priority: 'low' | 'medium' | 'high' | 'critical'
  text: string
  action?: string
}

export interface MissionStatus {
  phase: MissionPhase
  startTime?: string
  endTime?: string
  missionId: string
  mapping: MappingResult
  hazardReport: HazardReport
  clearance: ClearanceStatus
  intervention: InterventionStatus
  recommendations: Recommendation[]
  eventLog: MissionEvent[]
  lastUpdate: string
}

// ============================================================
// cDTb - Safe Access Decision
// ============================================================

export type GatingStatus = 'open' | 'closed' | 'conditional'

export interface SafeAccessDecision {
  safeToAccess: boolean
  reason: string
  gatingStatus: GatingStatus
  gasMonitor: GasMonitorResult
  hazardReport: HazardReport
  ventilationOk: boolean
  recommendations: Recommendation[]
  lastUpdate: string
  confidence: number   // 0–100
}

// ============================================================
// Scenario Runner
// ============================================================

export type ScenarioState = 'idle' | 'running' | 'paused' | 'complete'

export interface ScenarioStatus {
  state: ScenarioState
  scenarioId: string
  description: string
  elapsedSeconds: number
  activeEvents: string[]
  lastUpdate: string
}

// ============================================================
// API Response Wrappers
// ============================================================

export interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: string
  timestamp: string
}

export interface ServiceListResponse {
  services: ServiceRecord[]
  count: number
  timestamp: string
}

export interface PolicyListResponse {
  policies: AuthPolicy[]
  count: number
}

export interface OrchestrationLogResponse {
  logs: OrchestrationLog[]
  count: number
}

// ============================================================
// UI-only helper types
// ============================================================

export interface ServiceNode {
  id: string
  label: string
  type: 'iDT' | 'cDT' | 'core'
  port: number
  status: 'online' | 'offline' | 'warning'
}

export interface ServiceEdge {
  from: string
  to: string
  label?: string
}

export interface AddPolicyPayload {
  consumerId: string
  providerId: string
  serviceDefinition: string
}
