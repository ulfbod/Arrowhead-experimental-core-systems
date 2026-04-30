// Displays RabbitMQ queue and exchange statistics via the management API.

import { usePolling } from '../hooks/usePolling'
import { fetchQueues, fetchExchanges } from '../api'
import type { RabbitQueue, RabbitExchange } from '../types'

function QueueTable({ queues }: { queues: RabbitQueue[] }) {
  if (queues.length === 0) return <p style={s.dim}>No queues.</p>
  return (
    <table style={s.table}>
      <thead>
        <tr>
          {['Queue', 'Messages', 'Consumers', 'Publish rate'].map(h => (
            <th key={h} style={s.th}>{h}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {queues.map(q => (
          <tr key={q.name}>
            <td style={s.td}>{q.name}</td>
            <td style={s.tdNum}>{q.messages}</td>
            <td style={s.tdNum}>{q.consumers}</td>
            <td style={s.tdNum}>
              {q.message_stats?.publish_details?.rate?.toFixed(1) ?? '—'}/s
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function ExchangeTable({ exchanges }: { exchanges: RabbitExchange[] }) {
  const named = exchanges.filter(e => e.name !== '')
  if (named.length === 0) return <p style={s.dim}>No named exchanges.</p>
  return (
    <table style={s.table}>
      <thead>
        <tr>
          {['Exchange', 'Type', 'Durable', 'In rate'].map(h => (
            <th key={h} style={s.th}>{h}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {named.map(e => (
          <tr key={e.name}>
            <td style={s.td}>{e.name}</td>
            <td style={s.td}>{e.type}</td>
            <td style={s.tdNum}>{e.durable ? 'yes' : 'no'}</td>
            <td style={s.tdNum}>
              {e.message_stats?.publish_in_details?.rate?.toFixed(1) ?? '—'}/s
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

export function BrokerStats() {
  const queues    = usePolling(fetchQueues,    5_000)
  const exchanges = usePolling(fetchExchanges, 15_000)

  return (
    <div>
      <section style={s.section}>
        <h2 style={s.heading}>
          Queues
          {queues.stale && <span style={s.stale}> (stale)</span>}
          {queues.loading && queues.data === null && <span style={s.dim}> loading…</span>}
        </h2>
        {queues.error && !queues.stale && <p style={s.err}>{queues.error}</p>}
        <QueueTable queues={queues.data ?? []} />
      </section>

      <section style={s.section}>
        <h2 style={s.heading}>
          Exchanges
          {exchanges.stale && <span style={s.stale}> (stale)</span>}
          {exchanges.loading && exchanges.data === null && <span style={s.dim}> loading…</span>}
        </h2>
        {exchanges.error && !exchanges.stale && <p style={s.err}>{exchanges.error}</p>}
        <ExchangeTable exchanges={exchanges.data ?? []} />
      </section>
    </div>
  )
}

const s = {
  section: { marginTop: 24 },
  heading: { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  table:   { width: '100%', borderCollapse: 'collapse' as const, fontSize: '0.8rem' },
  th:      { textAlign: 'left' as const, padding: '4px 8px', borderBottom: '2px solid #ddd', whiteSpace: 'nowrap' as const },
  td:      { padding: '4px 8px', borderBottom: '1px solid #eee' },
  tdNum:   { padding: '4px 8px', borderBottom: '1px solid #eee', textAlign: 'right' as const },
  err:     { color: '#f44336', fontSize: '0.8rem' },
  dim:     { color: '#999', fontSize: '0.8rem' },
  stale:   { color: '#ff9800', fontWeight: 'normal' as const, fontSize: '0.75rem' },
}
