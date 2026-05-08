// CertRestView — mTLS/REST transport monitoring for experiment-7.
//
// Replaces experiment-6's RestView. The key difference:
//   experiment-6: consumer sends X-Consumer-Name header (self-reported identity)
//   experiment-7: consumer presents an X.509 client certificate (cert-verified identity)
//
// The PEP (cert-rest-authz) reads the consumer CN from the peer certificate —
// no header is trusted or accepted.

import { useState, useCallback } from 'react'
import { usePolling } from '../hooks/usePolling'
import {
  fetchCertRestAuthzStatus,
  fetchCertConsumerStats,
  fetchDataProviderStats,
  fetchAuthRules,
  checkCertRestAuthz,
} from '../api'
import type {
  CertRestAuthzStatus,
  CertConsumerStats,
  DataProviderStats,
  LookupResponse,
  AuthCheckResult,
} from '../types'

const MTLS_SERVICE = 'telemetry-rest'

const KNOWN_CONSUMERS = [
  'cert-consumer',
  'test-probe',
  'unauthorized',
]

function formatTime(iso: string): string {
  if (!iso) return '—'
  try { return new Date(iso).toLocaleTimeString() } catch { return iso }
}

// ── Architecture diagram ──────────────────────────────────────────────────────

function ArchitectureDiagram() {
  return (
    <pre style={s.diagram}>{`
  Experiment-7 mTLS REST path
  ══════════════════════════════════════════════════════════════════════

  cert-consumer  ──mTLS──►  cert-rest-authz:9098 (PEP)
  (client cert)                │
                               │  reads CN from peer certificate
                               │  (no X-Consumer-Name header)
                               │
                               ▼
                         AuthzForce PDP
                         XACML: subject=CN, resource=telemetry-rest, action=invoke
                               │
                      Permit ──┤──► proxy to data-provider-tls:9094 (HTTPS)
                      Deny    ──► 403 Forbidden                │
                                                               │
                                                    data-provider-tls
                                                    (Kafka SSL consumer)
                                                    GET /telemetry/latest

  vs. experiment-6 (self-reported identity):
      rest-consumer  ──HTTP──►  rest-authz:9093 (PEP)
                                  X-Consumer-Name: rest-consumer   ← trusted on faith
`.trim()}</pre>
  )
}

// ── Grants for telemetry-rest service ────────────────────────────────────────

