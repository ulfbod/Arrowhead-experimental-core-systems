// API layer for the experiment-6 dashboard.
//
// All paths use the /api/<prefix>/ convention so both the Vite dev proxy
// and the nginx Docker proxy route them to the correct backend.

import type {
  HealthProbe,
  LookupResponse,
  AuthRule,
  RabbitUser,
  RabbitTopicPermission,
  RabbitQueue,
  ConsumerStats,
  AnalyticsStats,
  RestConsumerStats,
  PolicySyncStatus,
  KafkaAuthzStatus,
  RestAuthzStatus,
  KafkaAuthCheckResult,
  DataProviderStats,
  ServiceQueryResponse,
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

// RabbitMQ management API requires Basic auth.
const RMQ_AUTH = 'Basic ' + btoa('admin:admin')

function rmqGet<T>(path: string, signal?: AbortSignal): Promise<T> {
  return get<T>(`/api/rabbitmq${path}`, {
    headers: { Authorization: RMQ_AUTH },
    signal,
  })
}

// ── Health probes ─────────────────────────────────────────────────────────────

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

export async function fetchRabbitMQHealth(signal?: AbortSignal): Promise<HealthProbe> {
  const start = Date.now()
  try {
    const resp = await fetch('/api/rabbitmq/api/overview', {
      headers: { Authorization: RMQ_AUTH },
      signal,
    })
    await resp.text()
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    return { status: 'ok', latencyMs: Date.now() - start }
  } catch (err) {
    return { status: 'down', latencyMs: Date.now() - start, error: String(err) }
  }
}

// ── ConsumerAuthorization ─────────────────────────────────────────────────────

export function fetchAuthRules(signal?: AbortSignal): Promise<LookupResponse> {
  return get<LookupResponse>('/api/consumerauth/authorization/lookup', { signal })
}

export async function addGrant(
  consumerSystemName: string,
  providerSystemName: string,
  serviceDefinition: string,
): Promise<AuthRule> {
  const resp = await fetch('/api/consumerauth/authorization/grant', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ consumerSystemName, providerSystemName, serviceDefinition }),
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`HTTP ${resp.status}: ${text}`)
  }
  return resp.json() as Promise<AuthRule>
}

export async function revokeGrant(id: number): Promise<void> {
  const resp = await fetch(`/api/consumerauth/authorization/revoke/${id}`, { method: 'DELETE' })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`HTTP ${resp.status}: ${text}`)
  }
}

// ── RabbitMQ ──────────────────────────────────────────────────────────────────

export function fetchRabbitUsers(signal?: AbortSignal): Promise<RabbitUser[]> {
  return rmqGet<RabbitUser[]>('/api/users', signal)
}

export function fetchTopicPermissions(signal?: AbortSignal): Promise<RabbitTopicPermission[]> {
  return rmqGet<RabbitTopicPermission[]>('/api/topic-permissions', signal)
}

export function fetchQueues(signal?: AbortSignal): Promise<RabbitQueue[]> {
  return rmqGet<RabbitQueue[]>('/api/queues', signal)
}

// ── Consumer stats ────────────────────────────────────────────────────────────

export function fetchConsumerStats(healthPath: string, signal?: AbortSignal): Promise<ConsumerStats> {
  return get<ConsumerStats>(`${healthPath}/stats`, { signal })
}

export function fetchAnalyticsStats(signal?: AbortSignal): Promise<AnalyticsStats> {
  return get<AnalyticsStats>('/api/analytics-consumer/stats', { signal })
}

export function fetchRestConsumerStats(signal?: AbortSignal): Promise<RestConsumerStats> {
  return get<RestConsumerStats>('/api/rest-consumer/stats', { signal })
}

// ── Policy engine ─────────────────────────────────────────────────────────────

export function fetchPolicySyncStatus(signal?: AbortSignal): Promise<PolicySyncStatus> {
  return get<PolicySyncStatus>('/api/policy-sync/status', { signal })
}

export async function updateSyncInterval(syncInterval: string): Promise<PolicySyncStatus> {
  const resp = await fetch('/api/policy-sync/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ syncInterval }),
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`HTTP ${resp.status}: ${text}`)
  }
  return resp.json() as Promise<PolicySyncStatus>
}

export function fetchKafkaAuthzStatus(signal?: AbortSignal): Promise<KafkaAuthzStatus> {
  return get<KafkaAuthzStatus>('/api/kafka-authz/status', { signal })
}

export function fetchRestAuthzStatus(signal?: AbortSignal): Promise<RestAuthzStatus> {
  return get<RestAuthzStatus>('/api/rest-authz/status', { signal })
}

// ── Kafka authorization check ─────────────────────────────────────────────────

export async function checkKafkaAuthz(
  consumer: string,
  service: string,
  signal?: AbortSignal,
): Promise<KafkaAuthCheckResult> {
  const resp = await fetch('/api/kafka-authz/auth/check', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ consumer, service }),
    signal,
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`HTTP ${resp.status}: ${text}`)
  }
  return resp.json() as Promise<KafkaAuthCheckResult>
}

// ── data-provider ─────────────────────────────────────────────────────────────

export function fetchDataProviderStats(signal?: AbortSignal): Promise<DataProviderStats> {
  return get<DataProviderStats>('/api/data-provider/stats', { signal })
}

// ── ServiceRegistry query ─────────────────────────────────────────────────────

export async function queryServiceRegistry(
  serviceDefinition: string,
  signal?: AbortSignal,
): Promise<ServiceQueryResponse> {
  const resp = await fetch('/api/serviceregistry/serviceregistry/query', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ serviceDefinition }),
    signal,
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`HTTP ${resp.status}: ${text}`)
  }
  return resp.json() as Promise<ServiceQueryResponse>
}

// ── REST authz live probe ─────────────────────────────────────────────────────

// Sends a request through rest-authz as the given consumer.
// Returns the HTTP status and raw response body so the caller can display
// both the authorization outcome (200 vs 403) and the telemetry payload.
export async function fetchThroughRestAuthz(
  consumerName: string,
  signal?: AbortSignal,
): Promise<{ status: number; body: string }> {
  const resp = await fetch('/api/rest-authz/telemetry/latest', {
    headers: { 'X-Consumer-Name': consumerName },
    signal,
  })
  const body = await resp.text()
  return { status: resp.status, body }
}
