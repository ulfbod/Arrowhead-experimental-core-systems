import { useState, useEffect } from 'react'
import {
  getCDT2Providers,
  getCDT1Providers,
  setOrchestrationMode,
  setNetworkDelay,
  runExperiment,
  getExperimentResults,
  failSensor,
  recoverSensor,
} from '../../api'
import type { SourceQoS, ExperimentResults, FailoverEvent } from '../../types'

// ---- Sub-components ----------------------------------------------------------------

function QoSBadge({ value, label }: { value: number; label: string }) {
  const pct = Math.round(value * 100)
  const color = pct >= 90 ? '#16a34a' : pct >= 70 ? '#d97706' : '#dc2626'
  return (
    <span style={{ marginRight: 12, fontSize: 13 }}>
      <span style={{ color: '#64748b' }}>{label}: </span>
      <span style={{ color, fontWeight: 600 }}>{label === 'Latency' ? `${value.toFixed(1)}ms` : `${pct}%`}</span>
    </span>
  )
}

function ProviderRow({ p, isActive }: { p: import('../../types').ProviderState; isActive: boolean }) {
  const bg = isActive ? '#dbeafe' : '#f8fafc'
  const border = isActive ? '1px solid #3b82f6' : '1px solid #cbd5e1'
  return (
    <div style={{ padding: '8px 12px', background: bg, border, borderRadius: 6, marginBottom: 6 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
        <span style={{ fontWeight: 600, color: '#0f172a' }}>{p.id}</span>
        {p.primary && <span style={{ fontSize: 11, background: '#dbeafe', color: '#1d4ed8', padding: '1px 6px', borderRadius: 3 }}>PRIMARY</span>}
        {isActive && <span style={{ fontSize: 11, background: '#dcfce7', color: '#166534', padding: '1px 6px', borderRadius: 3 }}>ACTIVE</span>}
        {!p.online && <span style={{ fontSize: 11, background: '#fee2e2', color: '#dc2626', padding: '1px 6px', borderRadius: 3 }}>OFFLINE</span>}
        {p.degraded && <span style={{ fontSize: 11, background: '#fef3c7', color: '#92400e', padding: '1px 6px', borderRadius: 3 }}>DEGRADED</span>}
      </div>
      <div>
        <QoSBadge value={p.qos.accuracy} label="Accuracy" />
        <QoSBadge value={p.qos.latencyMs} label="Latency" />
        <QoSBadge value={p.qos.reliability} label="Reliability" />
      </div>
    </div>
  )
}

function ProviderPanel({ title, qos }: { title: string; qos: SourceQoS | null }) {
  if (!qos) return (
    <div style={{ background: '#f8fafc', border: '1px solid #cbd5e1', borderRadius: 8, padding: 16, marginBottom: 16 }}>
      <div style={{ color: '#64748b' }}>{title} – loading…</div>
    </div>
  )
  const degradedBadge = qos.degraded
    ? <span style={{ marginLeft: 8, fontSize: 11, background: '#fef3c7', color: '#92400e', padding: '2px 6px', borderRadius: 3 }}>DEGRADED</span>
    : null
  return (
    <div style={{ background: '#f8fafc', border: '1px solid #cbd5e1', borderRadius: 8, padding: 16, marginBottom: 16 }}>
      <div style={{ fontWeight: 600, color: '#0f172a', marginBottom: 10 }}>
        {title} – <span style={{ color: '#64748b', fontSize: 13 }}>{qos.capability}</span>{degradedBadge}
      </div>
      {qos.providers.map(p => <ProviderRow key={p.id} p={p} isActive={p.active} />)}
      {qos.recentFailovers.length > 0 && (
        <FailoverHistory events={qos.recentFailovers} />
      )}
    </div>
  )
}

function FailoverHistory({ events }: { events: FailoverEvent[] }) {
  const recent = [...events].reverse().slice(0, 5)
  return (
    <div style={{ marginTop: 10 }}>
      <div style={{ fontSize: 12, color: '#94a3b8', marginBottom: 6 }}>Recent failover events</div>
      {recent.map(ev => (
        <div key={ev.eventId} style={{ background: '#f8fafc', border: '1px solid #e2e8f0', borderRadius: 4, padding: '6px 10px', marginBottom: 4, fontSize: 12 }}>
          <div style={{ color: '#0f172a' }}>
            <span style={{ color: '#dc2626' }}>{ev.prevProvider}</span>
            {' → '}
            <span style={{ color: '#16a34a' }}>{ev.nextProvider}</span>
            <span style={{ color: '#64748b', marginLeft: 8 }}>
              mode={ev.orchestrationMode} net={ev.networkDelayMs}ms
            </span>
          </div>
          <div style={{ color: '#64748b', marginTop: 2 }}>
            decision={ev.decisionDelayMs.toFixed(1)}ms &nbsp;
            total={ev.failToSwitchMs.toFixed(0)}ms &nbsp;
            acc: {(ev.qosBefore.accuracy * 100).toFixed(0)}% → {(ev.qosAfter.accuracy * 100).toFixed(0)}%
          </div>
        </div>
      ))}
    </div>
  )
}

function ExperimentControl({
  orchMode, setOrchMode, netDelay, setNetDelay, onRunExperiment, expRunning,
}: {
  orchMode: 'local' | 'central'
  setOrchMode: (m: 'local' | 'central') => void
  netDelay: number
  setNetDelay: (ms: number) => void
  onRunExperiment: () => void
  expRunning: boolean
}) {
  const btnStyle = (active: boolean) => ({
    padding: '6px 14px', borderRadius: 4, border: '1px solid', cursor: 'pointer', fontSize: 13, fontWeight: 600,
    background: active ? '#2563eb' : '#f1f5f9',
    color: active ? '#fff' : '#1e293b',
    borderColor: active ? '#2563eb' : '#94a3b8',
  })

  return (
    <div style={{ background: '#f8fafc', border: '1px solid #cbd5e1', borderRadius: 8, padding: 16, marginBottom: 16 }}>
      <div style={{ fontWeight: 600, color: '#0f172a', marginBottom: 12 }}>Experiment Controls</div>

      {/* Orchestration mode */}
      <div style={{ marginBottom: 12 }}>
        <div style={{ fontSize: 12, color: '#64748b', marginBottom: 6 }}>Orchestration Mode</div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button style={btnStyle(orchMode === 'local')} onClick={() => setOrchMode('local')}>Local Failover</button>
          <button style={btnStyle(orchMode === 'central')} onClick={() => setOrchMode('central')}>Centralized (Arrowhead)</button>
        </div>
      </div>

      {/* Network delay */}
      <div style={{ marginBottom: 12 }}>
        <div style={{ fontSize: 12, color: '#64748b', marginBottom: 6 }}>
          Simulated Network Delay: <strong style={{ color: '#0f172a' }}>{netDelay}ms</strong>
        </div>
        <input
          type="range" min={0} max={50} step={5} value={netDelay}
          onChange={e => setNetDelay(Number(e.target.value))}
          style={{ width: '100%', accentColor: '#3b82f6' }}
        />
        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: '#64748b' }}>
          <span>0ms</span><span>10ms</span><span>20ms</span><span>30ms</span><span>40ms</span><span>50ms</span>
        </div>
      </div>

      {/* Quick fault injection */}
      <div style={{ marginBottom: 12 }}>
        <div style={{ fontSize: 12, color: '#64748b', marginBottom: 6 }}>Manual Fault Injection (idt2a)</div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button style={{ padding: '5px 12px', background: '#fee2e2', color: '#dc2626', border: '1px solid #fca5a5', borderRadius: 4, cursor: 'pointer', fontSize: 12, fontWeight: 600 }}
            onClick={() => failSensor('idt2a')}>
            Fail idt2a
          </button>
          <button style={{ padding: '5px 12px', background: '#dcfce7', color: '#166534', border: '1px solid #86efac', borderRadius: 4, cursor: 'pointer', fontSize: 12, fontWeight: 600 }}
            onClick={() => recoverSensor('idt2a')}>
            Recover idt2a
          </button>
        </div>
      </div>

      {/* Experiment runner */}
      <div style={{ borderTop: '1px solid #e2e8f0', paddingTop: 12 }}>
        <div style={{ fontSize: 12, color: '#64748b', marginBottom: 6 }}>
          Full Experiment (0–50ms × local/central × 5 runs each → CSV)
        </div>
        <button
          style={{ padding: '8px 16px', background: expRunning ? '#dbeafe' : '#2563eb', color: expRunning ? '#1d4ed8' : '#fff', border: 'none', borderRadius: 4, cursor: expRunning ? 'not-allowed' : 'pointer', fontSize: 13, fontWeight: 600 }}
          onClick={onRunExperiment}
          disabled={expRunning}
        >
          {expRunning ? '⏳ Running…' : '▶ Run Full Experiment'}
        </button>
      </div>
    </div>
  )
}

