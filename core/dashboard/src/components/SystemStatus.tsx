import { useEffect, useState } from 'react'
import type { SystemStatus } from '../types'
import { checkHealth } from '../api'

interface SystemEntry extends SystemStatus {
  port: string
}

const SYSTEMS: SystemEntry[] = [
  { name: 'ServiceRegistry',            url: '',                             port: '8080', healthy: null },
  { name: 'Authentication',             url: '/authentication',              port: '8081', healthy: null },
  { name: 'ConsumerAuthorization',      url: '/authorization',               port: '8082', healthy: null },
  { name: 'DynamicOrchestration',       url: '/orchestration/dynamic',       port: '8083', healthy: null },
  { name: 'SimpleStoreOrchestration',   url: '/orchestration/simplestore',   port: '8084', healthy: null },
  { name: 'FlexibleStoreOrchestration', url: '/orchestration/flexiblestore', port: '8085', healthy: null },
]

export default function SystemStatus() {
  const [statuses, setStatuses] = useState<SystemEntry[]>(SYSTEMS)

  useEffect(() => {
    SYSTEMS.forEach((sys, i) => {
      checkHealth(sys.url)
        .then(() => {
          setStatuses(prev => prev.map((s, j) => j === i ? { ...s, healthy: true } : s))
        })
        .catch(() => {
          setStatuses(prev => prev.map((s, j) => j === i ? { ...s, healthy: false } : s))
        })
    })
  }, [])

  return (
    <div style={s.grid}>
      {statuses.map(sys => (
        <div key={sys.name} style={{ ...s.card, borderColor: dotColor(sys.healthy) }}>
          <span style={{ ...s.dot, background: dotColor(sys.healthy) }} />
          <span style={s.name}>{sys.name}</span>
          <span style={s.port}>{sys.port}</span>
        </div>
      ))}
    </div>
  )
}

function dotColor(healthy: boolean | null): string {
  if (healthy === null) return '#bbb'
  return healthy ? '#2a2' : '#c33'
}

const s = {
  grid: { display: 'flex', flexWrap: 'wrap' as const, gap: 8, marginBottom: 20 },
  card: { display: 'flex', alignItems: 'center', gap: 6, padding: '5px 10px', border: '1px solid #bbb', fontSize: '0.8rem', borderRadius: 3 },
  dot:  { width: 8, height: 8, borderRadius: '50%', flexShrink: 0 },
  name: { fontWeight: 600 },
  port: { color: '#888', fontSize: '0.75rem' },
}
