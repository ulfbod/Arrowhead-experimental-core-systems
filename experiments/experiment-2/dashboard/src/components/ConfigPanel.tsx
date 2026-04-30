// Form-based configuration panel.
// Divided into three sections matching the DashboardConfig namespace structure.

import { useState } from 'react'
import { useConfig } from '../config/context'
import type { DashboardConfig } from '../config/types'

function Field({
  label, hint, children,
}: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label style={s.field}>
      <span style={s.label}>{label}</span>
      {hint && <span style={s.hint}>{hint}</span>}
      {children}
    </label>
  )
}

function NumberInput({
  value, min, onChange,
}: { value: number; min?: number; onChange: (n: number) => void }) {
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

export function ConfigPanel() {
  const { config, setConfig, resetToDefaults } = useConfig()
  const [draft, setDraft] = useState<DashboardConfig>(config)

  function apply() { setConfig(draft) }

  function reset() {
    resetToDefaults()
    // Re-read after reset — resetToDefaults updates context; we mirror here.
    setDraft(config)
  }

  function updatePolling<K extends keyof DashboardConfig['polling']>(
    key: K, value: DashboardConfig['polling'][K],
  ) {
    setDraft(d => ({ ...d, polling: { ...d.polling, [key]: value } }))
  }

  function updateDisplay<K extends keyof DashboardConfig['display']>(
    key: K, value: DashboardConfig['display'][K],
  ) {
    setDraft(d => ({ ...d, display: { ...d.display, [key]: value } }))
  }

  function updateExp2<K extends keyof DashboardConfig['experiment2']>(
    key: K, value: DashboardConfig['experiment2'][K],
  ) {
    setDraft(d => ({ ...d, experiment2: { ...d.experiment2, [key]: value } }))
  }

  return (
    <div style={s.wrap}>
      <section style={s.section}>
        <h3 style={s.sectionHead}>Polling intervals</h3>
        <Field label="Health check (ms)" hint="How often to poll /health on each system">
          <NumberInput min={500} value={draft.polling.healthIntervalMs}
            onChange={v => updatePolling('healthIntervalMs', v)} />
        </Field>
        <Field label="Services list (ms)" hint="ServiceRegistry query rate">
          <NumberInput min={1000} value={draft.polling.servicesIntervalMs}
            onChange={v => updatePolling('servicesIntervalMs', v)} />
        </Field>
        <Field label="Telemetry (ms)" hint="Edge-adapter latest-reading poll rate">
          <NumberInput min={500} value={draft.polling.telemetryIntervalMs}
            onChange={v => updatePolling('telemetryIntervalMs', v)} />
        </Field>
        <Field label="Orchestration (ms)" hint="DynamicOrchestration query rate">
          <NumberInput min={1000} value={draft.polling.orchIntervalMs}
            onChange={v => updatePolling('orchIntervalMs', v)} />
        </Field>
        <Field label="Broker stats (ms)" hint="RabbitMQ management API poll rate">
          <NumberInput min={1000} value={draft.polling.brokerIntervalMs}
            onChange={v => updatePolling('brokerIntervalMs', v)} />
        </Field>
        <Field label="Fleet stats (ms)" hint="Robot-fleet stats poll rate">
          <NumberInput min={500} value={draft.polling.fleetStatsIntervalMs}
            onChange={v => updatePolling('fleetStatsIntervalMs', v)} />
        </Field>
        <Field label="Telemetry stats (ms)" hint="Edge-adapter /telemetry/stats poll rate">
          <NumberInput min={500} value={draft.polling.allTelemetryIntervalMs}
            onChange={v => updatePolling('allTelemetryIntervalMs', v)} />
        </Field>
      </section>

      <section style={s.section}>
        <h3 style={s.sectionHead}>Display</h3>
        <Field label="Telemetry history rows" hint="Maximum rows kept in the rolling history table">
          <NumberInput min={1} value={draft.display.maxTelemetryHistory}
            onChange={v => updateDisplay('maxTelemetryHistory', v)} />
        </Field>
        <Field label="Show health latency">
          <input
            type="checkbox"
            style={s.checkbox}
            checked={draft.display.showHealthLatency}
            onChange={e => updateDisplay('showHealthLatency', e.target.checked)}
          />
        </Field>
      </section>

      <section style={s.section}>
        <h3 style={s.sectionHead}>Experiment 2</h3>
        <Field label="Consumer system name" hint="requesterSystem.systemName sent in orchestration queries">
          <input
            type="text"
            style={s.input}
            value={draft.experiment2.consumerName}
            onChange={e => updateExp2('consumerName', e.target.value)}
          />
        </Field>
        <Field label="Service definition" hint="requestedService.serviceDefinition">
          <input
            type="text"
            style={s.input}
            value={draft.experiment2.serviceDefinition}
            onChange={e => updateExp2('serviceDefinition', e.target.value)}
          />
        </Field>
        <Field label="Poll interval label" hint="Human-readable label shown in the UI (informational only)">
          <input
            type="text"
            style={s.input}
            value={draft.experiment2.pollIntervalLabel}
            onChange={e => updateExp2('pollIntervalLabel', e.target.value)}
          />
        </Field>
      </section>

      <div style={s.actions}>
        <button style={s.btn} onClick={apply}>Apply</button>
        <button style={{ ...s.btn, ...s.btnSecondary }} onClick={reset}>Reset to defaults</button>
      </div>
    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  wrap:         { fontFamily: 'monospace', maxWidth: 560 },
  section:      { marginBottom: 24 },
  sectionHead:  { fontSize: '0.85rem', fontWeight: 'bold', color: '#333', marginBottom: 10, borderBottom: '1px solid #eee', paddingBottom: 4 },
  field:        { display: 'flex', flexDirection: 'column', gap: 2, marginBottom: 10 },
  label:        { fontSize: '0.8rem', fontWeight: 'bold', color: '#444' },
  hint:         { fontSize: '0.7rem', color: '#888' },
  input:        { fontFamily: 'monospace', fontSize: '0.8rem', padding: '4px 6px', border: '1px solid #ccc', borderRadius: 3, width: 200 },
  checkbox:     { width: 16, height: 16, cursor: 'pointer' },
  actions:      { display: 'flex', gap: 8, marginTop: 8 },
  btn:          { fontFamily: 'monospace', fontSize: '0.8rem', padding: '6px 16px', border: '1px solid #999', borderRadius: 3, cursor: 'pointer', background: '#1a1a2e', color: '#fff' },
  btnSecondary: { background: '#fff', color: '#333', borderColor: '#ccc' },
}
