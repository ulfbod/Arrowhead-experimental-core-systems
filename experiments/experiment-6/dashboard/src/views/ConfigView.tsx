// ConfigView — dashboard polling intervals and policy-sync SYNC_INTERVAL control.
//
// The SYNC_INTERVAL field sends a POST /config request to policy-sync, which
// applies the new interval immediately (runtime-configurable via atomic.Int64).
// This is the key difference from experiment-5: the sync period can be changed
// without restarting any service, enabling interactive demonstration of the
// sync-delay caveat.

import { useState } from 'react'
import { useConfig } from '../config/context'
import { usePolling } from '../hooks/usePolling'
import { fetchPolicySyncStatus, updateSyncInterval } from '../api'
import type { DashboardConfig } from '../config/types'

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label style={s.field}>
      <span style={s.label}>{label}</span>
      {hint && <span style={s.hint}>{hint}</span>}
      {children}
    </label>
  )
}

function NumberInput({ value, min, onChange }: { value: number; min?: number; onChange: (n: number) => void }) {
  return (
    <input
      type="number"
      style={s.input}
      value={value}
      min={min ?? 0}
      onChange={e => {
        const n = parseInt(e.target.value, 10)
        if (!isNaN(n) && n >= (min ?? 0)) onChange(n)
      }}
    />
  )
}

function SyncIntervalControl() {
  const { data: syncStatus, refresh } = usePolling(fetchPolicySyncStatus, 5_000)
  const [draft, setDraft] = useState('')
  const [state, setState] = useState<'idle' | 'loading' | 'ok' | 'error'>('idle')
  const [errMsg, setErrMsg] = useState('')

  const currentInterval = syncStatus?.syncInterval ?? '…'

  async function apply() {
    const val = draft.trim()
    if (!val) return
    setState('loading')
    setErrMsg('')
    try {
      await updateSyncInterval(val)
      setState('ok')
      setDraft('')
      refresh()
      setTimeout(() => setState('idle'), 2000)
    } catch (e) {
      setState('error')
      setErrMsg(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <section style={s.section}>
      <h3 style={s.sectionHead}>Policy-Sync Interval</h3>
      <p style={s.hint}>
        Current: <strong>{currentInterval}</strong>.
        Changing this takes effect after the current sleep completes (no restart needed).
      </p>
      <div style={s.form}>
        <input
          style={s.input}
          placeholder="e.g. 5s, 30s, 2m"
          value={draft}
          onChange={e => setDraft(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') void apply() }}
        />
        <button
          style={{ ...s.btn, ...(state === 'loading' ? s.btnDisabled : {}) }}
          disabled={state === 'loading' || !draft.trim()}
          onClick={() => void apply()}
        >
          {state === 'loading' ? 'applying…' : 'Apply'}
        </button>
        {state === 'ok' && <span style={s.ok}>✓ applied</span>}
      </div>
      {state === 'error' && <p style={s.err}>{errMsg}</p>}
      <p style={s.hint}>
        Use a short interval (e.g. 5s) to observe fast revocation propagation.
        Use a long interval (e.g. 60s) to demonstrate the sync-delay caveat:
        a revoked REST grant continues to produce Permit decisions until the next sync cycle.
      </p>
    </section>
  )
}

export function ConfigView() {
  const { config, setConfig, resetToDefaults } = useConfig()
  const [draft, setDraft] = useState<DashboardConfig>(config)

  function updatePolling<K extends keyof DashboardConfig['polling']>(k: K, v: DashboardConfig['polling'][K]) {
    setDraft(d => ({ ...d, polling: { ...d.polling, [k]: v } }))
  }

  return (
    <div style={s.wrap}>
      <SyncIntervalControl />

      <section style={s.section}>
        <h3 style={s.sectionHead}>Polling intervals</h3>
        <Field label="Health check (ms)">
          <NumberInput min={500} value={draft.polling.healthIntervalMs}
            onChange={v => updatePolling('healthIntervalMs', v)} />
        </Field>
        <Field label="Grants (ms)" hint="ConsumerAuth /authorization/lookup poll rate">
          <NumberInput min={1000} value={draft.polling.grantsIntervalMs}
            onChange={v => updatePolling('grantsIntervalMs', v)} />
        </Field>
        <Field label="RabbitMQ users (ms)" hint="RabbitMQ users + topic-permissions poll rate">
          <NumberInput min={1000} value={draft.polling.rmqUsersIntervalMs}
            onChange={v => updatePolling('rmqUsersIntervalMs', v)} />
        </Field>
        <Field label="Consumer stats (ms)" hint="Consumer /stats + queue delivery rates poll rate">
          <NumberInput min={500} value={draft.polling.consumerStatsIntervalMs}
            onChange={v => updatePolling('consumerStatsIntervalMs', v)} />
        </Field>
        <Field label="Policy sync (ms)" hint="policy-sync /status + kafka-authz /status + rest-authz /status poll rate">
          <NumberInput min={1000} value={draft.polling.policyIntervalMs}
            onChange={v => updatePolling('policyIntervalMs', v)} />
        </Field>
      </section>

      <section style={s.section}>
        <h3 style={s.sectionHead}>Display</h3>
        <Field label="Show health latency">
          <input
            type="checkbox"
            style={s.checkbox}
            checked={draft.display.showHealthLatency}
            onChange={e => setDraft(d => ({ ...d, display: { ...d.display, showHealthLatency: e.target.checked } }))}
          />
        </Field>
      </section>

      <div style={s.actions}>
        <button style={s.btn} onClick={() => setConfig(draft)}>Apply</button>
        <button style={{ ...s.btn, ...s.btnSecondary }} onClick={() => { resetToDefaults(); setDraft(config) }}>
          Reset to defaults
        </button>
      </div>
    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  wrap:         { fontFamily: 'monospace', maxWidth: 520 },
  section:      { marginBottom: 24 },
  sectionHead:  { fontSize: '0.85rem', fontWeight: 'bold', color: '#333', marginBottom: 10, borderBottom: '1px solid #eee', paddingBottom: 4 },
  field:        { display: 'flex', flexDirection: 'column', gap: 2, marginBottom: 10 },
  label:        { fontSize: '0.8rem', fontWeight: 'bold', color: '#444' },
  hint:         { fontSize: '0.7rem', color: '#888' },
  input:        { fontFamily: 'monospace', fontSize: '0.8rem', padding: '4px 6px', border: '1px solid #ccc', borderRadius: 3, width: 160 },
  checkbox:     { width: 16, height: 16, cursor: 'pointer' },
  form:         { display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 },
  actions:      { display: 'flex', gap: 8, marginTop: 8 },
  btn:          { fontFamily: 'monospace', fontSize: '0.8rem', padding: '6px 16px', border: '1px solid #999', borderRadius: 3, cursor: 'pointer', background: '#1a1a2e', color: '#fff' },
  btnSecondary: { background: '#fff', color: '#333', borderColor: '#ccc' },
  btnDisabled:  { opacity: 0.5, cursor: 'not-allowed' },
  ok:           { color: '#388e3c', fontSize: '0.8rem' },
  err:          { color: '#f44336', fontSize: '0.8rem', margin: '4px 0' },
}
