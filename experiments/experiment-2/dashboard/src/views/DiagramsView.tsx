// Diagram viewer — tab switcher for the three system diagrams.

import { useState } from 'react'
import { CoreDiagram }       from '../components/diagrams/CoreDiagram'
import { SupportDiagram }    from '../components/diagrams/SupportDiagram'
import { ExperimentDiagram } from '../components/diagrams/ExperimentDiagram'

type DiagramTab = 'core' | 'support' | 'experiment'

const TABS: { id: DiagramTab; label: string }[] = [
  { id: 'core',       label: 'Core Systems' },
  { id: 'support',    label: 'Support Systems' },
  { id: 'experiment', label: 'Experiment 2' },
]

export function DiagramsView() {
  const [active, setActive] = useState<DiagramTab>('experiment')

  return (
    <div>
      <section style={s.header}>
        <h2 style={s.heading}>System Diagrams</h2>
        <div style={s.tabs}>
          {TABS.map(t => (
            <button
              key={t.id}
              onClick={() => setActive(t.id)}
              aria-pressed={active === t.id}
              style={{ ...s.tab, ...(active === t.id ? s.tabActive : {}) }}
            >
              {t.label}
            </button>
          ))}
        </div>
      </section>

      <div style={s.canvas}>
        {active === 'core'       && <CoreDiagram />}
        {active === 'support'    && <SupportDiagram />}
        {active === 'experiment' && <ExperimentDiagram />}
      </div>
    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  header:   { marginBottom: 16 },
  heading:  { fontSize: '0.9rem', marginBottom: 10, color: '#555' },
  tabs:     { display: 'flex', gap: 4 },
  tab:      {
    fontFamily: 'monospace', fontSize: '0.78rem',
    padding: '4px 12px', border: '1px solid #ccc',
    borderRadius: 3, cursor: 'pointer', background: '#fff', color: '#555',
  },
  tabActive: {
    background: '#1a1a2e', color: '#fff', borderColor: '#1a1a2e',
  },
  canvas:   {
    background: '#fafafa', border: '1px solid #eee',
    borderRadius: 6, padding: 16, overflowX: 'auto',
  },
}
