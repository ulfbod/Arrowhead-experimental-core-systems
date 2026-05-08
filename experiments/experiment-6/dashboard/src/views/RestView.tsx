// RestView — REST/HTTP transport from the Arrowhead core systems perspective.
//
// Shows how a REST service fits into the Arrowhead framework lifecycle:
//   1. Service Registration (ServiceRegistry)   — data-provider publishes telemetry-rest
//   2. Authorization (ConsumerAuthorization)    — grants control who may call it
//   3. Discovery (DynamicOrchestration)         — consumers find the provider via Orch
//   4. Enforcement (rest-authz PEP)             — every request checked against AuthzForce
//   5. Live Demo                                — interactive request through the PEP

import { useState, useCallback } from 'react'
import { usePolling } from '../hooks/usePolling'
import {
  fetchRestAuthzStatus,
  fetchDataProviderStats,
  fetchRestConsumerStats,
  fetchAuthRules,
  queryServiceRegistry,
  fetchThroughRestAuthz,
} from '../api'
import type {
  RestAuthzStatus,
  DataProviderStats,
  RestConsumerStats,
  LookupResponse,
  ServiceQueryResponse,
} from '../types'

const REST_SERVICE = 'telemetry-rest'

// Well-known consumers for the demo dropdown.
const KNOWN_CONSUMERS = [
  'rest-consumer',
  'test-probe',
  'demo-consumer-1',   // not granted for REST — demonstrates Deny
  'unknown-system',
]

function formatTime(iso: string): string {
  if (!iso) return '—'
  try { return new Date(iso).toLocaleTimeString() } catch { return iso }
}

// ── Section 1: Architecture overview ─────────────────────────────────────────

function ArchitectureDiagram() {
  return (
    <pre style={s.diagram}>{`
  Arrowhead REST/HTTP lifecycle
  ══════════════════════════════════════════════════════

  ① Registration
      data-provider  ──POST /serviceregistry/register──►  ServiceRegistry
                         service: telemetry-rest
                         provider: data-provider:9094
                         uri: /telemetry/latest
                         interface: HTTP-INSECURE-JSON

  ② Authorization grant (operator action)
      ConsumerAuth admin  ──POST /authorization/grant──►  ConsumerAuthorization
                              consumer: rest-consumer
                              provider: data-provider
                              service:  telemetry-rest

                          CA grants ──► policy-sync ──► AuthzForce PDP
                          (every SYNC_INTERVAL)

  ③ Discovery
      rest-consumer  ──POST /orchestration/orchestrate──►  DynamicOrchestration
                                                               │ queries SR + CA
                                                               ▼
                                                     returns: http://rest-authz:9093
                                                     (PEP address, not data-provider directly)

  ④ Enforcement
      rest-consumer  ──GET /telemetry/latest──►  rest-authz (PEP)
                         X-Consumer-Name: rest-consumer
                                                     │
                                               POST AuthzForce /pdp
                                               XACML: consumer=rest-consumer
                                                      service=telemetry-rest
                                                     │
                                              Permit ──► proxy to data-provider
                                              Deny   ──► 403 Forbidden

  Sync-delay caveat:
      A revoked grant stays Permit until policy-sync uploads the
      next PolicySet version.  Adjust SYNC_INTERVAL on Config tab.
`.trim()}</pre>
  )
}

// ── Section 2: Registered services ───────────────────────────────────────────

