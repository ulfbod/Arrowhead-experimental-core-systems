// Shows the current DynamicOrchestration result for demo-consumer → telemetry.
// This visualises the late-binding discovery the consumer service performs.

import { usePolling } from '../hooks/usePolling'
import { useConfig } from '../config/context'
import { fetchOrchestration } from '../api'

export function OrchestrationTrace() {
  const { config } = useConfig()
  const { consumerName, serviceDefinition } = config.experiment2
  const { orchIntervalMs } = config.polling

  const { data, error, loading, stale } = usePolling(
    signal => fetchOrchestration(consumerName, serviceDefinition, signal),
    orchIntervalMs,
  )

  const results = data?.response ?? []

  return (
    <section style={s.section}>
      <h2 style={s.heading}>
        Orchestration Trace
        <span style={s.sub}> — {consumerName} → {serviceDefinition}</span>
        {stale && <span style={s.stale}> (stale)</span>}
        {loading && data === null && <span style={s.dim}> loading…</span>}
      </h2>

      <div style={s.flow}>
        <span style={s.box}>demo-consumer</span>
        <span style={s.arrow}>→ DynamicOrchestration →</span>
        {results.length === 0
          ? <span style={s.noResult}>no providers found</span>
          : results.map((r, i) => (
              <span key={i} style={s.providerBox}>
                {r.provider.systemName}@{r.provider.address}:{r.provider.port}{r.service.serviceUri}
              </span>
            ))
        }
      </div>

      {error && !stale && <p style={s.err}>{error}</p>}

      {results.length > 0 && (
        <table style={s.table}>
          <thead>
            <tr>
              {['Provider', 'Address', 'Port', 'URI', 'Interfaces'].map(h => (
                <th key={h} style={s.th}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {results.map((r, i) => (
              <tr key={i}>
                <td style={s.td}>{r.provider.systemName}</td>
                <td style={s.td}>{r.provider.address}</td>
                <td style={s.td}>{r.provider.port}</td>
                <td style={s.td}>{r.service.serviceUri}</td>
                <td style={s.td}>{r.service.interfaces.join(', ')}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  )
}

const s = {
  section:     { marginTop: 24 },
  heading:     { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  sub:         { fontWeight: 'normal' as const, color: '#999' },
  flow:        {
    display: 'flex', alignItems: 'center', flexWrap: 'wrap' as const,
    gap: 8, padding: '8px 12px', background: '#f3f3f3',
    borderRadius: 4, marginBottom: 12, fontSize: '0.8rem',
  },
  box:         {
    background: '#fff', border: '1px solid #bbb',
    borderRadius: 3, padding: '2px 8px',
  },
  providerBox: {
    background: '#e3f2fd', border: '1px solid #90caf9',
    borderRadius: 3, padding: '2px 8px',
  },
  arrow:       { color: '#888' },
  noResult:    { color: '#999', fontStyle: 'italic' as const },
  table:       { width: '100%', borderCollapse: 'collapse' as const, fontSize: '0.8rem' },
  th:          { textAlign: 'left' as const, padding: '4px 8px', borderBottom: '2px solid #ddd', whiteSpace: 'nowrap' as const },
  td:          { padding: '4px 8px', borderBottom: '1px solid #eee' },
  err:         { color: '#f44336', fontSize: '0.8rem' },
  dim:         { color: '#999', fontSize: '0.8rem' },
  stale:       { color: '#ff9800', fontWeight: 'normal' as const, fontSize: '0.75rem' },
}
