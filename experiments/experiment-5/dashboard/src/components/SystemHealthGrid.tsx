import { useConfig } from '../config/context'
import { usePolling } from '../hooks/usePolling'
import { fetchHealthProbe, fetchRabbitMQHealth } from '../api'
import { StatusDot } from './StatusDot'
import type { SystemDef, HealthProbe } from '../types'

const SYSTEMS: SystemDef[] = [
  // ── Core ───────────────────────────────────────────────────────────────────
  { id: 'serviceregistry', label: 'ServiceRegistry', healthPath: '/api/serviceregistry', layer: 'core' },
  { id: 'authentication',  label: 'Authentication',  healthPath: '/api/authentication',  layer: 'core' },
  { id: 'consumerauth',    label: 'ConsumerAuth',    healthPath: '/api/consumerauth',    layer: 'core' },
  { id: 'dynamicorch',     label: 'DynamicOrch',     healthPath: '/api/dynamicorch',     layer: 'core' },
  { id: 'ca',              label: 'CertAuth',        healthPath: '/api/ca',              layer: 'core' },
  // ── Support ────────────────────────────────────────────────────────────────
  {
    id: 'rabbitmq', label: 'RabbitMQ', healthPath: '/api/rabbitmq',
    healthFetcher: fetchRabbitMQHealth,
    layer: 'support',
  },
  { id: 'authzforce',  label: 'AuthzForce',   healthPath: '/api/authzforce',  layer: 'support' },
  { id: 'kafka',       label: 'Kafka',        healthPath: '/api/kafka-authz', layer: 'support' },
  // ── Experiment ─────────────────────────────────────────────────────────────
  { id: 'policy-sync',       label: 'PolicySync',       healthPath: '/api/policy-sync',       layer: 'experiment' },
  { id: 'topic-auth-xacml',  label: 'TopicAuthXACML',   healthPath: '/api/topic-auth-xacml',  layer: 'experiment' },
  { id: 'kafka-authz',       label: 'KafkaAuthz',       healthPath: '/api/kafka-authz',       layer: 'experiment' },
  { id: 'robot-fleet',       label: 'RobotFleet',       healthPath: '/api/robot-fleet',       layer: 'experiment' },
  { id: 'consumer-1',        label: 'Consumer-1',       healthPath: '/api/consumer-1',        layer: 'experiment' },
  { id: 'consumer-2',        label: 'Consumer-2',       healthPath: '/api/consumer-2',        layer: 'experiment' },
  { id: 'consumer-3',        label: 'Consumer-3',       healthPath: '/api/consumer-3',        layer: 'experiment' },
  { id: 'analytics-consumer', label: 'Analytics',       healthPath: '/api/analytics-consumer', layer: 'experiment' },
]

function probeToStatus(probe: HealthProbe | null): 'ok' | 'error' | 'loading' {
  if (!probe) return 'loading'
  return probe.status === 'ok' ? 'ok' : 'error'
}

function SystemCard({ sys, intervalMs, showLatency }: {
  sys: SystemDef
  intervalMs: number
  showLatency: boolean
}) {
  const healthUrl = `${sys.healthPath}/health`
  const fetcher = sys.healthFetcher
    ? sys.healthFetcher
    : (signal: AbortSignal) => fetchHealthProbe(healthUrl, signal)
  const { data: probe, loading } = usePolling<HealthProbe>(fetcher, intervalMs)
  const status = loading && probe === null ? 'loading' : probeToStatus(probe)

  return (
    <div style={{ ...s.card, ...(LAYER_STYLE[sys.layer] ?? {}) }} data-testid={`health-card-${sys.id}`}>
      <StatusDot status={status} />
      <span style={s.label}>{sys.label}</span>
      {showLatency && probe && probe.latencyMs > 0 && (
        <span style={s.latency}>{probe.latencyMs}ms</span>
      )}
      {probe?.status === 'down' && (
        <span style={s.err} title={probe.error}>down</span>
      )}
    </div>
  )
}

export function SystemHealthGrid() {
  const { config } = useConfig()
  const { healthIntervalMs } = config.polling
  const { showHealthLatency } = config.display

  return (
    <section>
      <h2 style={s.heading}>System Health</h2>
      <div style={s.grid}>
        {SYSTEMS.map(sys => (
          <SystemCard key={sys.id} sys={sys} intervalMs={healthIntervalMs} showLatency={showHealthLatency} />
        ))}
      </div>
      <div style={s.legend}>
        <span style={s.legendItem}><span style={{ ...s.dot, background: COLORS.core }}/>core</span>
        <span style={s.legendItem}><span style={{ ...s.dot, background: COLORS.support }}/>support</span>
        <span style={s.legendItem}><span style={{ ...s.dot, background: COLORS.experiment }}/>experiment</span>
      </div>
    </section>
  )
}

const COLORS = { core: '#dbeafe', support: '#d1fae5', experiment: '#ede9fe' }

const LAYER_STYLE: Record<string, React.CSSProperties> = {
  core:       { borderColor: '#93c5fd' },
  support:    { borderColor: '#6ee7b7' },
  experiment: { borderColor: '#c4b5fd' },
}

const s: Record<string, React.CSSProperties> = {
  heading:    { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  grid:       { display: 'flex', flexWrap: 'wrap', gap: 8 },
  card:       {
    display: 'flex', alignItems: 'center', gap: 4,
    background: '#fff', border: '1px solid #ddd',
    borderRadius: 4, padding: '6px 10px', fontSize: '0.8rem',
  },
  label:      { fontWeight: 'bold' },
  latency:    { color: '#888', fontSize: '0.7rem', marginLeft: 2 },
  err:        { color: '#f44336', fontSize: '0.75rem', marginLeft: 4 },
  legend:     { display: 'flex', gap: 12, marginTop: 8, fontSize: '0.75rem', color: '#666' },
  legendItem: { display: 'flex', alignItems: 'center', gap: 4 },
  dot:        { display: 'inline-block', width: 10, height: 10, borderRadius: 2 },
}
