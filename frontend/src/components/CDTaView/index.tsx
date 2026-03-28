import React, { useState, useCallback } from 'react'
import usePolling from '../../hooks/usePolling'
import {
  urls,
  startMission,
  abortMission,
  resetMission,
  forcePhase,
} from '../../api'
import type {
  MissionStatus,
  MissionPhase,
  Hazard,
  MissionEvent,
  Recommendation,
} from '../../types'

// ============================================================
// Constants
// ============================================================

const PHASES: MissionPhase[] = [
  'idle',
  'exploring',
  'hazard_scan',
  'clearance',
  'verifying',
  'complete',
]

const PHASE_LABELS: Record<MissionPhase, string> = {
  idle:        'Idle',
  exploring:   'Exploring',
  hazard_scan: 'Hazard Scan',
  clearance:   'Clearance',
  verifying:   'Verifying',
  complete:    'Complete',
}

// ============================================================
// Utility helpers
// ============================================================

function fmtTs(ts: string): string {
  try { return new Date(ts).toLocaleTimeString() } catch { return ts }
}

function severityColor(sev: string): string {
  if (sev === 'critical') return 'var(--red)'
  if (sev === 'high')     return 'var(--red)'
  if (sev === 'medium')   return 'var(--amber)'
  return 'var(--green)'
}

function severityBadge(sev: string): string {
  if (sev === 'critical' || sev === 'high') return 'badge-offline'
  if (sev === 'medium')                      return 'badge-warning'
  return 'badge-online'
}

function priorityIcon(priority: string): string {
  if (priority === 'critical') return '🔴'
  if (priority === 'high')     return '🟠'
  if (priority === 'medium')   return '🟡'
  return '🟢'
}

function levelBorderColor(level: string): string {
  if (level === 'error')   return 'var(--red)'
  if (level === 'warning') return 'var(--amber)'
  if (level === 'success') return 'var(--green)'
  return 'var(--blue)'
}

function clamp(v: number, lo = 0, hi = 100) { return Math.max(lo, Math.min(hi, v)) }

// ============================================================
// Sub-components
// ============================================================

interface ProgressBarProps {
  value: number
  color?: string
  height?: number
}
const ProgressBar: React.FC<ProgressBarProps> = ({ value, color = 'var(--blue)', height = 6 }) => (
  <div style={{ width: '100%', height, background: 'var(--bg-elevated)', borderRadius: 999, overflow: 'hidden', marginTop: 4 }}>
    <div style={{ height: '100%', width: `${clamp(value)}%`, background: color, borderRadius: 999, transition: 'width 0.4s ease' }} />
  </div>
)

// ============================================================
// Phase Stepper
// ============================================================

const PhaseStepper: React.FC<{ current: MissionPhase }> = ({ current }) => {
  const currentIdx = PHASES.indexOf(current)
  return (
    <div className="phase-stepper">
      {PHASES.map((phase, idx) => {
        const state = idx < currentIdx ? 'completed' : idx === currentIdx ? 'active' : 'pending'
        return (
          <div key={phase} className="phase-step" style={{ display: 'flex', alignItems: 'center' }}>
            {idx > 0 && (
              <div style={{
                width: 28, height: 2,
                background: state === 'pending' ? 'var(--border)' : 'var(--green)',
                flexShrink: 0,
              }} />
            )}
            <div style={{
              padding: '6px 14px',
              borderRadius: 999,
              fontSize: '0.75rem',
              fontWeight: state === 'active' ? 700 : 400,
              background:
                state === 'active'    ? 'var(--blue)'                           :
                state === 'completed' ? 'rgba(76,175,80,0.12)'                  :
                'var(--bg-elevated)',
              border: `1px solid ${
                state === 'active'    ? 'var(--blue)'  :
                state === 'completed' ? 'var(--green)' :
                'var(--border)'}`,
              color:
                state === 'active'    ? '#fff'           :
                state === 'completed' ? 'var(--green)'   :
                'var(--text-muted)',
              boxShadow: state === 'active' ? '0 0 12px rgba(33,150,243,0.4)' : 'none',
              whiteSpace: 'nowrap',
              transition: 'all 150ms ease',
              display: 'flex',
              alignItems: 'center',
              gap: 5,
            }}>
              {state === 'completed' && <span style={{ fontSize: '0.65rem' }}>✓</span>}
              {PHASE_LABELS[phase]}
            </div>
          </div>
        )
      })}
    </div>
  )
}

