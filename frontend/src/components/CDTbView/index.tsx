import React, { useState, useCallback } from 'react'
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip,
  ReferenceLine, ResponsiveContainer, Cell,
} from 'recharts'
import usePolling from '../../hooks/usePolling'
import { urls, openGate, closeGate } from '../../api'
import type { SafeAccessDecision, GasAlert } from '../../types'

// Gas thresholds matching Go backend logic
const GAS_THRESHOLDS: Record<string, number> = { ch4: 1.0, co: 25, co2: 2.0, o2: 19.5, no2: 3 }
const GAS_UNITS:      Record<string, string>  = { ch4: '%', co: 'ppm', co2: '%', o2: '%', no2: 'ppm' }
const GAS_LABELS:     Record<string, string>  = { ch4: 'CH₄', co: 'CO', co2: 'CO₂', o2: 'O₂', no2: 'NO₂' }

function fmtTs(ts: string | undefined) {
  if (!ts) return '—'
  try { return new Date(ts).toLocaleTimeString() } catch { return ts }
}

function gatingColor(s: string) {
  if (s === 'open')        return 'var(--green)'
  if (s === 'conditional') return 'var(--amber)'
  return 'var(--red)'
}

// ---- Safe/Unsafe Hero Card ----
const SafeHero: React.FC<{ decision: SafeAccessDecision }> = ({ decision }) => {
  const safe = decision.safe
  const color = safe ? 'var(--green)' : 'var(--red)'
  return (
    <div className="card" style={{
      background: safe ? 'rgba(76,175,80,0.07)' : 'rgba(244,67,54,0.07)',
      border: `1px solid ${color}`,
      padding: 24,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 20 }}>
        <div style={{
          width: 72, height: 72, borderRadius: '50%',
          background: `radial-gradient(circle at 35% 35%, ${color}55, ${color}22)`,
          border: `3px solid ${color}`,
          boxShadow: `0 0 24px ${color}44`,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: '2rem', flexShrink: 0,
        }}>
          {safe ? '✓' : '✗'}
        </div>
        <div>
          <div style={{ fontSize: '1.8rem', fontWeight: 900, color, lineHeight: 1 }}>
            {safe ? 'SAFE TO ACCESS' : 'ACCESS DENIED'}
          </div>
          <div style={{ fontSize: '0.85rem', color: 'var(--text-secondary)', marginTop: 6 }}>
            {decision.reason || (safe ? 'All conditions met' : 'Hazardous conditions present')}
          </div>
        </div>
        <div style={{ marginLeft: 'auto', textAlign: 'right' }}>
          <div style={{ fontSize: '0.68rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>Last checked</div>
          <div style={{ fontSize: '0.82rem', color: 'var(--text-secondary)', marginTop: 2 }}>{fmtTs(decision.lastUpdated)}</div>
        </div>
      </div>
    </div>
  )
}

// ---- Status Row Cards ----
const StatusCards: React.FC<{ decision: SafeAccessDecision }> = ({ decision }) => {
  const gc = gatingColor(decision.gatingStatus)
  const cards = [
    {
      title: 'Gate Status',
      value: (decision.gatingStatus ?? 'unknown').toUpperCase(),
      color: gc,
      sub: decision.gatingStatus === 'open' ? 'Entry permitted' : decision.gatingStatus === 'conditional' ? 'Conditional entry' : 'Entry blocked',
    },
    {
      title: 'Ventilation',
      value: decision.ventilationOk ? 'OK' : 'FAULT',
      color: decision.ventilationOk ? 'var(--green)' : 'var(--red)',
      sub: decision.ventilationOk ? 'Air quality acceptable' : 'Check ventilation system',
    },
    {
      title: 'Active Gas Alerts',
      value: String(decision.gasStatus?.activeAlerts?.length ?? 0),
      color: (decision.gasStatus?.activeAlerts?.length ?? 0) > 0 ? 'var(--red)' : 'var(--green)',
      sub: `${decision.gasStatus?.activeSensors ?? 0} sensor(s) online`,
    },
    {
      title: 'Hazard Risk',
      value: (decision.hazardStatus?.overallRisk ?? 'unknown').toUpperCase(),
      color: decision.hazardStatus?.overallRisk === 'critical' || decision.hazardStatus?.overallRisk === 'high'
        ? 'var(--red)' : decision.hazardStatus?.overallRisk === 'medium' ? 'var(--amber)' : 'var(--green)',
      sub: `${decision.hazardStatus?.hazards?.filter(h => !h.cleared).length ?? 0} active hazards`,
    },
  ]
  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(160px,1fr))', gap: 12 }}>
      {cards.map(c => (
        <div key={c.title} className="card card-sm" style={{ borderLeft: `3px solid ${c.color}` }}>
          <div style={{ fontSize: '0.65rem', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', marginBottom: 6 }}>{c.title}</div>
          <div style={{ fontSize: '1.3rem', fontWeight: 800, color: c.color, lineHeight: 1 }}>{c.value}</div>
          <div style={{ fontSize: '0.72rem', color: 'var(--text-secondary)', marginTop: 4 }}>{c.sub}</div>
        </div>
      ))}
    </div>
  )
}

// ---- Gas Chart ----
const GasChart: React.FC<{ decision: SafeAccessDecision }> = ({ decision }) => {
  const levels = decision.gasStatus?.averageLevels
  if (!levels) return (
    <div className="card">
      <div className="card-header"><span className="card-title">Gas Levels</span></div>
      <div style={{ padding: 20, color: 'var(--text-muted)', textAlign: 'center', fontSize: '0.8rem' }}>No gas data available</div>
    </div>
  )

  const gasKeys = ['ch4', 'co', 'co2', 'o2', 'no2'] as const
  const data = gasKeys.map(k => ({
    name: GAS_LABELS[k],
    value: Number(((levels as Record<string, number>)[k] ?? 0).toFixed(2)),
    threshold: GAS_THRESHOLDS[k],
    unit: GAS_UNITS[k],
    over: k !== 'o2'
      ? (levels as Record<string, number>)[k] > GAS_THRESHOLDS[k]
      : (levels as Record<string, number>)[k] < GAS_THRESHOLDS[k],
  }))

  return (
    <div className="card">
      <div className="card-header">
        <span className="card-title">Gas Levels (Average)</span>
        <span className={`badge ${decision.gasStatus?.environmentSafe ? 'badge-online' : 'badge-offline'}`}>
          {decision.gasStatus?.environmentSafe ? 'Safe' : 'Hazardous'}
        </span>
      </div>
      <ResponsiveContainer width="100%" height={220}>
        <BarChart data={data} margin={{ top: 10, right: 10, bottom: 0, left: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
          <XAxis dataKey="name" tick={{ fill: 'var(--text-secondary)', fontSize: 12 }} axisLine={false} tickLine={false} />
          <YAxis tick={{ fill: 'var(--text-muted)', fontSize: 10 }} axisLine={false} tickLine={false} width={35} />
          <Tooltip
            contentStyle={{ background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: 6, fontSize: 12 }}
            formatter={(val: number, _name: string, props: { payload?: { unit?: string; threshold?: number } }) => [
              `${val} ${props?.payload?.unit ?? ''}`,
              `Threshold: ${props?.payload?.threshold ?? '—'} ${props?.payload?.unit ?? ''}`,
            ]}
          />
          <Bar dataKey="value" radius={[4, 4, 0, 0]}>
            {data.map((d, i) => (
              <Cell key={i} fill={d.over ? 'var(--red)' : 'var(--blue)'} />
            ))}
          </Bar>
          {data.map((d, i) => (
            <ReferenceLine key={i} y={d.threshold} strokeDasharray="4 2" stroke="var(--amber)" strokeWidth={1} />
          ))}
        </BarChart>
      </ResponsiveContainer>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 8 }}>
        {data.map(d => (
          <div key={d.name} style={{ fontSize: '0.72rem', padding: '3px 8px', borderRadius: 4,
            background: d.over ? 'rgba(244,67,54,0.1)' : 'rgba(33,150,243,0.1)',
            color: d.over ? 'var(--red)' : 'var(--blue-light)',
            border: `1px solid ${d.over ? 'var(--red)' : 'transparent'}`,
          }}>
            {d.name}: <strong>{d.value}</strong> {d.unit}
          </div>
        ))}
      </div>
    </div>
  )
}

// ---- Gas Alerts ----
const AlertList: React.FC<{ alerts: GasAlert[] }> = ({ alerts }) => {
  const active = alerts.filter(a => a.active)
  return (
    <div className="card">
      <div className="card-header">
        <span className="card-title">Gas Alerts</span>
        <span className={`badge ${active.length > 0 ? 'badge-offline' : 'badge-online'}`}>{active.length} active</span>
      </div>
      {alerts.length === 0
        ? <div style={{ padding: 16, color: 'var(--text-muted)', fontSize: '0.8rem' }}>No active alerts</div>
        : <div className="log-list" style={{ maxHeight: 200 }}>
            {alerts.map(a => (
              <div key={a.id} className={`log-entry ${a.active ? 'error' : 'info'}`} style={{ opacity: a.active ? 1 : 0.6 }}>
                <span className="log-ts">{fmtTs(a.timestamp)}</span>
                <span className="log-msg">
                  <strong>{a.gas.toUpperCase()}</strong>: {a.level.toFixed(2)} (threshold {a.threshold})
                </span>
                <span className={`badge ${a.active ? 'badge-offline' : 'badge-online'} badge-nodot`} style={{ flexShrink: 0, fontSize: '0.65rem' }}>
                  {a.active ? 'ACTIVE' : 'CLEARED'}
                </span>
              </div>
            ))}
          </div>
      }
    </div>
  )
}

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
            <div key={i} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', borderRadius: 6, borderLeft: '3px solid var(--amber)', fontSize: '0.82rem', color: 'var(--text-primary)' }}>
              {r}
            </div>
          ))}
        </div>
    }
  </div>
)