function GrantsCard({ data, error }: { data: LookupResponse | null; error: string | null }) {
  if (error) return <div style={s.errBox}>ConsumerAuth unavailable: {error}</div>
  if (!data)  return <div style={s.muted}>loading…</div>

  const grants = (data.rules ?? []).filter(r => r.serviceDefinition === MTLS_SERVICE)

  if (grants.length === 0) {
    return (
      <div style={s.muted}>
        No grants for <code>{MTLS_SERVICE}</code> in ConsumerAuth.
        Add one on the Grants tab — allow ≤SYNC_INTERVAL for policy-sync to propagate.
      </div>
    )
  }

  return (
    <div style={s.tableWrap}>
      <table style={s.table}>
        <thead>
          <tr>{['id', 'consumer', 'provider', 'service'].map(h => <th key={h} style={s.th}>{h}</th>)}</tr>
        </thead>
        <tbody>
          {grants.map(g => (
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
        Grants are compiled to XACML by policy-sync every SYNC_INTERVAL and loaded
        into AuthzForce. cert-rest-authz checks AuthzForce on every mTLS request using
        the consumer identity from the client certificate CN.
      </p>
    </div>
  )
}

// ── Live stats cards ──────────────────────────────────────────────────────────

function StatsRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div style={s.stat}>
      <span style={s.statLabel}>{label}</span>
      <span style={s.statValue}>{value}</span>
    </div>
  )
}

function PepCard({ status }: { status: CertRestAuthzStatus | null }) {
  return (
    <div style={{ ...s.card, borderColor: '#c4b5fd' }}>
      <div style={s.cardTitle}>cert-rest-authz (PEP)</div>
      <div style={s.cardSub}>mTLS · identity from cert CN</div>
      <StatsRow label="requests total" value={status?.requestsTotal ?? '…'} />
      <StatsRow label="permitted"      value={<span style={{ color: '#166534' }}>{status?.permitted ?? '…'}</span>} />
      <StatsRow label="denied"         value={<span style={{ color: '#991b1b' }}>{status?.denied ?? '…'}</span>} />
      <p style={s.hint}>
        Consumer identity is the X.509 client certificate CN —
        verified cryptographically at the TLS layer, not self-reported.
      </p>
    </div>
  )
}

function CertConsumerCard({ stats }: { stats: CertConsumerStats | null }) {
  return (
    <div style={{ ...s.card, borderColor: '#a5f3fc' }}>
      <div style={s.cardTitle}>cert-consumer</div>
      <div style={s.cardSub}>REST client · transport: {stats?.transport ?? 'rest-mtls'}</div>
      <StatsRow label="messages received" value={stats?.msgCount ?? '…'} />
      <StatsRow label="denied count"      value={stats?.deniedCount ?? '…'} />
      <StatsRow label="last received"     value={stats ? formatTime(stats.lastReceivedAt) : '…'} />
      <StatsRow label="last denied at"    value={stats?.lastDeniedAt ? formatTime(stats.lastDeniedAt) : '—'} />
      <p style={s.hint}>
        cert-consumer presents its X.509 certificate (CN=cert-consumer) on
        every request. No X-Consumer-Name header is sent or accepted.
      </p>
    </div>
  )
}

function DataProviderCard({ stats }: { stats: DataProviderStats | null }) {
  return (
    <div style={{ ...s.card, borderColor: '#86efac' }}>
      <div style={s.cardTitle}>data-provider-tls (upstream)</div>
      <div style={s.cardSub}>Kafka SSL consumer · HTTPS server</div>
      <StatsRow label="messages from Kafka" value={stats?.msgCount ?? '…'} />
      <StatsRow label="robots tracked"      value={stats?.robotCount ?? '…'} />
      <StatsRow label="last received"       value={stats ? formatTime(stats.lastReceivedAt) : '…'} />
      <p style={s.hint}>
        Accessible only via cert-rest-authz (mTLS proxy). Not reachable directly
        from the browser — the nginx proxy uses <code>proxy_ssl_verify off</code>
        for the internal /stats endpoint only.
      </p>
    </div>
  )
}

// ── Authorization check tool ──────────────────────────────────────────────────
// Uses the plain HTTP /auth/check endpoint (port 9099), NOT the mTLS port (9098).
// This is for dashboard/testing purposes — the real enforcement path requires a cert.

function AuthCheckTool() {
  const [consumer, setConsumer] = useState('cert-consumer')
  const [service,  setService]  = useState('telemetry-rest')
  const [result,   setResult]   = useState<AuthCheckResult | null>(null)
  const [checking, setChecking] = useState(false)
  const [err,      setErr]      = useState<string | null>(null)

  const check = useCallback(async () => {
    if (!consumer.trim() || !service.trim()) return
    setChecking(true)
    setErr(null)
    setResult(null)
    try {
      const r = await checkCertRestAuthz(consumer.trim(), service.trim())
      setResult(r)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setChecking(false)
    }
  }, [consumer, service])

  return (
    <div style={s.checkWrap}>
      <p style={s.body}>
        Queries <code>POST /auth/check</code> on cert-rest-authz&apos;s plain HTTP port (9099).
        This simulates the authorization decision without needing a client certificate.
        The real mTLS path (port 9098) requires a cert and reads the consumer CN from it.
      </p>
      <div style={s.checkRow}>
        <label style={s.label}>Consumer</label>
        <input
          value={consumer}
          onChange={e => setConsumer(e.target.value)}
          style={s.input}
          placeholder="consumer name (as cert CN)"
          list="known-consumers-mtls"
        />
        <datalist id="known-consumers-mtls">
          {KNOWN_CONSUMERS.map(c => <option key={c} value={c} />)}
        </datalist>
      </div>
      <div style={s.checkRow}>
        <label style={s.label}>Service</label>
        <input
          value={service}
          onChange={e => setService(e.target.value)}
          style={s.input}
          placeholder="service definition"
        />
      </div>
      <button
        onClick={() => void check()}
        disabled={checking || !consumer.trim() || !service.trim()}
        style={s.btn}
      >
        {checking ? 'checking…' : 'Check Authorization'}
      </button>

      {err && <div style={s.errBox}>{err}</div>}

      {result && (
        <div style={{ ...s.resultBox, borderColor: result.permit ? '#86efac' : '#fca5a5' }}>
          <span style={{ ...s.decisionBadge, background: result.permit ? '#dcfce7' : '#fee2e2', color: result.permit ? '#166534' : '#991b1b' }}>
            {result.decision}
          </span>
          <span style={s.resultMeta}>{result.consumer} → {result.service}</span>
          <p style={s.hint}>
            {result.permit
              ? 'Permit — a client presenting a cert with CN matching this consumer would be proxied to data-provider-tls.'
              : 'Deny — no matching grant in AuthzForce. Add a grant on the Grants tab and wait SYNC_INTERVAL.'}
          </p>
        </div>
      )}

      <p style={s.hint}>
        Quick-fill: {KNOWN_CONSUMERS.map((c, i) => (
          <span key={c}>
            <button style={s.chipBtn} onClick={() => setConsumer(c)}>{c}</button>
            {i < KNOWN_CONSUMERS.length - 1 && ' '}
          </span>
        ))}
      </p>
    </div>
  )
}

// ── Top-level view ────────────────────────────────────────────────────────────

export function CertRestView() {
  const { data: certAuthzStatus }            = usePolling<CertRestAuthzStatus>(fetchCertRestAuthzStatus, 3_000)
  const { data: certStats }                  = usePolling<CertConsumerStats>(fetchCertConsumerStats,  3_000)
  const { data: dpStats }                    = usePolling<DataProviderStats>(fetchDataProviderStats,   3_000)
  const { data: caRules, error: caErr }      = usePolling<LookupResponse>(fetchAuthRules, 5_000)

  return (
    <div style={s.wrap}>

      {/* ── Overview ─────────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>mTLS / REST — Certificate-Based Authorization</h2>
        <p style={s.intro}>
          This view covers the REST transport path. Unlike experiment-6 (which used a
          self-reported <code>X-Consumer-Name</code> header), experiment-7 uses mutual TLS:
          the consumer presents an X.509 certificate, and <strong>cert-rest-authz</strong> reads
          the consumer identity from the certificate CN — verified cryptographically at the TLS
          layer, not declared by the consumer.
        </p>
        <ArchitectureDiagram />
      </section>

      {/* ── Active grants ────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>Active Grants for <code>{MTLS_SERVICE}</code></h3>
        <p style={s.body}>
          Grants in ConsumerAuth for the <code>{MTLS_SERVICE}</code> service definition.
          policy-sync compiles these into XACML every SYNC_INTERVAL.
        </p>
        <GrantsCard data={caRules} error={caErr} />
      </section>

      {/* ── Live stats ───────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>Live Stats</h3>
        <div style={s.cardRow}>
          <PepCard         status={certAuthzStatus} />
          <CertConsumerCard stats={certStats}       />
          <DataProviderCard stats={dpStats}         />
        </div>
      </section>

      {/* ── Auth check tool ──────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>Authorization Check (HTTP — no cert required)</h3>
        <AuthCheckTool />
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
  hint:         { color: '#888', fontSize: '0.75rem', marginTop: 4, lineHeight: 1.5 },
  muted:        { color: '#999', fontSize: '0.78rem', padding: '8px 0' },
  errBox:       { background: '#fff5f5', border: '1px solid #fca5a5', borderRadius: 4, padding: '6px 10px', fontSize: '0.78rem', color: '#991b1b', marginTop: 8 },

  diagram:      { background: '#1a1a2e', color: '#a0c4ff', padding: 16, borderRadius: 6, fontSize: '0.72rem', lineHeight: 1.65, overflowX: 'auto' },

  tableWrap:    { overflowX: 'auto' },
  table:        { borderCollapse: 'collapse', fontSize: '0.78rem', width: '100%' },
  th:           { textAlign: 'left', padding: '4px 12px', borderBottom: '1px solid #ddd', color: '#666', fontWeight: 'normal' },
  td:           { padding: '4px 12px', borderBottom: '1px solid #f0f0f0', color: '#333' },

  cardRow:      { display: 'flex', gap: 16, flexWrap: 'wrap' },
  card:         { background: '#fff', border: '1px solid', borderRadius: 6, padding: '12px 16px', flex: '1 1 200px' },
  cardTitle:    { fontWeight: 'bold', fontSize: '0.85rem', marginBottom: 2, color: '#1a1a2e' },
  cardSub:      { fontSize: '0.7rem', color: '#7c3aed', marginBottom: 10 },
  stat:         { display: 'flex', justifyContent: 'space-between', gap: 8, marginBottom: 4, fontSize: '0.78rem' },
  statLabel:    { color: '#888' },
  statValue:    { fontWeight: 'bold', color: '#333', fontFamily: 'monospace' },

  checkWrap:    { background: '#f9f9ff', border: '1px solid #e0e0f0', borderRadius: 6, padding: 16, maxWidth: 520 },
  checkRow:     { display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 },
  label:        { width: 70, fontSize: '0.78rem', color: '#555', flexShrink: 0 },
  input:        { flex: 1, fontFamily: 'monospace', fontSize: '0.8rem', border: '1px solid #ccc', borderRadius: 4, padding: '4px 8px' },
  btn:          { fontFamily: 'monospace', fontSize: '0.8rem', cursor: 'pointer', background: '#6d28d9', color: '#fff', border: 'none', borderRadius: 4, padding: '6px 16px', marginBottom: 10 },
  resultBox:    { border: '1px solid', borderRadius: 6, padding: '10px 14px', marginTop: 8, marginBottom: 6 },
  decisionBadge:{ display: 'inline-block', padding: '2px 12px', borderRadius: 12, fontSize: '0.8rem', fontWeight: 'bold', marginBottom: 6 },
  resultMeta:   { fontSize: '0.75rem', color: '#555', marginLeft: 10 },
  chipBtn:      { fontFamily: 'monospace', fontSize: '0.72rem', cursor: 'pointer', background: '#ede9fe', color: '#5b21b6', border: 'none', borderRadius: 3, padding: '1px 6px' },
}
