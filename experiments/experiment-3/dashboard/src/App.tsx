import { useState } from 'react'
import { ConfigProvider } from './config/context'
import { HealthView }     from './views/HealthView'
import { GrantsView }     from './views/GrantsView'
import { LiveDataView }   from './views/LiveDataView'
import { ConfigView }     from './views/ConfigView'

type Tab = 'health' | 'grants' | 'live' | 'config'

const TABS: { id: Tab; label: string }[] = [
  { id: 'health', label: 'Health' },
  { id: 'grants', label: 'Grants & Sync' },
  { id: 'live',   label: 'Live Data' },
  { id: 'config', label: 'Config' },
]

function NavBar({ active, onSelect }: { active: Tab; onSelect: (t: Tab) => void }) {
  return (
    <nav style={s.nav}>
      <span style={s.brand}>Arrowhead</span>
      <span style={s.subtitle}>Experiment 3 — Topic Auth</span>
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
  const [tab, setTab] = useState<Tab>('health')

  return (
    <div style={s.root}>
      <NavBar active={tab} onSelect={setTab} />
      <main style={s.main}>
        {tab === 'health' && <HealthView />}
        {tab === 'grants' && <GrantsView />}
        {tab === 'live'   && <LiveDataView />}
        {tab === 'config' && <ConfigView />}
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

const s: Record<string, React.CSSProperties> = {
  root:     { minHeight: '100vh', fontFamily: 'monospace' },
  nav:      {
    display: 'flex', alignItems: 'center', gap: 16,
    background: '#1a1a2e', color: '#fff',
    padding: '0 24px', height: 48,
  },
  brand:    { fontWeight: 'bold', fontSize: '0.9rem', color: '#a0c4ff', flexShrink: 0 },
  subtitle: { fontSize: '0.75rem', color: '#8899aa', flexShrink: 0 },
  tabs:     { display: 'flex', gap: 4, marginLeft: 8 },
  tab:      {
    background: 'transparent', border: 'none',
    color: '#ccc', cursor: 'pointer', padding: '4px 12px',
    borderRadius: 4, fontSize: '0.8rem', fontFamily: 'monospace',
  },
  tabActive: { background: '#2d2d50', color: '#fff', fontWeight: 'bold' },
  main:      { maxWidth: 1100, margin: '0 auto', padding: '24px' },
}
