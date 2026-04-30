// UML-style component diagram of the Arrowhead 5 core systems.
// Pure React SVG — no external dependencies.

const W = 600
const H = 260

// Box dimensions
const BW = 140
const BH = 38

// System boxes: [id, label, x-centre, y-centre, color]
const BOXES: [string, string, number, number, string][] = [
  ['sr',   'ServiceRegistry',  150, 60,  '#dbeafe'],
  ['ca',   'CA',               450, 60,  '#dbeafe'],
  ['auth', 'ConsumerAuth',     150, 180, '#dbeafe'],
  ['orch', 'DynamicOrch',      450, 180, '#dbeafe'],
]

// Arrows: [from-id, to-id, label]
const ARROWS: [string, string, string][] = [
  ['orch', 'sr',   'lookup'],
  ['orch', 'auth', 'verify'],
]

type BoxMap = Record<string, { cx: number; cy: number }>

function boxMap(): BoxMap {
  const m: BoxMap = {}
  for (const [id, , cx, cy] of BOXES) m[id] = { cx, cy }
  return m
}

function Arrow({ x1, y1, x2, y2, label }: { x1: number; y1: number; x2: number; y2: number; label: string }) {
  const mx = (x1 + x2) / 2
  const my = (y1 + y2) / 2
  return (
    <g>
      <defs>
        <marker id="ah" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
          <path d="M0,0 L0,6 L8,3 z" fill="#666" />
        </marker>
      </defs>
      <line x1={x1} y1={y1} x2={x2} y2={y2}
        stroke="#666" strokeWidth={1.5} markerEnd="url(#ah)" />
      <text x={mx} y={my - 5} fontSize={10} fill="#888" textAnchor="middle">{label}</text>
    </g>
  )
}

function SystemBox({ label, cx, cy, bg }: { label: string; cx: number; cy: number; bg: string }) {
  return (
    <g>
      <rect x={cx - BW / 2} y={cy - BH / 2} width={BW} height={BH}
        rx={4} fill={bg} stroke="#93c5fd" strokeWidth={1.5} />
      <text x={cx} y={cy + 5} fontSize={12} fontFamily="monospace"
        fontWeight="bold" fill="#1e3a5f" textAnchor="middle">
        {label}
      </text>
    </g>
  )
}

export function CoreDiagram() {
  const bm = boxMap()
  return (
    <svg width={W} height={H} viewBox={`0 0 ${W} ${H}`} style={{ maxWidth: '100%' }}>
      {/* Title */}
      <text x={W / 2} y={20} fontSize={13} fontFamily="monospace" fontWeight="bold"
        fill="#444" textAnchor="middle">
        Arrowhead 5 — Core Systems
      </text>

      {/* Dependency note */}
      <text x={W / 2} y={H - 10} fontSize={10} fontFamily="monospace"
        fill="#aaa" textAnchor="middle">
        arrows = runtime dependency (DynamicOrch queries SR and ConsumerAuth)
      </text>

      {/* Arrows */}
      {ARROWS.map(([from, to, lbl]) => {
        const f = bm[from], t = bm[to]
        // Attach at box edges
        const dx = t.cx - f.cx, dy = t.cy - f.cy
        const len = Math.sqrt(dx * dx + dy * dy)
        const ux = dx / len, uy = dy / len
        const x1 = f.cx + ux * (BW / 2), y1 = f.cy + uy * (BH / 2)
        const x2 = t.cx - ux * (BW / 2), y2 = t.cy - uy * (BH / 2)
        return <Arrow key={`${from}-${to}`} x1={x1} y1={y1} x2={x2} y2={y2} label={lbl} />
      })}

      {/* Boxes */}
      {BOXES.map(([id, label, cx, cy, bg]) => (
        <SystemBox key={id} label={label} cx={cx} cy={cy} bg={bg} />
      ))}

      {/* Legend */}
      <rect x={20} y={H - 40} width={14} height={14} rx={2} fill="#dbeafe" stroke="#93c5fd" strokeWidth={1} />
      <text x={40} y={H - 29} fontSize={10} fontFamily="monospace" fill="#666">core system</text>
    </svg>
  )
}
