import axios from 'axios'
import type {
  ServiceRecord,
  AuthPolicy,
  OrchestrationLog,
  RobotState,
  GasSensorState,
  LHDState,
  TeleRemoteState,
  MappingResult,
  GasMonitorResult,
  HazardReport,
  ClearanceStatus,
  InterventionStatus,
  MissionStatus,
  MissionPhase,
  SafeAccessDecision,
  ScenarioStatus,
  AddPolicyPayload,
} from '../types'

// ============================================================
// Port Constants
// ============================================================
export const PORTS = {
  ARROWHEAD:  8000,
  IDT1A:      8101,  // Robot A
  IDT1B:      8102,  // Robot B
  IDT2A:      8201,  // Gas A
  IDT2B:      8202,  // Gas B
  IDT3A:      8301,  // LHD A
  IDT3B:      8302,  // LHD B
  IDT4:       8401,  // Tele-Remote
  CDT1:       8501,  // Mapping
  CDT2:       8502,  // Gas Monitor
  CDT3:       8503,  // Hazard Detect
  CDT4:       8504,  // Clearance
  CDT5:       8505,  // Intervention
  CDTA:       8601,  // Mission (cDTa)
  CDTB:       8602,  // Safe Access (cDTb)
  SCENARIO:   8700,  // Scenario Runner
} as const

// ============================================================
// Base HTTP helpers
// ============================================================

function base(port: number): string {
  return `http://localhost:${port}`
}

async function get<T>(port: number, path: string): Promise<T> {
  const resp = await axios.get<T>(`${base(port)}${path}`, { timeout: 5000 })
  return resp.data
}

async function post<T>(port: number, path: string, body?: unknown): Promise<T> {
  const resp = await axios.post<T>(`${base(port)}${path}`, body ?? {}, { timeout: 5000 })
  return resp.data
}

async function del<T>(port: number, path: string): Promise<T> {
  const resp = await axios.delete<T>(`${base(port)}${path}`, { timeout: 5000 })
  return resp.data
}

// ============================================================
// Arrowhead Core  (port 8000)
// ============================================================

export const getServices = (): Promise<ServiceRecord[]> =>
  get<ServiceRecord[]>(PORTS.ARROWHEAD, '/serviceregistry/services')

export const getPolicies = (): Promise<AuthPolicy[]> =>
  get<AuthPolicy[]>(PORTS.ARROWHEAD, '/authorization/policies')

export const getOrchestrationLogs = (): Promise<OrchestrationLog[]> =>
  get<OrchestrationLog[]>(PORTS.ARROWHEAD, '/orchestration/logs')

export const addPolicy = (payload: AddPolicyPayload): Promise<AuthPolicy> =>
  post<AuthPolicy>(PORTS.ARROWHEAD, '/authorization/policies', payload)

export const deletePolicy = (id: string): Promise<void> =>
  del<void>(PORTS.ARROWHEAD, `/authorization/policies/${id}`)

// ============================================================
// iDT1a / iDT1b – Robots  (port 8101 / 8102)
// ============================================================

export const getRobotState = (_id: string, port: number): Promise<RobotState> =>
  get<RobotState>(port, '/state')

// ============================================================
// iDT2a / iDT2b – Gas Sensors  (port 8201 / 8202)
// ============================================================

export const getGasState = (_id: string, port: number): Promise<GasSensorState> =>
  get<GasSensorState>(port, '/state')

// ============================================================
// iDT3a / iDT3b – LHDs  (port 8301 / 8302)
// ============================================================

export const getLHDState = (_id: string, port: number): Promise<LHDState> =>
  get<LHDState>(port, '/state')

// ============================================================
// iDT4 – Tele-Remote  (port 8401)
// ============================================================

export const getTeleRemoteState = (): Promise<TeleRemoteState> =>
  get<TeleRemoteState>(PORTS.IDT4, '/state')

// ============================================================
// cDT1 – Mapping  (port 8501)
// ============================================================

export const getMapping = (): Promise<MappingResult> =>
  get<MappingResult>(PORTS.CDT1, '/mapping')

export const startMapping = (): Promise<void> =>
  post<void>(PORTS.CDT1, '/mapping/start')

export const stopMapping = (): Promise<void> =>
  post<void>(PORTS.CDT1, '/mapping/stop')

// ============================================================
// cDT2 – Gas Monitor  (port 8502)
// ============================================================

export const getGasMonitor = (): Promise<GasMonitorResult> =>
  get<GasMonitorResult>(PORTS.CDT2, '/monitor')

