// SVG bar chart: per-robot latency (mean + p95).
// No external charting libraries — pure SVG.

import type { TelemetryStatsResponse } from '../types'

interface Props {
  stats: TelemetryStatsResponse | null
}

const W = 480
const H = 180
const PAD = { top: 16, right: 16, bottom: 40, left: 52 }
const CHART_W = W - PAD.left - PAD.right
const CHART_H = H - PAD.top - PAD.bottom

export function LatencyChart({ stats }: Props) {
  if (!stats || Object.keys(stats.robots).length === 0) {
    return (
      <section style={s.section} data-testid="latency-chart">
        <h2 style={s.heading}>Latency per Robot</h2>
        <span style={s.muted}>No data yet.</span>
      </section>
    )
  }

  const robots = Object.entries(stats.robots)
  const maxVal = Math.max(...robots.map(([, r]) => r.latencyMs.p95), 1)
  const barW = Math.floor(CHART_W / robots.length)
  const groupW = Math.max(4, barW - 8)
  const halfW = Math.floor(groupW / 2) - 1

  const yScale = (v: number) => CHART_H - (v / maxVal) * CHART_H

  // Y-axis ticks: 0, 25%, 50%, 75%, 100% of max
  const ticks = [0, 0.25, 0.5, 0.75, 1].map(f => ({
    y: yScale(f * maxVal),
    label: (f * maxVal).toFixed(0),
  }))

  return (
    <section style={s.section} data-testid="latency-chart">
      <h2 style={s.heading}>Latency per Robot (ms)</h2>
      <svg width={W} height={H} style={s.svg}>
        <g transform={`translate(${PAD.left},${PAD.top})`}>
          {/* Grid lines + y-axis labels */}
          {ticks.map(t => (
            <g key={t.label}>
              <line x1={0} y1={t.y} x2={CHART_W} y2={t.y} stroke="#eee" />
              <text x={-4} y={t.y + 4} textAnchor="end" fontSize={9} fill="#888">{t.label}</text>
            </g>
          ))}
          {/* Bars */}
          {robots.map(([id, r], i) => {
            const cx = i * barW + barW / 2
            const meanH = (r.latencyMs.mean / maxVal) * CHART_H
            const p95H  = (r.latencyMs.p95  / maxVal) * CHART_H
            return (
              <g key={id} data-testid={`bar-${id}`}>
                {/* p95 bar (lighter) */}
                <rect x={cx - halfW} y={CHART_H - p95H} width={halfW} height={p95H}
                  fill="#93c5fd" opacity={0.7} />
                {/* mean bar (darker) */}
                <rect x={cx} y={CHART_H - meanH} width={halfW} height={meanH}
                  fill="#2563eb" opacity={0.85} />
                {/* x-axis label */}
                <text x={cx} y={CHART_H + 14} textAnchor="middle" fontSize={9} fill="#555">
                  {id.replace('robot-', 'r')}
                </text>
              </g>
            )
          })}
          {/* Axes */}
          <line x1={0} y1={0} x2={0} y2={CHART_H} stroke="#ccc" />
          <line x1={0} y1={CHART_H} x2={CHART_W} y2={CHART_H} stroke="#ccc" />
        </g>
        {/* Legend */}
        <g transform={`translate(${PAD.left},${H - 6})`}>
          <rect x={0} y={0} width={8} height={8} fill="#93c5fd" />
          <text x={11} y={8} fontSize={9} fill="#555">p95</text>
          <rect x={40} y={0} width={8} height={8} fill="#2563eb" />
          <text x={51} y={8} fontSize={9} fill="#555">mean</text>
        </g>
      </svg>
    </section>
  )
}

const s: Record<string, React.CSSProperties> = {
  section: { marginBottom: 20 },
  heading: { fontSize: '0.9rem', marginBottom: 6, color: '#555' },
  muted:   { color: '#888', fontSize: '0.8rem' },
  svg:     { display: 'block', overflow: 'visible' },
}
