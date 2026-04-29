import { useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'
import { orchestrateDynamic, orchestrateSimpleStore, orchestrateFlexibleStore } from '../api'
import type { OrchestrationResponse } from '../types'

type Strategy = 'dynamic' | 'simplestore' | 'flexiblestore'

const PORTS: Record<Strategy, string> = {
  dynamic:       '8083',
  simplestore:   '8084',
  flexiblestore: '8085',
}

export default function OrchestrationPanel() {
  const [open, setOpen]         = useState(false)
  const [strategy, setStrategy] = useState<Strategy>('dynamic')
  const [consumer, setConsumer] = useState('')
  const [service, setService]   = useState('')
  const [result, setResult]     = useState<OrchestrationResponse | null>(null)
  const [err, setErr]           = useState<string | null>(null)
  const [busy, setBusy]         = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true); setErr(null); setResult(null)
    const req = {
      requesterSystem: { systemName: consumer.trim(), address: 'localhost', port: 0 },
      requestedService: { serviceDefinition: service.trim() },
    }
    try {
      let resp: OrchestrationResponse
      if (strategy === 'dynamic')       resp = await orchestrateDynamic(req)
      else if (strategy === 'simplestore') resp = await orchestrateSimpleStore(req)
      else                              resp = await orchestrateFlexibleStore(req)
      setResult(resp)
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : String(ex))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section style={s.section}>
      <button style={s.toggle} onClick={() => setOpen(o => !o)}>
        {open ? '▾' : '▸'} Orchestration
        <span style={s.port}>:{PORTS[strategy]}</span>
      </button>

      {open && (
        <div style={s.body}>
          <div style={{ marginBottom: 10, display: 'flex', gap: 10, alignItems: 'center' }}>
            <label style={{ fontSize: '0.8rem' }}>Strategy:</label>
            {(['dynamic', 'simplestore', 'flexiblestore'] as Strategy[]).map(st => (
              <label key={st} style={{ fontSize: '0.82rem', cursor: 'pointer' }}>
                <input type="radio" name="strategy" value={st} checked={strategy === st}
                  onChange={(e: ChangeEvent<HTMLInputElement>) => setStrategy(e.target.value as Strategy)} />
                {' '}{st}
              </label>
            ))}
          </div>

          <form onSubmit={submit} style={s.form}>
            <input style={s.inp} placeholder="consumerSystemName *" value={consumer} onChange={(e: ChangeEvent<HTMLInputElement>) => setConsumer(e.target.value)} />
            <input style={s.inp} placeholder="serviceDefinition *"  value={service}  onChange={(e: ChangeEvent<HTMLInputElement>) => setService(e.target.value)} />
            <button style={s.btn} type="submit" disabled={busy}>{busy ? '…' : 'Orchestrate'}</button>
          </form>

          {err && <p style={s.err}>Error: {err}</p>}

          {result && (
            result.response.length === 0
              ? <p style={s.muted}>No matching providers found.</p>
              : (
                <table style={s.table}>
                  <thead><tr>{['provider', 'address:port', 'service', 'serviceUri', 'interfaces', 'priority'].map(h => <th key={h} style={s.th}>{h}</th>)}</tr></thead>
                  <tbody>
                    {result.response.map((r, i) => (
                      <tr key={i}>
                        <td style={s.td}>{r.provider.systemName}</td>
                        <td style={s.td}>{r.provider.address}:{r.provider.port}</td>
                        <td style={s.td}>{r.service.serviceDefinition}</td>
                        <td style={s.td}>{r.service.serviceUri}</td>
                        <td style={s.td}>{r.service.interfaces?.join(', ')}</td>
                        <td style={s.td}>{r.priority ?? '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )
          )}
        </div>
      )}
    </section>
  )
}

const s = {
  section: { marginBottom: 16 },
  toggle:  { background: 'none', border: 'none', cursor: 'pointer', fontFamily: 'monospace', fontSize: '0.9rem', padding: '4px 0', fontWeight: 600 },
  port:    { marginLeft: 6, fontSize: '0.75rem', color: '#888', fontWeight: 400 },
  body:    { marginTop: 8, padding: '12px 14px', border: '1px solid #ddd', background: '#fafafa' },
  form:    { display: 'flex', gap: 8, alignItems: 'center', marginBottom: 12, flexWrap: 'wrap' as const },
  inp:     { fontFamily: 'monospace', fontSize: '0.82rem', padding: '4px 6px', border: '1px solid #bbb', width: 200 },
  btn:     { padding: '4px 14px', fontFamily: 'monospace', fontSize: '0.82rem', cursor: 'pointer', background: '#111', color: '#fff', border: 'none' },
  muted:   { color: '#888', fontSize: '0.85rem' },
  err:     { color: '#c00', fontSize: '0.85rem' },
  table:   { width: '100%', borderCollapse: 'collapse' as const, fontSize: '0.82rem' },
  th:      { textAlign: 'left' as const, borderBottom: '2px solid #111', padding: '5px 8px', whiteSpace: 'nowrap' as const },
  td:      { padding: '5px 8px', borderBottom: '1px solid #eee', verticalAlign: 'top' as const },
}
