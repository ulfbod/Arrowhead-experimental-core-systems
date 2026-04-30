// Lists all services currently registered in the ServiceRegistry.

import { usePolling } from '../hooks/usePolling'
import { fetchAllServices } from '../api'
import type { ServiceInstance } from '../types'

function Row({ svc }: { svc: ServiceInstance }) {
  return (
    <tr>
      <td style={s.td}>{svc.serviceDefinition}</td>
      <td style={s.td}>{svc.providerSystem.systemName}</td>
      <td style={s.td}>{svc.providerSystem.address}:{svc.providerSystem.port}</td>
      <td style={s.td}>{svc.serviceUri}</td>
      <td style={s.td}>{svc.interfaces.join(', ')}</td>
    </tr>
  )
}

export function ServiceList() {
  const { data, error, loading, stale } = usePolling(fetchAllServices, 10_000)

  const services: ServiceInstance[] = data?.serviceQueryData ?? []

  return (
    <section style={s.section}>
      <h2 style={s.heading}>
        Registered Services
        {stale && <span style={s.stale}> (stale)</span>}
        {loading && data === null && <span style={s.dim}> loading…</span>}
      </h2>

      {error && !stale && <p style={s.err}>{error}</p>}

      {services.length === 0 && !loading
        ? <p style={s.dim}>No services registered.</p>
        : (
          <div style={s.scroll}>
            <table style={s.table}>
              <thead>
                <tr>
                  {['Service', 'System', 'Address', 'URI', 'Interfaces'].map(h => (
                    <th key={h} style={s.th}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {services.map(svc => <Row key={svc.id} svc={svc} />)}
              </tbody>
            </table>
          </div>
        )
      }
    </section>
  )
}

const s = {
  section: { marginTop: 24 },
  heading: { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  scroll:  { overflowX: 'auto' as const },
  table:   { width: '100%', borderCollapse: 'collapse' as const, fontSize: '0.8rem' },
  th:      { textAlign: 'left' as const, padding: '4px 8px', borderBottom: '2px solid #ddd', whiteSpace: 'nowrap' as const },
  td:      { padding: '4px 8px', borderBottom: '1px solid #eee', whiteSpace: 'nowrap' as const },
  err:     { color: '#f44336', fontSize: '0.8rem' },
  dim:     { color: '#999', fontSize: '0.8rem' },
  stale:   { color: '#ff9800', fontWeight: 'normal' as const, fontSize: '0.75rem' },
}
