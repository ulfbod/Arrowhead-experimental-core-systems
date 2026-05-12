// PKIRestView — mTLS/REST transport monitoring for experiment-8.
//
// Experiment-8 extends experiment-7's cert-rest-authz model with Arrowhead 5.2
// profile-based PKI. Key differences:
//
//   experiment-7: cert-consumer issues any cert from core CA → presents CN to cert-rest-authz
//   experiment-8: pki-consumer obtains a system cert (OU=sy) via the full on→de→sy lifecycle
//                 from profile-ca → presents CN+OU to pki-rest-authz, which enforces OU=sy
//
// pki-rest-authz rejects clients whose OU is not "sy" (wrong profile) at the TLS level —
// before the AuthzForce check even occurs.

import { useState, useCallback } from 'react'
import { usePolling } from '../hooks/usePolling'
import {
  fetchPKIRestAuthzStatus,
  fetchPKIConsumerStats,
  fetchDataProviderStats,
  fetchAuthRules,
  checkPKIRestAuthz,
} from '../api'
import type {
  PKIRestAuthzStatus,
  PKIConsumerStats,
  DataProviderStats,
  LookupResponse,
  AuthCheckResult,
} from '../types'

const MTLS_SERVICE = 'telemetry-rest'

const KNOWN_CONSUMERS = [
  'pki-consumer',
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
  Experiment-8 mTLS REST path (AH5.2 profile-based PKI)
  ══════════════════════════════════════════════════════════════════════

  pki-consumer  ──mTLS──►  pki-rest-authz:9108 (PEP)
  (OU=sy cert)               │
                             │  1. TLS handshake: require client cert
                             │     enforce OU=sy (Arrowhead 5.2 system profile)
                             │     reject if OU ≠ sy (wrong profile level)
                             │
                             │  2. reads CN from peer certificate
                             │     (identity = cert CN, not a header)
                             │
                             ▼
                       AuthzForce PDP (arrowhead-exp8)
                       XACML: subject=CN, resource=telemetry-rest, action=invoke
                             │
                    Permit ──┤──► proxy to data-provider-tls:9094 (HTTPS)
                    Deny    ──► 403 Forbidden              │
                                                           │
                                                data-provider-tls
                                                (Kafka SSL consumer)
                                                GET /telemetry/latest

  pki-consumer startup lifecycle (AH5.2 on→de→sy):
      1. GET /ca/info           →  profile-ca HTTP :8087  (fetch root PEM)
      2. POST /bootstrap/onboarding-cert → profile-ca :8087 (onboarding cert, OU=on)
      3. mTLS POST /profile/device-cert  → profile-ca :8088 (device cert,     OU=de)
      4. mTLS POST /profile/system-cert  → profile-ca :8088 (system cert,     OU=sy)

  vs. experiment-7 (flat PKI, no profile enforcement):
      cert-consumer  ──mTLS──►  cert-rest-authz:9098
      any cert from core CA accepted; no OU check
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
        into AuthzForce. pki-rest-authz checks AuthzForce on every mTLS request using
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

function PepCard({ status }: { status: PKIRestAuthzStatus | null }) {
  return (
    <div style={{ ...s.card, borderColor: '#c4b5fd' }}>
      <div style={s.cardTitle}>pki-rest-authz (PEP)</div>
      <div style={s.cardSub}>mTLS · OU=sy enforced · identity from cert CN</div>
      <StatsRow label="requests total" value={status?.requestsTotal ?? '…'} />
      <StatsRow label="permitted"      value={<span style={{ color: '#166534' }}>{status?.permitted ?? '…'}</span>} />
      <StatsRow label="denied"         value={<span style={{ color: '#991b1b' }}>{status?.denied ?? '…'}</span>} />
      <p style={s.hint}>
        Clients must present a certificate with OU=sy (Arrowhead 5.2 system profile).
        Certificates with OU=on or OU=de are rejected before the AuthzForce check.
      </p>
    </div>
  )
}

function PKIConsumerCard({ stats }: { stats: PKIConsumerStats | null }) {
  return (
    <div style={{ ...s.card, borderColor: '#a5f3fc' }}>
      <div style={s.cardTitle}>pki-consumer</div>
      <div style={s.cardSub}>REST client · transport: {stats?.transport ?? 'rest-mtls-pki'}</div>
      <StatsRow label="messages received" value={stats?.msgCount ?? '…'} />
      <StatsRow label="denied count"      value={stats?.deniedCount ?? '…'} />
      <StatsRow label="last received"     value={stats ? formatTime(stats.lastReceivedAt) : '…'} />
      <StatsRow label="last denied at"    value={stats?.lastDeniedAt ? formatTime(stats.lastDeniedAt) : '—'} />
      <p style={s.hint}>
        pki-consumer completes the full on→de→sy lifecycle at startup
        to obtain a system certificate (CN=pki-consumer, OU=sy). It presents
        this cert on every request — no X-Consumer-Name header is used.
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
        Accessible only via pki-rest-authz (mTLS proxy). The nginx proxy
        uses <code>proxy_ssl_verify on</code> with the profile-ca root cert
        to verify the data-provider-tls server certificate.
      </p>
    </div>
  )
}

// ── Authorization check tool ──────────────────────────────────────────────────
// Uses the plain HTTP /auth/check endpoint (port 9109), NOT the mTLS port (9108).
// This is for dashboard/testing purposes — the real enforcement path requires a
// system certificate (OU=sy) obtained via the full PKI lifecycle.

function AuthCheckTool() {
  const [consumer, setConsumer] = useState('pki-consumer')
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
      const r = await checkPKIRestAuthz(consumer.trim(), service.trim())
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
        Queries <code>POST /auth/check</code> on pki-rest-authz&apos;s plain HTTP port (9109).
        This simulates the AuthzForce authorization decision without needing a client certificate.
        The real mTLS path (port 9108) requires a system certificate (OU=sy) and reads identity
        from the client cert CN — no header is trusted.
      </p>
      <div style={s.checkRow}>
        <label style={s.label}>Consumer</label>
        <input
          value={consumer}
          onChange={e => setConsumer(e.target.value)}
          style={s.input}
          placeholder="consumer name (as cert CN)"
          list="known-consumers-pki-mtls"
        />
        <datalist id="known-consumers-pki-mtls">
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
              ? 'Permit — a client presenting a OU=sy cert with CN matching this consumer would be proxied to data-provider-tls.'
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

export function PKIRestView() {
  const { data: pkiAuthzStatus }             = usePolling<PKIRestAuthzStatus>(fetchPKIRestAuthzStatus, 3_000)
  const { data: pkiStats }                   = usePolling<PKIConsumerStats>(fetchPKIConsumerStats,     3_000)
  const { data: dpStats }                    = usePolling<DataProviderStats>(fetchDataProviderStats,    3_000)
  const { data: caRules, error: caErr }      = usePolling<LookupResponse>(fetchAuthRules, 5_000)

  return (
    <div style={s.wrap}>

      {/* ── Overview ─────────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>mTLS / REST — Profile-Based PKI Authorization</h2>
        <p style={s.intro}>
          This view covers the REST transport path in experiment-8. Unlike experiment-7
          (which used any cert from the core CA), experiment-8 uses Arrowhead 5.2
          profile-based PKI: <strong>pki-consumer</strong> completes the full{' '}
          <code>on → de → sy</code> certificate lifecycle to obtain a <em>system certificate</em>{' '}
          (OU=sy). <strong>pki-rest-authz</strong> enforces the OU=sy requirement at the TLS
          layer — clients with wrong profiles (OU=on, OU=de) are rejected before AuthzForce
          is consulted.
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
          <PepCard          status={pkiAuthzStatus} />
          <PKIConsumerCard  stats={pkiStats}        />
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
