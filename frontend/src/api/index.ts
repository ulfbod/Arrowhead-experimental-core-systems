import axios from 'axios'
import type {
  ServiceRecord,
  AuthPolicy,
  OrchestrationLog,
  RobotState,
  GasSensorState,
  LHDState,
  TeleRemoteState,
  CDT1State,
  GasMonitorResult,
  HazardReport,
  ClearanceStatus,
  InterventionStatus,
  MissionStatus,
  MissionPhase,
  SafeAccessDecision,
  ScenarioStatus,
  AddPolicyPayload,
  ServicesResponse,
  PoliciesResponse,
  OrchLogsResponse,
} from '../types'

// ============================================================
// Port constants
// ============================================================
export const PORTS = {
  ARROWHEAD: 8000,
  IDT1A:     8101,
  IDT1B:     8102,
  IDT2A:     8201,
  IDT2B:     8202,
  IDT3A:     8301,
  IDT3B:     8302,
  IDT4:      8401,
  CDT1:      8501,
  CDT2:      8502,
  CDT3:      8503,
  CDT4:      8504,
  CDT5:      8505,
  CDTA:      8601,
  CDTB:      8602,
  SCENARIO:  8700,
} as const

function base(port: number) { return `http://localhost:${port}` }

async function get<T>(port: number, path: string): Promise<T> {
  const r = await axios.get<T>(`${base(port)}${path}`, { timeout: 5000 })
  return r.data
}
async function post<T>(port: number, path: string, body?: unknown): Promise<T> {
  const r = await axios.post<T>(`${base(port)}${path}`, body ?? {}, { timeout: 5000 })
  return r.data
}
async function del<T>(port: number, path: string): Promise<T> {
  const r = await axios.delete<T>(`${base(port)}${path}`, { timeout: 5000 })
  return r.data
}

// ============================================================
// Arrowhead Core  (:8000)
// ============================================================

export const getServices = (): Promise<ServiceRecord[]> =>
  get<ServicesResponse>(PORTS.ARROWHEAD, '/registry').then(r => r.services ?? [])

export const getPolicies = (): Promise<AuthPolicy[]> =>
  get<PoliciesResponse>(PORTS.ARROWHEAD, '/authorization/policies').then(r => r.policies ?? [])

export const getOrchestrationLogs = (): Promise<OrchestrationLog[]> =>
  get<OrchLogsResponse>(PORTS.ARROWHEAD, '/orchestration/logs').then(r => r.logs ?? [])

export const addPolicy = (payload: AddPolicyPayload): Promise<AuthPolicy> =>
  post<AuthPolicy>(PORTS.ARROWHEAD, '/authorization/policy', payload)

export const deletePolicy = (id: string): Promise<void> =>
  del<void>(PORTS.ARROWHEAD, `/authorization/policy?id=${encodeURIComponent(id)}`)

// ============================================================
// iDT services
// ============================================================

export const getRobotState = (_id: string, port: number): Promise<RobotState> =>
  get<RobotState>(port, '/state')

export const getGasState = (_id: string, port: number): Promise<GasSensorState> =>
  get<GasSensorState>(port, '/state')

export const getLHDState = (_id: string, port: number): Promise<LHDState> =>
  get<LHDState>(port, '/state')

export const getTeleRemoteState = (): Promise<TeleRemoteState> =>
  get<TeleRemoteState>(PORTS.IDT4, '/state')

export const setRobotOnline = (port: number, online: boolean) =>
  post<void>(port, '/online', { online })

export const setRobotConnected = (port: number, connected: boolean) =>
  post<void>(port, '/connectivity', { connected })

export const injectRobotHazard = (port: number, type: string, severity: string) =>
  post<void>(port, '/hazard/inject', { type, severity })

// ============================================================
// cDT1 – Mapping  (:8501)
// ============================================================

export const getCDT1State = (): Promise<CDT1State> =>
  get<CDT1State>(PORTS.CDT1, '/state')

export const startMapping = (): Promise<void> =>
  post<void>(PORTS.CDT1, '/start')

export const stopMapping = (): Promise<void> =>
  post<void>(PORTS.CDT1, '/stop')

// ============================================================
// cDT2 – Gas Monitor  (:8502)
// ============================================================

export const getGasMonitor = (): Promise<GasMonitorResult> =>
  get<GasMonitorResult>(PORTS.CDT2, '/state')

export const triggerSpike = (): Promise<void> =>
  post<void>(PORTS.CDT2, '/simulate/spike')

