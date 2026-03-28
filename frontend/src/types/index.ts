// ============================================================
// Types matching the Go backend JSON responses exactly
// ============================================================

// ---- Arrowhead Core ----

export interface ServiceRecord {
  id: string
  name: string
  address: string
  port: number
  serviceType: string   // "iDT" | "cDT" | "core"
  capabilities: string[]
  metadata: Record<string, string>
  registeredAt: string
  lastSeen: string
  online: boolean
}

export interface AuthPolicy {
  id: string
  consumerId: string
  providerId: string
  serviceName: string
  allowed: boolean
  createdAt: string
}

export interface OrchestrationLog {
  id: string
  timestamp: string
  consumerId: string
  serviceName: string
  providerId: string
  authToken: string
  allowed: boolean
  message: string
}

export interface AddPolicyPayload {
  consumerId: string
  providerId: string
  serviceName: string
  allowed: boolean
}

// ---- Shared ----

export interface Position {
  x: number
  y: number
  z: number
}

export interface Hazard {
  id: string
  type: string
  severity: string   // "low" | "medium" | "high" | "critical"
  position: Position
  detectedAt: string
  cleared: boolean
  clearedAt?: string
}

export interface GasLevels {
  ch4: number
  co: number
  co2: number
  o2: number
  no2: number
}

export interface GasAlert {
  id: string
  gas: string
  level: number
  threshold: number
  location: Position
  timestamp: string
  active: boolean
}

// ---- iDT States ----

export interface RobotState {
  id: string
  name: string
  online: boolean
  connected: boolean
  position: Position
  batteryPct: number
  mappingProgress: number
  slamActive: boolean
  navigationStatus: string
  hazardsDetected: Hazard[]
  areaCoveredSqm: number
  lastUpdated: string
}

export interface GasSensorState {
  id: string
  name: string
  online: boolean
  connected: boolean
  position: Position
  gasLevels: GasLevels
  alerts: GasAlert[]
  environmentStatus: string   // "safe" | "warning" | "danger"
  lastUpdated: string
}

export interface LHDState {
  id: string
  name: string
  online: boolean
  connected: boolean
  position: Position
  payloadTons: number
  maxPayloadTons: number
  available: boolean
  trammingStatus: string   // "idle" | "tramming" | "loading" | "unloading"
  debrisClearedPct: number
  fuelPct: number
  lastUpdated: string
}

export interface TeleRemoteState {
  id: string
  name: string
  online: boolean
  operatorPresent: boolean
  overrideActive: boolean
  targetMachineId: string
  authorizationLevel: string
  lastCommand: string
  lastCommandTime: string
  lastUpdated: string
}

// ---- cDT Composite Results ----

export interface MappingResult {
  totalAreaSqm: number
  coveredAreaSqm: number
  coveragePct: number
  activeRobots: number   // count, not array
  map: number[][]
  timestamp: string
}

// cDT1 /state returns this wrapper
export interface CDT1State {
  mapping: MappingResult
  robot1: RobotState | null
  robot2: RobotState | null
  timestamp: string
}

export interface GasMonitorResult {
  averageLevels: GasLevels
  maxLevels: GasLevels
  activeAlerts: GasAlert[]
  environmentSafe: boolean
  activeSensors: number
  timestamp: string
}

export interface HazardReport {
  hazards: Hazard[]
  overallRisk: string       // "low" | "medium" | "high" | "critical"
  safeForEntry: boolean
  recommendedAction: string
  timestamp: string
}

export interface ClearanceStatus {
  totalDebrisPct: number
  activeVehicles: number
  estimatedEtaMinutes: number
  routeClear: boolean
  timestamp: string
}

export interface InterventionStatus {
  active: boolean
  operatorPresent: boolean
  targetMachine: string
  lastCommand: string
  timestamp: string
}

// ---- Upper cDT Mission ----

export type MissionPhase =
  | 'idle'
  | 'exploring'
  | 'hazard_scan'
  | 'clearance'
  | 'verifying'
  | 'complete'
  | 'failed'

export interface MissionStatus {
  phase: MissionPhase
  startedAt?: string
  completedAt?: string
  mapping?: MappingResult | null
  hazards?: HazardReport | null
  clearance?: ClearanceStatus | null
  intervention?: InterventionStatus | null
  recommendations: string[]
  log: string[]
  lastUpdated: string
}

export interface SafeAccessDecision {
  safe: boolean
  reason: string
  gasStatus?: GasMonitorResult | null
  hazardStatus?: HazardReport | null
  ventilationOk: boolean
  gatingStatus: string   // "closed" | "open" | "conditional"
  recommendations: string[]
  lastUpdated: string
}

// ---- Scenario Runner ----

export type ScenarioState = 'idle' | 'running' | 'complete' | 'failed'

export interface ScenarioStatus {
  state: ScenarioState
  log: string[]
  lastUpdated: string
  elapsedSeconds?: number
}

// ---- API Response Wrappers ----

export interface ServicesResponse {
  services: ServiceRecord[]
  count: number
}

export interface PoliciesResponse {
  policies: AuthPolicy[]
  count: number
}

export interface OrchLogsResponse {
  logs: OrchestrationLog[]
  count: number
}
