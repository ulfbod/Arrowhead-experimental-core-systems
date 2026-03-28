import React, { useState, useCallback } from 'react'
import SystemView from './components/SystemView'
import CDTaView from './components/CDTaView'
import CDTbView from './components/CDTbView'
import { ErrorBoundary } from './components/ErrorBoundary'
import usePolling from './hooks/usePolling'
import { urls, startScenario, resetScenario, injectHazard, triggerGasSpike, clearAll } from './api'
import type { ScenarioStatus } from './types'

type Tab = 'system' | 'cdta' | 'cdtb'

const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<Tab>('system')
  const [scenarioLoading, setScenarioLoading] = useState<string | null>(null)

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

        {/* Scenario action buttons */}
        <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
          <button
            className="btn btn-success btn-sm"
            disabled={!!scenarioLoading || scenario?.phase === 'running'}
            onClick={() => runAction('start', startScenario)}
          >
            {scenarioLoading === 'start' ? <span className="loading-spinner" /> : null}
            Start Scenario
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
          { id: 'system', label: 'System View' },
          { id: 'cdta',   label: 'cDTa: Inspection & Recovery' },
          { id: 'cdtb',   label: 'cDTb: Hazard Monitoring' },
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
