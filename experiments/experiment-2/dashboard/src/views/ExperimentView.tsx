// Experiment-2 view: orchestrates all experiment panels.

import { useState } from 'react'
import { TelemetryPanel } from '../components/TelemetryPanel'
import { OrchestrationTrace } from '../components/OrchestrationTrace'
import { SimulationControl } from '../components/SimulationControl'
import { FleetMetrics } from '../components/FleetMetrics'
import { LatencyChart } from '../components/LatencyChart'
import { ThroughputChart } from '../components/ThroughputChart'
import { AnalysisSummary } from '../components/AnalysisSummary'
import { MultiConsumerPanel } from '../components/MultiConsumerPanel'
import { useConfig } from '../config/context'
import { usePolling } from '../hooks/usePolling'
import { fetchTelemetryStats } from '../api'
import type { TelemetryStatsResponse } from '../types'

const FLOW = `robot-fleet (N robots)
  │  each robot publishes JSON to AMQP exchange "arrowhead"
  │  routing key: telemetry.{robotId}  — 5G network emulation applied
  ▼
RabbitMQ
  │  topic routing (telemetry.#)
  ▼
edge-adapter          ← registered in ServiceRegistry as "telemetry"
  │  tracks latest payload and rolling stats per robot
  │  serves GET /telemetry/latest  /telemetry/all  /telemetry/stats
  ▼
consumer × 3
     each re-orchestrates every 5 s via DynamicOrchestration
     fetches /telemetry/latest from discovered endpoint`

function Collapsible({ title, defaultOpen = true, children }: {
  title: string; defaultOpen?: boolean; children: React.ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div style={cs.wrap}>
      <button style={cs.toggle} onClick={() => setOpen(o => !o)} aria-expanded={open}>
        <span style={cs.arrow}>{open ? '▾' : '▸'}</span>
        {title}
      </button>
      {open && <div style={cs.body}>{children}</div>}
    </div>
  )
}

const cs: Record<string, React.CSSProperties> = {
  wrap:   { marginBottom: 12, border: '1px solid #e5e7eb', borderRadius: 6 },
  toggle: { width: '100%', textAlign: 'left', background: '#f9fafb', border: 'none', borderRadius: 6, padding: '8px 12px', cursor: 'pointer', fontSize: '0.85rem', fontWeight: 'bold', color: '#333', fontFamily: 'monospace', display: 'flex', alignItems: 'center', gap: 6 },
  arrow:  { fontSize: '0.7rem' },
  body:   { padding: '12px 14px', borderTop: '1px solid #e5e7eb' },
}

export function ExperimentView() {
  const { config } = useConfig()
  const { data: stats } = usePolling<TelemetryStatsResponse>(
    (signal) => fetchTelemetryStats(signal),
    config.polling.allTelemetryIntervalMs,
  )

  return (
    <div>
      <Collapsible title="Data Flow">
        <pre style={s.flow}>{FLOW}</pre>
      </Collapsible>

      <Collapsible title="Fleet Simulation Control">
        <SimulationControl />
      </Collapsible>

      <Collapsible title="Telemetry Feed">
        <TelemetryPanel />
      </Collapsible>

      <Collapsible title="Analysis">
        <AnalysisSummary stats={stats ?? null} />
        <LatencyChart stats={stats ?? null} />
        <ThroughputChart stats={stats ?? null} />
        <FleetMetrics />
      </Collapsible>

      <Collapsible title="Consumer Status" defaultOpen={false}>
        <MultiConsumerPanel />
      </Collapsible>

      <Collapsible title="Orchestration Trace" defaultOpen={false}>
        <OrchestrationTrace />
      </Collapsible>
    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  flow: {
    background: '#f8f8f8', border: '1px solid #ddd',
    borderRadius: 4, padding: '10px 14px',
    fontSize: '0.75rem', lineHeight: 1.6,
    margin: 0, overflowX: 'auto' as const,
  },
}
