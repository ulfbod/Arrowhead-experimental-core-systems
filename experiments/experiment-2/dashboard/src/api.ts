// API layer for the experiment-2 dashboard.
//
// All paths use the /api/<prefix>/ convention so that both the Vite dev proxy
// and the nginx Docker proxy can route them to the correct backend.

import type {
  HealthResponse,
  HealthProbe,
  QueryResponse,
  LookupResponse,
  TelemetryResponse,
  OrchResponse,
  RabbitQueue,
  RabbitExchange,
  FleetConfig,
  TelemetryStatsResponse,
  AllTelemetryEntry,
} from './types'

// ── Helpers ───────────────────────────────────────────────────────────────────

async function get<T>(url: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(url, init)
  if (!resp.ok) {
    const body = await resp.text()
    throw new Error(`HTTP ${resp.status}: ${body}`)
  }
  return resp.json() as Promise<T>
}

async function post<T>(url: string, body: unknown, init?: RequestInit): Promise<T> {
  const resp = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...init?.headers },
    body: JSON.stringify(body),
    signal: init?.signal,
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`HTTP ${resp.status}: ${text}`)
  }
  return resp.json() as Promise<T>
}

// RabbitMQ management API requires Basic auth.
export const RMQ_AUTH = 'Basic ' + btoa('guest:guest')
function rmqGet<T>(path: string, signal?: AbortSignal): Promise<T> {
  return get<T>(`/api/rabbitmq${path}`, { headers: { Authorization: RMQ_AUTH }, signal })
}

// ── Core system health ────────────────────────────────────────────────────────

export function fetchHealth(apiPrefix: string, signal?: AbortSignal): Promise<HealthResponse> {
  return get<HealthResponse>(`${apiPrefix}/health`, { signal })
}

// fetchHealthProbe measures response latency alongside the health status.
// url is the complete path to GET; any 2xx response is treated as healthy.
// headers are merged into the request (e.g. Authorization for RabbitMQ management API).
export async function fetchHealthProbe(
  url: string,
  signal?: AbortSignal,
  headers?: Record<string, string>,
): Promise<HealthProbe> {
  const start = Date.now()
  try {
    await get<unknown>(url, { signal, headers })
    return { status: 'ok', latencyMs: Date.now() - start }
  } catch (err) {
    return { status: 'down', latencyMs: Date.now() - start, error: String(err) }
  }
}

// ── ServiceRegistry ───────────────────────────────────────────────────────────

export function fetchAllServices(signal?: AbortSignal): Promise<QueryResponse> {
  return post<QueryResponse>('/api/sr/serviceregistry/query', {}, { signal })
}

// ── ConsumerAuthorization ─────────────────────────────────────────────────────

export function fetchAuthRules(signal?: AbortSignal): Promise<LookupResponse> {
  return get<LookupResponse>('/api/consumerauth/authorization/lookup', { signal })
}

// ── RabbitMQ ──────────────────────────────────────────────────────────────────

// fetchRabbitMQHealth checks the management API is reachable.
// Uses a direct fetch (bypassing the shared get() helper) so we never call
// resp.json() on the large overview body — only the HTTP status matters.
export async function fetchRabbitMQHealth(signal?: AbortSignal): Promise<HealthProbe> {
  const start = Date.now()
  try {
    const resp = await fetch('/api/rabbitmq/api/overview', {
      headers: { Authorization: RMQ_AUTH },
      signal,
    })
    await resp.text()  // drain body to release connection
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    return { status: 'ok', latencyMs: Date.now() - start }
  } catch (err) {
    return { status: 'down', latencyMs: Date.now() - start, error: String(err) }
  }
}

export function fetchQueues(signal?: AbortSignal): Promise<RabbitQueue[]> {
  return rmqGet<RabbitQueue[]>('/api/queues', signal)
}

export function fetchExchanges(signal?: AbortSignal): Promise<RabbitExchange[]> {
  return rmqGet<RabbitExchange[]>('/api/exchanges', signal)
}

// ── Telemetry (edge-adapter) ──────────────────────────────────────────────────

// Returns null when no telemetry has been received yet (HTTP 204).
export async function fetchTelemetry(signal?: AbortSignal): Promise<TelemetryResponse | null> {
  const resp = await fetch('/api/telemetry/telemetry/latest', { signal })
  if (resp.status === 204) return null
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  return resp.json() as Promise<TelemetryResponse>
}

// ── Robot Fleet ───────────────────────────────────────────────────────────────

export function fetchFleetConfig(signal?: AbortSignal): Promise<FleetConfig> {
  return get<FleetConfig>('/api/robot-fleet/config', { signal })
}

export async function postFleetConfig(cfg: FleetConfig, signal?: AbortSignal): Promise<void> {
  const resp = await fetch('/api/robot-fleet/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
    signal,
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`HTTP ${resp.status}: ${text}`)
  }
}

export function fetchTelemetryStats(signal?: AbortSignal): Promise<TelemetryStatsResponse> {
  return get<TelemetryStatsResponse>('/api/telemetry/telemetry/stats', { signal })
}

export function fetchAllTelemetry(signal?: AbortSignal): Promise<Record<string, AllTelemetryEntry>> {
  return get<Record<string, AllTelemetryEntry>>('/api/telemetry/telemetry/all', { signal })
}

// ── DynamicOrchestration ──────────────────────────────────────────────────────

// Mimics what the consumer service does on each poll cycle.
// consumerName and serviceDef are driven by dashboard config.
export function fetchOrchestration(
  consumerName: string,
  serviceDef: string,
  signal?: AbortSignal,
): Promise<OrchResponse> {
  return post<OrchResponse>(
    '/api/dynamicorch/orchestration/dynamic',
    {
      requesterSystem: { systemName: consumerName, address: 'localhost', port: 9002 },
      requestedService: { serviceDefinition: serviceDef, interfaces: ['HTTP-INSECURE-JSON'] },
    },
    { signal },
  )
}
