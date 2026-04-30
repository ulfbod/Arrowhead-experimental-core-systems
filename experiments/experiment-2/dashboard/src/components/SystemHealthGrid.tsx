// Displays a health-check grid for all systems.
// Each system is polled independently; the interval comes from dashboard config.

import { useConfig } from '../config/context'
import { usePolling } from '../hooks/usePolling'
import { fetchHealthProbe } from '../api'
import { StatusDot } from './StatusDot'
import type { SystemDef, HealthProbe } from '../types'

export const ALL_SYSTEMS: SystemDef[] = [
  { id: 'serviceregistry', label: 'ServiceRegistry', healthPath: '/api/sr',           layer: 'core' },
  { id: 'consumerauth',    label: 'ConsumerAuth',    healthPath: '/api/consumerauth',  layer: 'core',    dependsOn: [] },
  { id: 'dynamicorch',     label: 'DynamicOrch',     healthPath: '/api/dynamicorch',   layer: 'core',    dependsOn: ['serviceregistry', 'consumerauth'] },
  { id: 'ca',              label: 'CA',              healthPath: '/api/ca',            layer: 'core' },
  { id: 'rabbitmq',        label: 'RabbitMQ',        healthPath: '/api/rabbitmq',      layer: 'support' },
  { id: 'edge-adapter',    label: 'EdgeAdapter',     healthPath: '/api/telemetry',     layer: 'experiment', dependsOn: ['serviceregistry', 'ca', 'rabbitmq'] },
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
  const { data: probe, loading } = usePolling<HealthProbe>(
    signal => fetchHealthProbe(sys.healthPath, signal),
    intervalMs,
  )

  const status = loading && probe === null ? 'loading' : probeToStatus(probe)

  return (
    <div style={{ ...s.card, ...(LAYER_STYLE[sys.layer] ?? {}) }} data-testid={`health-card-${sys.id}`}>
      <StatusDot status={status} />
      <span style={s.label}>{sys.label}</span>
      {showLatency && probe && probe.latencyMs > 0 && (
        <span style={s.latency}>{probe.latencyMs}ms</span>
      )}
      {probe?.status === 'down' && <span style={s.err}>down</span>}
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
        {ALL_SYSTEMS.map(sys => (
          <SystemCard
            key={sys.id}
            sys={sys}
            intervalMs={healthIntervalMs}
            showLatency={showHealthLatency}
          />
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
