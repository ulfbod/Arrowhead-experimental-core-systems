import React, { useState, useCallback } from 'react'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ReferenceLine,
  ResponsiveContainer,
  Cell,
} from 'recharts'
import usePolling from '../../hooks/usePolling'
import { urls, openGate, closeGate } from '../../api'
import type {
  SafeAccessDecision,
  GasAlert,
  Recommendation,
  GatingStatus,
} from '../../types'

// ============================================================
// Constants – Gas thresholds
// ============================================================

const GAS_THRESHOLDS = {
  ch4:  20,   // % LEL – action level
  co:   25,   // ppm TWA
  co2:  0.5,  // % vol – IDLH boundary
  o2:   19.5, // % vol – minimum
  no2:  3,    // ppm TWA
}

const GAS_UNITS: Record<string, string> = {
  ch4:  '% LEL',
  co:   'ppm',
  co2:  '% vol',
  o2:   '% vol',
  no2:  'ppm',
}

const GAS_LABELS: Record<string, string> = {
  ch4: 'CH₄',
  co:  'CO',
  co2: 'CO₂',
  o2:  'O₂',
  no2: 'NO₂',
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

function riskBadgeClass(risk: string): string {
  if (risk === 'critical' || risk === 'high') return 'badge-offline'
  if (risk === 'medium')                       return 'badge-warning'
  return 'badge-online'
}

function gatingColor(status: GatingStatus): string {
  if (status === 'open')        return 'var(--green)'
  if (status === 'closed')      return 'var(--red)'
  return 'var(--amber)'
}

function gatingLabel(status: GatingStatus): string {
  if (status === 'open')   return 'OPEN'
  if (status === 'closed') return 'CLOSED'
  return 'CONDITIONAL'
}

function ventilationColor(status: string): string {
  if (status === 'nominal')  return 'var(--green)'
  if (status === 'reduced')  return 'var(--amber)'
  if (status === 'critical') return 'var(--red)'
  return 'var(--text-muted)'
}

// ============================================================
// Custom Recharts Tooltip
// ============================================================

interface TooltipPayloadItem {
  name: string
  value: number
  payload?: { unit?: string; threshold?: number }
}

const CustomTooltip: React.FC<{
  active?: boolean
  payload?: TooltipPayloadItem[]
  label?: string
}> = ({ active, payload, label }) => {
  if (!active || !payload || !payload.length) return null
  const item = payload[0]
  return (
    <div style={{
      background: 'var(--bg-elevated)',
      border: '1px solid var(--border)',
      borderRadius: 6,
      padding: '8px 12px',
      fontSize: '0.78rem',
    }}>
      <div style={{ fontWeight: 700, color: 'var(--text-primary)', marginBottom: 4 }}>
        {GAS_LABELS[label ?? ''] ?? label}
      </div>
      <div style={{ color: 'var(--text-secondary)' }}>
        Level: <strong style={{ color: 'var(--text-primary)' }}>
          {item.value.toFixed(2)} {GAS_UNITS[label ?? ''] ?? ''}
        </strong>
      </div>
      {item.payload?.threshold != null && (
        <div style={{ color: 'var(--text-muted)', marginTop: 2 }}>
          Threshold: {item.payload.threshold} {GAS_UNITS[label ?? ''] ?? ''}
        </div>
      )}
    </div>
  )
}

// ============================================================
// Sub-components
// ============================================================

// Safe / Unsafe hero card
const SafeAccessHero: React.FC<{ decision: SafeAccessDecision }> = ({ decision }) => {
  const { safeToAccess, reason, confidence } = decision
  const heroClass = safeToAccess ? 'safe' : 'unsafe'
  const heroColor = safeToAccess ? 'var(--green)' : 'var(--red)'

  return (
    <div className={`status-hero ${heroClass}`} style={{ position: 'relative', overflow: 'hidden' }}>
      {/* Background glow */}
      <div style={{
        position: 'absolute', inset: 0,
        background: `radial-gradient(ellipse at center, ${heroColor}18 0%, transparent 70%)`,
        pointerEvents: 'none',
      }} />

      <div style={{ position: 'relative', zIndex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 12 }}>
        {/* Big indicator */}
        <div style={{ fontSize: '0.75rem', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.15em', color: heroColor, opacity: 0.8 }}>
          Safe Access Status
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
          <span style={{
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            width: 72, height: 72, borderRadius: '50%',
            background: `${heroColor}20`,
            border: `3px solid ${heroColor}`,
            fontSize: '2rem',
            boxShadow: `0 0 32px ${heroColor}40`,
          }}>
            {safeToAccess ? '✓' : '✕'}
          </span>

          <div>
            <div className="status-hero-label" style={{ color: heroColor }}>
              {safeToAccess ? 'SAFE' : 'UNSAFE'}
            </div>
            <div style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', marginTop: 4, maxWidth: 280 }}>
              {reason}
            </div>
          </div>
        </div>

        {/* Confidence */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: '0.75rem' }}>
          <span style={{ color: 'var(--text-muted)' }}>Confidence:</span>
          <span style={{ fontWeight: 700, color: heroColor }}>{confidence}%</span>
          <div style={{
            width: 100, height: 4,
            background: 'rgba(255,255,255,0.1)',
            borderRadius: 999, overflow: 'hidden',
          }}>
            <div style={{
              height: '100%',
              width: `${confidence}%`,
              background: heroColor,
              borderRadius: 999,
            }} />
          </div>
        </div>

        <div style={{ fontSize: '0.68rem', color: 'var(--text-muted)' }}>
          Updated: {fmtTs(decision.lastUpdate)}
        </div>
      </div>
    </div>
  )
}

// Gating Status card
const GatingCard: React.FC<{ status: GatingStatus }> = ({ status }) => {
  const color = gatingColor(status)
  const label = gatingLabel(status)

  return (
    <div className="card" style={{ textAlign: 'center', borderTop: `4px solid ${color}` }}>
      <div style={{ fontSize: '0.68rem', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.1em', color: 'var(--text-muted)', marginBottom: 10 }}>
        Gate Status
      </div>
      <div style={{
        width: 64, height: 64, borderRadius: '50%',
        border: `4px solid ${color}`,
        background: `${color}15`,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        margin: '0 auto 10px',
        fontSize: '1.5rem',
        boxShadow: `0 0 20px ${color}30`,
      }}>
        {status === 'open' ? '🟢' : status === 'closed' ? '🔴' : '🟡'}
      </div>
      <div style={{ fontSize: '1.5rem', fontWeight: 800, color, lineHeight: 1 }}>{label}</div>
      {status === 'conditional' && (
        <div style={{ fontSize: '0.72rem', color: 'var(--amber)', marginTop: 4 }}>
          Access with escort only
        </div>
      )}
    </div>
  )
}

// Ventilation Status card
const VentilationCard: React.FC<{ status: string; ok: boolean }> = ({ status, ok }) => {
  const color = ventilationColor(status)
  return (
    <div className="card" style={{ textAlign: 'center', borderTop: `4px solid ${color}` }}>
      <div style={{ fontSize: '0.68rem', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.1em', color: 'var(--text-muted)', marginBottom: 10 }}>
        Ventilation
      </div>
      <div style={{
        fontSize: '2rem', marginBottom: 8,
        filter: `drop-shadow(0 0 8px ${color})`,
      }}>
        {status === 'nominal' ? '💨' : status === 'reduced' ? '⚠️' : '🛑'}
      </div>
      <div style={{ fontSize: '1.1rem', fontWeight: 700, color, lineHeight: 1 }}>
        {status.toUpperCase()}
      </div>
      <div style={{ marginTop: 6 }}>
        <span className={`badge ${ok ? 'badge-online' : 'badge-warning'} badge-nodot`}>
          {ok ? 'OK' : 'Check Required'}
        </span>
      </div>
    </div>
  )
}

// Gas Levels BarChart
interface GasChartData {
  gas: string
  value: number
  threshold: number
  unit: string
  isAlert: boolean
}

const GasLevelChart: React.FC<{ decision: SafeAccessDecision }> = ({ decision }) => {
  const levels = decision.gasMonitor.aggregatedLevels

  const chartData: GasChartData[] = [
    { gas: 'ch4',  value: levels.ch4,          threshold: GAS_THRESHOLDS.ch4,  unit: GAS_UNITS.ch4,  isAlert: levels.ch4 > GAS_THRESHOLDS.ch4 },
    { gas: 'co',   value: levels.co,            threshold: GAS_THRESHOLDS.co,   unit: GAS_UNITS.co,   isAlert: levels.co > GAS_THRESHOLDS.co },
    { gas: 'co2',  value: levels.co2,           threshold: GAS_THRESHOLDS.co2,  unit: GAS_UNITS.co2,  isAlert: levels.co2 > GAS_THRESHOLDS.co2 },
    { gas: 'o2',   value: levels.o2,            threshold: GAS_THRESHOLDS.o2,   unit: GAS_UNITS.o2,   isAlert: levels.o2 < GAS_THRESHOLDS.o2 },
    { gas: 'no2',  value: levels.no2 ?? 0,      threshold: GAS_THRESHOLDS.no2,  unit: GAS_UNITS.no2,  isAlert: (levels.no2 ?? 0) > GAS_THRESHOLDS.no2 },
  ]

  return (
    <div className="card">
      <div className="card-header">
        <span className="card-title">Gas Levels Dashboard</span>
        <span className={`badge ${decision.gasMonitor.overallStatus === 'safe' ? 'badge-online' : decision.gasMonitor.overallStatus === 'caution' ? 'badge-warning' : 'badge-offline'}`}>
          {decision.gasMonitor.overallStatus}
        </span>
      </div>

      {/* Bar chart */}
      <div style={{ height: 240, marginBottom: 16 }}>
        <ResponsiveContainer width="100%" height="100%">
          <BarChart
            data={chartData}
            margin={{ top: 8, right: 16, left: 0, bottom: 4 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.05)" />
            <XAxis
              dataKey="gas"
              tickFormatter={k => GAS_LABELS[k] ?? k}
              tick={{ fill: 'var(--text-secondary)', fontSize: 11 }}
              axisLine={{ stroke: 'var(--border)' }}
              tickLine={false}
            />
            <YAxis
              tick={{ fill: 'var(--text-secondary)', fontSize: 10 }}
              axisLine={{ stroke: 'var(--border)' }}
              tickLine={false}
              width={40}
            />
            <Tooltip content={<CustomTooltip />} cursor={{ fill: 'rgba(255,255,255,0.04)' }} />
            <Bar dataKey="value" radius={[4, 4, 0, 0]} maxBarSize={60}>
              {chartData.map((entry, index) => (
                <Cell
                  key={`cell-${index}`}
                  fill={entry.isAlert ? 'var(--red)' : 'var(--blue)'}
                  opacity={0.85}
                />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>

      {/* Gas detail rows */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {chartData.map(g => {
          const pct = g.gas === 'o2'
            ? Math.min(100, (g.value / 25) * 100)
            : Math.min(100, (g.value / (g.threshold * 2 || 1)) * 100)
          return (
            <div key={g.gas} style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <span style={{ width: 36, fontSize: '0.78rem', fontWeight: 700, color: g.isAlert ? 'var(--red)' : 'var(--text-secondary)' }}>
                {GAS_LABELS[g.gas]}
              </span>
              <div style={{ flex: 1, height: 6, background: 'var(--bg-elevated)', borderRadius: 999, overflow: 'hidden', position: 'relative' }}>
                <div style={{
                  height: '100%',
                  width: `${Math.min(100, pct)}%`,
                  background: g.isAlert ? 'var(--red)' : 'var(--blue)',
                  borderRadius: 999,
                  transition: 'width 0.4s ease',
                }} />
                {/* Threshold line */}
                <div style={{
                  position: 'absolute',
                  top: 0, bottom: 0,
                  left: g.gas === 'o2' ? `${(g.threshold / 25) * 100}%` : `${Math.min(100, (g.threshold / (g.threshold * 2)) * 100)}%`,
                  width: 2,
                  background: 'var(--amber)',
                  opacity: 0.8,
                }} />
              </div>
              <div style={{ width: 90, textAlign: 'right', fontSize: '0.75rem' }}>
                <span style={{ color: g.isAlert ? 'var(--red)' : 'var(--text-primary)', fontWeight: 600 }}>
                  {g.value.toFixed(2)}
                </span>
                <span style={{ color: 'var(--text-muted)', marginLeft: 3 }}>{g.unit}</span>
              </div>
              {g.isAlert && (
                <span className="badge badge-offline" style={{ fontSize: '0.65rem', flexShrink: 0 }}>ALERT</span>
              )}
            </div>
          )
        })}
      </div>

      <div style={{ marginTop: 10, display: 'flex', gap: 16, fontSize: '0.68rem', color: 'var(--text-muted)' }}>
        <span>
          <span style={{ display: 'inline-block', width: 10, height: 4, background: 'var(--amber)', borderRadius: 2, marginRight: 4 }} />
          Threshold line
        </span>
        <span>Active sensors: {decision.gasMonitor.activeSensors.join(', ') || '—'}</span>
      </div>
    </div>
  )
}

// Active Gas Alerts
const GasAlertList: React.FC<{ alerts: GasAlert[] }> = ({ alerts }) => (
  <div className="card">
    <div className="card-header">
      <span className="card-title">Active Gas Alerts</span>
      <span className={`badge ${alerts.length > 0 ? 'badge-offline' : 'badge-online'}`}>
        {alerts.length} alert{alerts.length !== 1 ? 's' : ''}
      </span>
    </div>
    {alerts.length === 0 ? (
      <div className="empty-state" style={{ padding: 20, color: 'var(--green)' }}>
        <span style={{ fontSize: '1.5rem' }}>✓</span>
        <span>All gas levels nominal</span>
      </div>
    ) : (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {alerts.map(alert => (
          <div key={alert.id} style={{
            padding: '10px 12px',
            background: 'var(--bg-elevated)',
            borderRadius: 'var(--radius)',
            borderLeft: `3px solid ${severityColor(alert.severity)}`,
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'flex-start',
            gap: 12,
          }}>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 3 }}>
                <span style={{ fontWeight: 700, fontSize: '0.85rem', color: 'var(--text-primary)' }}>
                  {GAS_LABELS[alert.gas] ?? alert.gas}
                </span>
                <span className={`badge ${riskBadgeClass(alert.severity)}`}>{alert.severity}</span>
              </div>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                {alert.level.toFixed(2)} / {alert.threshold} {GAS_UNITS[alert.gas] ?? ''}
                &nbsp;·&nbsp; {alert.location}
              </div>
              <div style={{ fontSize: '0.68rem', color: 'var(--text-muted)', marginTop: 2 }}>
                {fmtTs(alert.timestamp)}
                {alert.acknowledged && <span style={{ color: 'var(--green)', marginLeft: 8 }}>Acknowledged</span>}
              </div>
            </div>
          </div>
        ))}
      </div>
    )}
  </div>
)

// Hazard Classification Summary
const HazardSummary: React.FC<{ decision: SafeAccessDecision }> = ({ decision }) => {
  const { hazardReport } = decision
  const bySeverity = hazardReport.hazards.reduce((acc, h) => {
    if (!h.cleared) {
      acc[h.severity] = (acc[h.severity] ?? 0) + 1
    }
    return acc
  }, {} as Record<string, number>)

  return (
    <div className="card">
      <div className="card-header">
        <span className="card-title">Hazard Classification</span>
        <span className={`badge ${riskBadgeClass(hazardReport.riskLevel)}`}>
          {hazardReport.riskLevel} risk
        </span>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 10, marginBottom: 14 }}>
        {[
          { label: 'Total Hazards',   value: hazardReport.totalHazards,   color: 'var(--text-primary)' },
          { label: 'Active Hazards',  value: hazardReport.activeHazards,  color: 'var(--amber)' },
          { label: 'Cleared Hazards', value: hazardReport.clearedHazards, color: 'var(--green)' },
          { label: 'Risk Level',      value: hazardReport.riskLevel.toUpperCase(), color: severityColor(hazardReport.riskLevel) },
        ].map(s => (
          <div key={s.label} style={{
            background: 'var(--bg-elevated)',
            borderRadius: 'var(--radius)',
            padding: '10px 12px',
            textAlign: 'center',
          }}>
            <div style={{ fontSize: '1.4rem', fontWeight: 800, color: s.color, lineHeight: 1 }}>
              {s.value}
            </div>
            <div style={{ fontSize: '0.68rem', color: 'var(--text-muted)', marginTop: 3, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              {s.label}
            </div>
          </div>
        ))}
      </div>

      {/* Severity breakdown */}
      {Object.keys(bySeverity).length > 0 && (
        <div>
          <div style={{ fontSize: '0.68rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 6 }}>
            By Severity
          </div>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            {(['critical', 'high', 'medium', 'low'] as const).map(sev => {
              const count = bySeverity[sev] ?? 0
              if (count === 0) return null
              return (
                <div key={sev} style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 5,
                  padding: '4px 10px',
                  background: 'var(--bg-elevated)',
                  borderRadius: 999,
                  border: `1px solid ${severityColor(sev)}40`,
                }}>
                  <span style={{ color: severityColor(sev), fontWeight: 700, fontSize: '0.88rem' }}>{count}</span>
                  <span style={{ fontSize: '0.68rem', color: 'var(--text-secondary)', textTransform: 'capitalize' }}>{sev}</span>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

// Recommendations
const RecommendationsList: React.FC<{ recs: Recommendation[] }> = ({ recs }) => (
  <div className="card">
    <div className="card-header">
      <span className="card-title">Safety Recommendations</span>
      <span className="badge badge-muted badge-nodot">{recs.length}</span>
    </div>
    {recs.length === 0 ? (
      <div className="empty-state" style={{ padding: 20 }}>No active recommendations</div>
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
            <span style={{ fontSize: '1rem', flexShrink: 0 }}>
              {r.priority === 'critical' ? '🔴' : r.priority === 'high' ? '🟠' : r.priority === 'medium' ? '🟡' : '🟢'}
            </span>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: '0.82rem', color: 'var(--text-primary)', lineHeight: 1.4 }}>{r.text}</div>
              {r.action && (
                <div style={{ fontSize: '0.72rem', color: 'var(--blue-light)', marginTop: 3 }}>→ {r.action}</div>
              )}
            </div>
          </div>
        ))}
      </div>
    )}
  </div>
)

// Gate Controls
const GateControls: React.FC<{ gatingStatus: GatingStatus; onRefresh: () => void }> = ({ gatingStatus, onRefresh }) => {
  const [loading, setLoading] = useState<string | null>(null)

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
        <span className="card-title">Gate Controls</span>
        <span style={{
          fontSize: '0.82rem',
          fontWeight: 700,
          color: gatingColor(gatingStatus),
        }}>
          Currently: {gatingLabel(gatingStatus)}
        </span>
      </div>

      <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center' }}>
        <button
          className="btn btn-success"
          disabled={!!loading || gatingStatus === 'open'}
          onClick={() => run('open', openGate)}
          style={{ minWidth: 130 }}
        >
          {loading === 'open' ? <span className="loading-spinner" /> : (
            <span style={{ marginRight: 4 }}>🟢</span>
          )}
          Open Gate
        </button>

        <button
          className="btn btn-danger"
          disabled={!!loading || gatingStatus === 'closed'}
          onClick={() => run('close', closeGate)}
          style={{ minWidth: 130 }}
        >
          {loading === 'close' ? <span className="loading-spinner" /> : (
            <span style={{ marginRight: 4 }}>🔴</span>
          )}
          Close Gate
        </button>

        <div style={{ fontSize: '0.72rem', color: 'var(--text-muted)', marginLeft: 'auto' }}>
          Manual override — use with caution
        </div>
      </div>

      {gatingStatus === 'open' && (
        <div className="alert alert-warning" style={{ marginTop: 12, marginBottom: 0 }}>
          Gate is OPEN — ensure all personnel are equipped with personal gas monitors
        </div>
      )}
      {gatingStatus === 'closed' && (
        <div className="alert alert-danger" style={{ marginTop: 12, marginBottom: 0 }}>
          Gate is CLOSED — access prohibited until conditions are safe
        </div>
      )}
      {gatingStatus === 'conditional' && (
        <div className="alert alert-warning" style={{ marginTop: 12, marginBottom: 0 }}>
          Conditional access — authorised personnel with escort only
        </div>
      )}
    </div>
  )
}

// ============================================================
// Loading / Error
// ============================================================

const LoadingCard: React.FC = () => (
  <div className="card" style={{ display: 'flex', alignItems: 'center', gap: 12, padding: 24, color: 'var(--text-muted)' }}>
    <span className="loading-spinner" />
    <span>Connecting to cDTb Safe Access Controller…</span>
  </div>
)

// ============================================================
// Main CDTbView
// ============================================================

const CDTbView: React.FC = () => {
  const { data: decision, error, loading, refetch } = usePolling<SafeAccessDecision>(urls.safeAccess, 3000)

  if (loading && !decision) return (
    <div className="page-container"><LoadingCard /></div>
  )

  return (
    <div className="page-container" style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>

      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
        <div>
          <h2 style={{ color: 'var(--text-primary)' }}>cDTb — Hazard Monitoring &amp; Safe Access</h2>
          <p style={{ fontSize: '0.78rem', marginTop: 2 }}>Ventilation · Gas · Gating decision support</p>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {error && <span style={{ fontSize: '0.72rem', color: 'var(--red)' }}>⚠ {error}</span>}
          <button className="btn btn-ghost btn-sm" onClick={refetch}>↻ Refresh</button>
          {decision && (
            <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
              Updated {fmtTs(decision.lastUpdate)}
            </span>
          )}
        </div>
      </div>

      {!decision && error && (
        <div className="card card-highlight-red" style={{ padding: 16, color: 'var(--red-light)', fontSize: '0.83rem' }}>
          Connection error: {error}
        </div>
      )}

      {decision && (
        <>
          {/* Row 1: Safe Access Hero + Gating + Ventilation */}
          <section>
            <div className="section-title">Access Decision</div>
            <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 1fr', gap: 16 }}>
              <SafeAccessHero decision={decision} />
              <GatingCard status={decision.gatingStatus} />
              <VentilationCard
                status={decision.gasMonitor.ventilationStatus}
                ok={decision.ventilationOk}
              />
            </div>
          </section>

          {/* Row 2: Gas Chart + Alert List */}
          <section>
            <div className="section-title">Gas Monitoring</div>
            <div style={{ display: 'grid', gridTemplateColumns: '3fr 2fr', gap: 16 }}>
              <GasLevelChart decision={decision} />
              <GasAlertList alerts={decision.gasMonitor.activeAlerts} />
            </div>
          </section>

          {/* Row 3: Hazard Summary + Recommendations + Gate Controls */}
          <section>
            <div className="section-title">Hazard &amp; Safety Intelligence</div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <HazardSummary decision={decision} />
              <RecommendationsList recs={decision.recommendations} />
            </div>
          </section>

          {/* Row 4: Gate Controls */}
          <section>
            <div className="section-title">Gate Management</div>
            <GateControls gatingStatus={decision.gatingStatus} onRefresh={refetch} />
          </section>
        </>
      )}
    </div>
  )
}

export default CDTbView
