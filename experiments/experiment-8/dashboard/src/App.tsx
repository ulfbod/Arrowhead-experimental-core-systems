import { useState } from 'react'
import { ConfigProvider } from './config/context'
import { HealthView }        from './views/HealthView'
import { GrantsView }        from './views/GrantsView'
import { PolicyView }        from './views/PolicyView'
import { LiveDataView }      from './views/LiveDataView'
import { KafkaView }         from './views/KafkaView'
import { PKIRestView }       from './views/PKIRestView'
import { PKISecurityView }   from './views/PKISecurityView'
import { PKIAddedValueView } from './views/PKIAddedValueView'
import { ConfigView }        from './views/ConfigView'

type Tab = 'health' | 'grants' | 'policy' | 'live' | 'kafka' | 'pki-rest' | 'pki-security' | 'pki-added-value' | 'config'

const TABS: { id: Tab; label: string }[] = [
  { id: 'health',          label: 'Health' },
  { id: 'grants',          label: 'Grants' },
  { id: 'policy',          label: 'Policy Projection' },
  { id: 'live',            label: 'Live Data' },
  { id: 'kafka',           label: 'Kafka' },
  { id: 'pki-rest',        label: 'mTLS / REST' },
  { id: 'pki-security',    label: '🔒 PKI Security' },
  { id: 'pki-added-value', label: '🔑 PKI Added Value' },
  { id: 'config',          label: 'Config' },
]

function NavBar({ active, onSelect }: { active: Tab; onSelect: (t: Tab) => void }) {
  return (
    <nav style={s.nav}>
      <span style={s.brand}>Arrowhead</span>
      <span style={s.subtitle}>Experiment 8 — Arrowhead 5.2 Profile-Based PKI</span>
      <div style={s.tabs}>
        {TABS.map(tab => (
          <button
            key={tab.id}
            onClick={() => onSelect(tab.id)}
            aria-current={active === tab.id ? 'page' : undefined}
            style={{
              ...s.tab,
              ...(active === tab.id ? s.tabActive : {}),
              ...(tab.id === 'pki-security' ? s.tabSecurity : {}),
              ...(active === tab.id && tab.id === 'pki-security' ? s.tabSecurityActive : {}),
              ...(tab.id === 'pki-added-value' ? s.tabAddedValue : {}),
              ...(active === tab.id && tab.id === 'pki-added-value' ? s.tabAddedValueActive : {}),
            }}
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
        {tab === 'health'          && <HealthView />}
        {tab === 'grants'          && <GrantsView />}
        {tab === 'policy'          && <PolicyView />}
        {tab === 'live'            && <LiveDataView />}
        {tab === 'kafka'           && <KafkaView />}
        {tab === 'pki-rest'        && <PKIRestView />}
        {tab === 'pki-security'    && <PKISecurityView />}
        {tab === 'pki-added-value' && <PKIAddedValueView />}
        {tab === 'config'          && <ConfigView />}
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
  root:                { minHeight: '100vh', fontFamily: 'monospace' },
  nav:                 {
    display: 'flex', alignItems: 'center', gap: 16,
    background: '#1a1a2e', color: '#fff',
    padding: '0 24px', height: 52, flexWrap: 'wrap',
  },
  brand:               { fontWeight: 'bold', fontSize: '0.9rem', color: '#a0c4ff', flexShrink: 0 },
  subtitle:            { fontSize: '0.75rem', color: '#8899aa', flexShrink: 0 },
  tabs:                { display: 'flex', gap: 4, marginLeft: 8, flexWrap: 'wrap' },
  tab:                 {
    background: 'transparent', border: 'none',
    color: '#ccc', cursor: 'pointer', padding: '4px 12px',
    borderRadius: 4, fontSize: '0.8rem', fontFamily: 'monospace',
  },
  tabActive:           { background: '#2d2d50', color: '#fff', fontWeight: 'bold' },
  tabSecurity:         { color: '#fbbf24' },
  tabSecurityActive:   { background: '#2d2d10', color: '#fbbf24', fontWeight: 'bold' },
  tabAddedValue:       { color: '#6ee7b7' },
  tabAddedValueActive: { background: '#0d2d20', color: '#6ee7b7', fontWeight: 'bold' },
  main:                { maxWidth: 1200, margin: '0 auto', padding: '24px' },
}
