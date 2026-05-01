// Types for the experiment-2 dashboard.
// Core system types mirror SPEC.md.  Broker and telemetry types are experiment-specific.

// ── Core systems ──────────────────────────────────────────────────────────────

export interface HealthResponse {
  status: string
  system?: string
}

export type HealthStatus = 'ok' | 'error' | 'loading'

export interface SystemDef {
  id: string
  label: string
  healthPath: string   // proxy prefix, e.g. /api/sr
  healthUrl?: string              // override: exact URL to GET for health check
                                  // defaults to `${healthPath}/health`
  healthHeaders?: Record<string, string>  // extra request headers (e.g. Authorization)
  // Full override: replaces the default fetchHealthProbe call entirely.
  // Use when the service needs a different auth/fetch path (e.g. RabbitMQ).
  healthFetcher?: (signal: AbortSignal) => Promise<HealthProbe>
  layer: 'core' | 'support' | 'experiment'
  dependsOn?: string[]  // ids of systems this one depends on
}

// Result of a timed health probe.
export interface HealthProbe {
  status:    'ok' | 'degraded' | 'down'
  latencyMs: number
  error?:    string
}

export interface ServiceInstance {
  id: number
  serviceDefinition: string
  providerSystem: { systemName: string; address: string; port: number }
  serviceUri: string
  interfaces: string[]
  version: number
  metadata?: Record<string, string>
}

export interface QueryResponse {
  serviceQueryData: ServiceInstance[]
  unfilteredHits: number
}

export interface AuthRule {
  id: number
  consumerSystemName: string
  providerSystemName: string
  serviceDefinition: string
}

export interface LookupResponse {
  rules: AuthRule[]
  count: number
}

// ── RabbitMQ management API ───────────────────────────────────────────────────

export interface RabbitQueue {
  name: string
  messages: number
  consumers: number
  message_stats?: {
    publish_details?: { rate: number }
    deliver_details?: { rate: number }
  }
}

export interface RabbitExchange {
  name: string
  type: string
  durable: boolean
  message_stats?: {
    publish_in_details?: { rate: number }
  }
}

// ── Telemetry (edge-adapter) ──────────────────────────────────────────────────

export interface ImuData {
  roll: number
  pitch: number
  yaw: number
}

export interface PositionData {
  x: number
  y: number
  z: number
}

export interface TelemetryPayload {
  robotId: string
  temperature: number
  humidity: number
  timestamp: string
  seq: number
  imu?: ImuData
  position?: PositionData
}

// LatencyStats matches the Go LatencyStats struct JSON output.
export interface LatencyStats {
  mean: number
  p50: number
  p95: number
  p99: number
  max: number
}

// RobotStatsEntry matches Go's RobotStatsEntry JSON: latency is nested under "latencyMs".
export interface RobotStatsEntry {
  lastSeq: number
  msgCount: number
  rateHz: number
  latencyMs: LatencyStats
  lastReceivedAt: string
}

// AggregateStats matches Go's AggregateStats JSON: latency is nested under "latencyMs".
export interface AggregateStats {
  robotCount: number
  totalMsgCount: number
  totalRateHz: number
  totalKbps: number
  latencyMs: LatencyStats
}

export interface TelemetryStatsResponse {
  robots: Record<string, RobotStatsEntry>
  aggregate: AggregateStats
}

export interface AllTelemetryEntry {
  receivedAt: string
  payload: TelemetryPayload
}

// ── Robot Fleet ───────────────────────────────────────────────────────────────

export interface RobotConfig {
  id: string
  networkPreset: string
}

export interface FleetConfig {
  payloadType: 'basic' | 'imu'
  payloadHz: number
  robots: RobotConfig[]
}

export interface TelemetryResponse {
  receivedAt: string
  payload: TelemetryPayload
}

// ── Orchestration ─────────────────────────────────────────────────────────────

export interface OrchResult {
  provider: { systemName: string; address: string; port: number }
  service: { serviceDefinition: string; serviceUri: string; interfaces: string[] }
}

export interface OrchResponse {
  response: OrchResult[]
}
