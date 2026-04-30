// Full experiment-2 system diagram.
// Shows the AMQP data path, Arrowhead service discovery, and consumer flow.

const W = 700
const H = 380

type Rect = { x: number; y: number; w: number; h: number }

function box(x: number, y: number, w = 130, h = 40): Rect {
  return { x, y, w, h }
}

function Box({
  r, fill, stroke, label, sublabel,
}: {
  r: Rect; fill: string; stroke: string; label: string; sublabel?: string
}) {
  const cx = r.x + r.w / 2
  return (
    <g>
      <rect x={r.x} y={r.y} width={r.w} height={r.h} rx={4}
        fill={fill} stroke={stroke} strokeWidth={1.5} />
      <text x={cx} y={r.y + r.h / 2 - (sublabel ? 6 : 0) + 1}
        fontSize={11} fontFamily="monospace" fontWeight="bold"
        fill="#1a1a2e" textAnchor="middle">
        {label}
      </text>
      {sublabel && (
        <text x={cx} y={r.y + r.h / 2 + 10}
          fontSize={9} fontFamily="monospace" fill="#666" textAnchor="middle">
          {sublabel}
        </text>
      )}
    </g>
  )
}

function Arr({
  x1, y1, x2, y2, label, color, dashed, id,
}: {
  x1: number; y1: number; x2: number; y2: number
  label?: string; color?: string; dashed?: boolean; id: string
}) {
  const c = color ?? '#666'
  const mx = (x1 + x2) / 2
  const my = (y1 + y2) / 2
  return (
    <g>
      <defs>
        <marker id={id} markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
          <path d="M0,0 L0,6 L8,3 z" fill={c} />
        </marker>
      </defs>
      <line x1={x1} y1={y1} x2={x2} y2={y2}
        stroke={c} strokeWidth={1.5}
        strokeDasharray={dashed ? '5,3' : undefined}
        markerEnd={`url(#${id})`} />
      {label && (
        <text x={mx} y={my - 5} fontSize={9} fontFamily="monospace"
          fill={c} textAnchor="middle">
          {label}
        </text>
      )}
    </g>
  )
}

// ── Layout ───────────────────────────────────────────────────────────────────
// Row 1 (y≈60):  robot-sim → [RabbitMQ: exchange → queue] → edge-adapter
// Row 2 (y≈180): consumer ←→ DynamicOrch ←→ ServiceRegistry  |  CA
// Vertical:      edge-adapter ↔ ServiceRegistry (registration)
//                consumer → edge-adapter (fetch)

const rRobot    = box(20,  50,  110, 40)
const rExchange = box(175, 50,  130, 40)
const rQueue    = box(355, 50,  110, 40)
const rEdge     = box(510, 50,  140, 40)
const rRmq      = { x: 165, y: 38, w: 310, h: 64 }   // dashed bounding box

const rConsumer = box(20,  200, 110, 40)
const rOrch     = box(190, 200, 150, 40)
const rSR       = box(400, 200, 150, 40)
const rCA       = box(575, 200, 100, 40)

