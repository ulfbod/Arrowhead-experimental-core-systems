// All API calls follow SPEC.md and the AH5 core system contracts exactly.

import type {
  QueryResponse, OrchestrationRequest, OrchestrationResponse,
  LookupResponse, VerifyResponse, AuthRule, StoreRule, FlexibleRule,
  RulesResponse, ServiceInstance, HealthResponse,
} from './types'

// ── Helpers ───────────────────────────────────────────────────────────────────

async function post<T>(url: string, body: object): Promise<T> {
  const resp = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const data = await resp.json() as { error?: string }
  if (!resp.ok) throw new Error(data.error ?? `HTTP ${resp.status}`)
  return data as unknown as T
}

async function get<T>(url: string): Promise<T> {
  const resp = await fetch(url)
  const data = await resp.json() as { error?: string }
  if (!resp.ok) throw new Error(data.error ?? `HTTP ${resp.status}`)
  return data as unknown as T
}

async function del(url: string, body?: object): Promise<void> {
  const resp = await fetch(url, {
    method: 'DELETE',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!resp.ok) {
    const data = await resp.json() as { error?: string }
    throw new Error(data.error ?? `HTTP ${resp.status}`)
  }
}

// ── ServiceRegistry ───────────────────────────────────────────────────────────

export async function queryAll(): Promise<QueryResponse> {
  return post('/serviceregistry/query', {})
}

export async function registerService(body: object): Promise<ServiceInstance> {
  return post('/serviceregistry/register', body)
}

export async function unregisterService(body: object): Promise<void> {
  return del('/serviceregistry/unregister', body)
}

// ── ConsumerAuthorization ─────────────────────────────────────────────────────

export async function lookupRules(consumer = '', provider = '', service = ''): Promise<LookupResponse> {
  const params = new URLSearchParams()
  if (consumer) params.set('consumer', consumer)
  if (provider) params.set('provider', provider)
  if (service)  params.set('service', service)
  return get(`/consumerauthorization/authorization/lookup${params.size ? '?' + params : ''}`)
}

export async function grantRule(body: { consumerSystemName: string; providerSystemName: string; serviceDefinition: string }): Promise<AuthRule> {
  return post('/consumerauthorization/authorization/grant', body)
}

export async function revokeRule(id: number): Promise<void> {
  return del(`/consumerauthorization/authorization/revoke/${id}`)
}

export async function verifyAuthorization(body: { consumerSystemName: string; providerSystemName: string; serviceDefinition: string }): Promise<VerifyResponse> {
  return post('/consumerauthorization/authorization/verify', body)
}

// ── DynamicOrchestration ──────────────────────────────────────────────────────

export async function orchestrateDynamic(req: OrchestrationRequest): Promise<OrchestrationResponse> {
  return post('/serviceorchestration/orchestration/pull', req)
}

// ── SimpleStoreOrchestration ──────────────────────────────────────────────────

export async function orchestrateSimpleStore(req: OrchestrationRequest): Promise<OrchestrationResponse> {
  // /simplestore-pull is a dev-proxy virtual path that rewrites to the simplestore backend
  return post('/simplestore-pull', req)
}

export async function listSimpleStoreRules(): Promise<RulesResponse> {
  return post('/serviceorchestration/orchestration/mgmt/simple-store/query', {})
}

export async function createSimpleStoreRule(body: object): Promise<StoreRule> {
  return post('/serviceorchestration/orchestration/mgmt/simple-store/create', body)
}

export async function deleteSimpleStoreRule(id: string): Promise<void> {
  return del(`/serviceorchestration/orchestration/simplestore/rules/${id}`)
}

export async function modifySimpleStorePriorities(priorities: Record<string, number>): Promise<RulesResponse> {
  return post('/serviceorchestration/orchestration/mgmt/simple-store/modify-priorities', { priorities })
}

// ── FlexibleStoreOrchestration ────────────────────────────────────────────────

export async function orchestrateFlexibleStore(req: OrchestrationRequest): Promise<OrchestrationResponse> {
  // /flexiblestore-pull is a dev-proxy virtual path that rewrites to the flexiblestore backend
  return post('/flexiblestore-pull', req)
}

export async function listFlexibleStoreRules(): Promise<RulesResponse> {
  return get('/serviceorchestration/orchestration/flexiblestore/rules')
}

export async function createFlexibleStoreRule(body: object): Promise<FlexibleRule> {
  return post('/serviceorchestration/orchestration/flexiblestore/rules', body)
}

export async function deleteFlexibleStoreRule(id: number): Promise<void> {
  return del(`/serviceorchestration/orchestration/flexiblestore/rules/${id}`)
}

// ── Health ────────────────────────────────────────────────────────────────────

export async function checkHealth(baseUrl: string): Promise<HealthResponse> {
  const resp = await fetch(`${baseUrl}/health`)
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  return resp.json() as Promise<HealthResponse>
}
