// Types for the experiment-3 dashboard.

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
  tags: string[]  // JSON array in RabbitMQ 3.12+
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

// ── Consumer stats (from /stats endpoint on each consumer-direct service) ─────

export interface ConsumerStats {
  name:           string
  msgCount:       number
  lastReceivedAt: string
}
