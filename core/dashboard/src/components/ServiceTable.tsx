import type { ServiceInstance } from '../types'

interface Props {
  services: ServiceInstance[]
  loading: boolean
  selected: ServiceInstance | null
  onSelect: (s: ServiceInstance) => void
}

const COLS = ['serviceDefinition', 'systemName', 'address', 'port', 'interfaces', 'version', 'metadata'] as const

export default function ServiceTable({ services, loading, selected, onSelect }: Props) {
  if (loading) return <p style={s.muted}>Loading…</p>
  if (services.length === 0) return <p style={s.muted}>No services found.</p>

  return (
    <table style={s.table}>
      <thead>
        <tr>
          {COLS.map(h => <th key={h} style={s.th}>{h}</th>)}
        </tr>
      </thead>
      <tbody>
        {services.map(svc => {
          const isSelected = selected?.id === svc.id
          return (
            <tr
              key={svc.id}
              style={{ cursor: 'pointer', background: isSelected ? '#f0f4ff' : undefined }}
              onClick={() => onSelect(svc)}
            >
              <td style={s.td}>{svc.serviceDefinition}</td>
              <td style={s.td}>{svc.providerSystem.systemName}</td>
              <td style={s.td}>{svc.providerSystem.address}</td>
              <td style={s.td}>{svc.providerSystem.port}</td>
              <td style={s.td}>{svc.interfaces.join(', ')}</td>
              <td style={s.td}>{svc.version}</td>
              <td style={s.td}>
                {svc.metadata && Object.keys(svc.metadata).length > 0
                  ? Object.entries(svc.metadata).map(([k, v]) => `${k}=${v}`).join(', ')
                  : <span style={{ color: '#bbb' }}>—</span>}
              </td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

const s = {
  muted:  { color: '#888', fontSize: '0.9rem' },
  table:  { width: '100%', borderCollapse: 'collapse' as const, fontSize: '0.85rem' },
  th:     { textAlign: 'left' as const, borderBottom: '2px solid #111', padding: '6px 8px', whiteSpace: 'nowrap' as const, background: '#f8f8f8' },
  td:     { padding: '6px 8px', borderBottom: '1px solid #eee', verticalAlign: 'top' as const },
}
