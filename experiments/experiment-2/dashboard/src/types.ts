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
  healthPath: string  // full /api/... path to GET /health
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

export interface TelemetryPayload {
  robotId: string
  temperature: number
  humidity: number
  timestamp: string
  seq: number
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