export function ExperimentDiagram() {
  return (
    <svg width={W} height={H} viewBox={`0 0 ${W} ${H}`} style={{ maxWidth: '100%' }}>
      {/* Title */}
      <text x={W / 2} y={20} fontSize={13} fontFamily="monospace" fontWeight="bold"
        fill="#444" textAnchor="middle">
        Experiment 2 — Full System View
      </text>

      {/* RabbitMQ bounding box */}
      <rect x={rRmq.x} y={rRmq.y} width={rRmq.w} height={rRmq.h}
        rx={6} fill="none" stroke="#6ee7b7" strokeWidth={1} strokeDasharray="6,3" />
      <text x={rRmq.x + 4} y={rRmq.y - 4} fontSize={9} fontFamily="monospace" fill="#4ade80">
        RabbitMQ
      </text>

      {/* Arrowhead core bounding box */}
      <rect x={185} y={188} width={500} height={64}
        rx={6} fill="none" stroke="#93c5fd" strokeWidth={1} strokeDasharray="6,3" />
      <text x={189} y={184} fontSize={9} fontFamily="monospace" fill="#60a5fa">
        Arrowhead Core
      </text>

      {/* Data path arrows (row 1) */}
      <Arr id="a1" x1={rRobot.x + rRobot.w}    y1={70}  x2={rExchange.x}           y2={70}  label="publish" color="#9ca3af" />
      <Arr id="a2" x1={rExchange.x + rExchange.w} y1={70} x2={rQueue.x}              y2={70}  label="route telemetry.#" color="#9ca3af" />
      <Arr id="a3" x1={rQueue.x + rQueue.w}     y1={70}  x2={rEdge.x}               y2={70}  label="consume" color="#9ca3af" />

      {/* edge-adapter ↕ ServiceRegistry (registration + lookup) */}
      <Arr id="a4" x1={rEdge.x + 70} y1={rEdge.y + rEdge.h} x2={rSR.x + 75} y2={rSR.y}
        label="register" color="#93c5fd" dashed />

      {/* Consumer orchestration flow */}
      <Arr id="a5" x1={rConsumer.x + rConsumer.w} y1={220} x2={rOrch.x}   y2={220}
        label="orchestrate" color="#a78bfa" />
      <Arr id="a6" x1={rOrch.x + rOrch.w}         y1={220} x2={rSR.x}     y2={220}
        label="query" color="#a78bfa" />

      {/* Consumer → edge-adapter (data fetch — curved via vertical line) */}
      <polyline
        points={`${rConsumer.x + 55},${rConsumer.y}  ${rConsumer.x + 55},${rEdge.y + rEdge.h + 20}  ${rEdge.x + 70},${rEdge.y + rEdge.h + 20}  ${rEdge.x + 70},${rEdge.y + rEdge.h}`}
        fill="none" stroke="#a78bfa" strokeWidth={1.5} strokeDasharray="5,3" />
      <text x={280} y={rEdge.y + rEdge.h + 35} fontSize={9} fontFamily="monospace" fill="#a78bfa" textAnchor="middle">
        fetch /telemetry/latest (discovered via DynamicOrch)
      </text>

      {/* CA ←→ edge-adapter registration uses CA */}
      <Arr id="a7" x1={rCA.x + 50} y1={rCA.y} x2={rEdge.x + 110} y2={rEdge.y + rEdge.h}
        label="cert" color="#93c5fd" dashed />

      {/* Boxes (draw last so they're on top) */}
      <Box r={rRobot}    fill="#fce7f3" stroke="#f9a8d4" label="robot-sim"    sublabel="publisher" />
      <Box r={rExchange} fill="#d1fae5" stroke="#6ee7b7" label="exchange"     sublabel="arrowhead" />
      <Box r={rQueue}    fill="#d1fae5" stroke="#6ee7b7" label="queue"        sublabel="telemetry" />
      <Box r={rEdge}     fill="#ede9fe" stroke="#c4b5fd" label="edge-adapter" sublabel=":9001" />
      <Box r={rConsumer} fill="#ede9fe" stroke="#c4b5fd" label="consumer"     sublabel=":9002" />
      <Box r={rOrch}     fill="#dbeafe" stroke="#93c5fd" label="DynamicOrch"  sublabel=":8085" />
      <Box r={rSR}       fill="#dbeafe" stroke="#93c5fd" label="ServiceReg"   sublabel=":8080" />
      <Box r={rCA}       fill="#dbeafe" stroke="#93c5fd" label="CA"           sublabel=":8086" />

      {/* Legend */}
      {[
        ['#dbeafe', '#93c5fd', 'core'],
        ['#d1fae5', '#6ee7b7', 'support'],
        ['#ede9fe', '#c4b5fd', 'experiment'],
        ['#fce7f3', '#f9a8d4', 'simulator'],
      ].map(([fill, stroke, label], i) => (
        <g key={label} transform={`translate(${20 + i * 110}, ${H - 28})`}>
          <rect width={12} height={12} rx={2} fill={fill} stroke={stroke} strokeWidth={1} />
          <text x={16} y={10} fontSize={9} fontFamily="monospace" fill="#666">{label}</text>
        </g>
      ))}
    </svg>
  )
}
