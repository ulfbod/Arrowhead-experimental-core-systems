import type { ServiceInstance } from '../types'

interface Props {
  service: ServiceInstance
  onClose: () => void
}

export default function ServiceDetail({ service: svc, onClose }: Props) {
  return (
    <aside style={s.panel}>
      <div style={s.header}>
        <h3 style={s.title}>Detail</h3>
        <button style={s.closeBtn} onClick={onClose} aria-label="Close">✕</button>
      </div>

      <Field label="id"                value={String(svc.id)} />
      <Field label="serviceDefinition" value={svc.serviceDefinition} />
      <Field label="serviceUri"        value={svc.serviceUri} />
      <Field label="version"           value={String(svc.version)} />
      {svc.secure && <Field label="secure" value={svc.secure} />}

      <h4 style={s.subheading}>Provider System</h4>
      <Field label="systemName" value={svc.providerSystem.systemName} />
      <Field label="address"    value={svc.providerSystem.address} />
      <Field label="port"       value={String(svc.providerSystem.port)} />
      {svc.providerSystem.authenticationInfo && (
        <Field label="authenticationInfo" value={svc.providerSystem.authenticationInfo} />
      )}

      <h4 style={s.subheading}>Interfaces</h4>
      <ul style={s.list}>
        {svc.interfaces.map(iface => <li key={iface}>{iface}</li>)}
      </ul>

      {svc.metadata && Object.keys(svc.metadata).length > 0 && (
        <>
          <h4 style={s.subheading}>Metadata</h4>
          {Object.entries(svc.metadata).map(([k, v]) => (
            <Field key={k} label={k} value={v} />
          ))}
        </>
      )}
    </aside>
  )
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ marginBottom: 6 }}>
      <div style={{ fontSize: '0.72rem', color: '#888' }}>{label}</div>
      <div style={{ fontSize: '0.85rem', fontFamily: 'monospace', wordBreak: 'break-all' }}>{value}</div>
    </div>
  )
}

const s = {
  panel:      { width: 280, flexShrink: 0, borderLeft: '1px solid #ddd', paddingLeft: 20, marginLeft: 20 },
  header:     { display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 14 },
  title:      { margin: 0, fontSize: '0.95rem' },
  closeBtn:   { background: 'none', border: 'none', cursor: 'pointer', fontSize: '1rem', padding: 0, lineHeight: 1 },
  subheading: { fontSize: '0.72rem', textTransform: 'uppercase' as const, color: '#888', margin: '14px 0 6px', letterSpacing: '0.06em' },
  list:       { margin: 0, padding: '0 0 0 14px', fontSize: '0.85rem' },
}
