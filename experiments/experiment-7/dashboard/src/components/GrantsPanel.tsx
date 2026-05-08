// Lists ConsumerAuth grants and provides add/revoke controls.
// Grants drive policy-sync → AuthzForce, which enforces AMQP, Kafka, and REST paths.

import { useState, useCallback } from 'react'
import { usePolling } from '../hooks/usePolling'
import { fetchAuthRules, addGrant, revokeGrant } from '../api'
import type { AuthRule } from '../types'

const SERVICE_OPTIONS = ['telemetry', 'telemetry-rest', 'sensors']
const PROVIDER_MAP: Record<string, string> = {
  'telemetry':      'robot-fleet',
  'telemetry-rest': 'data-provider',
  'sensors':        'robot-fleet',
}

export function GrantsPanel() {
  const { data, error, stale, refresh } = usePolling(fetchAuthRules, 5_000)
  const rules: AuthRule[] = data?.rules ?? []

  const [newConsumer, setNewConsumer] = useState('')
  const [newService, setNewService] = useState('telemetry')
  const [addState, setAddState] = useState<'idle' | 'loading' | 'error'>('idle')
  const [addError, setAddError] = useState('')

  const handleAdd = useCallback(async () => {
    if (!newConsumer.trim()) return
    setAddState('loading')
    setAddError('')
    const provider = PROVIDER_MAP[newService] ?? 'robot-fleet'
    try {
      await addGrant(newConsumer.trim(), provider, newService)
      setNewConsumer('')
      setAddState('idle')
      refresh()
    } catch (e) {
      setAddState('error')
      setAddError(e instanceof Error ? e.message : String(e))
    }
  }, [newConsumer, newService, refresh])

  const handleRevoke = useCallback(async (id: number) => {
    try {
      await revokeGrant(id)
      refresh()
    } catch (e) {
      alert('Revoke failed: ' + String(e))
    }
  }, [refresh])

  return (
    <div style={s.wrap}>

      {/* ── Grants table ─────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>
          Authorization Grants
          {stale && <span style={s.stale}> (stale)</span>}
        </h2>
        {error && !stale && <p style={s.err}>{error}</p>}

        {rules.length === 0
          ? <p style={s.dim}>No grants.</p>
          : (
            <table style={s.table}>
              <thead>
                <tr>
                  {['#', 'Consumer', 'Provider', 'Service', ''].map(h => (
                    <th key={h} style={s.th}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rules.map(r => (
                  <tr key={r.id}>
                    <td style={s.td}>{r.id}</td>
                    <td style={s.td}>{r.consumerSystemName}</td>
                    <td style={s.td}>{r.providerSystemName}</td>
                    <td style={s.td}>{r.serviceDefinition}</td>
                    <td style={s.td}>
                      <button style={s.revokeBtn} onClick={() => void handleRevoke(r.id)}>
                        revoke
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )
        }
      </section>

      {/* ── Add grant form ───────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>Add Grant</h3>
        <div style={s.form}>
          <input
            style={s.input}
            placeholder="consumer system name"
            value={newConsumer}
            onChange={e => setNewConsumer(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') void handleAdd() }}
          />
          <select
            style={s.select}
            value={newService}
            onChange={e => setNewService(e.target.value)}
          >
            {SERVICE_OPTIONS.map(s => <option key={s} value={s}>{s}</option>)}
          </select>
          <span style={s.dim}>→ {PROVIDER_MAP[newService] ?? 'robot-fleet'}</span>
          <button
            style={{ ...s.btn, ...(addState === 'loading' ? s.btnDisabled : {}) }}
            disabled={addState === 'loading' || !newConsumer.trim()}
            onClick={() => void handleAdd()}
          >
            {addState === 'loading' ? 'adding…' : 'Add'}
          </button>
        </div>
        {addState === 'error' && <p style={s.err}>{addError}</p>}
        <p style={s.hint}>
          Grants propagate to AuthzForce within one policy-sync cycle (see Config tab).
          All three PEPs (AMQP, Kafka, REST) enforce the same policy.
        </p>
      </section>
    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  wrap:       { fontFamily: 'monospace' },
  section:    { marginBottom: 28 },
  heading:    { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  subheading: { fontSize: '0.85rem', fontWeight: 'bold', color: '#333', marginBottom: 8 },
  table:      { width: '100%', borderCollapse: 'collapse', fontSize: '0.8rem' },
  th:         { textAlign: 'left', padding: '4px 8px', borderBottom: '2px solid #ddd' },
  td:         { padding: '4px 8px', borderBottom: '1px solid #eee' },
  err:        { color: '#f44336', fontSize: '0.8rem' },
  dim:        { color: '#999', fontSize: '0.8rem' },
  stale:      { color: '#ff9800', fontWeight: 'normal', fontSize: '0.75rem' },
  hint:       { color: '#888', fontSize: '0.75rem', marginTop: 4 },
  form:       { display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' },
  input:      { fontFamily: 'monospace', fontSize: '0.8rem', padding: '4px 6px', border: '1px solid #ccc', borderRadius: 3, width: 220 },
  select:     { fontFamily: 'monospace', fontSize: '0.8rem', padding: '4px 6px', border: '1px solid #ccc', borderRadius: 3 },
  btn:        { fontFamily: 'monospace', fontSize: '0.8rem', padding: '5px 14px', border: '1px solid #999', borderRadius: 3, cursor: 'pointer', background: '#1a1a2e', color: '#fff' },
  btnDisabled: { opacity: 0.5, cursor: 'not-allowed' },
  revokeBtn:  { fontFamily: 'monospace', fontSize: '0.75rem', padding: '2px 8px', border: '1px solid #ccc', borderRadius: 3, cursor: 'pointer', background: '#fff', color: '#c0392b' },
}
