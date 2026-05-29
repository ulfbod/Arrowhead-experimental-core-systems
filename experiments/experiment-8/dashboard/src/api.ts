// API layer for the experiment-8 dashboard.
//
// All paths use the /api/<prefix>/ convention so both the Vite dev proxy
// and the nginx Docker proxy route them to the correct backend.
//
// Key differences from experiment-7:
//   - CA:            /api/profile-ca/  → profile-ca:8087 (plain HTTP only)
//   - REST PEP:      /api/pki-rest-authz/ → pki-rest-authz:9109 (HTTP health port)
//   - REST consumer: /api/pki-consumer/   → pki-consumer:9107
//   - kafka-authz:   port 9101 (was 9091 in exp-7)
//   - policy-sync:   port 9105 (was 9095 in exp-7)

import type {
  HealthProbe,
  LookupResponse,
  AuthRule,
  RabbitUser,
  RabbitTopicPermission,
  RabbitQueue,
  ConsumerStats,
  AnalyticsStats,
  PKIConsumerStats,
  PolicySyncStatus,
  KafkaAuthzStatus,
  PKIRestAuthzStatus,
  AuthCheckResult,
  DataProviderStats,
  CAInfo,
  IssuedCert,
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
  return get<LookupResponse>('/api/consumerauth/consumerauthorization/authorization/lookup', { signal })
}

export async function addGrant(
  consumerSystemName: string,
  providerSystemName: string,
  serviceDefinition: string,
): Promise<AuthRule> {
  const resp = await fetch('/api/consumerauth/consumerauthorization/authorization/grant', {
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
  const resp = await fetch(`/api/consumerauth/consumerauthorization/authorization/revoke/${id}`, { method: 'DELETE' })
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

export function fetchPKIConsumerStats(signal?: AbortSignal): Promise<PKIConsumerStats> {
  return get<PKIConsumerStats>('/api/pki-consumer/stats', { signal })
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

export function fetchPKIRestAuthzStatus(signal?: AbortSignal): Promise<PKIRestAuthzStatus> {
  return get<PKIRestAuthzStatus>('/api/pki-rest-authz/status', { signal })
}

// ── Authorization checks ──────────────────────────────────────────────────────

export async function checkKafkaAuthz(
  consumer: string,
  service: string,
  signal?: AbortSignal,
): Promise<AuthCheckResult> {
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
  return resp.json() as Promise<AuthCheckResult>
}

// pki-rest-authz /auth/check is on the plain HTTP port (9109), not the mTLS port (9108).
// Consumer identity is provided explicitly in the JSON body — this endpoint is for
// dashboard/test use only; the real mTLS path reads identity from the client cert CN.
export async function checkPKIRestAuthz(
  consumer: string,
  service: string,
  signal?: AbortSignal,
): Promise<AuthCheckResult> {
  const resp = await fetch('/api/pki-rest-authz/auth/check', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ consumer, service }),
    signal,
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`HTTP ${resp.status}: ${text}`)
  }
  return resp.json() as Promise<AuthCheckResult>
}

// ── data-provider-tls ─────────────────────────────────────────────────────────

export function fetchDataProviderStats(signal?: AbortSignal): Promise<DataProviderStats> {
  return get<DataProviderStats>('/api/data-provider-tls/stats', { signal })
}

// ── profile-ca (Arrowhead 5.2 Local Cloud CA) ─────────────────────────────────

export function fetchCAInfo(signal?: AbortSignal): Promise<CAInfo> {
  return get<CAInfo>('/api/profile-ca/ca/info', { signal })
}

// Issue an onboarding cert via the plain HTTP bootstrap endpoint (no auth required).
// This is the only cert step available from the browser — device and system cert
// issuance requires mTLS (presenting the prior-profile cert) and cannot be
// initiated directly from the browser.
export async function issueOnboardingCert(
  systemName: string,
  signal?: AbortSignal,
): Promise<IssuedCert> {
  const resp = await fetch('/api/profile-ca/bootstrap/onboarding-cert', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ systemName }),
    signal,
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`HTTP ${resp.status}: ${text}`)
  }
  return resp.json() as Promise<IssuedCert>
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