export const triggerSpike = (): Promise<void> =>
  post<void>(PORTS.CDT2, '/spike')

// ============================================================
// cDT3 – Hazard Detection  (port 8503)
// ============================================================

export const getHazardReport = (): Promise<HazardReport> =>
  get<HazardReport>(PORTS.CDT3, '/hazards')

// ============================================================
// cDT4 – Clearance  (port 8504)
// ============================================================

export const getClearanceStatus = (): Promise<ClearanceStatus> =>
  get<ClearanceStatus>(PORTS.CDT4, '/clearance')

export const startClearance = (): Promise<void> =>
  post<void>(PORTS.CDT4, '/clearance/start')

export const stopClearance = (): Promise<void> =>
  post<void>(PORTS.CDT4, '/clearance/stop')

// ============================================================
// cDT5 – Intervention  (port 8505)
// ============================================================

export const getInterventionStatus = (): Promise<InterventionStatus> =>
  get<InterventionStatus>(PORTS.CDT5, '/intervention')

// ============================================================
// cDTa – Mission Controller  (port 8601)
// ============================================================

export const getMissionStatus = (): Promise<MissionStatus> =>
  get<MissionStatus>(PORTS.CDTA, '/mission')

export const startMission = (): Promise<void> =>
  post<void>(PORTS.CDTA, '/mission/start')

export const abortMission = (): Promise<void> =>
  post<void>(PORTS.CDTA, '/mission/abort')

export const resetMission = (): Promise<void> =>
  post<void>(PORTS.CDTA, '/mission/reset')

export const forcePhase = (phase: MissionPhase): Promise<void> =>
  post<void>(PORTS.CDTA, '/mission/phase', { phase })

// ============================================================
// cDTb – Safe Access / Gating  (port 8602)
// ============================================================

export const getSafeAccessDecision = (): Promise<SafeAccessDecision> =>
  get<SafeAccessDecision>(PORTS.CDTB, '/decision')

export const openGate = (): Promise<void> =>
  post<void>(PORTS.CDTB, '/gate/open')

export const closeGate = (): Promise<void> =>
  post<void>(PORTS.CDTB, '/gate/close')

// ============================================================
// Scenario Runner  (port 8700)
// ============================================================

export const getScenarioState = (): Promise<ScenarioStatus> =>
  get<ScenarioStatus>(PORTS.SCENARIO, '/scenario')

export const startScenario = (): Promise<void> =>
  post<void>(PORTS.SCENARIO, '/scenario/start')

export const resetScenario = (): Promise<void> =>
  post<void>(PORTS.SCENARIO, '/scenario/reset')

export const injectHazard = (): Promise<void> =>
  post<void>(PORTS.SCENARIO, '/scenario/inject-hazard')

export const triggerGasSpike = (): Promise<void> =>
  post<void>(PORTS.SCENARIO, '/scenario/gas-spike')

export const clearAll = (): Promise<void> =>
  post<void>(PORTS.SCENARIO, '/scenario/clear')

// ============================================================
// URL builders (for usePolling)
// ============================================================
export const urls = {
  services:         `http://localhost:${PORTS.ARROWHEAD}/serviceregistry/services`,
  policies:         `http://localhost:${PORTS.ARROWHEAD}/authorization/policies`,
  orchLogs:         `http://localhost:${PORTS.ARROWHEAD}/orchestration/logs`,
  robotA:           `http://localhost:${PORTS.IDT1A}/state`,
  robotB:           `http://localhost:${PORTS.IDT1B}/state`,
  gasA:             `http://localhost:${PORTS.IDT2A}/state`,
  gasB:             `http://localhost:${PORTS.IDT2B}/state`,
  lhdA:             `http://localhost:${PORTS.IDT3A}/state`,
  lhdB:             `http://localhost:${PORTS.IDT3B}/state`,
  teleRemote:       `http://localhost:${PORTS.IDT4}/state`,
  mapping:          `http://localhost:${PORTS.CDT1}/mapping`,
  gasMonitor:       `http://localhost:${PORTS.CDT2}/monitor`,
  hazardReport:     `http://localhost:${PORTS.CDT3}/hazards`,
  clearance:        `http://localhost:${PORTS.CDT4}/clearance`,
  intervention:     `http://localhost:${PORTS.CDT5}/intervention`,
  mission:          `http://localhost:${PORTS.CDTA}/mission`,
  safeAccess:       `http://localhost:${PORTS.CDTB}/decision`,
  scenario:         `http://localhost:${PORTS.SCENARIO}/scenario`,
}
