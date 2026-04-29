import { useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'
import { registerService as register } from '../api'

interface Props {
  onRegistered: () => void
}

interface Fields {
  serviceDefinition: string
  systemName: string
  address: string
  port: string
  serviceUri: string
  interfaces: string
  version: string
  metadata: string
  secure: string
}

const EMPTY: Fields = {
  serviceDefinition: '',
  systemName: '',
  address: '',
  port: '',
  serviceUri: '',
  interfaces: '',
  version: '1',
  metadata: '',
  secure: '',
}

function parseMetadata(raw: string): Record<string, string> | undefined {
  const trimmed = raw.trim()
  if (!trimmed) return undefined
  return Object.fromEntries(
    trimmed.split(',').map(pair => {
      const eq = pair.indexOf('=')
      return eq === -1
        ? [pair.trim(), '']
        : [pair.slice(0, eq).trim(), pair.slice(eq + 1).trim()]
    })
  )
}

export default function RegisterForm({ onRegistered }: Props) {
  const [open, setOpen]     = useState(false)
  const [fields, setFields] = useState<Fields>(EMPTY)
  const [status, setStatus] = useState<{ ok: boolean; msg: string } | null>(null)
  const [busy, setBusy]     = useState(false)

  function set(key: keyof Fields) {
    return (e: ChangeEvent<HTMLInputElement>) =>
      setFields(f => ({ ...f, [key]: e.target.value }))
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setStatus(null)
    try {
      const ifaces = fields.interfaces.split(',').map(s => s.trim()).filter(Boolean)
      const meta   = parseMetadata(fields.metadata)
      const body = {
        serviceDefinition: fields.serviceDefinition.trim(),
        providerSystem: {
          systemName: fields.systemName.trim(),
          address:    fields.address.trim(),
          port:       parseInt(fields.port, 10),
        },
        serviceUri:  fields.serviceUri.trim(),
        interfaces:  ifaces,
        version:     parseInt(fields.version, 10) || 1,
        ...(meta                  ? { metadata: meta }              : {}),
        ...(fields.secure.trim()  ? { secure: fields.secure.trim() } : {}),
      }
      const result = await register(body)
      setStatus({ ok: true, msg: `Registered — id=${result.id}` })
      setFields(EMPTY)
      onRegistered()
    } catch (err) {
      setStatus({ ok: false, msg: err instanceof Error ? err.message : String(err) })
    } finally {
      setBusy(false)
    }
  }

  return (
    <section style={s.section}>
      <button style={s.toggle} onClick={() => setOpen(o => !o)}>
        {open ? '▾' : '▸'} Register Service
      </button>

      {open && (
        <form onSubmit={submit} style={s.form}>
          <div style={s.grid}>
            <Field label="serviceDefinition *" value={fields.serviceDefinition} onChange={set('serviceDefinition')} placeholder="temperature-service" />
            <Field label="serviceUri *"         value={fields.serviceUri}        onChange={set('serviceUri')}        placeholder="/temperature" />
            <Field label="systemName *"         value={fields.systemName}        onChange={set('systemName')}        placeholder="sensor-1" />
            <Field label="address *"            value={fields.address}           onChange={set('address')}           placeholder="192.168.0.10" />
            <Field label="port *"               value={fields.port}              onChange={set('port')}              placeholder="9001"      type="number" />
            <Field label="version"              value={fields.version}           onChange={set('version')}           placeholder="1"         type="number" />
            <Field label="interfaces * (comma-separated)" value={fields.interfaces} onChange={set('interfaces')} placeholder="HTTP-INSECURE-JSON" span />
            <Field label="metadata (key=value, …)"        value={fields.metadata}   onChange={set('metadata')}   placeholder="region=eu, unit=celsius" span />
            <Field label="secure"               value={fields.secure}            onChange={set('secure')}            placeholder="NOT_SECURE" />
          </div>
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <button style={s.btn} type="submit" disabled={busy}>
              {busy ? 'Registering…' : 'Register'}
            </button>
            {status && (
              <span style={{ marginLeft: 12, fontSize: '0.85rem', color: status.ok ? '#080' : '#c00' }}>
                {status.msg}
              </span>
            )}
          </div>
        </form>
      )}
    </section>
  )
}

function Field({
  label, value, onChange, placeholder, type = 'text', span,
}: {
  label: string
  value: string
  onChange: (e: ChangeEvent<HTMLInputElement>) => void
  placeholder: string
  type?: string
  span?: boolean
}) {
  return (
    <label style={{ display: 'flex', flexDirection: 'column', fontSize: '0.78rem', gap: 3, ...(span ? { gridColumn: '1 / -1' } : {}) }}>
      {label}
      <input
        style={s.input}
        type={type}
        value={value}
        onChange={onChange}
        placeholder={placeholder}
      />
    </label>
  )
}

const s = {
  section: { marginBottom: 20 },
  toggle:  { background: 'none', border: 'none', cursor: 'pointer', fontFamily: 'monospace', fontSize: '0.9rem', padding: '4px 0', fontWeight: 600 },
  form:    { marginTop: 10, padding: '14px 16px', border: '1px solid #ddd', background: '#fafafa' },
  grid:    { display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px 20px', marginBottom: 12 },
  input:   { fontFamily: 'monospace', fontSize: '0.85rem', padding: '4px 6px', border: '1px solid #bbb', width: '100%', boxSizing: 'border-box' as const },
  btn:     { padding: '5px 18px', fontFamily: 'monospace', fontSize: '0.85rem', cursor: 'pointer', background: '#111', color: '#fff', border: 'none' },
}
