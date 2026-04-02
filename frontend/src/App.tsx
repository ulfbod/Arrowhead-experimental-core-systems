import React, { useState, useCallback } from 'react'
import SystemView from './components/SystemView'
import CDTaView from './components/CDTaView'
import CDTbView from './components/CDTbView'
import QoSView from './components/QoSView'
import SimulationView from './components/SimulationView'
import { ErrorBoundary } from './components/ErrorBoundary'
import usePolling from './hooks/usePolling'
import { urls, startScenario, resetScenario, injectHazard, triggerGasSpike, clearAll, startMission } from './api'
import type { ScenarioStatus } from './types'

type Tab = 'system' | 'cdta' | 'cdtb' | 'qos' | 'simulation'
type DemoScenario = 'inspection' | 'hazard' | 'emergency'

const DEMO_SCENARIOS: Record<DemoScenario, { label: string; description: string; steps: string[] }> = {
  inspection: {
    label: 'Inspection & Recovery',
    description: 'A robot performs periodic tunnel inspections. The cDTa digital twin tracks robot state and triggers recovery if the robot stalls or loses connectivity.',
    steps: [
      'System starts and robot begins patrol route',
      'Robot encounters obstacle — stall detected by cDTa',
      'cDTa issues recovery command; robot resumes patrol',
      'Inspection data logged to system view',
    ],
  },
  hazard: {
    label: 'Hazard Detection',
    description: 'A gas sensor spike triggers an alert. The cDTb digital twin monitors environmental conditions and escalates to an alarm when thresholds are exceeded.',
    steps: [
      'Environmental sensors stream baseline readings',
      'Gas concentration rises above warning threshold',
      'cDTb raises a hazard alert — alarm activates',
      'Readings return to normal; alert clears automatically',
    ],
  },
  emergency: {
    label: 'Emergency (Combined)',
    description: 'Simultaneously injects a robot hazard and a gas spike to demonstrate the system\'s ability to handle concurrent incidents across both digital twins.',
    steps: [
      'Robot is on patrol; sensors show nominal readings',
      'Robot stall and gas spike are triggered at the same time',
      'cDTa and cDTb both raise independent alerts',
      'System state shows concurrent failure modes',
      'Both conditions clear after recovery commands',
    ],
  },
}

