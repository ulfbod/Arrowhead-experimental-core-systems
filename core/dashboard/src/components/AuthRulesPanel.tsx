import { useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'
import { lookupRules, grantRule, revokeRule } from '../api'
import type { AuthRule } from '../types'

export default function AuthRulesPanel() {
  const [open, setOpen]     = useState(false)
  const [rules, setRules]   = useState<AuthRule[]>([])
  const [loaded, setLoaded] = useState(false)
  const [err, setErr]       = useState<string | null>(null)

  const [consumer, setConsumer]   = useState('')
  const [provider, setProvider]   = useState('')
  const [svcDef, setSvcDef]       = useState('')
  const [busy, setBusy]           = useState(false)
  const [status, setStatus]       = useState<{ ok: boolean; msg: string } | null>(null)

  async function load() {
    setErr(null)
    try {
      const resp = await lookupRules()
      setRules(resp.rules)
      setLoaded(true)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setStatus(null)
    try {
      await grantRule({ consumerSystemName: consumer.trim(), providerSystemName: provider.trim(), serviceDefinition: svcDef.trim() })
      setStatus({ ok: true, msg: 'Rule granted.' })
      setConsumer(''); setProvider(''); setSvcDef('')
      load()
    } catch (ex) {
      setStatus({ ok: false, msg: ex instanceof Error ? ex.message : String(ex) })
    } finally {
      setBusy(false)
    }
  }

  async function revoke(id: number) {
    try {
      await revokeRule(id)
      setRules(prev => prev.filter(r => r.id !== id))
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : String(ex))
    }
  }

  return (
    <section style={s.section}>
      <button style={s.toggle} onClick={() => { setOpen(o => !o); if (!loaded) load() }}>
        {open ? '▾' : '▸'} ConsumerAuthorization Rules
        <span style={s.port}>:8082</span>
      </button>

      {open && (
        <div style={s.body}>
          {err && <p style={s.err}>{err}</p>}

          <form onSubmit={submit} style={s.form}>
            <input style={s.inp} placeholder="consumerSystemName *" value={consumer} onChange={(e: ChangeEvent<HTMLInputElement>) => setConsumer(e.target.value)} />
            <input style={s.inp} placeholder="providerSystemName *" value={provider} onChange={(e: ChangeEvent<HTMLInputElement>) => setProvider(e.target.value)} />
            <input style={s.inp} placeholder="serviceDefinition *"  value={svcDef}   onChange={(e: ChangeEvent<HTMLInputElement>) => setSvcDef(e.target.value)} />
            <button style={s.btn} type="submit" disabled={busy}>{busy ? '…' : 'Grant'}</button>
            {status && <span style={{ marginLeft: 8, fontSize: '0.82rem', color: status.ok ? '#080' : '#c00' }}>{status.msg}</span>}
          </form>

          {loaded && rules.length === 0 && <p style={s.muted}>No rules.</p>}
          {rules.length > 0 && (
            <table style={s.table}>
              <thead><tr>{['id', 'consumer', 'provider', 'service', ''].map(h => <th key={h} style={s.th}>{h}</th>)}</tr></thead>
              <tbody>
                {rules.map(r => (
                  <tr key={r.id}>
                    <td style={s.td}>{r.id}</td>
                    <td style={s.td}>{r.consumerSystemName}</td>
                    <td style={s.td}>{r.providerSystemName}</td>
                    <td style={s.td}>{r.serviceDefinition}</td>
                    <td style={s.td}><button style={s.del} onClick={() => revoke(r.id)}>revoke</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
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
  inp:     { fontFamily: 'monospace', fontSize: '0.82rem', padding: '4px 6px', border: '1px solid #bbb', width: 170 },
  btn:     { padding: '4px 14px', fontFamily: 'monospace', fontSize: '0.82rem', cursor: 'pointer', background: '#111', color: '#fff', border: 'none' },
  del:     { padding: '2px 8px', fontFamily: 'monospace', fontSize: '0.78rem', cursor: 'pointer', background: '#c00', color: '#fff', border: 'none' },
  muted:   { color: '#888', fontSize: '0.85rem' },
  err:     { color: '#c00', fontSize: '0.85rem' },
  table:   { width: '100%', borderCollapse: 'collapse' as const, fontSize: '0.82rem' },
  th:      { textAlign: 'left' as const, borderBottom: '2px solid #111', padding: '5px 8px' },
  td:      { padding: '5px 8px', borderBottom: '1px solid #eee' },
}
