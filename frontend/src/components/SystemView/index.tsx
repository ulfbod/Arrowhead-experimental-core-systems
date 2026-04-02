import React, { useState, useCallback } from 'react'
import usePolling from '../../hooks/usePolling'
import { urls, addPolicy, deletePolicy } from '../../api'
import type {
  ServiceRecord,
  AuthPolicy,
  OrchestrationLog,
  RobotState,
  GasSensorState,
  LHDState,
  AddPolicyPayload,
  ServicesResponse,
  PoliciesResponse,
  OrchLogsResponse,
} from '../../types'

// ============================================================
// Constants
// ============================================================

interface ServiceDef {
  id: string
  label: string
  type: 'iDT' | 'cDT' | 'core'
  port: number
  stateUrl?: string
  category: string
}

const SERVICE_DEFS: ServiceDef[] = [
  { id: 'arrowhead', label: 'Arrowhead Core', type: 'core', port: 8000, category: 'Orchestration' },
  { id: 'idt1a', label: 'iDT1a Robot A',  type: 'iDT', port: 8101, stateUrl: 'http://localhost:8101/state', category: 'Robots' },
  { id: 'idt1b', label: 'iDT1b Robot B',  type: 'iDT', port: 8102, stateUrl: 'http://localhost:8102/state', category: 'Robots' },
  { id: 'idt2a', label: 'iDT2a Gas A',    type: 'iDT', port: 8201, stateUrl: 'http://localhost:8201/state', category: 'Gas Sensors' },
  { id: 'idt2b', label: 'iDT2b Gas B',    type: 'iDT', port: 8202, stateUrl: 'http://localhost:8202/state', category: 'Gas Sensors' },
  { id: 'idt3a', label: 'iDT3a LHD A',   type: 'iDT', port: 8301, stateUrl: 'http://localhost:8301/state', category: 'LHDs' },
  { id: 'idt3b', label: 'iDT3b LHD B',   type: 'iDT', port: 8302, stateUrl: 'http://localhost:8302/state', category: 'LHDs' },
  { id: 'idt4',  label: 'iDT4 Tele-Remote', type: 'iDT', port: 8401, stateUrl: 'http://localhost:8401/state', category: 'Tele-Remote' },
  { id: 'cdt1',  label: 'cDT1 Mapping',  type: 'cDT', port: 8501, stateUrl: 'http://localhost:8501/state', category: 'Composite DTs' },
  { id: 'cdt2',  label: 'cDT2 Gas Monitor', type: 'cDT', port: 8502, stateUrl: 'http://localhost:8502/state', category: 'Composite DTs' },
  { id: 'cdt3',  label: 'cDT3 Hazard Detect', type: 'cDT', port: 8503, stateUrl: 'http://localhost:8503/state', category: 'Composite DTs' },
  { id: 'cdt4',  label: 'cDT4 Clearance', type: 'cDT', port: 8504, stateUrl: 'http://localhost:8504/state', category: 'Composite DTs' },
  { id: 'cdt5',  label: 'cDT5 Intervention', type: 'cDT', port: 8505, stateUrl: 'http://localhost:8505/state', category: 'Composite DTs' },
  { id: 'cdta',  label: 'cDTa Mission',  type: 'cDT', port: 8601, stateUrl: 'http://localhost:8601/state', category: 'Mission DTs' },
  { id: 'cdtb',  label: 'cDTb Safe Access', type: 'cDT', port: 8602, stateUrl: 'http://localhost:8602/state', category: 'Mission DTs' },
  { id: 'scenario', label: 'Scenario Runner', type: 'core', port: 8700, stateUrl: 'http://localhost:8700/state', category: 'Orchestration' },
]

// Service composition edges: consumer → provider
const GRAPH_EDGES: [string, string, string][] = [
  ['cdt1', 'idt1a', 'mapping'],
  ['cdt1', 'idt1b', 'mapping'],
  ['cdt2', 'idt2a', 'gas'],
  ['cdt2', 'idt2b', 'gas'],
  ['cdt3', 'idt2a', 'gas'],
  ['cdt3', 'idt1a', 'scan'],
  ['cdt4', 'idt3a', 'clear'],
  ['cdt4', 'idt3b', 'clear'],
  ['cdt5', 'idt4',  'intervene'],
  ['cdta', 'cdt1',  'map'],
  ['cdta', 'cdt3',  'hazard'],
  ['cdta', 'cdt4',  'clear'],
  ['cdta', 'cdt5',  'intervene'],
  ['cdtb', 'cdt2',  'gas'],
  ['cdtb', 'cdt3',  'hazard'],
]

