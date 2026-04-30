// Support systems view — currently shows RabbitMQ stats.
// Additional support systems can be added here as new panels.

import { BrokerStats } from '../components/BrokerStats'

export function SupportSystemsView() {
  return (
    <div>
      <section style={s.header}>
        <h2 style={s.heading}>RabbitMQ</h2>
        <p style={s.sub}>
          Management API via <code>/api/rabbitmq/</code>.
          Exchange <code>arrowhead</code> carries <code>telemetry.robot</code> messages.
        </p>
      </section>
      <BrokerStats />
    </div>
  )
}

const s = {
  header:  { marginBottom: 8 },
  heading: { fontSize: '0.9rem', marginBottom: 4, color: '#555' },
  sub:     { fontSize: '0.8rem', color: '#888', margin: 0 },
}
