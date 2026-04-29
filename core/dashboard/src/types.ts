// Types mirror SPEC.md and the AH5 core system APIs exactly.

// ── ServiceRegistry ──────────────────────────────────────────────────────────

export interface System {
  systemName: string
  address: string
  port: number
  authenticationInfo?: string
}

export interface ServiceInstance {
  id: number
  serviceDefinition: string
  providerSystem: System
  serviceUri: string
  interfaces: string[]
  version: number
  metadata?: Record<string, string>
  secure?: string
}

export interface QueryResponse {
  serviceQueryData: ServiceInstance[]
  unfilteredHits: number
}

// ── ConsumerAuthorization ────────────────────────────────────────────────────

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

export interface VerifyResponse {
  authorized: boolean
  ruleId?: number
}

// ── Orchestration (shared) ───────────────────────────────────────────────────

export interface OrchSystem {
  systemName: string
  address: string
  port: number
}

export interface ServiceFilter {
  serviceDefinition: string
  interfaces?: string[]
  metadata?: Record<string, string>
}

export interface OrchestrationRequest {
  requesterSystem: OrchSystem
  requestedService: ServiceFilter
}

export interface ServiceInfo {
  serviceDefinition: string
  serviceUri: string
  interfaces: string[]
  version?: number
  metadata?: Record<string, string>
}

export interface OrchestrationResult {
  provider: OrchSystem
  service: ServiceInfo
  priority?: number
}

export interface OrchestrationResponse {
  response: OrchestrationResult[]
}

// ── SimpleStore / FlexibleStore rules ────────────────────────────────────────

export interface StoreRule {
  id: number
  consumerSystemName: string
  serviceDefinition: string
  provider: OrchSystem
  serviceUri: string
  interfaces: string[]
  metadata?: Record<string, string>
}

export interface FlexibleRule extends StoreRule {
  priority: number
  metadataFilter?: Record<string, string>
}

export interface RulesResponse {
  rules: StoreRule[] | FlexibleRule[]
  count: number
}

// ── System health ────────────────────────────────────────────────────────────

export interface HealthResponse {
  status: string
  system?: string
}

export interface SystemStatus {
  name: string
  url: string
  healthy: boolean | null  // null = not yet checked
}