const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<Tab>('system')
  const [scenarioLoading, setScenarioLoading] = useState<string | null>(null)
  const [demoScenario, setDemoScenario] = useState<DemoScenario>('inspection')
  const [simSpeed, setSimSpeed] = useState<number>(2)
  const [showScenarioDesc, setShowScenarioDesc] = useState<boolean>(true)

  const { data: scenario } = usePolling<ScenarioStatus>(urls.scenario, 3000)

  const runAction = useCallback(
    async (label: string, fn: () => Promise<void>) => {
      setScenarioLoading(label)
      try {
        await fn()
      } catch (_e) {
        // swallow – backend may not be up during dev
      } finally {
        setScenarioLoading(null)
      }
    },
    []
  )

  const startDemoScenario = useCallback(async () => {
    if (demoScenario === 'emergency') {
      setScenarioLoading('start')
      try {
        await Promise.all([injectHazard(), triggerGasSpike()])
      } catch (_e) {
        // swallow
      } finally {
        setScenarioLoading(null)
      }
    } else if (demoScenario === 'hazard') {
      runAction('start', triggerGasSpike)
    } else {
      // inspection: start the robot mission via cDTa
      runAction('start', () => Promise.all([startScenario(), startMission()]).then(() => undefined))
    }
  }, [demoScenario, runAction])

  const scenarioStateColor =
    scenario?.phase === 'running'   ? 'var(--green)' :
    scenario?.phase === 'failed'    ? 'var(--red)'   :
    scenario?.phase === 'completed' ? 'var(--blue)'  : 'var(--text-muted)'

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column', background: 'var(--bg-primary)' }}>
      {/* ====================================================
          TOP BAR – branding + scenario controls
          ==================================================== */}
      <header
        className="sticky top-0 z-20"
        style={{
          background: 'var(--bg-surface)',
          borderBottom: '1px solid var(--border)',
          padding: '10px 24px',
          display: 'flex',
          alignItems: 'center',
          gap: '24px',
          flexWrap: 'wrap',
        }}
      >
        {/* Brand */}
        <div style={{ display: 'flex', flexDirection: 'column', lineHeight: 1.2, flexShrink: 0 }}>
          <span style={{ fontWeight: 800, fontSize: '1.05rem', letterSpacing: '-0.02em', color: 'var(--text-primary)' }}>
            MineIO Digital Twin
          </span>
          <span style={{ fontSize: '0.68rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
            Monitoring &amp; Decision Support
          </span>
        </div>

        {/* Scenario status pill */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border)',
            borderRadius: '999px',
            padding: '4px 12px',
            fontSize: '0.75rem',
            flexShrink: 0,
          }}
        >
          <span
            style={{
              width: 8, height: 8,
              borderRadius: '50%',
              background: scenarioStateColor,
              boxShadow: scenario?.phase === 'running' ? `0 0 6px ${scenarioStateColor}` : 'none',
              flexShrink: 0,
            }}
          />
          <span style={{ color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
            Scenario:&nbsp;
          </span>
          <span style={{ fontWeight: 600, color: scenarioStateColor }}>
            {scenario?.phase ?? 'unknown'}
          </span>
        </div>

        {/* Demo scenario selector */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexShrink: 0 }}>
          <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
            Demo:
          </span>
          <select
            value={demoScenario}
            onChange={e => setDemoScenario(e.target.value as DemoScenario)}
            style={{
              fontSize: '0.78rem',
              padding: '4px 8px',
              borderRadius: '6px',
              border: '1px solid var(--border)',
              background: 'var(--bg-elevated)',
              color: 'var(--text-primary)',
              cursor: 'pointer',
            }}
          >
            {(Object.keys(DEMO_SCENARIOS) as DemoScenario[]).map(k => (
              <option key={k} value={k}>{DEMO_SCENARIOS[k].label}</option>
            ))}
          </select>
          <button
            title="Toggle scenario description"
            onClick={() => setShowScenarioDesc(v => !v)}
            style={{
              fontSize: '0.72rem',
              padding: '3px 7px',
              borderRadius: '6px',
              border: '1px solid var(--border)',
              background: showScenarioDesc ? 'var(--blue)' : 'var(--bg-elevated)',
              color: showScenarioDesc ? '#fff' : 'var(--text-secondary)',
              cursor: 'pointer',
            }}
          >
            ?
          </button>
        </div>

        {/* Speed controls */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px', flexShrink: 0 }}>
          <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
            Speed:
          </span>
          {([1, 2, 5, 10] as number[]).map(s => (
            <button
              key={s}
              onClick={() => setSimSpeed(s)}
              style={{
                fontSize: '0.72rem',
                padding: '3px 8px',
                borderRadius: '5px',
                border: '1px solid var(--border)',
                background: simSpeed === s ? 'var(--blue)' : 'var(--bg-elevated)',
                color: simSpeed === s ? '#fff' : 'var(--text-secondary)',
                cursor: 'pointer',
                fontWeight: simSpeed === s ? 600 : 400,
              }}
            >
              {s}×
            </button>
          ))}
        </div>

        {/* Scenario action buttons */}
        <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
          <button
            className="btn btn-success btn-sm"
            disabled={!!scenarioLoading || scenario?.phase === 'running'}
            onClick={startDemoScenario}
          >
            {scenarioLoading === 'start' ? <span className="loading-spinner" /> : null}
            {demoScenario === 'emergency' ? 'Trigger Emergency' : 'Start Scenario'}
          </button>

          <button
            className="btn btn-outline btn-sm"
            disabled={!!scenarioLoading}
            onClick={() => runAction('reset', resetScenario)}
          >
            {scenarioLoading === 'reset' ? <span className="loading-spinner" /> : null}
            Reset
          </button>

          <button
            className="btn btn-warning btn-sm"
            disabled={!!scenarioLoading}
            onClick={() => runAction('hazard', injectHazard)}
          >
            {scenarioLoading === 'hazard' ? <span className="loading-spinner" /> : null}
            Inject Hazard
          </button>

          <button
            className="btn btn-warning btn-sm"
            disabled={!!scenarioLoading}
            onClick={() => runAction('spike', triggerGasSpike)}
            style={{ background: 'var(--amber-dark)', borderColor: 'var(--amber-dark)' }}
          >
            {scenarioLoading === 'spike' ? <span className="loading-spinner" /> : null}
            Gas Spike
          </button>

          <button
            className="btn btn-danger btn-sm"
            disabled={!!scenarioLoading}
            onClick={() => runAction('clear', clearAll)}
          >
            {scenarioLoading === 'clear' ? <span className="loading-spinner" /> : null}
            Clear All
          </button>
        </div>

        {/* Spacer */}
        <div style={{ flex: 1 }} />

        {/* Live indicator */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: '0.72rem', color: 'var(--text-muted)', flexShrink: 0 }}>
          <span className="status-dot online pulse" />
          LIVE
        </div>
      </header>

      {/* ====================================================
          DEMO SCENARIO DESCRIPTION BANNER
          ==================================================== */}
      {showScenarioDesc && (
        <div className="scenario-banner">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16 }}>
            <div style={{ flex: 1 }}>
              <div style={{ fontWeight: 700, fontSize: '0.82rem', marginBottom: 4, color: 'var(--text-primary)' }}>
                Demo: {DEMO_SCENARIOS[demoScenario].label}
              </div>
              <div style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', marginBottom: 8 }}>
                {DEMO_SCENARIOS[demoScenario].description}
              </div>
              <ol style={{ margin: 0, paddingLeft: '1.4em', fontSize: '0.76rem', color: 'var(--text-secondary)' }}>
                {DEMO_SCENARIOS[demoScenario].steps.map((s, i) => (
                  <li key={i} style={{ marginBottom: 2 }}>{s}</li>
                ))}
              </ol>
            </div>
            <button
              onClick={() => setShowScenarioDesc(false)}
              style={{
                background: 'none', border: 'none', cursor: 'pointer',
                color: 'var(--text-muted)', fontSize: '1rem', padding: '0 4px', lineHeight: 1,
              }}
              title="Dismiss"
            >
              ×
            </button>
          </div>
        </div>
      )}

      {/* ====================================================
          TAB NAV
          ==================================================== */}
      <div
        style={{
          background: 'var(--bg-surface)',
          borderBottom: '1px solid var(--border)',
          padding: '0 24px',
          display: 'flex',
          gap: 0,
        }}
      >
        {([
          { id: 'system',     label: 'System View' },
          { id: 'cdta',       label: 'cDTa: Inspection & Recovery' },
          { id: 'cdtb',       label: 'cDTb: Hazard Monitoring' },
          { id: 'qos',        label: 'QoS & Failover' },
          { id: 'simulation', label: 'Simulation' },
        ] as { id: Tab; label: string }[]).map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            style={{
              padding: '12px 20px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === tab.id
                ? '2px solid var(--blue)'
                : '2px solid transparent',
              color: activeTab === tab.id ? 'var(--blue)' : 'var(--text-secondary)',
              fontWeight: activeTab === tab.id ? 600 : 400,
              fontSize: '0.85rem',
              cursor: 'pointer',
              transition: 'all 150ms ease',
              whiteSpace: 'nowrap',
            }}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* ====================================================
          PAGE CONTENT
          ==================================================== */}
      <main style={{ flex: 1, overflowY: 'auto' }}>
        <ErrorBoundary name="System View">
          {activeTab === 'system' && <SystemView />}
        </ErrorBoundary>
        <ErrorBoundary name="cDTa View">
          {activeTab === 'cdta' && <CDTaView />}
        </ErrorBoundary>
        <ErrorBoundary name="cDTb View">
          {activeTab === 'cdtb' && <CDTbView />}
        </ErrorBoundary>
        <ErrorBoundary name="QoS View">
          {activeTab === 'qos' && <QoSView />}
        </ErrorBoundary>
        <ErrorBoundary name="Simulation View">
          {activeTab === 'simulation' && <SimulationView simSpeed={simSpeed} />}
        </ErrorBoundary>
      </main>

      {/* ====================================================
          FOOTER
          ==================================================== */}
      <footer
        style={{
          background: 'var(--bg-surface)',
          borderTop: '1px solid var(--border)',
          padding: '8px 24px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          fontSize: '0.72rem',
          color: 'var(--text-muted)',
        }}
      >
        <span>MineIO Digital Twin System — Research Prototype</span>
        <span>Polling: 3 s &nbsp;|&nbsp; {new Date().getFullYear()}</span>
      </footer>
    </div>
  )
}

export default App
