// Full experiment-2 system diagram.
// Shows the AMQP data path, Arrowhead service discovery, and consumer flow.

const W = 730
const H = 480

type Rect = { x: number; y: number; w: number; h: number }

function box(x: number, y: number, w = 130, h = 38): Rect {
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
// Row 1 (y≈65): robot-fleet → [RabbitMQ: exchange → queue] → edge-adapter
// Row 2 (y≈200-300): consumer / consumer-2 / consumer-3 (stacked)
//                    Arrowhead Core: ConsumerAuth, DynamicOrch, ServiceReg, CA

const rRobot    = box(20,  52,  110, 40)
const rExchange = box(180, 52,  120, 40)
const rQueue    = box(350, 52,  110, 40)
const rEdge     = box(512, 52,  160, 40)
const rRmq      = { x: 170, y: 40, w: 312, h: 64 }

// Stacked consumers (left column)
const rCons1    = box(20, 185, 110, 35)
const rCons2    = box(20, 238, 110, 35)
const rCons3    = box(20, 291, 110, 35)

// Arrowhead Core systems
const rConsAuth = box(200, 185, 130, 35)
const rOrch     = box(200, 238, 130, 35)
const rSR       = box(375, 210, 135, 35)
const rCA       = box(550, 210, 100, 35)

// Arrowhead Core bounding box
const rCore     = { x: 190, y: 175, w: 472, h: 115 }

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
      <rect x={rCore.x} y={rCore.y} width={rCore.w} height={rCore.h}
        rx={6} fill="none" stroke="#93c5fd" strokeWidth={1} strokeDasharray="6,3" />
      <text x={rCore.x + 4} y={rCore.y - 4} fontSize={9} fontFamily="monospace" fill="#60a5fa">
        Arrowhead Core
      </text>

      {/* AMQP data path (row 1) */}
      <Arr id="a1" x1={rRobot.x + rRobot.w}      y1={72} x2={rExchange.x}            y2={72} label="publish"           color="#9ca3af" />
      <Arr id="a2" x1={rExchange.x + rExchange.w} y1={72} x2={rQueue.x}               y2={72} label="route telemetry.#" color="#9ca3af" />
      <Arr id="a3" x1={rQueue.x + rQueue.w}       y1={72} x2={rEdge.x}                y2={72} label="consume"           color="#9ca3af" />

      {/* edge-adapter → ServiceRegistry (registration) */}
      <Arr id="a4"
        x1={rEdge.x + 80} y1={rEdge.y + rEdge.h}
        x2={rSR.x + 67}   y2={rSR.y}
        label="register" color="#93c5fd" dashed />

      {/* CA → edge-adapter (cert) */}
      <Arr id="a7"
        x1={rCA.x + 50}    y1={rCA.y}
        x2={rEdge.x + 130} y2={rEdge.y + rEdge.h}
        label="cert" color="#93c5fd" dashed />

      {/* Consumers → DynamicOrch (orchestrate) — arrow from consumer-2 (middle) */}
      <Arr id="a5"
        x1={rCons2.x + rCons2.w} y1={rCons2.y + rCons2.h / 2}
        x2={rOrch.x}              y2={rOrch.y + rOrch.h / 2}
        label="orchestrate" color="#a78bfa" />

      {/* DynamicOrch → ConsumerAuth (auth check) */}
      <Arr id="a8"
        x1={rOrch.x + rOrch.w / 2}     y1={rOrch.y}
        x2={rConsAuth.x + rConsAuth.w / 2} y2={rConsAuth.y + rConsAuth.h}
        label="auth check" color="#a78bfa" />

      {/* DynamicOrch → ServiceReg (query) */}
      <Arr id="a6"
        x1={rOrch.x + rOrch.w} y1={rOrch.y + rOrch.h / 2}
        x2={rSR.x}              y2={rSR.y + rSR.h / 2}
        label="query" color="#a78bfa" />

      {/* Consumers → edge-adapter (fetch /telemetry/latest) */}
      <polyline
        points={`${rCons2.x + 55},${rCons1.y}  ${rCons2.x + 55},${rEdge.y + rEdge.h + 22}  ${rEdge.x + 80},${rEdge.y + rEdge.h + 22}  ${rEdge.x + 80},${rEdge.y + rEdge.h}`}
        fill="none" stroke="#a78bfa" strokeWidth={1.5} strokeDasharray="5,3" />
      <text x={320} y={rEdge.y + rEdge.h + 38} fontSize={9} fontFamily="monospace"
        fill="#a78bfa" textAnchor="middle">
        fetch /telemetry/latest (discovered via DynamicOrch)
      </text>

      {/* Boxes (draw last so on top) */}
      <Box r={rRobot}    fill="#fce7f3" stroke="#f9a8d4" label="robot-fleet"  sublabel=":9003" />
      <Box r={rExchange} fill="#d1fae5" stroke="#6ee7b7" label="exchange"     sublabel="arrowhead" />
      <Box r={rQueue}    fill="#d1fae5" stroke="#6ee7b7" label="queue"        sublabel="telemetry" />
      <Box r={rEdge}     fill="#ede9fe" stroke="#c4b5fd" label="edge-adapter" sublabel=":9001" />
      <Box r={rCons1}    fill="#ede9fe" stroke="#c4b5fd" label="consumer"     sublabel=":9002" />
      <Box r={rCons2}    fill="#ede9fe" stroke="#c4b5fd" label="consumer-2"   sublabel=":9004" />
      <Box r={rCons3}    fill="#ede9fe" stroke="#c4b5fd" label="consumer-3"   sublabel=":9005" />
      <Box r={rConsAuth} fill="#dbeafe" stroke="#93c5fd" label="ConsumerAuth" sublabel=":8082" />
      <Box r={rOrch}     fill="#dbeafe" stroke="#93c5fd" label="DynamicOrch"  sublabel=":8083" />
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