// ============================================================
// Component Status Cards
// ============================================================

const ComponentStatusCards: React.FC<{ mission: MissionStatus }> = ({ mission }) => {
  const { mapping, hazardReport, clearance, intervention } = mission

  const cards = [
    {
      title: 'cDT1 Mapping',
      value: `${Math.round(mapping.coveragePercent)}%`,
      sub: `${mapping.activeRobots.length} robot(s) active`,
      detail: `${mapping.areaCoveredM2.toFixed(0)} / ${mapping.totalAreaM2.toFixed(0)} m²`,
      color: 'var(--blue)',
      progress: mapping.coveragePercent,
      status: mapping.status,
    },
    {
      title: 'cDT3 Risk Level',
      value: hazardReport.riskLevel.toUpperCase(),
      sub: `${hazardReport.activeHazards} active hazards`,
      detail: `${hazardReport.clearedHazards} cleared`,
      color: severityColor(hazardReport.riskLevel),
      progress: null,
      status: hazardReport.riskLevel,
    },
    {
      title: 'cDT4 Clearance',
      value: `${Math.round(clearance.clearancePercent)}%`,
      sub: clearance.status,
      detail: `${clearance.hazardsCleared} cleared / ${clearance.hazardsPending} pending`,
      color: 'var(--green)',
      progress: clearance.clearancePercent,
      status: clearance.status,
    },
    {
      title: 'cDT5 Operator',
      value: intervention.status.toUpperCase(),
      sub: intervention.operatorAssigned ?? 'No operator',
      detail: intervention.reason ?? '—',
      color:
        intervention.status === 'active'    ? 'var(--green)'  :
        intervention.status === 'requested' ? 'var(--amber)'  :
        intervention.status === 'aborted'   ? 'var(--red)'    : 'var(--text-secondary)',
      progress: null,
      status: intervention.status,
    },
  ]

  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: 12 }}>
      {cards.map(c => (
        <div key={c.title} className="card card-sm" style={{ borderLeft: `3px solid ${c.color}` }}>
          <div style={{ fontSize: '0.68rem', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', marginBottom: 6 }}>
            {c.title}
          </div>
          <div style={{ fontSize: '1.4rem', fontWeight: 800, color: c.color, lineHeight: 1 }}>
            {c.value}
          </div>
          <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: 4 }}>{c.sub}</div>
          <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)', marginTop: 2 }}>{c.detail}</div>
          {c.progress !== null && (
            <ProgressBar value={c.progress} color={c.color} height={4} />
          )}
        </div>
      ))}
    </div>
  )
}

// ============================================================
// Mapping Progress Display
// ============================================================