function ExperimentResultsTable({ results }: { results: ExperimentResults }) {
  return (
    <div style={{ background: '#f8fafc', border: '1px solid #cbd5e1', borderRadius: 8, padding: 16, marginBottom: 16 }}>
      <div style={{ fontWeight: 600, color: '#0f172a', marginBottom: 4 }}>
        Experiment Results
        <span style={{ fontSize: 11, color: '#64748b', marginLeft: 8 }}>
          CSV: {results.csvPath}
        </span>
      </div>
      <div style={{ fontSize: 12, color: '#64748b', marginBottom: 10 }}>
        Completed {new Date(results.completedAt).toLocaleTimeString()}
        {' | '}
        Total runs: {results.runs.filter(r => r.success).length}/{results.runs.length} successful
      </div>

      <div style={{ overflowX: 'auto' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
          <thead>
            <tr style={{ borderBottom: '1px solid #e2e8f0' }}>
              {['Network Delay (ms)', 'Local Decision (ms)', 'Central Decision (ms)', 'Local Runs', 'Central Runs'].map(h => (
                <th key={h} style={{ padding: '6px 10px', color: '#64748b', textAlign: 'left', background: '#f8fafc' }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {results.summary.map(s => (
              <tr key={s.networkDelayMs} style={{ borderBottom: '1px solid #e2e8f0' }}>
                <td style={{ padding: '5px 10px', color: '#0f172a' }}>{s.networkDelayMs}</td>
                <td style={{ padding: '5px 10px', color: '#16a34a', fontWeight: 600 }}>{s.avgLocalDecisionMs.toFixed(1)}</td>
                <td style={{ padding: '5px 10px', color: '#dc2626', fontWeight: 600 }}>{s.avgCentralDecisionMs.toFixed(1)}</td>
                <td style={{ padding: '5px 10px', color: '#64748b' }}>{s.localRuns}</td>
                <td style={{ padding: '5px 10px', color: '#64748b' }}>{s.centralRuns}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div style={{ marginTop: 12, padding: 10, background: '#f1f5f9', border: '1px solid #cbd5e1', borderRadius: 6, fontSize: 11, color: '#334155', fontFamily: 'monospace' }}>
        <div style={{ color: '#64748b', marginBottom: 4 }}># gnuplot command:</div>
        <div>{'plot "failover_delay_vs_network_delay.csv" using 1:2 with lines title "Local", \\'}</div>
        <div>{'     "" using 1:3 with lines title "Centralized"'}</div>
      </div>
    </div>
  )
}

// ---- Main view ---------------------------------------------------------------------

export default function QoSView() {
  const [cdt1QoS, setCdt1QoS] = useState<SourceQoS | null>(null)
  const [cdt2QoS, setCdt2QoS] = useState<SourceQoS | null>(null)
  const [orchMode, setOrchModeState] = useState<'local' | 'central'>('local')
  const [netDelay, setNetDelayState] = useState(0)
  const [expRunning, setExpRunning] = useState(false)
  const [expResults, setExpResults] = useState<ExperimentResults | null>(null)
  const [error, setError] = useState<string | null>(null)

  // Poll QoS state every 2 seconds
  useEffect(() => {
    const poll = async () => {
      try {
        const [q1, q2] = await Promise.all([getCDT1Providers(), getCDT2Providers()])
        setCdt1QoS(q1)
        setCdt2QoS(q2)
      } catch (e) {
        // silently ignore poll errors
      }
    }
    poll()
    const id = setInterval(poll, 2000)
    return () => clearInterval(id)
  }, [])

  // Poll experiment results while running
  useEffect(() => {
    if (!expRunning) return
    const poll = async () => {
      try {
        const resp = await getExperimentResults()
        if (resp.status === 'completed' && resp.results) {
          setExpResults(resp.results)
          setExpRunning(false)
        }
      } catch { /* ignore */ }
    }
    const id = setInterval(poll, 2000)
    return () => clearInterval(id)
  }, [expRunning])

  const handleSetOrchMode = async (mode: 'local' | 'central') => {
    try {
      await setOrchestrationMode(mode)
      setOrchModeState(mode)
      setError(null)
    } catch (e: any) {
      setError(`Failed to set mode: ${e.message}`)
    }
  }

  const handleSetNetDelay = async (ms: number) => {
    setNetDelayState(ms)
    try {
      await setNetworkDelay(ms)
      setError(null)
    } catch (e: any) {
      setError(`Failed to set delay: ${e.message}`)
    }
  }

  const handleRunExperiment = async () => {
    try {
      await runExperiment(5)
      setExpRunning(true)
      setError(null)
    } catch (e: any) {
      setError(`Failed to start experiment: ${e.message}`)
    }
  }

  return (
    <div style={{ padding: 20, color: '#0f172a' }}>
      <h2 style={{ margin: '0 0 4px', color: '#0f172a' }}>QoS &amp; Failover Evaluation</h2>
      <p style={{ margin: '0 0 20px', color: '#64748b', fontSize: 14 }}>
        Monitor provider health, inject failures, and run network latency experiments.
      </p>

      {error && (
        <div style={{ background: '#fee2e2', border: '1px solid #fca5a5', color: '#dc2626', padding: '8px 12px', borderRadius: 6, marginBottom: 12, fontSize: 13 }}>
          {error}
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        {/* Left column: controls + experiment */}
        <div>
          <ExperimentControl
            orchMode={orchMode}
            setOrchMode={handleSetOrchMode}
            netDelay={netDelay}
            setNetDelay={handleSetNetDelay}
            onRunExperiment={handleRunExperiment}
            expRunning={expRunning}
          />
          {expResults && <ExperimentResultsTable results={expResults} />}
        </div>

        {/* Right column: provider QoS state */}
        <div>
          <ProviderPanel title="cDT2 – Gas Monitoring" qos={cdt2QoS} />
          <ProviderPanel title="cDT1 – Mapping" qos={cdt1QoS} />
        </div>
      </div>
    </div>
  )
}