// ============================================================
// Utility helpers
// ============================================================


function badgeClass(status: string): string {
  if (status === 'online' || status === 'nominal') return 'badge-online'
  if (status === 'offline') return 'badge-offline'
  return 'badge-warning'
}

function typeColor(type: string): string {
  if (type === 'iDT') return 'var(--blue)'
  if (type === 'cDT') return 'var(--purple)'
  return 'var(--cyan)'
}

function fmtTs(ts: string): string {
  try {
    return new Date(ts).toLocaleTimeString()
  } catch {
    return ts
  }
}

function fmtPct(n: number): string {
  return `${Math.round(n)}%`
}

function clamp(val: number, min = 0, max = 100): number {
  return Math.max(min, Math.min(max, val))
}

// ============================================================
// Sub-components
// ============================================================

interface ProgressBarProps {
  value: number   // 0–100
  color?: string
  height?: number
}

const ProgressBar: React.FC<ProgressBarProps> = ({ value, color = 'var(--blue)', height = 5 }) => (
  <div style={{
    width: '100%', height, background: 'var(--bg-elevated)',
    borderRadius: 999, overflow: 'hidden', marginTop: 4,
  }}>
    <div style={{
      height: '100%', width: `${clamp(value)}%`,
      background: color, borderRadius: 999,
      transition: 'width 0.4s ease',
    }} />
  </div>
)

// ============================================================
// Service Card
// ============================================================

interface ServiceCardProps {
  def: ServiceDef
  record?: ServiceRecord
}

// Per-service telemetry cards need their own poll
const RobotCard: React.FC<{ url: string; label: string }> = ({ url }) => {
  const { data } = usePolling<RobotState>(url, 3000)
  if (!data) return null
  return (
    <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 4 }}>
      <div className="metric-bar-container" style={{ marginBottom: 0 }}>
        <div className="metric-bar-label">
          <span className="metric-bar-name">Battery</span>
          <span className="metric-bar-value">{fmtPct(data.batteryPct)}</span>
        </div>
        <ProgressBar
          value={data.batteryPct}
          color={data.batteryPct < 20 ? 'var(--red)' : data.batteryPct < 50 ? 'var(--amber)' : 'var(--green)'}
        />
      </div>
      <div className="metric-bar-container" style={{ marginBottom: 0 }}>
        <div className="metric-bar-label">
          <span className="metric-bar-name">Mapping</span>
          <span className="metric-bar-value">{fmtPct(data.mappingProgress)}</span>
        </div>
        <ProgressBar value={data.mappingProgress} color="var(--blue)" />
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.72rem', color: 'var(--text-secondary)', marginTop: 2 }}>
        <span>Hazards: <strong style={{ color: (data.hazardsDetected?.length ?? 0) > 0 ? 'var(--amber)' : 'var(--green)' }}>{data.hazardsDetected?.length ?? 0}</strong></span>
        <span style={{ color: data.online ? 'var(--green)' : 'var(--red)' }}>{data.online ? 'online' : 'offline'}</span>
      </div>
    </div>
  )
}

const GasCard: React.FC<{ url: string }> = ({ url }) => {
  const { data } = usePolling<GasSensorState>(url, 3000)
  if (!data) return null
  const levels = data.gasLevels
  if (!levels) return null
  const gasItems = [
    { label: 'CH4', value: levels.ch4, max: 5,   unit: '%',   thresh: 1.0 },
    { label: 'CO',  value: levels.co,  max: 100, unit: 'ppm', thresh: 25 },
    { label: 'O2',  value: levels.o2,  max: 25,  unit: '%',   thresh: null },
  ]
  return (
    <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 4 }}>
      {gasItems.map(g => {
        const pct = Math.min(100, (g.value / g.max) * 100)
        const warn = g.thresh ? g.value > g.thresh : g.value < 19.5
        return (
          <div key={g.label} className="metric-bar-container" style={{ marginBottom: 0 }}>
            <div className="metric-bar-label">
              <span className="metric-bar-name">{g.label}</span>
              <span className="metric-bar-value" style={{ color: warn ? 'var(--amber)' : 'var(--text-primary)' }}>
                {g.value.toFixed(1)} {g.unit}
              </span>
            </div>
            <ProgressBar value={pct} color={warn ? 'var(--amber)' : 'var(--green)'} height={4} />
          </div>
        )
      })}
    </div>
  )
}