const MappingProgress: React.FC<{ mission: MissionStatus }> = ({ mission }) => {
  const m = mission.mapping
  return (
    <div className="card">
      <div className="card-header">
        <span className="card-title">Mapping Progress</span>
        <span className={`badge ${
          m.status === 'mapping'  ? 'badge-info'    :
          m.status === 'complete' ? 'badge-online'  :
          m.status === 'paused'   ? 'badge-warning' : 'badge-muted'
        }`}>
          {m.status}
        </span>
      </div>
      <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap' }}>
        {/* Coverage bar */}
        <div style={{ flex: '1 1 200px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
            <span style={{ fontSize: '0.78rem', color: 'var(--text-secondary)' }}>Coverage</span>
            <span style={{ fontSize: '0.78rem', fontWeight: 700, color: 'var(--blue)' }}>
              {Math.round(m.coveragePercent)}%
            </span>
          </div>
          <ProgressBar value={m.coveragePercent} color="var(--blue)" height={10} />
        </div>

        {/* Stats */}
        <div style={{ display: 'flex', gap: 20, flexShrink: 0, flexWrap: 'wrap' }}>
          {[
            { label: 'Active Robots', value: m.activeRobots.length.toString() },
            { label: 'Area Covered',  value: `${m.areaCoveredM2.toFixed(0)} m²` },
            { label: 'Total Area',    value: `${m.totalAreaM2.toFixed(0)} m²` },
          ].map(s => (
            <div key={s.label} style={{ textAlign: 'center' }}>
              <div style={{ fontSize: '1.3rem', fontWeight: 800, color: 'var(--text-primary)' }}>{s.value}</div>
              <div style={{ fontSize: '0.68rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>{s.label}</div>
            </div>
          ))}
        </div>
      </div>

      {/* Active robots */}
      {m.activeRobots.length > 0 && (
        <div style={{ marginTop: 12, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          {m.activeRobots.map(r => (
            <span key={r} className="badge badge-info badge-nodot" style={{ fontSize: '0.72rem' }}>
              {r}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}

// ============================================================
// Hazard List Table
// ============================================================

const HazardTable: React.FC<{ hazards: Hazard[] }> = ({ hazards }) => (
  <div className="card" style={{ padding: 0 }}>
    <div className="card-header" style={{ padding: '14px 18px 10px' }}>
      <span className="card-title">Detected Hazards</span>
      <div style={{ display: 'flex', gap: 8 }}>
        <span className="badge badge-offline">{hazards.filter(h => !h.cleared).length} active</span>
        <span className="badge badge-online">{hazards.filter(h => h.cleared).length} cleared</span>
      </div>
    </div>
    <div className="table-container" style={{ border: 'none', borderRadius: 0 }}>
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>Type</th>
            <th>Severity</th>
            <th>Position</th>
            <th>Detected</th>
            <th>Detected By</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {hazards.length === 0 ? (
            <tr>
              <td colSpan={7} style={{ textAlign: 'center', padding: 24, color: 'var(--text-muted)' }}>
                No hazards detected
              </td>
            </tr>
          ) : (
            hazards.map(h => (
              <tr key={h.id} style={{ opacity: h.cleared ? 0.5 : 1 }}>
                <td className="mono" style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>{h.id}</td>
                <td style={{ color: 'var(--text-primary)', fontWeight: 500 }}>{h.type}</td>
                <td>
                  <span className={`badge ${severityBadge(h.severity)}`}>{h.severity}</span>
                </td>
                <td className="mono" style={{ fontSize: '0.72rem', color: 'var(--text-secondary)' }}>
                  ({h.position.x.toFixed(1)}, {h.position.y.toFixed(1)}, {h.position.z.toFixed(1)})
                </td>
                <td style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>{fmtTs(h.detectedAt)}</td>
                <td style={{ color: 'var(--text-secondary)', fontSize: '0.78rem' }}>{h.detectedBy}</td>
                <td>
                  <span className={`badge ${h.cleared ? 'badge-online' : 'badge-offline'}`}>
                    {h.cleared ? 'Cleared' : 'Active'}
                  </span>
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  </div>
)

// ============================================================
// Recommendations Panel
// ============================================================

const RecommendationsPanel: React.FC<{ recs: Recommendation[] }> = ({ recs }) => (
  <div className="card">
    <div className="card-header">
      <span className="card-title">Mission Recommendations</span>
      <span className="badge badge-muted badge-nodot">{recs.length}</span>
    </div>
    {recs.length === 0 ? (
      <div className="empty-state" style={{ padding: '20px' }}>
        No recommendations at this time
      </div>
    ) : (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {recs.map(r => (
          <div key={r.id} style={{
            display: 'flex',
            gap: 10,
            padding: '10px 12px',
            background: 'var(--bg-elevated)',
            borderRadius: 'var(--radius)',
            borderLeft: `3px solid ${severityColor(r.priority)}`,
          }}>
            <span style={{ fontSize: '1rem', flexShrink: 0 }}>{priorityIcon(r.priority)}</span>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontSize: '0.82rem', color: 'var(--text-primary)', lineHeight: 1.4 }}>{r.text}</div>
              {r.action && (
                <div style={{ fontSize: '0.72rem', color: 'var(--blue-light)', marginTop: 3 }}>
                  → {r.action}
                </div>
              )}
            </div>
            <span className={`badge ${severityBadge(r.priority)} badge-nodot`} style={{ flexShrink: 0, alignSelf: 'flex-start' }}>
              {r.priority}
            </span>
          </div>
        ))}
      </div>
    )}
  </div>
)

// ============================================================
// Mission Event Log
// ============================================================

const MissionEventLog: React.FC<{ events: MissionEvent[] }> = ({ events }) => (
  <div className="card">
    <div className="card-header">
      <span className="card-title">Mission Event Log</span>
      <span className="badge badge-muted badge-nodot">{events.length} entries</span>
    </div>
    <div className="log-list" style={{ maxHeight: 280 }}>
      {events.length === 0 ? (
        <div style={{ padding: '20px', color: 'var(--text-muted)', textAlign: 'center', fontSize: '0.8rem' }}>
          No events yet
        </div>
      ) : (
        [...events].reverse().map(ev => (
          <div key={ev.id} className={`log-entry ${ev.level}`}>
            <span className="log-ts">{fmtTs(ev.timestamp)}</span>
            <span style={{
              fontSize: '0.68rem',
              background: 'var(--bg-primary)',
              padding: '1px 5px',
              borderRadius: 4,
              color: 'var(--text-muted)',
              flexShrink: 0,
            }}>
              {PHASE_LABELS[ev.phase]}
            </span>
            <span className="log-msg">{ev.event}</span>
          </div>
        ))
      )}
    </div>
  </div>
)

// ============================================================
// Mission Controls
// ============================================================

const MissionControls: React.FC<{ phase: MissionPhase; onRefresh: () => void }> = ({ phase, onRefresh }) => {
  const [loading, setLoading] = useState<string | null>(null)
  const [selectedPhase, setSelectedPhase] = useState<MissionPhase>('idle')

  const run = useCallback(async (label: string, fn: () => Promise<void>) => {
    setLoading(label)
    try { await fn() } catch (_e) { /* backend may be down */ } finally {
      setLoading(null)
      onRefresh()
    }
  }, [onRefresh])

  return (
    <div className="card">
      <div className="card-header">
        <span className="card-title">Mission Controls</span>
        <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
          Current phase: <strong style={{ color: 'var(--blue)' }}>{PHASE_LABELS[phase]}</strong>
        </span>
      </div>
      <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', alignItems: 'center' }}>
        <button
          className="btn btn-success"
          disabled={!!loading || phase !== 'idle'}
          onClick={() => run('start', startMission)}
        >
          {loading === 'start' ? <span className="loading-spinner" /> : '▶'} Start Mission
        </button>

        <button
          className="btn btn-danger"
          disabled={!!loading || phase === 'idle' || phase === 'complete'}
          onClick={() => run('abort', abortMission)}
        >
          {loading === 'abort' ? <span className="loading-spinner" /> : '■'} Abort
        </button>

        <button
          className="btn btn-outline"
          disabled={!!loading}
          onClick={() => run('reset', resetMission)}
        >
          {loading === 'reset' ? <span className="loading-spinner" /> : '↺'} Reset
        </button>

        <div style={{ display: 'flex', gap: 6, alignItems: 'center', marginLeft: 'auto', flexWrap: 'wrap' }}>
          <select
            className="select"
            style={{ width: 160 }}
            value={selectedPhase}
            onChange={e => setSelectedPhase(e.target.value as MissionPhase)}
          >
            {PHASES.map(p => (
              <option key={p} value={p}>{PHASE_LABELS[p]}</option>
            ))}
          </select>
          <button
            className="btn btn-warning"
            disabled={!!loading}
            onClick={() => run('force', () => forcePhase(selectedPhase))}
          >
            {loading === 'force' ? <span className="loading-spinner" /> : null}
            Force Phase
          </button>
        </div>
      </div>
    </div>
  )
}

// ============================================================
// Loading/Error placeholder
// ============================================================

const LoadingCard: React.FC<{ msg: string }> = ({ msg }) => (
  <div className="card" style={{ display: 'flex', alignItems: 'center', gap: 12, padding: 24, color: 'var(--text-muted)' }}>
    <span className="loading-spinner" />
    <span>{msg}</span>
  </div>
)

const ErrorCard: React.FC<{ msg: string }> = ({ msg }) => (
  <div className="card card-highlight-red" style={{ padding: 16, color: 'var(--red-light)', fontSize: '0.83rem' }}>
    Connection error: {msg}
  </div>
)

// ============================================================
// Main CDTaView
// ============================================================

const CDTaView: React.FC = () => {
  const { data: mission, error, loading, refetch } = usePolling<MissionStatus>(urls.mission, 3000)

  if (loading && !mission) return (
    <div className="page-container">
      <LoadingCard msg="Connecting to cDTa Mission Controller…" />
    </div>
  )

  return (
    <div className="page-container" style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>

      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
        <div>
          <h2 style={{ color: 'var(--text-primary)' }}>cDTa — Inspection &amp; Recovery</h2>
          <p style={{ fontSize: '0.78rem', marginTop: 2 }}>Mission decision support system</p>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {error && <span style={{ fontSize: '0.72rem', color: 'var(--red)' }}>⚠ {error}</span>}
          <button className="btn btn-ghost btn-sm" onClick={refetch}>
            ↻ Refresh
          </button>
          {mission && (
            <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
              Updated {fmtTs(mission.lastUpdate)}
            </span>
          )}
        </div>
      </div>

      {/* Error banner */}
      {error && !mission && <ErrorCard msg={error} />}

      {mission && (
        <>
          {/* Phase Stepper */}
          <div className="card">
            <div className="card-header">
              <span className="card-title">Mission Phase</span>
              <span className="badge badge-info badge-nodot">
                {mission.missionId ? `ID: ${mission.missionId}` : 'No active mission'}
              </span>
            </div>
            <PhaseStepper current={mission.phase} />
            {mission.startTime && (
              <div style={{ marginTop: 10, fontSize: '0.72rem', color: 'var(--text-muted)', display: 'flex', gap: 16 }}>
                <span>Started: {fmtTs(mission.startTime)}</span>
                {mission.endTime && <span>Ended: {fmtTs(mission.endTime)}</span>}
              </div>
            )}
          </div>

          {/* Component Status Cards */}
          <section>
            <div className="section-title">Component Status</div>
            <ComponentStatusCards mission={mission} />
          </section>

          {/* Mapping + Hazard side by side */}
          <section>
            <div className="section-title">Operational Status</div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <MappingProgress mission={mission} />

              {/* Clearance summary */}
              <div className="card">
                <div className="card-header">
                  <span className="card-title">Clearance Operation</span>
                  <span className={`badge ${
                    mission.clearance.status === 'complete'     ? 'badge-online'  :
                    mission.clearance.status === 'in_progress'  ? 'badge-info'    :
                    mission.clearance.status === 'failed'        ? 'badge-offline' : 'badge-muted'
                  }`}>
                    {mission.clearance.status}
                  </span>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                  <div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                      <span style={{ fontSize: '0.78rem', color: 'var(--text-secondary)' }}>Clearance Progress</span>
                      <span style={{ fontSize: '0.78rem', fontWeight: 700, color: 'var(--green)' }}>
                        {Math.round(mission.clearance.clearancePercent)}%
                      </span>
                    </div>
                    <ProgressBar value={mission.clearance.clearancePercent} color="var(--green)" height={8} />
                  </div>
                  {[
                    { label: 'Hazards Cleared',  value: mission.clearance.hazardsCleared.toString(), color: 'var(--green)' },
                    { label: 'Hazards Pending',  value: mission.clearance.hazardsPending.toString(),  color: 'var(--amber)' },
                    { label: 'Est. Completion',  value: `${mission.clearance.estimatedCompletionMin} min`, color: 'var(--text-primary)' },
                  ].map(s => (
                    <div key={s.label} style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.82rem' }}>
                      <span style={{ color: 'var(--text-secondary)' }}>{s.label}</span>
                      <strong style={{ color: s.color }}>{s.value}</strong>
                    </div>
                  ))}
                  {mission.clearance.activeAssets.length > 0 && (
                    <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 4 }}>
                      {mission.clearance.activeAssets.map(a => (
                        <span key={a} className="badge badge-info badge-nodot" style={{ fontSize: '0.68rem' }}>{a}</span>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </div>
          </section>

          {/* Hazard Table */}
          <section>
            <div className="section-title">Hazard Registry</div>
            <HazardTable hazards={mission.hazardReport.hazards} />
          </section>

          {/* Recommendations + Event Log */}
          <section>
            <div className="section-title">Intelligence</div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <RecommendationsPanel recs={mission.recommendations} />
              <MissionEventLog events={mission.eventLog} />
            </div>
          </section>

          {/* Mission Controls */}
          <section>
            <div className="section-title">Controls</div>
            <MissionControls phase={mission.phase} onRefresh={refetch} />
          </section>
        </>
      )}
    </div>
  )
}

export default CDTaView
