// Types for the experiment-7 dashboard.

// ── Core system health ────────────────────────────────────────────────────────

export interface HealthResponse {
  status: string
}

export type HealthStatus = 'ok' | 'error' | 'loading'

export interface SystemDef {
  id:    string
  label: string
  healthPath: string
  healthFetcher?: (signal: AbortSignal) => Promise<HealthProbe>
  layer: 'core' | 'support' | 'experiment'
}

export interface HealthProbe {
  status:    'ok' | 'degraded' | 'down'
  latencyMs: number
  error?:    string
}

// ── ConsumerAuthorization ─────────────────────────────────────────────────────

export interface AuthRule {
  id:                  number
  consumerSystemName:  string
  providerSystemName:  string
  serviceDefinition:   string
}

export interface LookupResponse {
  rules: AuthRule[]
  count: number
}

// ── RabbitMQ management API ───────────────────────────────────────────────────

export interface RabbitUser {
  name: string
  tags: string[]
}

export interface RabbitTopicPermission {
  user:     string
  vhost:    string
  exchange: string
  write:    string
  read:     string
}

export interface RabbitQueue {
  name:      string
  messages:  number
  consumers: number
  message_stats?: {
    deliver_details?: { rate: number }
    publish_details?: { rate: number }
  }
}

// ── Consumer stats (from /stats endpoint on each consumer service) ─────

export interface ConsumerStats {
  name:           string
  msgCount:       number
  lastReceivedAt: string
}

export interface AnalyticsStats extends ConsumerStats {
  transport:    string
  lastDeniedAt: string
}

export interface CertConsumerStats extends ConsumerStats {
  transport:    string   // "rest-mtls"
  deniedCount:  number
  lastDeniedAt: string
}

// ── policy-sync status ────────────────────────────────────────────────────────

export interface PolicySyncStatus {
  synced:       boolean
  version:      number
  lastSyncedAt: string
  grants:       number
  syncInterval: string
  error?:       string
}

// ── kafka-authz status ────────────────────────────────────────────────────────

export interface KafkaAuthzStatus {
  totalServed:    number
  activeStreams:  number
}

// ── cert-rest-authz status (HTTP port 9099) ───────────────────────────────────

export interface CertRestAuthzStatus {
  requestsTotal: number
  permitted:     number
  denied:        number
}

// ── kafka-authz / cert-rest-authz authorization check ────────────────────────

export interface AuthCheckResult {
  consumer: string
  service:  string
  permit:   boolean
  decision: string
}

// ── data-provider-tls ─────────────────────────────────────────────────────────

export interface DataProviderStats {
  msgCount:       number
  robotCount:     number
  lastReceivedAt: string
}

// ── CertificateAuthority ──────────────────────────────────────────────────────

export interface CAInfo {
  commonName:  string
  certificate: string
}

export interface IssuedCert {
  systemName:  string
  certificate: string
  privateKey:  string
  issuedAt:    string
  expiresAt:   string
}

export interface VerifyCertResult {
  valid:      boolean
  systemName: string
  reason:     string
}

// ── ServiceRegistry query ─────────────────────────────────────────────────────

export interface ServiceRegistryProvider {
  systemName: string
  address:    string
  port:       number
}

export interface ServiceInstance {
  id:                number
  serviceDefinition: string
  providerSystem:    ServiceRegistryProvider
  serviceUri:        string
  interfaces:        string[]
  version:           number
}

export interface ServiceQueryResponse {
  serviceQueryData: ServiceInstance[]  // spec field name (not "serviceInstances")
  unfilteredHits:   number
}
