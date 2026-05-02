import { useState } from 'react'
import { useConfig } from '../config/context'
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

export function ConfigView() {
  const { config, setConfig, resetToDefaults } = useConfig()
  const [draft, setDraft] = useState<DashboardConfig>(config)

  function updatePolling<K extends keyof DashboardConfig['polling']>(k: K, v: DashboardConfig['polling'][K]) {
    setDraft(d => ({ ...d, polling: { ...d.polling, [k]: v } }))
  }

  return (
    <div style={s.wrap}>
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
  wrap:         { fontFamily: 'monospace', maxWidth: 480 },
  section:      { marginBottom: 24 },
  sectionHead:  { fontSize: '0.85rem', fontWeight: 'bold', color: '#333', marginBottom: 10, borderBottom: '1px solid #eee', paddingBottom: 4 },
  field:        { display: 'flex', flexDirection: 'column', gap: 2, marginBottom: 10 },
  label:        { fontSize: '0.8rem', fontWeight: 'bold', color: '#444' },
  hint:         { fontSize: '0.7rem', color: '#888' },
  input:        { fontFamily: 'monospace', fontSize: '0.8rem', padding: '4px 6px', border: '1px solid #ccc', borderRadius: 3, width: 160 },
  checkbox:     { width: 16, height: 16, cursor: 'pointer' },
  actions:      { display: 'flex', gap: 8, marginTop: 8 },
  btn:          { fontFamily: 'monospace', fontSize: '0.8rem', padding: '6px 16px', border: '1px solid #999', borderRadius: 3, cursor: 'pointer', background: '#1a1a2e', color: '#fff' },
  btnSecondary: { background: '#fff', color: '#333', borderColor: '#ccc' },
}