// ---- Gate Controls ----
const GateControls: React.FC<{ decision: SafeAccessDecision; onRefresh: () => void }> = ({ decision, onRefresh }) => {
  const [loading, setLoading] = useState<string | null>(null)
  const run = useCallback(async (label: string, fn: () => Promise<void>) => {
    setLoading(label)
    try { await fn() } catch (_e) { /* ignore */ } finally { setLoading(null); onRefresh() }
  }, [onRefresh])

  return (
    <div className="card">
      <div className="card-header">
        <span className="card-title">Gate Controls</span>
        <span style={{ fontSize: '0.75rem', padding: '4px 10px', borderRadius: 999,
          background: gatingColor(decision.gatingStatus) + '22',
          color: gatingColor(decision.gatingStatus),
          border: `1px solid ${gatingColor(decision.gatingStatus)}55`,
        }}>
          {(decision.gatingStatus ?? 'unknown').toUpperCase()}
        </span>
      </div>
      <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
        <button
          className="btn btn-success"
          disabled={!!loading || decision.gatingStatus === 'open'}
          onClick={() => run('open', openGate)}
        >
          {loading === 'open' ? <span className="loading-spinner" /> : null}
          Open Gate
        </button>
        <button
          className="btn btn-danger"
          disabled={!!loading || decision.gatingStatus === 'closed'}
          onClick={() => run('close', closeGate)}
        >
          {loading === 'close' ? <span className="loading-spinner" /> : null}
          Close Gate
        </button>
      </div>
      {!decision.safe && (
        <div style={{ marginTop: 10, padding: '8px 12px', background: 'rgba(244,67,54,0.08)', borderRadius: 6, fontSize: '0.78rem', color: 'var(--red-light)', border: '1px solid rgba(244,67,54,0.25)' }}>
          ⚠ Conditions unsafe — opening gate is not recommended
        </div>
      )}
    </div>
  )
}

