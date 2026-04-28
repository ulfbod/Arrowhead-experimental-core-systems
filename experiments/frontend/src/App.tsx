import { useEffect, useState } from 'react'

// ── Types matching SPEC.md ────────────────────────────────────────────────────

interface System {
  systemName: string
  address: string
  port: number
  authenticationInfo?: string
}

interface ServiceInstance {
  id: number
  serviceDefinition: string
  providerSystem: System
  serviceUri: string
  interfaces: string[]
  version: number
  metadata?: Record<string, string>
  secure?: string
}

interface QueryResponse {
  serviceQueryData: ServiceInstance[]
  unfilteredHits: number
}

// ── API ───────────────────────────────────────────────────────────────────────

// Vite proxies /serviceregistry/* → http://localhost:8080 (see vite.config.ts)

async function apiQuery(body: object): Promise<QueryResponse> {
  const resp = await fetch('/serviceregistry/query', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const data = await resp.json()
  if (!resp.ok) throw new Error(data.error ?? `HTTP ${resp.status}`)
  return data as QueryResponse
}

async function apiRegister(body: object): Promise<ServiceInstance> {
  const resp = await fetch('/serviceregistry/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const data = await resp.json()
  if (!resp.ok) throw new Error(data.error ?? `HTTP ${resp.status}`)
  return data as ServiceInstance
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// Parses "key=value, key2=value2" into { key: "value", key2: "value2" }.
// Returns undefined if the string is empty.
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

function splitInterfaces(raw: string): string[] {
  return raw.split(',').map(s => s.trim()).filter(Boolean)
}

// ── Register form ─────────────────────────────────────────────────────────────

interface RegFields {
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

const REG_EMPTY: RegFields = {
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

function RegisterForm({ onRegistered }: { onRegistered: () => void }) {
  const [fields, setFields] = useState<RegFields>(REG_EMPTY)
  const [status, setStatus] = useState<{ ok: boolean; msg: string } | null>(null)
  const [busy, setBusy] = useState(false)

  function set(key: keyof RegFields) {
    return (e: React.ChangeEvent<HTMLInputElement>) =>
      setFields(f => ({ ...f, [key]: e.target.value }))
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setStatus(null)
    try {
      const body = {
        serviceDefinition: fields.serviceDefinition.trim(),
        providerSystem: {
          systemName: fields.systemName.trim(),
          address: fields.address.trim(),
          port: parseInt(fields.port, 10),
        },
        serviceUri: fields.serviceUri.trim(),
        interfaces: splitInterfaces(fields.interfaces),
        version: parseInt(fields.version, 10) || 1,
        ...(parseMetadata(fields.metadata) ? { metadata: parseMetadata(fields.metadata) } : {}),
        ...(fields.secure.trim() ? { secure: fields.secure.trim() } : {}),
      }
      const result = await apiRegister(body)
      setStatus({ ok: true, msg: `Registered — id=${result.id}` })
      onRegistered()
    } catch (err) {
      setStatus({ ok: false, msg: (err as Error).message })
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <div style={s.grid}>
        <Field label="serviceDefinition *" value={fields.serviceDefinition} onChange={set('serviceDefinition')} placeholder="temperature-service" />
        <Field label="serviceUri *"         value={fields.serviceUri}        onChange={set('serviceUri')}        placeholder="/temperature" />
        <Field label="systemName *"         value={fields.systemName}        onChange={set('systemName')}        placeholder="sensor-1" />
        <Field label="address *"            value={fields.address}           onChange={set('address')}           placeholder="192.168.0.10" />
        <Field label="port *"               value={fields.port}              onChange={set('port')}              placeholder="9001" type="number" />
        <Field label="version"              value={fields.version}           onChange={set('version')}           placeholder="1" type="number" />
        <Field label="interfaces * (comma-separated)" value={fields.interfaces} onChange={set('interfaces')} placeholder="HTTP-SECURE-JSON" span />
        <Field label="metadata (key=value, …)"        value={fields.metadata}   onChange={set('metadata')}    placeholder='region=eu, unit=celsius' span />
        <Field label="secure"               value={fields.secure}            onChange={set('secure')}            placeholder="NOT_SECURE" />
      </div>
      <button style={s.btn} type="submit" disabled={busy}>
        {busy ? 'Registering…' : 'Register'}
      </button>
      {status && (
        <span style={{ ...s.statusMsg, color: status.ok ? '#080' : '#c00' }}>
          {status.msg}
        </span>
      )}
    </form>
  )
}

// ── Query form ────────────────────────────────────────────────────────────────

interface QueryFields {
  serviceDefinition: string
  interfaces: string
  metadata: string
  versionRequirement: string
}

const QUERY_EMPTY: QueryFields = {
  serviceDefinition: '',
  interfaces: '',
  metadata: '',
  versionRequirement: '0',
}

function QueryForm({ onResults }: { onResults: (r: QueryResponse) => void }) {
  const [fields, setFields] = useState<QueryFields>(QUERY_EMPTY)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function set(key: keyof QueryFields) {
    return (e: React.ChangeEvent<HTMLInputElement>) =>
      setFields(f => ({ ...f, [key]: e.target.value }))
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const ifaces = splitInterfaces(fields.interfaces)
      const meta = parseMetadata(fields.metadata)
      const ver = parseInt(fields.versionRequirement, 10) || 0
      const body = {
        ...(fields.serviceDefinition.trim() ? { serviceDefinition: fields.serviceDefinition.trim() } : {}),
        ...(ifaces.length ? { interfaces: ifaces } : {}),
        ...(meta ? { metadata: meta } : {}),
        ...(ver > 0 ? { versionRequirement: ver } : {}),
      }
      const result = await apiQuery(body)
      onResults(result)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <div style={s.grid}>
        <Field label="serviceDefinition (blank = all)" value={fields.serviceDefinition} onChange={set('serviceDefinition')} placeholder="" />
        <Field label="versionRequirement (0 = any)"    value={fields.versionRequirement} onChange={set('versionRequirement')} placeholder="0" type="number" />
        <Field label="interfaces (comma-separated)"    value={fields.interfaces}         onChange={set('interfaces')}         placeholder="" span />
        <Field label="metadata filter (key=value, …)"  value={fields.metadata}           onChange={set('metadata')}           placeholder='region=eu' span />
      </div>
      <button style={s.btn} type="submit" disabled={busy}>
        {busy ? 'Querying…' : 'Query'}
      </button>
      {error && <span style={{ ...s.statusMsg, color: '#c00' }}>{error}</span>}
    </form>
  )
}

// ── Results table ─────────────────────────────────────────────────────────────

function ResultsTable({ resp }: { resp: QueryResponse | null }) {
  if (!resp) return null

  if (resp.serviceQueryData.length === 0) {
    return <p style={s.muted}>No services found (unfilteredHits: {resp.unfilteredHits})</p>
  }

  return (
    <>
      <p style={s.info}>
        {resp.serviceQueryData.length} result(s) &mdash; unfilteredHits: {resp.unfilteredHits}
      </p>
      <table style={s.table}>
        <thead>
          <tr>
            {['serviceDefinition', 'systemName', 'address', 'port', 'interfaces', 'metadata'].map(h => (
              <th key={h} style={s.th}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {resp.serviceQueryData.map(svc => (
            <tr key={svc.id}>
              <td style={s.td}>{svc.serviceDefinition}</td>
              <td style={s.td}>{svc.providerSystem.systemName}</td>
              <td style={s.td}>{svc.providerSystem.address}</td>
              <td style={s.td}>{svc.providerSystem.port}</td>
              <td style={s.td}>{svc.interfaces.join(', ')}</td>
              <td style={s.td}>
                {svc.metadata
                  ? Object.entries(svc.metadata).map(([k, v]) => `${k}=${v}`).join(', ')
                  : ''}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}

// ── Instructions ─────────────────────────────────────────────────────────────

function Instructions() {
  return (
    <details style={s.details}>
      <summary style={s.summary}>How to use</summary>
      <div style={s.instrBody}>
        <p style={s.instrPara}>
          This frontend talks to the <strong>Arrowhead Service Registry</strong> running
          at <code>localhost:8080</code>. Use it to register services and query what
          is currently registered.
        </p>

        <h3 style={s.instrH}>Register Service</h3>
        <p style={s.instrPara}>
          Fill in the form and click <strong>Register</strong>. Required fields are
          marked with <code>*</code>. After a successful registration the Results table
          refreshes automatically.
        </p>
        <ul style={s.instrList}>
          <li><code>serviceDefinition</code> — logical name of the service (e.g. <code>temperature-service</code>)</li>
          <li><code>serviceUri</code> — path the service is reachable at (e.g. <code>/temperature</code>)</li>
          <li><code>systemName / address / port</code> — identifies the host providing the service</li>
          <li><code>interfaces</code> — comma-separated list, e.g. <code>HTTP-SECURE-JSON, HTTP-INSECURE-JSON</code></li>
          <li><code>version</code> — integer, defaults to 1. Registering the same service + system + version again <em>updates</em> the existing entry</li>
          <li><code>metadata</code> — optional key=value pairs, e.g. <code>region=eu, unit=celsius</code></li>
          <li><code>secure</code> — optional security mode, e.g. <code>NOT_SECURE</code> or <code>CERTIFICATE</code></li>
        </ul>

        <h3 style={s.instrH}>Query Services</h3>
        <p style={s.instrPara}>
          All filter fields are optional. Leave a field blank to skip that filter.
          Filters are ANDed — only services matching <em>all</em> supplied filters are returned.
          Click <strong>Query</strong> to search; click <strong>Refresh</strong> in the
          Results section to reload with no filters.
        </p>
        <ul style={s.instrList}>
          <li><code>serviceDefinition</code> — exact match; blank returns all</li>
          <li><code>interfaces</code> — comma-separated; service must provide <em>all</em> listed interfaces (case-insensitive)</li>
          <li><code>metadata filter</code> — key=value pairs; service must contain <em>all</em> listed pairs</li>
          <li><code>versionRequirement</code> — exact version match; 0 means no version filter</li>
        </ul>

        <h3 style={s.instrH}>Results</h3>
        <p style={s.instrPara}>
          Shows the response from the last query. <code>unfilteredHits</code> is the
          total number of registered services before filters were applied.
          Use <strong>Refresh</strong> to re-run an unfiltered query at any time.
        </p>
      </div>
    </details>
  )
}

// ── App ───────────────────────────────────────────────────────────────────────

export default function App() {
  const [queryResp, setQueryResp] = useState<QueryResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)

  function loadAll() {
    setLoading(true)
    setLoadError(null)
    apiQuery({})
      .then(setQueryResp)
      .catch(e => setLoadError((e as Error).message))
      .finally(() => setLoading(false))
  }

  // Load all services on mount
  useEffect(loadAll, [])

  return (
    <div style={s.page}>
      <h1 style={s.heading}>Arrowhead Service Registry</h1>

      <Instructions />

      <Section title="Register Service">
        <RegisterForm onRegistered={loadAll} />
      </Section>

      <Section title="Query Services">
        <QueryForm onResults={setQueryResp} />
      </Section>

      <Section
        title="Results"
        action={<button style={s.btnSmall} onClick={loadAll}>Refresh</button>}
      >
        {loading  && <p style={s.muted}>Loading…</p>}
        {loadError && <p style={{ ...s.muted, color: '#c00' }}>Error: {loadError}</p>}
        {!loading && !loadError && <ResultsTable resp={queryResp} />}
      </Section>
    </div>
  )
}

// ── Shared components ─────────────────────────────────────────────────────────

function Section({
  title,
  action,
  children,
}: {
  title: string
  action?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section style={s.section}>
      <div style={s.sectionHeader}>
        <h2 style={s.sectionTitle}>{title}</h2>
        {action}
      </div>
      {children}
    </section>
  )
}

function Field({
  label,
  value,
  onChange,
  placeholder,
  type = 'text',
  span,
}: {
  label: string
  value: string
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  placeholder: string
  type?: string
  span?: boolean
}) {
  return (
    <label style={{ ...s.fieldLabel, ...(span ? s.span : {}) }}>
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

// ── Styles ────────────────────────────────────────────────────────────────────

const s = {
  page:          { fontFamily: 'monospace', maxWidth: 900, margin: '40px auto', padding: '0 20px' },
  heading:       { fontSize: '1.1rem', borderBottom: '2px solid #111', paddingBottom: 6, marginBottom: '1.4rem' },
  section:       { marginBottom: '2rem' },
  sectionHeader: { display: 'flex', alignItems: 'baseline', gap: 12, marginBottom: 10 },
  sectionTitle:  { fontSize: '1rem', margin: 0 },
  grid:          { display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px 20px', marginBottom: 10 },
  span:          { gridColumn: '1 / -1' },
  fieldLabel:    { display: 'flex', flexDirection: 'column' as const, fontSize: '0.8rem', gap: 3 },
  input:         { fontFamily: 'monospace', fontSize: '0.85rem', padding: '4px 6px', border: '1px solid #aaa', width: '100%', boxSizing: 'border-box' as const },
  btn:           { padding: '5px 18px', fontFamily: 'monospace', cursor: 'pointer', background: '#111', color: '#fff', border: 'none' },
  btnSmall:      { padding: '2px 12px', fontFamily: 'monospace', fontSize: '0.8rem', cursor: 'pointer', background: '#111', color: '#fff', border: 'none' },
  statusMsg:     { marginLeft: 12, fontSize: '0.85rem' },
  muted:         { color: '#888', fontSize: '0.9rem' },
  info:          { color: '#080', fontSize: '0.85rem', margin: '0 0 6px' },
  table:         { width: '100%', borderCollapse: 'collapse' as const, fontSize: '0.85rem' },
  th:            { textAlign: 'left' as const, borderBottom: '2px solid #111', padding: '4px 8px', whiteSpace: 'nowrap' as const },
  td:            { padding: '5px 8px', borderBottom: '1px solid #ddd', verticalAlign: 'top' as const },
}
