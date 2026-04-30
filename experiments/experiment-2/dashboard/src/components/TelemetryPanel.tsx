// Shows the latest telemetry reading from the edge-adapter.
// Interval and history size come from dashboard config.

import { useState, useEffect, useRef, useCallback } from 'react'
import { fetchTelemetry } from '../api'
import { useConfig } from '../config/context'
import type { TelemetryResponse } from '../types'

export function TelemetryPanel() {
  const { config } = useConfig()
  const { telemetryIntervalMs, } = config.polling
  const maxHistory = config.display.maxTelemetryHistory

  const [history, setHistory] = useState<TelemetryResponse[]>([])
  const [error, setError]     = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const abortRef              = useRef<AbortController | null>(null)

  const poll = useCallback(() => {
    abortRef.current?.abort()
    const ctrl = new AbortController()
    abortRef.current = ctrl

    fetchTelemetry(ctrl.signal)
      .then(data => {
        if (ctrl.signal.aborted) return
        setError(null)
        setLoading(false)
        if (data === null) return // 204 — no data yet, keep existing history
        setHistory(prev => {
          // Skip duplicate readings (same seq number).
          if (prev.length > 0 && prev[0].payload.seq === data.payload.seq) return prev
          return [data, ...prev].slice(0, maxHistory)
        })
      })
      .catch(err => {
        if (ctrl.signal.aborted) return
        setLoading(false)
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [maxHistory])

  useEffect(() => {
    poll()
    const id = setInterval(poll, telemetryIntervalMs)
    return () => {
      clearInterval(id)
      abortRef.current?.abort()
    }
  }, [poll, telemetryIntervalMs])

  const latest = history[0] ?? null

  return (
    <section style={s.section}>
      <h2 style={s.heading}>Live Telemetry</h2>

      {loading && history.length === 0 && <p style={s.dim}>loading…</p>}
      {error && <p style={s.err}>{error}</p>}
      {!loading && history.length === 0 && !error && (
        <p style={s.dim}>No data yet — waiting for robot-simulator to publish.</p>
      )}

      {latest && (
        <div style={s.latest}>
          <span style={s.badge}>latest</span>
          <span>seq <b>{latest.payload.seq}</b></span>
          <span style={s.sep}>·</span>
          <span>🌡 <b>{latest.payload.temperature}°C</b></span>
          <span style={s.sep}>·</span>
          <span>💧 <b>{latest.payload.humidity}%</b></span>
          <span style={s.sep}>·</span>
          <span style={s.ts}>{latest.receivedAt}</span>
        </div>
      )}

      {history.length > 1 && (
        <div style={s.histWrap}>
          <table style={s.table}>
            <thead>
              <tr>
                {['seq', 'temp °C', 'humidity %', 'received at'].map(h => (
                  <th key={h} style={s.th}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {history.slice(1).map(r => (
                <tr key={r.payload.seq}>
                  <td style={s.td}>{r.payload.seq}</td>
                  <td style={s.tdNum}>{r.payload.temperature}</td>
                  <td style={s.tdNum}>{r.payload.humidity}</td>
                  <td style={s.td}>{r.receivedAt}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}

const s = {
  section:  { marginTop: 24 },
  heading:  { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  latest:   {
    display: 'flex', alignItems: 'center', flexWrap: 'wrap' as const,
    gap: 8, padding: '8px 12px', background: '#e8f5e9',
    border: '1px solid #a5d6a7', borderRadius: 4, marginBottom: 12,
    fontSize: '0.85rem',
  },
  badge:    {
    background: '#4caf50', color: '#fff', borderRadius: 3,
    padding: '1px 6px', fontSize: '0.7rem', fontWeight: 'bold' as const,
  },
  sep:      { color: '#bbb' },
  ts:       { color: '#888', fontSize: '0.75rem' },
  histWrap: { overflowX: 'auto' as const, maxHeight: 240, overflowY: 'auto' as const },
  table:    { width: '100%', borderCollapse: 'collapse' as const, fontSize: '0.78rem' },
  th:       { textAlign: 'left' as const, padding: '3px 8px', borderBottom: '2px solid #ddd', whiteSpace: 'nowrap' as const },
  td:       { padding: '3px 8px', borderBottom: '1px solid #eee' },
  tdNum:    { padding: '3px 8px', borderBottom: '1px solid #eee', textAlign: 'right' as const },
  err:      { color: '#f44336', fontSize: '0.8rem' },
  dim:      { color: '#999', fontSize: '0.8rem' },
}
