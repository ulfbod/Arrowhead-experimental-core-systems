// Types for the experiment-6 dashboard.

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

export interface RestConsumerStats extends ConsumerStats {
  transport:    string
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

// ── rest-authz status ─────────────────────────────────────────────────────────

export interface RestAuthzStatus {
  requestsTotal: number
  permitted:     number
  denied:        number
}
