// Lists all ConsumerAuthorization rules.

import { usePolling } from '../hooks/usePolling'
import { fetchAuthRules } from '../api'
import type { AuthRule } from '../types'

function Row({ rule }: { rule: AuthRule }) {
  return (
    <tr>
      <td style={s.td}>{rule.consumerSystemName}</td>
      <td style={s.td}>{rule.providerSystemName}</td>
      <td style={s.td}>{rule.serviceDefinition}</td>
    </tr>
  )
}

export function AuthRulesList() {
  const { data, error, loading, stale } = usePolling(fetchAuthRules, 15_000)

  const rules: AuthRule[] = data?.rules ?? []

  return (
    <section style={s.section}>
      <h2 style={s.heading}>
        Authorization Rules
        {stale && <span style={s.stale}> (stale)</span>}
        {loading && data === null && <span style={s.dim}> loading…</span>}
      </h2>

      {error && !stale && <p style={s.err}>{error}</p>}

      {rules.length === 0 && !loading
        ? <p style={s.dim}>No rules.</p>
        : (
          <table style={s.table}>
            <thead>
              <tr>
                {['Consumer', 'Provider', 'Service'].map(h => (
                  <th key={h} style={s.th}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rules.map(r => <Row key={r.id} rule={r} />)}
            </tbody>
          </table>
        )
      }
    </section>
  )
}

const s = {
  section: { marginTop: 24 },
  heading: { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  table:   { width: '100%', borderCollapse: 'collapse' as const, fontSize: '0.8rem' },
  th:      { textAlign: 'left' as const, padding: '4px 8px', borderBottom: '2px solid #ddd' },
  td:      { padding: '4px 8px', borderBottom: '1px solid #eee' },
  err:     { color: '#f44336', fontSize: '0.8rem' },
  dim:     { color: '#999', fontSize: '0.8rem' },
  stale:   { color: '#ff9800', fontWeight: 'normal' as const, fontSize: '0.75rem' },
}
