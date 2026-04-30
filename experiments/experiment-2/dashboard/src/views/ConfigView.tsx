// Configuration view — wraps ConfigPanel with a page heading.

import { ConfigPanel } from '../components/ConfigPanel'

export function ConfigView() {
  return (
    <div>
      <section style={s.section}>
        <h2 style={s.heading}>Dashboard Configuration</h2>
        <p style={s.description}>
          Settings are persisted to localStorage and take effect immediately on Apply.
          Intervals are in milliseconds.
        </p>
      </section>
      <ConfigPanel />
    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  section:     { marginBottom: 20 },
  heading:     { fontSize: '0.9rem', marginBottom: 4, color: '#555' },
  description: { fontSize: '0.75rem', color: '#888', margin: 0 },
}
