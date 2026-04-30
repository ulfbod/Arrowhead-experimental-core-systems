import { useState } from 'react'
import { ConfigProvider }     from './config/context'
import { CoreSystemsView }    from './views/CoreSystemsView'
import { SupportSystemsView } from './views/SupportSystemsView'
import { ExperimentView }     from './views/ExperimentView'
import { ConfigView }         from './views/ConfigView'
import { DiagramsView }       from './views/DiagramsView'

type Tab = 'core' | 'support' | 'experiment' | 'diagrams' | 'config'

const TABS: { id: Tab; label: string }[] = [
  { id: 'core',       label: 'Core Systems' },
  { id: 'support',    label: 'Support Systems' },
  { id: 'experiment', label: 'Experiment 2' },
  { id: 'diagrams',   label: 'Diagrams' },
  { id: 'config',     label: 'Config' },
]

function NavBar({ active, onSelect }: { active: Tab; onSelect: (t: Tab) => void }) {
  return (
    <nav style={s.nav}>
      <span style={s.brand}>Arrowhead</span>
      <div style={s.tabs}>
        {TABS.map(tab => (
          <button
            key={tab.id}
            onClick={() => onSelect(tab.id)}
            aria-current={active === tab.id ? 'page' : undefined}
            style={{ ...s.tab, ...(active === tab.id ? s.tabActive : {}) }}
          >
            {tab.label}
          </button>
        ))}
      </div>
    </nav>
  )
}

function AppContent() {
  const [tab, setTab] = useState<Tab>('experiment')

  return (
    <div style={s.root}>
      <NavBar active={tab} onSelect={setTab} />
      <main style={s.main}>
        {tab === 'core'       && <CoreSystemsView />}
        {tab === 'support'    && <SupportSystemsView />}
        {tab === 'experiment' && <ExperimentView />}
        {tab === 'diagrams'   && <DiagramsView />}
        {tab === 'config'     && <ConfigView />}
      </main>
    </div>
  )
}

export default function App() {
  return (
    <ConfigProvider>
      <AppContent />
    </ConfigProvider>
  )
}

const s = {
  root:      { minHeight: '100vh', fontFamily: 'monospace' },
  nav:       {
    display: 'flex', alignItems: 'center', gap: 24,
    background: '#1a1a2e', color: '#fff',
    padding: '0 24px', height: 48,
  },
  brand:     { fontWeight: 'bold' as const, fontSize: '0.9rem', color: '#a0c4ff', flexShrink: 0 },
  tabs:      { display: 'flex', gap: 4 },
  tab:       {
    background: 'transparent', border: 'none',
    color: '#ccc', cursor: 'pointer', padding: '4px 12px',
    borderRadius: 4, fontSize: '0.8rem', fontFamily: 'monospace',
  },
  tabActive: { background: '#2d2d50', color: '#fff', fontWeight: 'bold' as const },
  main:      { maxWidth: 1200, margin: '0 auto', padding: '24px 24px' },
}
