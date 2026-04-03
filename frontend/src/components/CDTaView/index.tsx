import React, { useState, useCallback } from 'react'
import usePolling from '../../hooks/usePolling'
import { urls, startMission, abortMission, resetMission, forcePhase, setMappingSpeed, setClearanceSpeed } from '../../api'
import type { MissionStatus, MissionPhase, Hazard } from '../../types'

const PHASES: MissionPhase[] = ['idle','exploring','hazard_scan','clearance','verifying','complete']
const PHASE_LABELS: Record<MissionPhase, string> = {
  idle: 'Idle', exploring: 'Exploring', hazard_scan: 'Hazard Scan',
  clearance: 'Clearance', verifying: 'Verifying', complete: 'Complete', failed: 'Failed',
}

function fmtTs(ts: string | undefined): string {
  if (!ts) return '—'
  try { return new Date(ts).toLocaleTimeString() } catch { return ts }
}

function riskColor(r: string) {
  if (r === 'critical' || r === 'high') return 'var(--red)'
  if (r === 'medium') return 'var(--amber)'
  return 'var(--green)'
}

function clamp(v: number) { return Math.max(0, Math.min(100, v)) }

const Bar: React.FC<{ value: number; color?: string; h?: number }> = ({ value, color = 'var(--blue)', h = 6 }) => (
  <div style={{ width: '100%', height: h, background: 'var(--bg-elevated)', borderRadius: 999, overflow: 'hidden', marginTop: 4 }}>
    <div style={{ height: '100%', width: `${clamp(value)}%`, background: color, borderRadius: 999, transition: 'width 0.4s ease' }} />
  </div>
)

// ---- Phase Stepper ----
const PhaseStepper: React.FC<{ current: MissionPhase }> = ({ current }) => {
  const idx = PHASES.indexOf(current)
  return (
    <div style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 4, paddingTop: 8 }}>
      {PHASES.map((p, i) => {
        const state = i < idx ? 'done' : i === idx ? 'active' : 'pending'
        return (
          <React.Fragment key={p}>
            {i > 0 && <div style={{ width: 20, height: 2, background: state === 'pending' ? 'var(--border)' : 'var(--green)', flexShrink: 0 }} />}
            <div style={{
              padding: '5px 12px', borderRadius: 999, fontSize: '0.72rem',
              fontWeight: state === 'active' ? 700 : 400,
              background: state === 'active' ? 'var(--blue)' : state === 'done' ? 'rgba(76,175,80,0.1)' : 'var(--bg-elevated)',
              border: `1px solid ${state === 'active' ? 'var(--blue)' : state === 'done' ? 'var(--green)' : 'var(--border)'}`,
              color: state === 'active' ? '#fff' : state === 'done' ? 'var(--green)' : 'var(--text-muted)',
              boxShadow: state === 'active' ? '0 0 10px rgba(33,150,243,0.35)' : 'none',
              whiteSpace: 'nowrap',
            }}>
              {state === 'done' && '✓ '}{PHASE_LABELS[p]}
            </div>
          </React.Fragment>
        )
      })}
    </div>
  )
}

// ---- Component Status Cards ----
const ComponentCards: React.FC<{ m: MissionStatus }> = ({ m }) => {
  const mapping  = m.mapping
  const hazards  = m.hazards
  const clear    = m.clearance
  const interv   = m.intervention

  const activeHazards  = hazards?.hazards?.filter(h => !h.cleared).length ?? 0
  const clearedHazards = hazards?.hazards?.filter(h => h.cleared).length ?? 0

  const cards = [
    {
      title: 'cDT1 Mapping',
      value: `${Math.round(mapping?.coveragePct ?? 0)}%`,
      sub: `${mapping?.activeRobots ?? 0} robot(s) active`,
      detail: mapping ? `${mapping.coveredAreaSqm.toFixed(0)} / ${mapping.totalAreaSqm.toFixed(0)} m²` : '—',
      color: 'var(--blue)',
      progress: mapping?.coveragePct ?? 0,
    },
    {
      title: 'cDT3 Risk Level',
      value: (hazards?.overallRisk ?? 'unknown').toUpperCase(),
      sub: `${activeHazards} active hazards`,
      detail: `${clearedHazards} cleared`,
      color: riskColor(hazards?.overallRisk ?? 'low'),
      progress: null as number | null,
    },
    {
      title: 'cDT4 Clearance',
      value: `${Math.round(clear?.totalDebrisPct ?? 0)}%`,
      sub: clear?.routeClear ? 'Route clear' : 'In progress',
      detail: `ETA: ${clear?.estimatedEtaMinutes ?? '—'} min`,
      color: 'var(--green)',
      progress: clear?.totalDebrisPct ?? 0,
    },
    {
      title: 'cDT5 Operator',
      value: interv?.active ? 'ACTIVE' : 'STANDBY',
      sub: interv?.operatorPresent ? 'Operator present' : 'No operator',
      detail: interv?.lastCommand || '—',
      color: interv?.active ? 'var(--green)' : 'var(--text-secondary)',
      progress: null as number | null,
    },
  ]

  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(180px,1fr))', gap: 12 }}>
      {cards.map(c => (
        <div key={c.title} className="card card-sm" style={{ borderLeft: `3px solid ${c.color}` }}>
          <div style={{ fontSize: '0.65rem', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', marginBottom: 6 }}>{c.title}</div>
          <div style={{ fontSize: '1.3rem', fontWeight: 800, color: c.color, lineHeight: 1 }}>{c.value}</div>
          <div style={{ fontSize: '0.72rem', color: 'var(--text-secondary)', marginTop: 4 }}>{c.sub}</div>
          <div style={{ fontSize: '0.68rem', color: 'var(--text-muted)', marginTop: 2 }}>{c.detail}</div>
          {c.progress !== null && <Bar value={c.progress} color={c.color} h={4} />}
        </div>
      ))}
    </div>
  )
}

