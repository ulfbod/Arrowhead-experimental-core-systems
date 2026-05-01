// Analysis Summary — narrative text + key numbers derived from telemetry stats.

import type { TelemetryStatsResponse } from '../types'

interface Props {
  stats: TelemetryStatsResponse | null
}

export function AnalysisSummary({ stats }: Props) {
  return (
    <section style={s.section} data-testid="analysis-summary">
      <h2 style={s.heading}>Analysis Summary</h2>
      {!stats && <span style={s.muted}>Waiting for telemetry data…</span>}
      {stats && (
        <div style={s.grid}>
          <Stat label="Active robots"    value={String(stats.aggregate.robotCount)} />
          <Stat label="Total messages"   value={String(stats.aggregate.totalMsgCount)} />
          <Stat label="Mean latency"     value={`${stats.aggregate.latencyMs.mean.toFixed(1)} ms`} />
          <Stat label="p95 latency"      value={`${stats.aggregate.latencyMs.p95.toFixed(1)} ms`} />
          <Stat label="Msg rate (total)" value={`${totalRate(stats).toFixed(1)} Hz`} />
        </div>
      )}
    </section>
  )
}

function totalRate(s: TelemetryStatsResponse): number {
  return Object.values(s.robots).reduce((acc, r) => acc + r.rateHz, 0)
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div style={st.stat}>
      <div style={st.statLabel}>{label}</div>
      <div style={st.statValue}>{value}</div>
    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  section: { marginBottom: 20 },
  heading: { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  muted:   { color: '#888', fontSize: '0.8rem' },
  grid:    { display: 'flex', flexWrap: 'wrap', gap: 12 },
}

const st: Record<string, React.CSSProperties> = {
  stat:      { background: '#f8f8f8', border: '1px solid #e5e7eb', borderRadius: 4, padding: '8px 14px', minWidth: 120 },
  statLabel: { fontSize: '0.7rem', color: '#888', marginBottom: 2 },
  statValue: { fontSize: '1.1rem', fontWeight: 'bold', fontFamily: 'monospace', color: '#1a1a2e' },
}
