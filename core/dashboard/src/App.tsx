import { useEffect, useState } from 'react'
import { queryAll } from './api'
import type { QueryResponse, ServiceInstance } from './types'
import MetricsBar from './components/MetricsBar'
import RegisterForm from './components/RegisterForm'
import ServiceTable from './components/ServiceTable'
import ServiceDetail from './components/ServiceDetail'
import SystemStatus from './components/SystemStatus'
import AuthRulesPanel from './components/AuthRulesPanel'
import OrchestrationPanel from './components/OrchestrationPanel'

export default function App() {
  const [resp, setResp]         = useState<QueryResponse | null>(null)
  const [loading, setLoading]   = useState(true)
  const [error, setError]       = useState<string | null>(null)
  const [selected, setSelected] = useState<ServiceInstance | null>(null)
  const [filter, setFilter]     = useState('')

  function load() {
    setLoading(true)
    setError(null)
    setSelected(null)
    queryAll()
      .then(setResp)
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }

  // Load all services on mount
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(load, [])

  const services = resp?.serviceQueryData ?? []
  const lc = filter.toLowerCase().trim()
  const filtered = lc
    ? services.filter(s =>
        s.serviceDefinition.toLowerCase().includes(lc) ||
        s.providerSystem.systemName.toLowerCase().includes(lc)
      )
    : services

  function handleSelect(svc: ServiceInstance) {
    setSelected(prev => prev?.id === svc.id ? null : svc)
  }

  return (
    <div style={s.page}>
      <header style={s.header}>
        <h1 style={s.title}>Arrowhead Core Dashboard</h1>
        <button style={s.btn} onClick={load} disabled={loading}>
          {loading ? 'Loading…' : 'Refresh'}
        </button>
      </header>

      <SystemStatus />

      {resp && <MetricsBar services={resp.serviceQueryData} />}

      <RegisterForm onRegistered={load} />

      <div style={s.toolbar}>
        <input
          style={s.search}
          type="search"
          placeholder="Filter by service or system name…"
          value={filter}
          onChange={e => setFilter(e.target.value)}
        />
        {filter && (
          <span style={s.filterHint}>
            {filtered.length} of {services.length} shown
          </span>
        )}
      </div>

      {error && <p style={s.error}>Error: {error}</p>}

      {!error && (
        <div style={s.body}>
          <div style={s.tableWrap}>
            <ServiceTable
              services={filtered}
              loading={loading}
              selected={selected}
              onSelect={handleSelect}
            />
          </div>
          {selected && (
            <ServiceDetail
              service={selected}
              onClose={() => setSelected(null)}
            />
          )}
        </div>
      )}

      <AuthRulesPanel />
      <OrchestrationPanel />

      <footer style={s.footer}>
        {!loading && !error && resp && (
          <span>Total registered: {resp.unfilteredHits}</span>
        )}
      </footer>
    </div>
  )
}

const s = {
  page:       { fontFamily: 'monospace', maxWidth: 1200, margin: '32px auto', padding: '0 24px' },
  header:     { display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', marginBottom: 4 },
  title:      { fontSize: '1rem', margin: 0, fontWeight: 700 },
  btn:        { padding: '4px 14px', fontFamily: 'monospace', fontSize: '0.85rem', cursor: 'pointer', background: '#111', color: '#fff', border: 'none' },
  toolbar:    { display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 },
  search:     { fontFamily: 'monospace', fontSize: '0.85rem', padding: '5px 8px', border: '1px solid #ccc', width: 320 },
  filterHint: { fontSize: '0.8rem', color: '#888' },
  error:      { color: '#c00', fontSize: '0.9rem' },
  body:       { display: 'flex', alignItems: 'flex-start' },
  tableWrap:  { flex: 1, overflowX: 'auto' as const },
  footer:     { marginTop: 16, fontSize: '0.8rem', color: '#aaa' },
}