const LHDCard: React.FC<{ url: string }> = ({ url }) => {
  const { data } = usePolling<LHDState>(url, 3000)
  if (!data) return null
  return (
    <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 4 }}>
      <div className="metric-bar-container" style={{ marginBottom: 0 }}>
        <div className="metric-bar-label">
          <span className="metric-bar-name">Debris</span>
          <span className="metric-bar-value">{fmtPct(data.debrisClearedPct)}</span>
        </div>
        <ProgressBar value={data.debrisClearedPct} color="var(--amber)" />
      </div>
      <div className="metric-bar-container" style={{ marginBottom: 0 }}>
        <div className="metric-bar-label">
          <span className="metric-bar-name">Fuel</span>
          <span className="metric-bar-value">{fmtPct(data.fuelPct)}</span>
        </div>
        <ProgressBar value={data.fuelPct} color={data.fuelPct < 20 ? 'var(--red)' : 'var(--blue)'} />
      </div>
      <div style={{ fontSize: '0.72rem', color: 'var(--text-secondary)', marginTop: 2, display: 'flex', justifyContent: 'space-between' }}>
        <span>Payload: <strong style={{ color: 'var(--text-primary)' }}>{(data.payloadTons ?? 0).toFixed(1)} t</strong></span>
        <span style={{ color: data.online ? 'var(--green)' : 'var(--red)' }}>{data.trammingStatus ?? (data.online ? 'online' : 'offline')}</span>
      </div>
    </div>
  )
}