// ---- Hazard Table ----
const HazardTable: React.FC<{ hazards: Hazard[] }> = ({ hazards }) => (
  <div className="card" style={{ padding: 0 }}>
    <div className="card-header" style={{ padding: '12px 16px 8px' }}>
      <span className="card-title">Detected Hazards</span>
      <div style={{ display: 'flex', gap: 8 }}>
        <span className="badge badge-offline">{hazards.filter(h => !h.cleared).length} active</span>
        <span className="badge badge-online">{hazards.filter(h => h.cleared).length} cleared</span>
      </div>
    </div>
    <div className="table-container" style={{ border: 'none', borderRadius: 0 }}>
      <table>
        <thead><tr><th>Type</th><th>Severity</th><th>Position</th><th>Detected</th><th>Status</th></tr></thead>
        <tbody>
          {hazards.length === 0
            ? <tr><td colSpan={5} style={{ textAlign: 'center', padding: 20, color: 'var(--text-muted)' }}>No hazards detected</td></tr>
            : hazards.map(h => (
              <tr key={h.id} style={{ opacity: h.cleared ? 0.5 : 1 }}>
                <td style={{ fontWeight: 500 }}>{h.type}</td>
                <td><span className={`badge ${h.severity === 'critical' || h.severity === 'high' ? 'badge-offline' : h.severity === 'medium' ? 'badge-warning' : 'badge-online'}`}>{h.severity}</span></td>
                <td className="mono" style={{ fontSize: '0.7rem', color: 'var(--text-secondary)' }}>({h.position.x.toFixed(1)},{h.position.y.toFixed(1)})</td>
                <td style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>{fmtTs(h.detectedAt)}</td>
                <td><span className={`badge ${h.cleared ? 'badge-online' : 'badge-offline'}`}>{h.cleared ? 'Cleared' : 'Active'}</span></td>
              </tr>
            ))
          }
        </tbody>
      </table>
    </div>
  </div>
)

// ---- Recommendations ----
const Recommendations: React.FC<{ recs: string[] }> = ({ recs }) => (
  <div className="card">
    <div className="card-header">
      <span className="card-title">Recommendations</span>
      <span className="badge badge-muted badge-nodot">{recs.length}</span>
    </div>
    {recs.length === 0
      ? <div style={{ padding: 16, color: 'var(--text-muted)', fontSize: '0.8rem' }}>No recommendations</div>
      : <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          {recs.map((r, i) => (
            <div key={i} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', borderRadius: 6, borderLeft: '3px solid var(--blue)', fontSize: '0.82rem', color: 'var(--text-primary)' }}>
              {r}
            </div>
          ))}
        </div>
    }
  </div>
)

// ---- Event Log ----
const EventLog: React.FC<{ logs: string[] }> = ({ logs }) => (
  <div className="card">
    <div className="card-header">
      <span className="card-title">Mission Log</span>
      <span className="badge badge-muted badge-nodot">{logs.length}</span>
    </div>
    <div className="log-list" style={{ maxHeight: 260 }}>
      {logs.length === 0
        ? <div style={{ padding: 16, color: 'var(--text-muted)', fontSize: '0.8rem', textAlign: 'center' }}>No events yet</div>
        : [...logs].reverse().map((entry, i) => (
            <div key={i} className="log-entry info">
              <span className="log-msg">{entry}</span>
            </div>
          ))
      }
    </div>
  </div>
)

