// Live message counts for all consumers across both transport paths:
//   - AMQP consumers (consumer-1/2/3): /stats + RabbitMQ queue rates
//   - Kafka analytics consumer: /stats (SSE path via kafka-authz)

import { usePolling } from '../hooks/usePolling'
import { fetchConsumerStats, fetchQueues, fetchAnalyticsStats } from '../api'
import type { ConsumerStats, AnalyticsStats, RabbitQueue } from '../types'

const AMQP_CONSUMERS = [
  { label: 'Consumer-1', path: '/api/consumer-1', queue: 'demo-consumer-1-queue' },
  { label: 'Consumer-2', path: '/api/consumer-2', queue: 'demo-consumer-2-queue' },
  { label: 'Consumer-3', path: '/api/consumer-3', queue: 'demo-consumer-3-queue' },
]

function formatTime(iso: string): string {
  if (!iso) return '—'
  try { return new Date(iso).toLocaleTimeString() } catch { return iso }
}

function AmqpConsumerCard({
  label, path, queue, queueMap, intervalMs,
}: {
  label: string
  path:  string
  queue: string
  queueMap: Map<string, RabbitQueue>
  intervalMs: number
}) {
  const fetcher = (signal: AbortSignal) => fetchConsumerStats(path, signal)
  const { data: stats, error } = usePolling<ConsumerStats>(fetcher, intervalMs)
  const q = queueMap.get(queue)
  const rate = q?.message_stats?.deliver_details?.rate

  return (
    <div style={s.card}>
      <div style={s.cardTitle}>{label}</div>
      <div style={s.transport}>AMQP / RabbitMQ</div>
      {error
        ? <div style={s.err}>unavailable</div>
        : (
          <>
            <div style={s.stat}>
              <span style={s.statLabel}>messages received</span>
              <span style={s.statValue}>{stats?.msgCount ?? '…'}</span>
            </div>
            <div style={s.stat}>
              <span style={s.statLabel}>last received</span>
              <span style={s.statValue}>{stats ? formatTime(stats.lastReceivedAt) : '…'}</span>
            </div>
            <div style={s.stat}>
              <span style={s.statLabel}>deliver rate</span>
              <span style={s.statValue}>
                {rate !== undefined ? `${rate.toFixed(1)} msg/s` : '—'}
              </span>
            </div>
            <div style={s.stat}>
              <span style={s.statLabel}>queue depth</span>
              <span style={s.statValue}>{q?.messages ?? '—'}</span>
            </div>
          </>
        )
      }
    </div>
  )
}

function AnalyticsConsumerCard({ intervalMs }: { intervalMs: number }) {
  const fetcher = (signal: AbortSignal) => fetchAnalyticsStats(signal)
  const { data: stats, error } = usePolling<AnalyticsStats>(fetcher, intervalMs)

  return (
    <div style={{ ...s.card, borderColor: '#a5f3fc' }}>
      <div style={s.cardTitle}>Analytics Consumer</div>
      <div style={{ ...s.transport, color: '#0891b2' }}>Kafka / SSE</div>
      {error
        ? <div style={s.err}>unavailable</div>
        : (
          <>
            <div style={s.stat}>
              <span style={s.statLabel}>messages received</span>
              <span style={s.statValue}>{stats?.msgCount ?? '…'}</span>
            </div>
            <div style={s.stat}>
              <span style={s.statLabel}>last received</span>
              <span style={s.statValue}>{stats ? formatTime(stats.lastReceivedAt) : '…'}</span>
            </div>
            <div style={s.stat}>
              <span style={s.statLabel}>last denied at</span>
              <span style={s.statValue}>{stats?.lastDeniedAt ? formatTime(stats.lastDeniedAt) : '—'}</span>
            </div>
          </>
        )
      }
    </div>
  )
}

export function ConsumerStatsPanel() {
  const { data: queues } = usePolling<RabbitQueue[]>(fetchQueues, 3_000)
  const queueMap = new Map<string, RabbitQueue>()
  for (const q of queues ?? []) queueMap.set(q.name, q)

  return (
    <section style={s.section}>
      <h2 style={s.heading}>Live Consumer Data</h2>
      <p style={s.hint}>Message counts and delivery rates, refreshed every 3 s.</p>
      <div style={s.grid}>
        {AMQP_CONSUMERS.map(c => (
          <AmqpConsumerCard key={c.label} {...c} queueMap={queueMap} intervalMs={3_000} />
        ))}
        <AnalyticsConsumerCard intervalMs={3_000} />
      </div>
    </section>
  )
}

const s: Record<string, React.CSSProperties> = {
  section:    { marginBottom: 28 },
  heading:    { fontSize: '0.9rem', marginBottom: 6, color: '#555' },
  hint:       { color: '#888', fontSize: '0.75rem', marginBottom: 12 },
  grid:       { display: 'flex', gap: 16, flexWrap: 'wrap' },
  card:       {
    background: '#fff', border: '1px solid #c4b5fd', borderRadius: 6,
    padding: '12px 16px', minWidth: 200, flex: '1 1 200px',
  },
  cardTitle:  { fontWeight: 'bold', fontSize: '0.85rem', marginBottom: 4, color: '#1a1a2e' },
  transport:  { fontSize: '0.7rem', color: '#7c3aed', marginBottom: 8 },
  stat:       { display: 'flex', justifyContent: 'space-between', gap: 8, marginBottom: 4, fontSize: '0.78rem' },
  statLabel:  { color: '#888' },
  statValue:  { fontWeight: 'bold', color: '#333', fontFamily: 'monospace' },
  err:        { color: '#f44336', fontSize: '0.78rem' },
}
