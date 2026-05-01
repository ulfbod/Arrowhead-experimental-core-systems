// Per-robot telemetry statistics table, polled from /telemetry/stats.

import { useConfig } from '../config/context'
import { usePolling } from '../hooks/usePolling'
import { fetchTelemetryStats } from '../api'
import type { TelemetryStatsResponse } from '../types'

export function FleetMetrics() {
  const { config } = useConfig()
  const { data, loading } = usePolling<TelemetryStatsResponse>(
    (signal) => fetchTelemetryStats(signal),
    config.polling.allTelemetryIntervalMs,
  )

  const robots = data ? Object.entries(data.robots) : []

  return (
    <section style={s.section} data-testid="fleet-metrics">
      <h2 style={s.heading}>Fleet Metrics</h2>
      {loading && !data && <span style={s.muted}>Loading…</span>}
      {robots.length === 0 && !loading && <span style={s.muted}>No telemetry data yet.</span>}
      {robots.length > 0 && (
        <table style={s.table}>
          <thead>
            <tr>
              <th style={s.th}>Robot</th>
              <th style={s.th}>Rate (Hz)</th>
              <th style={s.th}>Latency mean</th>
              <th style={s.th}>Latency p95</th>
              <th style={s.th}>Messages</th>
            </tr>
          </thead>
          <tbody>
            {robots.map(([id, r]) => (
              <tr key={id} data-testid={`robot-row-${id}`}>
                <td style={s.td}>{id}</td>
                <td style={s.td}>{r.rateHz.toFixed(1)}</td>
                <td style={s.td}>{r.latencyMs.mean.toFixed(1)} ms</td>
                <td style={s.td}>{r.latencyMs.p95.toFixed(1)} ms</td>
                <td style={s.td}>{r.msgCount}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {data && (
        <div style={s.agg} data-testid="aggregate-stats">
          <span>Robots: <b>{data.aggregate.robotCount}</b></span>
          <span>Total msgs: <b>{data.aggregate.totalMsgCount}</b></span>
          <span>Mean latency: <b>{data.aggregate.latencyMs.mean.toFixed(1)} ms</b></span>
          <span>p95 latency: <b>{data.aggregate.latencyMs.p95.toFixed(1)} ms</b></span>
        </div>
      )}
    </section>
  )
}

const s: Record<string, React.CSSProperties> = {
  section: { marginBottom: 20 },
  heading: { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  muted:   { color: '#888', fontSize: '0.8rem' },
  table:   { borderCollapse: 'collapse', fontSize: '0.8rem', width: '100%' },
  th:      { textAlign: 'left', padding: '4px 10px', borderBottom: '1px solid #ddd', color: '#555', fontWeight: 'bold' },
  td:      { padding: '4px 10px', borderBottom: '1px solid #f0f0f0', fontFamily: 'monospace' },
  agg:     { display: 'flex', gap: 20, marginTop: 8, fontSize: '0.8rem', color: '#444' },
}
