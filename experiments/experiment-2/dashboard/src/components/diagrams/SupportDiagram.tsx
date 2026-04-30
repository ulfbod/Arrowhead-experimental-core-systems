// UML-style diagram of the RabbitMQ support system topology.
// Shows the AMQP exchange → queue → consumer flow used in experiment-2.

const W = 600
const H = 280

function Box({
  x, y, w, h, fill, stroke, label, sublabel,
}: {
  x: number; y: number; w: number; h: number
  fill: string; stroke: string
  label: string; sublabel?: string
}) {
  return (
    <g>
      <rect x={x} y={y} width={w} height={h} rx={4} fill={fill} stroke={stroke} strokeWidth={1.5} />
      <text x={x + w / 2} y={y + h / 2 - (sublabel ? 6 : 0)} fontSize={12}
        fontFamily="monospace" fontWeight="bold" fill="#1a1a2e" textAnchor="middle">
        {label}
      </text>
      {sublabel && (
        <text x={x + w / 2} y={y + h / 2 + 10} fontSize={10}
          fontFamily="monospace" fill="#666" textAnchor="middle">
          {sublabel}
        </text>
      )}
    </g>
  )
}

function Arrow({
  x1, y1, x2, y2, label, dashed,
}: {
  x1: number; y1: number; x2: number; y2: number
  label?: string; dashed?: boolean
}) {
  const mx = (x1 + x2) / 2
  const my = (y1 + y2) / 2
  return (
    <g>
      <defs>
        <marker id="arrowS" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
          <path d="M0,0 L0,6 L8,3 z" fill="#666" />
        </marker>
      </defs>
      <line x1={x1} y1={y1} x2={x2} y2={y2}
        stroke="#666" strokeWidth={1.5}
        strokeDasharray={dashed ? '5,3' : undefined}
        markerEnd="url(#arrowS)" />
      {label && (
        <text x={mx} y={my - 6} fontSize={10} fontFamily="monospace"
          fill="#888" textAnchor="middle">
          {label}
        </text>
      )}
    </g>
  )
}

export function SupportDiagram() {
  // Layout: publisher → exchange → queue → consumer
  const pubX = 20,   midY = 120
  const excX = 160,  excW = 150, excH = 50
  const qX   = 370,  qW   = 130, qH   = 50
  const conX = 470

  return (
    <svg width={W} height={H} viewBox={`0 0 ${W} ${H}`} style={{ maxWidth: '100%' }}>
      <text x={W / 2} y={20} fontSize={13} fontFamily="monospace" fontWeight="bold"
        fill="#444" textAnchor="middle">
        Support Systems — RabbitMQ Topology
      </text>

      {/* robot-simulator publisher */}
      <Box x={pubX} y={midY - 25} w={120} h={50}
        fill="#fce7f3" stroke="#f9a8d4" label="robot-sim" sublabel="publisher" />

      {/* RabbitMQ exchange */}
      <Box x={excX} y={midY - 25} w={excW} h={excH}
        fill="#d1fae5" stroke="#6ee7b7" label="arrowhead" sublabel="topic exchange" />

      {/* Queue */}
      <Box x={qX} y={midY - 25} w={qW} h={qH}
        fill="#d1fae5" stroke="#6ee7b7" label="telemetry" sublabel="queue" />

      {/* edge-adapter consumer */}
      <Box x={conX + 40} y={midY - 25} w={120} h={50}
        fill="#ede9fe" stroke="#c4b5fd" label="edge-adapter" sublabel="consumer" />

      {/* Arrows */}
      <Arrow x1={pubX + 120} y1={midY} x2={excX} y2={midY}
        label="AMQP publish" />
      <Arrow x1={excX + excW} y1={midY} x2={qX} y2={midY}
        label="telemetry.#" />
      <Arrow x1={qX + qW} y1={midY} x2={conX + 40} y2={midY}
        label="consume" />

      {/* Routing key annotation */}
      <text x={excX + excW / 2} y={midY + 38} fontSize={9} fontFamily="monospace"
        fill="#888" textAnchor="middle">binding: telemetry.robot → telemetry queue</text>

      {/* RabbitMQ bounding box */}
      <rect x={excX - 10} y={midY - 45} width={excW + qW + 40} height={100}
        rx={6} fill="none" stroke="#6ee7b7" strokeWidth={1} strokeDasharray="6,3" />
      <text x={excX - 10} y={midY - 50} fontSize={10} fontFamily="monospace"
        fill="#6ee7b7">RabbitMQ</text>

      {/* Legend */}
      <rect x={20} y={H - 45} width={12} height={12} rx={2} fill="#d1fae5" stroke="#6ee7b7" strokeWidth={1} />
      <text x={38} y={H - 35} fontSize={10} fontFamily="monospace" fill="#666">RabbitMQ internals</text>
      <rect x={20} y={H - 28} width={12} height={12} rx={2} fill="#ede9fe" stroke="#c4b5fd" strokeWidth={1} />
      <text x={38} y={H - 18} fontSize={10} fontFamily="monospace" fill="#666">experiment service</text>
    </svg>
  )
}