// ---- Main ----
const CDTbView: React.FC = () => {
  const { data: decision, error, loading, refetch } = usePolling<SafeAccessDecision>(urls.safeAccess, 3000)

  if (loading && !decision) return (
    <div className="page-container">
      <div className="card" style={{ display: 'flex', alignItems: 'center', gap: 12, padding: 24, color: 'var(--text-muted)' }}>
        <span className="loading-spinner" /> Connecting to cDTb Safe-Access Controller…
      </div>
    </div>
  )

  return (
    <div className="page-container" style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
        <div>
          <h2 style={{ color: 'var(--text-primary)' }}>cDTb — Hazard Monitoring &amp; Gating</h2>
          <p style={{ fontSize: '0.78rem', marginTop: 2 }}>Safe-access decision support</p>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {error && <span style={{ fontSize: '0.72rem', color: 'var(--red)' }}>⚠ {error}</span>}
          <button className="btn btn-ghost btn-sm" onClick={refetch}>↻ Refresh</button>
          {decision && <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>Updated {fmtTs(decision.lastUpdated)}</span>}
        </div>
      </div>

      {!decision && error && (
        <div className="card" style={{ color: 'var(--red-light)', padding: 16, fontSize: '0.83rem' }}>
          Cannot connect to cDTb: {error}
        </div>
      )}

      {decision && (
        <>
          <SafeHero decision={decision} />

          <section>
            <div className="section-title">System Status</div>
            <StatusCards decision={decision} />
          </section>

          <section>
            <div className="section-title">Gas Monitoring</div>
            <GasChart decision={decision} />
          </section>

          <section>
            <div className="section-title">Alerts &amp; Recommendations</div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <AlertList alerts={decision.gasStatus?.activeAlerts ?? []} />
              <Recommendations recs={decision.recommendations ?? []} />
            </div>
          </section>

          <section>
            <div className="section-title">Gate Control</div>
            <GateControls decision={decision} onRefresh={refetch} />
          </section>
        </>
      )}
    </div>
  )
}

export default CDTbView
