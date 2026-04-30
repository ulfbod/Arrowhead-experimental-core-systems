// Shows health/orchestration status for all three consumers.

import { useConfig } from '../config/context'
import { usePolling } from '../hooks/usePolling'
import { fetchOrchestration } from '../api'
import { StatusDot } from './StatusDot'
import type { OrchResponse } from '../types'

const CONSUMERS = [
  { name: 'demo-consumer',   label: 'Consumer 1' },
  { name: 'demo-consumer-2', label: 'Consumer 2' },
  { name: 'demo-consumer-3', label: 'Consumer 3' },
]

function ConsumerCard({ name, label, intervalMs, serviceDef }: {
  name: string; label: string; intervalMs: number; serviceDef: string
}) {
  const { data, loading } = usePolling<OrchResponse>(
    (signal) => fetchOrchestration(name, serviceDef, signal),
    intervalMs,
  )

  const resolved = data?.response?.length ?? 0
  const status = loading && !data ? 'loading' : (data ? 'ok' : 'error')

  return (
    <div style={s.card} data-testid={`consumer-card-${name}`}>
      <StatusDot status={status} />
      <div>
        <div style={s.cardLabel}>{label}</div>
        <div style={s.cardName}>{name}</div>
        {data && <div style={s.cardDetail}>Resolved: {resolved} provider{resolved !== 1 ? 's' : ''}</div>}
      </div>
    </div>
  )
}

export function MultiConsumerPanel() {
  const { config } = useConfig()
  const { orchIntervalMs, } = config.polling
  const { serviceDefinition } = config.experiment2

  return (
    <section style={s.section} data-testid="multi-consumer-panel">
      <h2 style={s.heading}>Consumer Status</h2>
      <div style={s.row}>
        {CONSUMERS.map(c => (
          <ConsumerCard
            key={c.name}
            name={c.name}
            label={c.label}
            intervalMs={orchIntervalMs}
            serviceDef={serviceDefinition}
          />
        ))}
      </div>
    </section>
  )
}

const s: Record<string, React.CSSProperties> = {
  section:    { marginBottom: 20 },
  heading:    { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  row:        { display: 'flex', gap: 12, flexWrap: 'wrap' },
  card:       { display: 'flex', gap: 8, alignItems: 'flex-start', background: '#fff', border: '1px solid #c4b5fd', borderRadius: 4, padding: '8px 12px', minWidth: 180 },
  cardLabel:  { fontSize: '0.8rem', fontWeight: 'bold', color: '#333' },
  cardName:   { fontSize: '0.7rem', color: '#888', fontFamily: 'monospace' },
  cardDetail: { fontSize: '0.75rem', color: '#555', marginTop: 2 },
}