const ServiceCard: React.FC<ServiceCardProps> = ({ def, record }) => {
  const status = record ? (record.online ? 'online' : 'offline') : 'offline'
  const isRobot   = ['idt1a', 'idt1b'].includes(def.id)
  const isGas     = ['idt2a', 'idt2b'].includes(def.id)
  const isLHD     = ['idt3a', 'idt3b'].includes(def.id)

  return (
    <div
      className="card card-sm"
      style={{
        borderLeft: `3px solid ${typeColor(def.type)}`,
        display: 'flex',
        flexDirection: 'column',
        gap: 6,
      }}
    >
      {/* Header row */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
        <div style={{ minWidth: 0 }}>
          <div style={{
            fontWeight: 600,
            fontSize: '0.82rem',
            color: 'var(--text-primary)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}>
            {def.label}
          </div>
          <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)', marginTop: 1 }}>
            :{def.port}
          </div>
        </div>
        <span className={`badge ${badgeClass(status)}`} style={{ flexShrink: 0 }}>
          {status}
        </span>
      </div>

      {/* Type badge */}
      <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
        <span
          className="badge badge-nodot"
          style={{
            background: `color-mix(in srgb, ${typeColor(def.type)} 15%, transparent)`,
            border: `1px solid color-mix(in srgb, ${typeColor(def.type)} 35%, transparent)`,
            color: typeColor(def.type),
            fontSize: '0.65rem',
          }}
        >
          {def.type}
        </span>
        <span style={{ fontSize: '0.68rem', color: 'var(--text-muted)' }}>{def.category}</span>
      </div>

      {/* Last seen */}
      {record?.lastSeen && (
        <div style={{ fontSize: '0.68rem', color: 'var(--text-muted)' }}>
          Last seen: {fmtTs(record.lastSeen)}
        </div>
      )}

      {/* Telemetry */}
      {isRobot && def.stateUrl && <RobotCard url={def.stateUrl} label={def.label} />}
      {isGas   && def.stateUrl && <GasCard url={def.stateUrl} />}
      {isLHD   && def.stateUrl && <LHDCard url={def.stateUrl} />}
    </div>
  )
}

// ============================================================
// Service Composition Graph (SVG)
// ============================================================

interface GraphNode {
  id: string
  label: string
  type: 'iDT' | 'cDT' | 'core'
  cx: number
  cy: number
}

const GRAPH_NODES: GraphNode[] = [
  // iDTs (left column)
  { id: 'idt1a', label: 'iDT1a\nRobot A',    type: 'iDT', cx: 120, cy: 60  },
  { id: 'idt1b', label: 'iDT1b\nRobot B',    type: 'iDT', cx: 120, cy: 130 },
  { id: 'idt2a', label: 'iDT2a\nGas A',      type: 'iDT', cx: 120, cy: 200 },
  { id: 'idt2b', label: 'iDT2b\nGas B',      type: 'iDT', cx: 120, cy: 270 },
  { id: 'idt3a', label: 'iDT3a\nLHD A',      type: 'iDT', cx: 120, cy: 340 },
  { id: 'idt3b', label: 'iDT3b\nLHD B',      type: 'iDT', cx: 120, cy: 410 },
  { id: 'idt4',  label: 'iDT4\nTele-Remote', type: 'iDT', cx: 120, cy: 480 },
  // cDTs (middle column)
  { id: 'cdt1',  label: 'cDT1\nMapping',     type: 'cDT', cx: 340, cy: 95  },
  { id: 'cdt2',  label: 'cDT2\nGas Mon.',    type: 'cDT', cx: 340, cy: 200 },
  { id: 'cdt3',  label: 'cDT3\nHazard',      type: 'cDT', cx: 340, cy: 305 },
  { id: 'cdt4',  label: 'cDT4\nClearance',   type: 'cDT', cx: 340, cy: 375 },
  { id: 'cdt5',  label: 'cDT5\nIntervene',   type: 'cDT', cx: 340, cy: 480 },
  // Mission DTs (right column)
  { id: 'cdta',  label: 'cDTa\nMission',     type: 'cDT', cx: 560, cy: 280 },
  { id: 'cdtb',  label: 'cDTb\nSafe Access', type: 'cDT', cx: 560, cy: 390 },
]

const NODE_W = 90
const NODE_H = 44

const ServiceGraph: React.FC<{ serviceMap: Map<string, ServiceRecord> }> = ({ serviceMap }) => {
  const getNode = (id: string) => GRAPH_NODES.find(n => n.id === id)

  return (
    <div className="graph-container" style={{ overflowX: 'auto' }}>
      <svg
        viewBox="0 0 700 560"
        style={{ width: '100%', minWidth: 600, height: 'auto', display: 'block', padding: 16 }}
        xmlns="http://www.w3.org/2000/svg"
      >
        <defs>
          <marker id="arrow" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
            <path d="M0,0 L0,6 L8,3 z" fill="#94a3b8" />
          </marker>
          <marker id="arrow-blue" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
            <path d="M0,0 L0,6 L8,3 z" fill="var(--blue)" />
          </marker>
          <marker id="arrow-purple" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
            <path d="M0,0 L0,6 L8,3 z" fill="var(--purple)" />
          </marker>
        </defs>

        {/* Column labels */}
        {[
          { x: 120, label: 'Individual DTs (iDTs)' },
          { x: 340, label: 'Composite DTs (cDTs)' },
          { x: 560, label: 'Mission DTs' },
        ].map(col => (
          <text key={col.x} x={col.x} y={18} textAnchor="middle"
            style={{ fill: 'var(--text-muted)', fontSize: 9, fontFamily: 'sans-serif', textTransform: 'uppercase', letterSpacing: 1 }}>
            {col.label}
          </text>
        ))}

        {/* Edges */}
        {GRAPH_EDGES.map(([fromId, toId, label], i) => {
          const from = getNode(fromId)
          const to   = getNode(toId)
          if (!from || !to) return null
          const x1 = from.cx + NODE_W / 2
          const y1 = from.cy
          const x2 = to.cx - NODE_W / 2
          const y2 = to.cy
          const mx = (x1 + x2) / 2
          const isFromCDT = from.type === 'cDT'
          const markerColor = isFromCDT ? 'url(#arrow-purple)' : 'url(#arrow-blue)'
          const strokeColor = isFromCDT ? 'rgba(156,39,176,0.4)' : 'rgba(33,150,243,0.35)'
          return (
            <g key={i}>
              <path
                d={`M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}`}
                fill="none"
                stroke={strokeColor}
                strokeWidth={1.5}
                markerEnd={markerColor}
              />
              <text x={mx} y={(y1 + y2) / 2 - 3} textAnchor="middle"
                style={{ fill: 'var(--text-muted)', fontSize: 7, fontFamily: 'sans-serif' }}>
                {label}
              </text>
            </g>
          )
        })}

        {/* Nodes */}
        {GRAPH_NODES.map(node => {
          const rec = serviceMap.get(node.id)
          const status = rec ? (rec.online ? 'online' : 'offline') : 'offline'
          const fill = node.type === 'iDT' ? '#dbeafe' : node.type === 'cDT' ? '#ede9fe' : '#cffafe'
          const stroke = typeColor(node.type)
          const dotColor = status === 'online' ? 'var(--green)' : status === 'offline' ? 'var(--red)' : 'var(--amber)'
          const lines = node.label.split('\n')
          return (
            <g key={node.id} className="graph-node">
              <rect
                x={node.cx - NODE_W / 2}
                y={node.cy - NODE_H / 2}
                width={NODE_W}
                height={NODE_H}
                rx={6}
                fill={fill}
                stroke={stroke}
                strokeWidth={1.5}
              />
              {/* Status dot */}
              <circle cx={node.cx + NODE_W / 2 - 8} cy={node.cy - NODE_H / 2 + 8} r={4} fill={dotColor} />
              {/* Label */}
              {lines.map((line, li) => (
                <text key={li}
                  x={node.cx}
                  y={node.cy + (li - (lines.length - 1) / 2) * 13}
                  textAnchor="middle"
                  dominantBaseline="middle"
                  style={{
                    fill: li === 0 ? 'var(--text-primary)' : 'var(--text-secondary)',
                    fontSize: li === 0 ? 9 : 8,
                    fontFamily: 'sans-serif',
                    fontWeight: li === 0 ? 600 : 400,
                  }}
                >
                  {line}
                </text>
              ))}
            </g>
          )
        })}
      </svg>
    </div>
  )
}

// ============================================================
// Add Policy Modal
// ============================================================

interface AddPolicyModalProps {
  onClose: () => void
  onAdd: (p: AddPolicyPayload) => Promise<void>
}

const AddPolicyModal: React.FC<AddPolicyModalProps> = ({ onClose, onAdd }) => {
  const [form, setForm] = useState<AddPolicyPayload>({
    consumerId: '', providerId: '', serviceName: '', allowed: true,
  })
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    try { await onAdd(form) } finally { setLoading(false) }
    onClose()
  }

  return (
    <div style={{
      position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.7)',
      display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
    }}>
      <div className="card" style={{ width: 420, maxWidth: '95vw' }}>
        <div className="card-header">
          <span className="card-title">Add Authorization Policy</span>
          <button className="btn btn-ghost btn-sm" onClick={onClose}>✕</button>
        </div>
        <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {(['consumerId', 'providerId', 'serviceName'] as const).map(field => (
            <div key={field}>
              <label className="label">{field}</label>
              <input
                className="input"
                value={form[field]}
                onChange={e => setForm(f => ({ ...f, [field]: e.target.value }))}
                required
                placeholder={`Enter ${field}`}
              />
            </div>
          ))}
          <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 4 }}>
            <button type="button" className="btn btn-outline btn-sm" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn-primary btn-sm" disabled={loading}>
              {loading ? <span className="loading-spinner" /> : 'Add Policy'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ============================================================
// Main SystemView
// ============================================================

const SystemView: React.FC = () => {
  const { data: svcResp, loading: svcLoading, refetch: refetchSvc }
    = usePolling<ServicesResponse>(urls.services, 3000)
  const { data: polResp, refetch: refetchPolicies }
    = usePolling<PoliciesResponse>(urls.policies, 3000)
  const { data: orchResp }
    = usePolling<OrchLogsResponse>(urls.orchLogs, 3000)

  const services: ServiceRecord[] = svcResp?.services ?? []
  const policies: AuthPolicy[]    = polResp?.policies  ?? []
  const orchLogs: OrchestrationLog[] = orchResp?.logs  ?? []

  const [showAddPolicy, setShowAddPolicy] = useState(false)
  const [deletingId, setDeletingId] = useState<string | null>(null)

  // Build a map: service id → ServiceRecord
  const serviceMap = new Map<string, ServiceRecord>()
  services.forEach(s => serviceMap.set(s.id, s))
  // Arrowhead Core never registers itself — infer online from a successful fetch
  if (svcResp) {
    serviceMap.set('arrowhead', {
      id: 'arrowhead', name: 'Arrowhead Core', address: 'localhost', port: 8000,
      serviceType: 'core', capabilities: [], metadata: {},
      registeredAt: '', lastSeen: new Date().toISOString(), online: true,
    })
  }

  const handleAddPolicy = useCallback(async (payload: AddPolicyPayload) => {
    await addPolicy(payload)
    refetchPolicies()
  }, [refetchPolicies])

  const handleDeletePolicy = useCallback(async (id: string) => {
    setDeletingId(id)
    try {
      await deletePolicy(id)
      refetchPolicies()
    } finally {
      setDeletingId(null)
    }
  }, [refetchPolicies])

  // Group services by category for the telemetry display
  const robotDefs    = SERVICE_DEFS.filter(d => d.category === 'Robots')
  const gasDefs      = SERVICE_DEFS.filter(d => d.category === 'Gas Sensors')
  const lhdDefs      = SERVICE_DEFS.filter(d => d.category === 'LHDs')

  return (
    <div className="page-container" style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>

      {/* ====================================================
          Section 1 – Service Status Grid
          ==================================================== */}
      <section>
        <div className="section-title">
          <span>Service Status</span>
          <span style={{ color: 'var(--text-muted)', fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>
            {svcResp ? `${services.filter(s => s.online).length}/${services.length} online` : 'loading…'}
          </span>
          <button className="btn btn-ghost btn-sm" style={{ marginLeft: 'auto' }} onClick={refetchSvc}>
            {svcLoading ? <span className="loading-spinner" /> : '↻ Refresh'}
          </button>
        </div>

        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))',
          gap: 12,
        }}>
          {SERVICE_DEFS.map(def => (
            <ServiceCard
              key={def.id}
              def={def}
              record={serviceMap.get(def.id)}
            />
          ))}
        </div>
      </section>

      {/* ====================================================
          Section 2 – Service Composition Graph
          ==================================================== */}
      <section>
        <div className="section-title">Service Composition Graph</div>
        <div className="card" style={{ padding: 0 }}>
          <div className="card-header" style={{ padding: '16px 20px 12px' }}>
            <span className="card-title">DT Dependency Architecture</span>
            <div style={{ display: 'flex', gap: 12 }}>
              {[
                { label: 'iDT', color: 'var(--blue)' },
                { label: 'cDT', color: 'var(--purple)' },
              ].map(l => (
                <span key={l.label} style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: '0.72rem', color: 'var(--text-secondary)' }}>
                  <span style={{ width: 10, height: 10, background: l.color, borderRadius: 2 }} />
                  {l.label}
                </span>
              ))}
            </div>
          </div>
          <div style={{ padding: '0 12px 16px' }}>
            <ServiceGraph serviceMap={serviceMap} />
          </div>
        </div>
      </section>

      {/* ====================================================
          Section 3 – Orchestration Logs
          ==================================================== */}
      <section>
        <div className="section-title">Arrowhead Orchestration Logs</div>
        <div className="card" style={{ padding: 0 }}>
          <div className="card-header" style={{ padding: '14px 18px 10px' }}>
            <span className="card-title">Last 20 Orchestration Events</span>
            <span className="badge badge-info badge-nodot" style={{ fontSize: '0.68rem' }}>
              {orchLogs?.length ?? 0} entries
            </span>
          </div>
          <div className="table-container" style={{ border: 'none', borderRadius: 0 }}>
            <table>
              <thead>
                <tr>
                  <th>Timestamp</th>
                  <th>Consumer</th>
                  <th>Provider</th>
                  <th>Service</th>
                  <th>Allowed</th>
                  <th>Reason</th>
                </tr>
              </thead>
              <tbody>
                {!orchLogs || orchLogs.length === 0 ? (
                  <tr>
                    <td colSpan={6} style={{ textAlign: 'center', padding: 24, color: 'var(--text-muted)' }}>
                      No orchestration logs yet
                    </td>
                  </tr>
                ) : (
                  orchLogs.slice(0, 20).map(log => (
                    <tr key={log.id}>
                      <td className="mono" style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
                        {fmtTs(log.timestamp)}
                      </td>
                      <td style={{ color: 'var(--blue-light)' }}>{log.consumerId}</td>
                      <td style={{ color: 'var(--text-secondary)' }}>{log.providerId}</td>
                      <td><code>{log.serviceName}</code></td>
                      <td>
                        <span className={`badge ${log.allowed ? 'badge-online' : 'badge-offline'}`}>
                          {log.allowed ? 'Yes' : 'No'}
                        </span>
                      </td>
                      <td style={{ color: 'var(--text-muted)', fontSize: '0.75rem' }}>
                        {log.message ?? '—'}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      </section>

      {/* ====================================================
          Section 4 – Authorization Policies
          ==================================================== */}
      <section>
        <div className="section-title">Authorization Policies</div>
        <div className="card" style={{ padding: 0 }}>
          <div className="card-header" style={{ padding: '14px 18px 10px' }}>
            <span className="card-title">Access Control Rules</span>
            <button
              className="btn btn-primary btn-sm"
              onClick={() => setShowAddPolicy(true)}
            >
              + Add Policy
            </button>
          </div>
          <div className="table-container" style={{ border: 'none', borderRadius: 0 }}>
            <table>
              <thead>
                <tr>
                  <th>Consumer</th>
                  <th>Provider</th>
                  <th>Service</th>
                  <th>Status</th>
                  <th>Created</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {!policies || policies.length === 0 ? (
                  <tr>
                    <td colSpan={6} style={{ textAlign: 'center', padding: 24, color: 'var(--text-muted)' }}>
                      No policies configured
                    </td>
                  </tr>
                ) : (
                  policies.map(p => (
                    <tr key={p.id}>
                      <td style={{ color: 'var(--blue-light)' }}>{p.consumerId}</td>
                      <td style={{ color: 'var(--text-secondary)' }}>{p.providerId}</td>
                      <td><code>{p.serviceName}</code></td>
                      <td>
                        <span className={`badge ${p.allowed ? 'badge-online' : 'badge-offline'}`}>
                          {p.allowed ? 'Allowed' : 'Denied'}
                        </span>
                      </td>
                      <td style={{ color: 'var(--text-muted)', fontSize: '0.72rem' }}>
                        {fmtTs(p.createdAt)}
                      </td>
                      <td>
                        <button
                          className="btn btn-danger btn-sm"
                          disabled={deletingId === p.id}
                          onClick={() => handleDeletePolicy(p.id)}
                        >
                          {deletingId === p.id ? <span className="loading-spinner" /> : 'Delete'}
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      </section>

      {/* ====================================================
          Section 5 – Per-Service Telemetry Summary
          ==================================================== */}
      <section>
        <div className="section-title">Service Telemetry</div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>

          {/* Robots */}
          <div>
            <h6 style={{ marginBottom: 8, color: 'var(--blue)', fontSize: '0.7rem' }}>Robots</h6>
            <div className="grid-2">
              {robotDefs.map(d => (
                <div key={d.id} className="card card-sm card-highlight-blue">
                  <div style={{ fontWeight: 600, fontSize: '0.82rem', marginBottom: 6 }}>{d.label}</div>
                  {d.stateUrl && <RobotCard url={d.stateUrl} label={d.label} />}
                </div>
              ))}
            </div>
          </div>

          {/* Gas Sensors */}
          <div>
            <h6 style={{ marginBottom: 8, color: 'var(--amber)', fontSize: '0.7rem' }}>Gas Sensors</h6>
            <div className="grid-2">
              {gasDefs.map(d => (
                <div key={d.id} className="card card-sm card-highlight-amber">
                  <div style={{ fontWeight: 600, fontSize: '0.82rem', marginBottom: 6 }}>{d.label}</div>
                  {d.stateUrl && <GasCard url={d.stateUrl} />}
                </div>
              ))}
            </div>
          </div>

          {/* LHDs */}
          <div>
            <h6 style={{ marginBottom: 8, color: 'var(--green)', fontSize: '0.7rem' }}>LHDs</h6>
            <div className="grid-2">
              {lhdDefs.map(d => (
                <div key={d.id} className="card card-sm card-highlight-green">
                  <div style={{ fontWeight: 600, fontSize: '0.82rem', marginBottom: 6 }}>{d.label}</div>
                  {d.stateUrl && <LHDCard url={d.stateUrl} />}
                </div>
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* Modal */}
      {showAddPolicy && (
        <AddPolicyModal
          onClose={() => setShowAddPolicy(false)}
          onAdd={handleAddPolicy}
        />
      )}
    </div>
  )
}

export default SystemView
