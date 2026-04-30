// SVG bar chart: per-robot message rate (Hz).
// No external charting libraries — pure SVG.

import type { TelemetryStatsResponse } from '../types'

interface Props {
  stats: TelemetryStatsResponse | null
}

const W = 480
const H = 160
const PAD = { top: 16, right: 16, bottom: 40, left: 52 }
const CHART_W = W - PAD.left - PAD.right
const CHART_H = H - PAD.top - PAD.bottom

export function ThroughputChart({ stats }: Props) {
  if (!stats || Object.keys(stats.robots).length === 0) {
    return (
      <section style={s.section} data-testid="throughput-chart">
        <h2 style={s.heading}>Throughput per Robot (Hz)</h2>
        <span style={s.muted}>No data yet.</span>
      </section>
    )
  }

  const robots = Object.entries(stats.robots)
  const maxVal = Math.max(...robots.map(([, r]) => r.rateHz), 1)
  const barW = Math.floor(CHART_W / robots.length)
  const groupW = Math.max(4, barW - 12)

  const yScale = (v: number) => CHART_H - (v / maxVal) * CHART_H

  const ticks = [0, 0.5, 1].map(f => ({
    y: yScale(f * maxVal),
    label: (f * maxVal).toFixed(1),
  }))

  return (
    <section style={s.section} data-testid="throughput-chart">
      <h2 style={s.heading}>Throughput per Robot (Hz)</h2>
      <svg width={W} height={H} style={s.svg}>
        <g transform={`translate(${PAD.left},${PAD.top})`}>
          {ticks.map(t => (
            <g key={t.label}>
              <line x1={0} y1={t.y} x2={CHART_W} y2={t.y} stroke="#eee" />
              <text x={-4} y={t.y + 4} textAnchor="end" fontSize={9} fill="#888">{t.label}</text>
            </g>
          ))}
          {robots.map(([id, r], i) => {
            const cx = i * barW + barW / 2 - groupW / 2
            const barH = (r.rateHz / maxVal) * CHART_H
            return (
              <g key={id} data-testid={`throughput-bar-${id}`}>
                <rect x={cx} y={CHART_H - barH} width={groupW} height={barH}
                  fill="#16a34a" opacity={0.8} />
                <text x={cx + groupW / 2} y={CHART_H + 14} textAnchor="middle" fontSize={9} fill="#555">
                  {id.replace('robot-', 'r')}
                </text>
              </g>
            )
          })}
          <line x1={0} y1={0} x2={0} y2={CHART_H} stroke="#ccc" />
          <line x1={0} y1={CHART_H} x2={CHART_W} y2={CHART_H} stroke="#ccc" />
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