// ============================================================
// cDT3 – Hazard Detection  (:8503)
// ============================================================

export const getHazardReport = (): Promise<HazardReport> =>
  get<HazardReport>(PORTS.CDT3, '/state')

// ============================================================
// cDT4 – Clearance  (:8504)
// ============================================================

export const getClearanceStatus = (): Promise<ClearanceStatus> =>
  get<ClearanceStatus>(PORTS.CDT4, '/state')

export const startClearance = (): Promise<void> =>
  post<void>(PORTS.CDT4, '/clearance/start')

export const stopClearance = (): Promise<void> =>
  post<void>(PORTS.CDT4, '/clearance/stop')

// ============================================================
// cDT5 – Intervention  (:8505)
// ============================================================

export const getInterventionStatus = (): Promise<InterventionStatus> =>
  get<InterventionStatus>(PORTS.CDT5, '/state')

// ============================================================
// cDTa – Mission  (:8601)
// ============================================================

export const getMissionStatus = (): Promise<MissionStatus> =>
  get<MissionStatus>(PORTS.CDTA, '/state')

export const startMission = (): Promise<void> =>
  post<void>(PORTS.CDTA, '/mission/start')

export const abortMission = (): Promise<void> =>
  post<void>(PORTS.CDTA, '/mission/abort')

export const resetMission = (): Promise<void> =>
  post<void>(PORTS.CDTA, '/mission/reset')

export const forcePhase = (phase: MissionPhase): Promise<void> =>
  post<void>(PORTS.CDTA, '/force/phase', { phase })

// ============================================================
// cDTb – Safe Access  (:8602)
// ============================================================

export const getSafeAccessDecision = (): Promise<SafeAccessDecision> =>
  get<SafeAccessDecision>(PORTS.CDTB, '/state')

export const openGate = (): Promise<void> =>
  post<void>(PORTS.CDTB, '/gate/open')

export const closeGate = (): Promise<void> =>
  post<void>(PORTS.CDTB, '/gate/close')

// ============================================================
// Scenario Runner  (:8700)
// ============================================================

export const getScenarioState = (): Promise<ScenarioStatus> =>
  get<ScenarioStatus>(PORTS.SCENARIO, '/state')

export const startScenario = (): Promise<void> =>
  post<void>(PORTS.SCENARIO, '/scenario/start')

export const resetScenario = (): Promise<void> =>
  post<void>(PORTS.SCENARIO, '/scenario/reset')

export const injectHazard = (): Promise<void> =>
  post<void>(PORTS.SCENARIO, '/scenario/inject-hazard', { robotId: 'idt1a', type: 'misfire', severity: 'high' })

export const triggerGasSpike = (): Promise<void> =>
  post<void>(PORTS.SCENARIO, '/scenario/gas-spike')

export const clearAll = (): Promise<void> =>
  post<void>(PORTS.SCENARIO, '/scenario/clear-all')

// ============================================================
// URL map for usePolling
// ============================================================
export const urls = {
  services:     `http://localhost:${PORTS.ARROWHEAD}/registry`,
  policies:     `http://localhost:${PORTS.ARROWHEAD}/authorization/policies`,
  orchLogs:     `http://localhost:${PORTS.ARROWHEAD}/orchestration/logs`,
  robotA:       `http://localhost:${PORTS.IDT1A}/state`,
  robotB:       `http://localhost:${PORTS.IDT1B}/state`,
  gasA:         `http://localhost:${PORTS.IDT2A}/state`,
  gasB:         `http://localhost:${PORTS.IDT2B}/state`,
  lhdA:         `http://localhost:${PORTS.IDT3A}/state`,
  lhdB:         `http://localhost:${PORTS.IDT3B}/state`,
  teleRemote:   `http://localhost:${PORTS.IDT4}/state`,
  cdt1:         `http://localhost:${PORTS.CDT1}/state`,
  cdt2:         `http://localhost:${PORTS.CDT2}/state`,
  cdt3:         `http://localhost:${PORTS.CDT3}/state`,
  cdt4:         `http://localhost:${PORTS.CDT4}/state`,
  cdt5:         `http://localhost:${PORTS.CDT5}/state`,
  mission:      `http://localhost:${PORTS.CDTA}/state`,
  safeAccess:   `http://localhost:${PORTS.CDTB}/state`,
  scenario:     `http://localhost:${PORTS.SCENARIO}/state`,
}
