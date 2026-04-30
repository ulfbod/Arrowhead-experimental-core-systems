// Experiment-2 view: shows the live AMQP→HTTP data flow.

import { TelemetryPanel } from '../components/TelemetryPanel'
import { OrchestrationTrace } from '../components/OrchestrationTrace'

const FLOW = `robot-simulator
  │  publishes JSON to AMQP exchange "arrowhead"
  │  routing key: telemetry.robot
  ▼
RabbitMQ
  │  topic routing (telemetry.#)
  ▼
edge-adapter          ← registered in ServiceRegistry as "telemetry"
  │  stores latest payload
  │  serves GET /telemetry/latest
  ▼
consumer
     re-orchestrates every 5 s via DynamicOrchestration
     fetches /telemetry/latest from discovered endpoint`

export function ExperimentView() {
  return (
    <div>
      <section style={s.section}>
        <h2 style={s.heading}>Data Flow</h2>
        <pre style={s.flow}>{FLOW}</pre>
      </section>

      <TelemetryPanel />
      <OrchestrationTrace />
    </div>
  )
}

const s = {
  section: { marginBottom: 8 },
  heading: { fontSize: '0.9rem', marginBottom: 6, color: '#555' },
  flow:    {
    background: '#f8f8f8', border: '1px solid #ddd',
    borderRadius: 4, padding: '10px 14px',
    fontSize: '0.75rem', lineHeight: 1.6,
    margin: 0, overflowX: 'auto' as const,
  },
}
