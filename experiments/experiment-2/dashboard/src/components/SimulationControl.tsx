// Fleet Simulation Control — lets the user reconfigure the robot-fleet service live.

import { useState, useEffect } from 'react'
import { fetchFleetConfig, postFleetConfig } from '../api'
import type { FleetConfig, RobotConfig } from '../types'

const PRESETS = ['5g_excellent', '5g_good', '5g_moderate', '5g_poor', '5g_edge']

const EMPTY_CONFIG: FleetConfig = { payloadType: 'imu', payloadHz: 10, robots: [] }

function buildUniformRobots(count: number, preset: string): RobotConfig[] {
  return Array.from({ length: count }, (_, i) => ({ id: `robot-${i + 1}`, networkPreset: preset }))
}

export function SimulationControl() {
  const [cfg, setCfg] = useState<FleetConfig>(EMPTY_CONFIG)
  const [robotCount, setRobotCount] = useState(3)
  const [uniformPreset, setUniformPreset] = useState('5g_good')
  const [status, setStatus] = useState<'idle' | 'saving' | 'ok' | 'err'>('idle')
  const [err, setErr] = useState('')

  useEffect(() => {
    fetchFleetConfig().then(c => {
      setCfg(c)
      setRobotCount(c.robots.length || 3)
      if (c.robots.length > 0) setUniformPreset(c.robots[0].networkPreset)
    }).catch(() => { /* service may not be up yet */ })
  }, [])

  async function handleApply() {
    setStatus('saving')
    setErr('')
    const next: FleetConfig = {
      ...cfg,
      robots: buildUniformRobots(robotCount, uniformPreset),
    }
    try {
      await postFleetConfig(next)
      setCfg(next)
      setStatus('ok')
    } catch (e) {
      setErr(String(e))
      setStatus('err')
    }
  }

  return (
    <section style={s.section}>
      <h2 style={s.heading}>Fleet Simulation Control</h2>
      <div style={s.form}>
        <label style={s.field}>
          <span style={s.label}>Robot count</span>
          <input
            data-testid="robot-count"
            type="number" min={1} max={20} style={s.input}
            value={robotCount}
            onChange={e => {
              const n = parseInt(e.target.value, 10)
              if (!isNaN(n) && n >= 1) setRobotCount(n)
            }}
          />
        </label>
        <label style={s.field}>
          <span style={s.label}>Payload type</span>
          <select
            data-testid="payload-type"
            style={s.input}
            value={cfg.payloadType}
            onChange={e => setCfg(c => ({ ...c, payloadType: e.target.value as 'basic' | 'imu' }))}
          >
            <option value="basic">basic</option>
            <option value="imu">imu</option>
          </select>
        </label>
        <label style={s.field}>
          <span style={s.label}>Publish rate (Hz)</span>
          <input
            data-testid="payload-hz"
            type="number" min={1} max={100} style={s.input}
            value={cfg.payloadHz}
            onChange={e => {
              const n = parseInt(e.target.value, 10)
              if (!isNaN(n) && n >= 1) setCfg(c => ({ ...c, payloadHz: n }))
            }}
          />
        </label>
        <label style={s.field}>
          <span style={s.label}>Network preset (all robots)</span>
          <select
            data-testid="network-preset"
            style={s.input}
            value={uniformPreset}
            onChange={e => setUniformPreset(e.target.value)}
          >
            {PRESETS.map(p => <option key={p} value={p}>{p}</option>)}
          </select>
        </label>
        <div style={s.actions}>
          <button style={s.btn} data-testid="apply-btn" onClick={handleApply} disabled={status === 'saving'}>
            {status === 'saving' ? 'Saving…' : 'Apply'}
          </button>
          {status === 'ok' && <span style={s.ok}>✓ applied</span>}
          {status === 'err' && <span style={s.errText}>{err}</span>}
        </div>
      </div>
    </section>
  )
}

const s: Record<string, React.CSSProperties> = {
  section: { marginBottom: 20 },
  heading: { fontSize: '0.9rem', marginBottom: 10, color: '#555' },
  form:    { display: 'flex', flexWrap: 'wrap', gap: 12, alignItems: 'flex-end' },
  field:   { display: 'flex', flexDirection: 'column', gap: 3 },
  label:   { fontSize: '0.75rem', fontWeight: 'bold', color: '#444' },
  input:   { fontFamily: 'monospace', fontSize: '0.8rem', padding: '4px 6px', border: '1px solid #ccc', borderRadius: 3, width: 160 },
  actions: { display: 'flex', alignItems: 'center', gap: 8 },
  btn:     { fontFamily: 'monospace', fontSize: '0.8rem', padding: '5px 14px', border: '1px solid #999', borderRadius: 3, cursor: 'pointer', background: '#1a1a2e', color: '#fff' },
  ok:      { color: '#16a34a', fontSize: '0.8rem' },
  errText: { color: '#dc2626', fontSize: '0.75rem' },
}
