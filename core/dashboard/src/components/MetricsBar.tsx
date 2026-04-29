import type { ServiceInstance } from '../types'

interface Props {
  services: ServiceInstance[]
}

export default function MetricsBar({ services }: Props) {
  const totalServices = services.length
  const uniqueSystems = new Set(
    services.map(s =>
      `${s.providerSystem.systemName}@${s.providerSystem.address}:${s.providerSystem.port}`
    )
  ).size

  return (
    <div style={{ display: 'flex', gap: 8, alignItems: 'center', padding: '8px 0', marginBottom: 14, borderBottom: '1px solid #eee', fontSize: '0.9rem' }}>
      <span><strong>{totalServices}</strong> service{totalServices !== 1 ? 's' : ''}</span>
      <span style={{ color: '#bbb' }}>·</span>
      <span><strong>{uniqueSystems}</strong> system{uniqueSystems !== 1 ? 's' : ''}</span>
    </div>
  )
}
