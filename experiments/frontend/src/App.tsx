import { useEffect, useState } from 'react'

// ── Types matching SPEC.md ────────────────────────────────────────────────────

interface System {
  systemName: string
  address: string
  port: number
  authenticationInfo?: string
}

interface ServiceInstance {
  id: number
  serviceDefinition: string
  providerSystem: System
  serviceUri: string
  interfaces: string[]
  version: number
  metadata?: Record<string, string>
  secure?: string
}

interface QueryResponse {
  serviceQueryData: ServiceInstance[]
  unfilteredHits: number
}

// ── API ───────────────────────────────────────────────────────────────────────

// POST /serviceregistry/query with an empty body returns all registered services.
// Body matches QueryRequest from SPEC.md: all fields optional, empty = no filter.
// Vite proxies this path to http://localhost:8080 (see vite.config.ts).
async function fetchServices(): Promise<QueryResponse> {
  const resp = await fetch('/serviceregistry/query', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({}),
  })
  if (!resp.ok) {
    const body = await resp.json().catch(() => ({}))
    throw new Error((body as { error?: string }).error ?? `HTTP ${resp.status}`)
  }
  return resp.json() as Promise<QueryResponse>
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function App() {
  const [services, setServices] = useState<ServiceInstance[]>([])
  const [unfilteredHits, setUnfilteredHits] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    fetchServices()
      .then(data => {
        setServices(data.serviceQueryData)
        setUnfilteredHits(data.unfilteredHits)
      })
      .catch(e => setError((e as Error).message))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div style={s.page}>
      <h1 style={s.heading}>Arrowhead Service Registry</h1>

      {loading && <p style={s.muted}>Loading…</p>}

      {error && <p style={s.error}>Error: {error}</p>}

      {!loading && !error && services.length === 0 && (
        <p style={s.muted}>No services found.</p>
      )}

      {services.length > 0 && (
        <>
          <p style={s.info}>
            {services.length} service(s) &mdash; unfilteredHits: {unfilteredHits}
          </p>
          <table style={s.table}>
            <thead>
              <tr>
                {COLUMNS.map(col => (
                  <th key={col} style={s.th}>{col}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {services.map(svc => (
                <ServiceRow key={svc.id} svc={svc} />
              ))}
            </tbody>
          </table>
        </>
      )}
    </div>
  )
}

const COLUMNS = [
  'serviceDefinition',
  'systemName',
  'address',
  'port',
  'interfaces',
  'metadata',
]

function ServiceRow({ svc }: { svc: ServiceInstance }) {
  const meta = svc.metadata
    ? Object.entries(svc.metadata).map(([k, v]) => `${k}=${v}`).join(', ')
    : ''

  return (
    <tr>
      <td style={s.td}>{svc.serviceDefinition}</td>
      <td style={s.td}>{svc.providerSystem.systemName}</td>
      <td style={s.td}>{svc.providerSystem.address}</td>
      <td style={s.td}>{svc.providerSystem.port}</td>
      <td style={s.td}>{svc.interfaces.join(', ')}</td>
      <td style={s.td}>{meta}</td>
    </tr>
  )
}

// ── Styles ────────────────────────────────────────────────────────────────────

const s = {
  page:    { fontFamily: 'monospace', maxWidth: 900, margin: '40px auto', padding: '0 20px' },
  heading: { fontSize: '1.1rem', borderBottom: '2px solid #111', paddingBottom: 6, marginBottom: '1.4rem' },
  muted:   { color: '#888', fontSize: '0.9rem' },
  error:   { color: '#c00', fontSize: '0.9rem' },
  info:    { color: '#080', fontSize: '0.85rem', marginBottom: 0 },
  table:   { width: '100%', borderCollapse: 'collapse' as const, fontSize: '0.85rem', marginTop: 12 },
  th:      { textAlign: 'left' as const, borderBottom: '2px solid #111', padding: '4px 8px', whiteSpace: 'nowrap' as const },
  td:      { padding: '5px 8px', borderBottom: '1px solid #ddd', verticalAlign: 'top' as const },
}