function RegisteredServicesCard({ data, error }: { data: ServiceQueryResponse | null; error: string | null }) {
  if (error) return <div style={s.errBox}>ServiceRegistry unavailable: {error}</div>
  if (!data)  return <div style={s.muted}>loading…</div>

  const instances = data.serviceQueryData ?? []
  if (instances.length === 0) {
    return <div style={s.muted}>No instances of <code>{REST_SERVICE}</code> registered.</div>
  }

  return (
    <div style={s.tableWrap}>
      <table style={s.table}>
        <thead>
          <tr>
            {['systemName', 'address', 'port', 'serviceUri', 'interfaces', 'version'].map(h => (
              <th key={h} style={s.th}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {instances.map(inst => (
            <tr key={inst.id}>
              <td style={s.td}>{inst.providerSystem.systemName}</td>
              <td style={s.td}>{inst.providerSystem.address}</td>
              <td style={s.td}>{inst.providerSystem.port}</td>
              <td style={s.td}>{inst.serviceUri}</td>
              <td style={s.td}>{inst.interfaces.join(', ')}</td>
              <td style={s.td}>{inst.version}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p style={s.hint}>
        {instances.length} instance{instances.length !== 1 ? 's' : ''} registered
        (unfilteredHits: {data.unfilteredHits}).
        In this experiment data-provider does not self-register — the table will be empty.
        In a full Arrowhead deployment the provider would register here and rest-consumer
        would discover it via DynamicOrchestration.
      </p>
    </div>
  )
}

// ── Section 3: Active grants for REST ────────────────────────────────────────

function RestGrantsCard({ data, error }: { data: LookupResponse | null; error: string | null }) {
  if (error) return <div style={s.errBox}>ConsumerAuth unavailable: {error}</div>
  if (!data)  return <div style={s.muted}>loading…</div>

  const restGrants = (data.rules ?? []).filter(r => r.serviceDefinition === REST_SERVICE)

  if (restGrants.length === 0) {
    return (
      <div style={s.muted}>
        No grants for <code>{REST_SERVICE}</code> found in ConsumerAuth.
        Add one on the Grants tab — allow ≤10 s for policy-sync to propagate.
      </div>
    )
  }

  return (
    <div style={s.tableWrap}>
      <table style={s.table}>
        <thead>
          <tr>
            {['id', 'consumer', 'provider', 'service'].map(h => (
              <th key={h} style={s.th}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {restGrants.map(g => (
            <tr key={g.id}>
              <td style={s.td}>{g.id}</td>
              <td style={s.td}>{g.consumerSystemName}</td>
              <td style={s.td}>{g.providerSystemName}</td>
              <td style={s.td}>{g.serviceDefinition}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p style={s.hint}>
        These grants are compiled into XACML by policy-sync every SYNC_INTERVAL
        and loaded into AuthzForce. rest-authz checks AuthzForce on every request.
      </p>
    </div>
  )
}

// ── Section 4: PEP + data stats ───────────────────────────────────────────────

function StatsRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div style={s.stat}>
      <span style={s.statLabel}>{label}</span>
      <span style={s.statValue}>{value}</span>
    </div>
  )
}

function PepCard({ status }: { status: RestAuthzStatus | null }) {
  return (
    <div style={{ ...s.card, borderColor: '#fde68a' }}>
      <div style={s.cardTitle}>rest-authz (PEP)</div>
      <div style={{ ...s.transport, color: '#92400e' }}>REST / HTTP enforcement</div>
      <StatsRow label="requests total" value={status?.requestsTotal ?? '…'} />
      <StatsRow label="permitted"      value={<span style={{ color: '#166534' }}>{status?.permitted ?? '…'}</span>} />
      <StatsRow label="denied"         value={<span style={{ color: '#991b1b' }}>{status?.denied ?? '…'}</span>} />
      <div style={{ borderTop: '1px solid #fde68a', margin: '8px 0' }} />
      <StatsRow
        label="endpoint"
        value={
          <a
            href={`http://${window.location.hostname}:9093/telemetry/latest?consumer=rest-consumer`}
            target="_blank"
            rel="noopener noreferrer"
            style={s.link}
          >
            {window.location.hostname}:9093 ↗
          </a>
        }
      />
      <p style={s.hint}>
        Opens in a new browser tab. Uses <code>?consumer=rest-consumer</code> —
        the browser-accessible equivalent of the <code>X-Consumer-Name</code> header.
        Returns 200 with telemetry data when Permit, 403 when not authorized.
      </p>
    </div>
  )
}

function DataProviderCard({ stats }: { stats: DataProviderStats | null }) {
  return (
    <div style={{ ...s.card, borderColor: '#a3e635' }}>
      <div style={s.cardTitle}>data-provider (upstream)</div>
      <div style={{ ...s.transport, color: '#365314' }}>Kafka consumer + REST server</div>
      <StatsRow label="messages from Kafka" value={stats?.msgCount ?? '…'} />
      <StatsRow label="robots tracked"      value={stats?.robotCount ?? '…'} />
      <StatsRow label="last received"       value={stats ? formatTime(stats.lastReceivedAt) : '…'} />
    </div>
  )
}

function RestConsumerCard({ stats }: { stats: RestConsumerStats | null }) {
  return (
    <div style={{ ...s.card, borderColor: '#c4b5fd' }}>
      <div style={s.cardTitle}>rest-consumer</div>
      <div style={{ ...s.transport, color: '#5b21b6' }}>REST client (polls rest-authz)</div>
      <StatsRow label="messages received" value={stats?.msgCount ?? '…'} />
      <StatsRow label="denied count"      value={stats?.deniedCount ?? '…'} />
      <StatsRow label="last received"     value={stats ? formatTime(stats.lastReceivedAt) : '…'} />
      <StatsRow label="last denied at"    value={stats?.lastDeniedAt ? formatTime(stats.lastDeniedAt) : '—'} />
    </div>
  )
}

// ── Section 5: Live authorization demo ───────────────────────────────────────

type DemoResult = { status: number; body: string } | null

function LiveDemo() {
  const [consumer, setConsumer]  = useState('rest-consumer')
  const [result,   setResult]    = useState<DemoResult>(null)
  const [fetching, setFetching]  = useState(false)
  const [err,      setErr]       = useState<string | null>(null)

  const run = useCallback(async () => {
    setFetching(true)
    setErr(null)
    setResult(null)
    try {
      const r = await fetchThroughRestAuthz(consumer.trim())
      setResult(r)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setFetching(false)
    }
  }, [consumer])

  const permitted = result !== null && result.status === 200
  const denied    = result !== null && result.status === 403

  let prettyBody = ''
  if (result?.body) {
    try { prettyBody = JSON.stringify(JSON.parse(result.body), null, 2) }
    catch { prettyBody = result.body }
  }

  return (
    <div style={s.demoWrap}>
      <p style={s.body}>
        Sends <code>GET /telemetry/latest</code> to rest-authz with the selected consumer
        identity. rest-authz queries AuthzForce and either forwards the request to
        data-provider (Permit) or returns 403 (Deny).
      </p>

      <div style={s.checkRow}>
        <label style={s.label}>Consumer</label>
        <select
          value={consumer}
          onChange={e => setConsumer(e.target.value)}
          style={s.select}
        >
          {KNOWN_CONSUMERS.map(c => <option key={c} value={c}>{c}</option>)}
        </select>
        <input
          value={consumer}
          onChange={e => setConsumer(e.target.value)}
          style={{ ...s.input, marginLeft: 8 }}
          placeholder="or type any name"
        />
      </div>

      <button onClick={run} disabled={fetching || !consumer.trim()} style={s.btn}>
        {fetching ? 'fetching…' : 'Fetch via rest-authz →'}
      </button>

      {err && <div style={s.errBox}>{err}</div>}

      {result && (
        <div style={{
          ...s.resultBox,
          borderColor: permitted ? '#86efac' : denied ? '#fca5a5' : '#ddd',
        }}>
          <div style={s.resultHeader}>
            <span style={{
              ...s.decisionBadge,
              background: permitted ? '#dcfce7' : denied ? '#fee2e2' : '#f3f4f6',
              color:      permitted ? '#166534' : denied ? '#991b1b' : '#374151',
            }}>
              {result.status} {permitted ? 'OK — Permit' : denied ? 'Forbidden — Deny' : `(HTTP ${result.status})`}
            </span>
            <span style={s.resultMeta}>
              consumer: <strong>{consumer}</strong>
              {' · '}service: <strong>{REST_SERVICE}</strong>
            </span>
          </div>

          {permitted && prettyBody && prettyBody !== 'null' && (
            <pre style={s.responseBody}>{prettyBody}</pre>
          )}
          {denied && (
            <p style={{ ...s.hint, color: '#991b1b', marginTop: 6 }}>
              No valid grant for <strong>{consumer}</strong> → <strong>{REST_SERVICE}</strong> in AuthzForce.
              Add a grant on the Grants tab and wait one SYNC_INTERVAL.
            </p>
          )}
          {permitted && (prettyBody === 'null' || !prettyBody) && (
            <p style={s.hint}>Authorized — data-provider returned null (no telemetry yet).</p>
          )}
        </div>
      )}
    </div>
  )
}

// ── Top-level view ────────────────────────────────────────────────────────────

export function RestView() {
  const { data: restStatus }   = usePolling<RestAuthzStatus>(fetchRestAuthzStatus,   3_000)
  const { data: dpStats }      = usePolling<DataProviderStats>(fetchDataProviderStats, 3_000)
  const { data: rcStats }      = usePolling<RestConsumerStats>(fetchRestConsumerStats, 3_000)
  const { data: caRules,   error: caErr }   = usePolling<LookupResponse>(fetchAuthRules, 5_000)
  const { data: srResult, error: srErr }   = usePolling<ServiceQueryResponse>(
    (sig) => queryServiceRegistry(REST_SERVICE, sig), 10_000,
  )

  return (
    <div style={s.wrap}>

      {/* ── Overview ───────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>REST / HTTP — Arrowhead Core Perspective</h2>
        <p style={s.intro}>
          This view follows a REST service through the full Arrowhead framework lifecycle:
          registration, authorization grant, orchestration-based discovery, and
          policy-enforced access. The same XACML policy that governs AMQP and Kafka also
          governs REST — enforced by the <strong>rest-authz</strong> reverse proxy.
        </p>
        <ArchitectureDiagram />
      </section>

      {/* ── Registered services ────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>① Service Registration — ServiceRegistry</h3>
        <p style={s.body}>
          Active instances of <code>{REST_SERVICE}</code> registered with the Arrowhead
          ServiceRegistry. A consumer asks DynamicOrchestration for this service definition
          and receives the rest-authz address (the PEP), not data-provider's address directly.
        </p>
        <RegisteredServicesCard data={srResult} error={srErr} />
      </section>

      {/* ── Grants ─────────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>② Authorization Grants — ConsumerAuthorization</h3>
        <p style={s.body}>
          Grants in ConsumerAuth for <code>{REST_SERVICE}</code>. policy-sync compiles
          these into XACML every SYNC_INTERVAL and loads them into AuthzForce.
          Add or revoke grants on the <strong>Grants</strong> tab.
        </p>
        <RestGrantsCard data={caRules} error={caErr} />
      </section>

      {/* ── Live stats ─────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>③ Enforcement &amp; Data Flow — Live Stats</h3>
        <div style={s.cardRow}>
          <PepCard          status={restStatus} />
          <DataProviderCard stats={dpStats}     />
          <RestConsumerCard stats={rcStats}     />
        </div>
      </section>

      {/* ── Live demo ──────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>④ Live Authorization Demo</h3>
        <LiveDemo />
      </section>

    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  wrap:         { fontFamily: 'monospace' },
  section:      { marginBottom: 32 },
  heading:      { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  subheading:   { fontSize: '0.85rem', fontWeight: 'bold', color: '#333', marginBottom: 8 },
  intro:        { fontSize: '0.8rem', color: '#444', lineHeight: 1.6, marginBottom: 12 },
  body:         { fontSize: '0.78rem', color: '#555', marginBottom: 10 },
  hint:         { color: '#888', fontSize: '0.75rem', marginTop: 4 },
  muted:        { color: '#999', fontSize: '0.78rem', padding: '8px 0' },
  errBox:       { background: '#fff5f5', border: '1px solid #fca5a5', borderRadius: 4, padding: '6px 10px', fontSize: '0.78rem', color: '#991b1b' },

  diagram:      {
    background: '#f8f8f8', border: '1px solid #ddd', borderRadius: 4,
    padding: 16, fontSize: '0.72rem', lineHeight: 1.7, overflowX: 'auto',
  },

  tableWrap:    { overflowX: 'auto' },
  table:        { borderCollapse: 'collapse', fontSize: '0.78rem', width: '100%' },
  th:           { textAlign: 'left', padding: '4px 12px', borderBottom: '1px solid #ddd', color: '#666', fontWeight: 'normal' },
  td:           { padding: '4px 12px', borderBottom: '1px solid #f0f0f0', color: '#333' },

  cardRow:      { display: 'flex', gap: 16, flexWrap: 'wrap' },
  card:         {
    background: '#fff', border: '1px solid #c4b5fd', borderRadius: 6,
    padding: '12px 16px', flex: '1 1 200px',
  },
  cardTitle:    { fontWeight: 'bold', fontSize: '0.85rem', marginBottom: 4, color: '#1a1a2e' },
  transport:    { fontSize: '0.7rem', color: '#7c3aed', marginBottom: 10 },
  stat:         { display: 'flex', justifyContent: 'space-between', gap: 8, marginBottom: 4, fontSize: '0.78rem' },
  statLabel:    { color: '#888' },
  statValue:    { fontWeight: 'bold', color: '#333', fontFamily: 'monospace' },
  link:         { color: '#92400e', textDecoration: 'none', fontFamily: 'monospace', fontSize: '0.78rem', fontWeight: 'bold' },

  demoWrap:     { background: '#f9f9ff', border: '1px solid #e0e0f0', borderRadius: 6, padding: 16, maxWidth: 640 },
  checkRow:     { display: 'flex', alignItems: 'center', gap: 0, marginBottom: 8, flexWrap: 'wrap' },
  label:        { width: 70, fontSize: '0.78rem', color: '#555', flexShrink: 0 },
  select:       { fontFamily: 'monospace', fontSize: '0.78rem', border: '1px solid #ccc', borderRadius: 4, padding: '4px 8px' },
  input:        { fontFamily: 'monospace', fontSize: '0.78rem', border: '1px solid #ccc', borderRadius: 4, padding: '4px 8px', flex: 1, minWidth: 120 },
  btn:          {
    fontFamily: 'monospace', fontSize: '0.8rem', cursor: 'pointer',
    background: '#92400e', color: '#fff', border: 'none',
    borderRadius: 4, padding: '6px 16px', marginBottom: 10,
  },
  resultBox:    { border: '1px solid', borderRadius: 6, padding: '10px 14px', marginTop: 8 },
  resultHeader: { display: 'flex', alignItems: 'center', gap: 12, marginBottom: 6, flexWrap: 'wrap' },
  decisionBadge:{ display: 'inline-block', padding: '2px 12px', borderRadius: 12, fontSize: '0.8rem', fontWeight: 'bold' },
  resultMeta:   { fontSize: '0.75rem', color: '#555' },
  responseBody: {
    background: '#f0f7f0', border: '1px solid #c6f6d5', borderRadius: 4,
    padding: '8px 12px', fontSize: '0.72rem', maxHeight: 200, overflowY: 'auto',
    lineHeight: 1.5, marginTop: 6,
  },
}