// ---- Speed Controls ----
const SpeedControls: React.FC = () => {
  const [mappingDur, setMappingDur] = useState(30)
  const [clearanceDur, setClearanceDur] = useState(30)
  const [loading, setLoading] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const apply = useCallback(async (kind: 'mapping' | 'clearance', dur: number) => {
    setLoading(kind)
    setError(null)
    try {
      if (kind === 'mapping') {
        await setMappingSpeed(dur)
        setMappingDur(dur)
      } else {
        await setClearanceSpeed(dur)
        setClearanceDur(dur)
      }
    } catch (e: any) {
      setError(e?.message ?? 'Error')
    } finally {
      setLoading(null)
    }
  }, [])

  const sliderRow = (
    label: string,
    kind: 'mapping' | 'clearance',
    value: number,
    color: string,
  ) => (
    <div style={{ marginBottom: 14 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
        <span style={{ fontSize: '0.78rem', color: 'var(--text-secondary)' }}>{label}</span>
        <span style={{ fontSize: '0.78rem', fontWeight: 700, color }}>
          {loading === kind ? '…' : `${value} s`}
        </span>
      </div>
      <input
        type="range" min={10} max={100} step={10} value={value}
        disabled={loading === kind}
        onChange={e => apply(kind, Number(e.target.value))}
        style={{ width: '100%', accentColor: color }}
      />
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.65rem', color: 'var(--text-muted)', marginTop: 2 }}>
        <span>10 s (fast)</span><span>50 s</span><span>100 s (slow)</span>
      </div>
    </div>
  )

  return (
    <div className="card">
      <div className="card-header">
        <span className="card-title">Demo Speed</span>
        <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
          time for each operation to reach 100 %
        </span>
      </div>
      {sliderRow('Mapping Progress (iDT1a + iDT1b SLAM rate)', 'mapping', mappingDur, 'var(--blue)')}
      {sliderRow('Clearance Operation (iDT3a + iDT3b debris rate)', 'clearance', clearanceDur, 'var(--green)')}
      {error && (
        <div style={{ fontSize: '0.72rem', color: 'var(--red)', marginTop: 4 }}>⚠ {error}</div>
      )}
      <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)', marginTop: 6, lineHeight: 1.5 }}>
        Changes take effect immediately and survive scenario resets. Phase transitions trigger when
        mapping coverage &gt; 30 % or clearance &gt; 80 %.
      </div>
    </div>
  )
}

// ---- Controls ----
const Controls: React.FC<{ phase: MissionPhase; onRefresh: () => void }> = ({ phase, onRefresh }) => {
  const [loading, setLoading] = useState<string | null>(null)
  const [sel, setSel] = useState<MissionPhase>('exploring')

  const run = useCallback(async (label: string, fn: () => Promise<void>) => {
    setLoading(label)
    try { await fn() } catch (_e) { /* ignore */ } finally { setLoading(null); onRefresh() }
  }, [onRefresh])

  return (
    <div className="card">
      <div className="card-header">
        <span className="card-title">Mission Controls</span>
        <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
          Phase: <strong style={{ color: 'var(--blue)' }}>{PHASE_LABELS[phase] ?? phase}</strong>
        </span>
      </div>
      <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', alignItems: 'center' }}>
        <button className="btn btn-success" disabled={!!loading || phase !== 'idle'} onClick={() => run('start', startMission)}>
          ▶ Start Mission
        </button>
        <button className="btn btn-danger" disabled={!!loading || phase === 'idle' || phase === 'complete'} onClick={() => run('abort', abortMission)}>
          ■ Abort
        </button>
        <button className="btn btn-outline" disabled={!!loading} onClick={() => run('reset', resetMission)}>
          ↺ Reset
        </button>
        <div style={{ display: 'flex', gap: 6, alignItems: 'center', marginLeft: 'auto' }}>
          <select className="select" style={{ width: 150 }} value={sel} onChange={e => setSel(e.target.value as MissionPhase)}>
            {PHASES.map(p => <option key={p} value={p}>{PHASE_LABELS[p]}</option>)}
          </select>
          <button className="btn btn-warning" disabled={!!loading} onClick={() => run('force', () => forcePhase(sel))}>
            Force Phase
          </button>
        </div>
      </div>
    </div>
  )
}

// ---- Main ----
const CDTaView: React.FC = () => {
  const { data: mission, error, loading, refetch } = usePolling<MissionStatus>(urls.mission, 3000)

  if (loading && !mission) return (
    <div className="page-container">
      <div className="card" style={{ display: 'flex', alignItems: 'center', gap: 12, padding: 24, color: 'var(--text-muted)' }}>
        <span className="loading-spinner" /> Connecting to cDTa Mission Controller…
      </div>
    </div>
  )

  return (
    <div className="page-container" style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
        <div>
          <h2 style={{ color: 'var(--text-primary)' }}>cDTa — Inspection &amp; Recovery</h2>
          <p style={{ fontSize: '0.78rem', marginTop: 2 }}>Post-blast mission decision support</p>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {error && <span style={{ fontSize: '0.72rem', color: 'var(--red)' }}>⚠ {error}</span>}
          <button className="btn btn-ghost btn-sm" onClick={refetch}>↻ Refresh</button>
          {mission && <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>Updated {fmtTs(mission.lastUpdated)}</span>}
        </div>
      </div>

      {!mission && error && (
        <div className="card" style={{ color: 'var(--red-light)', padding: 16, fontSize: '0.83rem' }}>
          Cannot connect to cDTa: {error}
        </div>
      )}

      {mission && (
        <>
          {/* Phase */}
          <div className="card">
            <div className="card-header">
              <span className="card-title">Mission Phase</span>
            </div>
            <PhaseStepper current={mission.phase} />
            <div style={{ marginTop: 10, fontSize: '0.72rem', color: 'var(--text-muted)', display: 'flex', gap: 16 }}>
              {mission.startedAt && <span>Started: {fmtTs(mission.startedAt)}</span>}
              {mission.completedAt && <span>Completed: {fmtTs(mission.completedAt)}</span>}
            </div>
          </div>

          {/* Component status */}
          <section>
            <div className="section-title">Component Status</div>
            <ComponentCards m={mission} />
          </section>

          {/* Mapping + Clearance */}
          <section>
            <div className="section-title">Operational Status</div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              {/* Mapping card */}
              <div className="card">
                <div className="card-header">
                  <span className="card-title">Mapping Progress</span>
                  <span className="badge badge-info badge-nodot">{mission.mapping?.activeRobots ?? 0} robots</span>
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                  <span style={{ fontSize: '0.78rem', color: 'var(--text-secondary)' }}>Coverage</span>
                  <span style={{ fontSize: '0.78rem', fontWeight: 700, color: 'var(--blue)' }}>{Math.round(mission.mapping?.coveragePct ?? 0)}%</span>
                </div>
                <Bar value={mission.mapping?.coveragePct ?? 0} color="var(--blue)" h={10} />
                <div style={{ display: 'flex', gap: 16, marginTop: 12 }}>
                  {[
                    { label: 'Area Covered', value: `${(mission.mapping?.coveredAreaSqm ?? 0).toFixed(0)} m²` },
                    { label: 'Total Area', value: `${(mission.mapping?.totalAreaSqm ?? 0).toFixed(0)} m²` },
                  ].map(s => (
                    <div key={s.label} style={{ textAlign: 'center' }}>
                      <div style={{ fontSize: '1.1rem', fontWeight: 800, color: 'var(--text-primary)' }}>{s.value}</div>
                      <div style={{ fontSize: '0.65rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>{s.label}</div>
                    </div>
                  ))}
                </div>
              </div>

              {/* Clearance card */}
              <div className="card">
                <div className="card-header">
                  <span className="card-title">Clearance Operation</span>
                  <span className={`badge ${mission.clearance?.routeClear ? 'badge-online' : 'badge-info'}`}>
                    {mission.clearance?.routeClear ? 'Route Clear' : 'In Progress'}
                  </span>
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                  <span style={{ fontSize: '0.78rem', color: 'var(--text-secondary)' }}>Debris Cleared</span>
                  <span style={{ fontSize: '0.78rem', fontWeight: 700, color: 'var(--green)' }}>{Math.round(mission.clearance?.totalDebrisPct ?? 0)}%</span>
                </div>
                <Bar value={mission.clearance?.totalDebrisPct ?? 0} color="var(--green)" h={10} />
                <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 12, fontSize: '0.82rem' }}>
                  <span style={{ color: 'var(--text-secondary)' }}>Active Vehicles</span>
                  <strong style={{ color: 'var(--text-primary)' }}>{mission.clearance?.activeVehicles ?? 0}</strong>
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 6, fontSize: '0.82rem' }}>
                  <span style={{ color: 'var(--text-secondary)' }}>Est. Completion</span>
                  <strong style={{ color: 'var(--text-primary)' }}>{mission.clearance?.estimatedEtaMinutes ?? '—'} min</strong>
                </div>
              </div>
            </div>
          </section>

          {/* Hazard table */}
          <section>
            <div className="section-title">Hazard Registry</div>
            <HazardTable hazards={mission.hazards?.hazards ?? []} />
          </section>

          {/* Recommendations + Log */}
          <section>
            <div className="section-title">Intelligence</div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <Recommendations recs={mission.recommendations ?? []} />
              <EventLog logs={mission.log ?? []} />
            </div>
          </section>

          {/* Speed + Mission controls */}
          <section>
            <div className="section-title">Controls</div>
            <SpeedControls />
            <div style={{ marginTop: 16 }}>
              <Controls phase={mission.phase} onRefresh={refetch} />
            </div>
          </section>
        </>
      )}
    </div>
  )
}

export default CDTaView
