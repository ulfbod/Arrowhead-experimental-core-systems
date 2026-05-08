// SecurityView — X.509 / mTLS security monitoring for experiment-7.
//
// Shows:
//   1. CA Certificate — live info from GET /ca/info
//   2. Certificate Issuance Demo — issue and inspect a leaf cert via POST /ca/certificate/issue
//   3. TLS Transport Summary — which transports are TLS-secured and how
//   4. mTLS PEP Status — cert-rest-authz counters; identity-from-CN explanation

import { useState, useCallback } from 'react'
import { usePolling } from '../hooks/usePolling'
import {
  fetchCAInfo,
  issueCert,
  fetchCertRestAuthzStatus,
  fetchCertConsumerStats,
} from '../api'
import type { CAInfo, IssuedCert, CertRestAuthzStatus, CertConsumerStats } from '../types'

function formatTime(iso: string): string {
  if (!iso) return '—'
  try { return new Date(iso).toLocaleString() } catch { return iso }
}

function pemPreview(pem: string): string {
  const lines = pem.trim().split('\n')
  if (lines.length <= 4) return pem
  return [lines[0], lines[1], '  …', lines[lines.length - 2], lines[lines.length - 1]].join('\n')
}

// ── Section 1: CA Certificate ────────────────────────────────────────────────

function CAInfoPanel({ info }: { info: CAInfo | null }) {
  const [expanded, setExpanded] = useState(false)

  if (!info) return <div style={s.muted}>Loading CA info…</div>

  return (
    <div style={s.caCard}>
      <div style={s.caHeader}>
        <span style={s.caBadge}>CA</span>
        <span style={s.caName}>{info.commonName}</span>
        <span style={s.caMeta}>self-signed ECDSA P-256 · in-memory · rotates on restart</span>
      </div>
      <div style={s.pemBlock}>
        <div style={s.pemLabel}>
          Root Certificate (PEM)
          <button style={s.toggleBtn} onClick={() => setExpanded(e => !e)}>
            {expanded ? 'collapse' : 'expand'}
          </button>
        </div>
        <pre style={s.pem}>
          {expanded ? info.certificate.trim() : pemPreview(info.certificate)}
        </pre>
      </div>
      <p style={s.hint}>
        The CA generates a new root certificate at every startup (ephemeral, in-memory).
        All services obtain their leaf certificates from this CA at startup via
        <code> POST /ca/certificate/issue</code>. Leaf certificates include a DNS SAN
        matching the system name, enabling Go 1.15+ hostname verification.
      </p>
    </div>
  )
}

// ── Section 2: Certificate Issuance Demo ─────────────────────────────────────

function CertIssueDemo() {
  const [systemName, setSystemName] = useState('my-system')
  const [result, setResult] = useState<IssuedCert | null>(null)
  const [issuing, setIssuing] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [showKey, setShowKey] = useState(false)

  const issue = useCallback(async () => {
    if (!systemName.trim()) return
    setIssuing(true)
    setErr(null)
    setResult(null)
    try {
      const cert = await issueCert(systemName.trim())
      setResult(cert)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setIssuing(false)
    }
  }, [systemName])

  return (
    <div style={s.demoWrap}>
      <p style={s.body}>
        Issue a leaf certificate for any system name. The CA signs it with ECDSA P-256,
        sets <code>CN=&lt;systemName&gt;</code> and <code>DNS SAN=&lt;systemName&gt;</code>,
        and returns the PEM-encoded certificate and private key.
      </p>
      <div style={s.row}>
        <label style={s.label}>System Name</label>
        <input
          value={systemName}
          onChange={e => setSystemName(e.target.value)}
          style={s.input}
          placeholder="e.g. my-service"
          onKeyDown={e => { if (e.key === 'Enter') void issue() }}
        />
        <button
          onClick={() => void issue()}
          disabled={issuing || !systemName.trim()}
          style={s.btn}
        >
          {issuing ? 'issuing…' : 'Issue Certificate'}
        </button>
      </div>

      {err && <div style={s.errBox}>{err}</div>}

      {result && (
        <div style={s.certResult}>
          <div style={s.certMeta}>
            <div style={s.metaRow}><span style={s.metaLabel}>systemName</span><span style={s.metaValue}>{result.systemName}</span></div>
            <div style={s.metaRow}><span style={s.metaLabel}>CN (Subject)</span><span style={s.metaValue}>{result.systemName}</span></div>
            <div style={s.metaRow}><span style={s.metaLabel}>DNS SAN</span><span style={s.metaValue}>{result.systemName}</span></div>
            <div style={s.metaRow}><span style={s.metaLabel}>issued at</span><span style={s.metaValue}>{formatTime(result.issuedAt)}</span></div>
            <div style={s.metaRow}><span style={s.metaLabel}>expires at</span><span style={s.metaValue}>{formatTime(result.expiresAt)}</span></div>
          </div>
          <div style={s.pemLabel}>Certificate (PEM)</div>
          <pre style={s.pem}>{result.certificate.trim()}</pre>
          <div style={s.pemLabel}>
            Private Key (EC)
            <button style={s.toggleBtn} onClick={() => setShowKey(k => !k)}>
              {showKey ? 'hide' : 'show'}
            </button>
          </div>
          {showKey && <pre style={s.pem}>{result.privateKey.trim()}</pre>}
          {!showKey && <pre style={{ ...s.pem, color: '#aaa' }}>-----BEGIN EC PRIVATE KEY----- (hidden)</pre>}
        </div>
      )}
    </div>
  )
}

// ── Section 3: TLS Transport Summary ─────────────────────────────────────────

interface TransportRow {
  transport:   string
  protocol:    string
  tlsMode:     string
  identity:    string
  color:       string
}

const TRANSPORTS: TransportRow[] = [
  {
    transport: 'AMQP (RabbitMQ)',
    protocol:  'AMQPS (port 5671)',
    tlsMode:   'Server TLS',
    identity:  'Broker verifies client certs (RabbitMQ mTLS)',
    color:     '#fef3c7',
  },
  {
    transport: 'Kafka',
    protocol:  'Kafka SSL (port 9092)',
    tlsMode:   'mTLS (Keystore + Truststore)',
    identity:  'PKCS12 keystore / JKS truststore via cert-provisioner',
    color:     '#d1fae5',
  },
  {
    transport: 'REST (cert-rest-authz)',
    protocol:  'HTTPS mTLS (port 9098)',
    tlsMode:   'mTLS — client cert required',
    identity:  'Consumer identity = client cert CN (not self-reported header)',
    color:     '#ede9fe',
  },
]

function TLSTransportTable() {
  return (
    <div style={s.tableWrap}>
      <table style={s.table}>
        <thead>
          <tr>
            {['Transport', 'Protocol', 'TLS Mode', 'Identity Source'].map(h => (
              <th key={h} style={s.th}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {TRANSPORTS.map(t => (
            <tr key={t.transport} style={{ background: t.color }}>
              <td style={s.td}><strong>{t.transport}</strong></td>
              <td style={s.td}><code>{t.protocol}</code></td>
              <td style={s.td}>{t.tlsMode}</td>
              <td style={s.td}>{t.identity}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p style={s.hint}>
        All leaf certificates are issued by the Arrowhead CA ({' '}
        <code>GET /ca/info</code> for the root PEM). cert-provisioner runs once at
        startup and writes certs to the <code>/certs</code> shared volume used by Kafka
        and RabbitMQ.
      </p>
    </div>
  )
}

// ── Section 4: mTLS PEP Status ────────────────────────────────────────────────

function StatsRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div style={s.stat}>
      <span style={s.statLabel}>{label}</span>
      <span style={s.statValue}>{value}</span>
    </div>
  )
}

function CertRestAuthzCard({ status }: { status: CertRestAuthzStatus | null }) {
  return (
    <div style={{ ...s.pepCard, borderColor: '#c4b5fd' }}>
      <div style={s.cardTitle}>cert-rest-authz</div>
      <div style={s.cardSubtitle}>mTLS reverse-proxy PEP (port 9098)</div>
      <StatsRow label="requests total" value={status?.requestsTotal ?? '…'} />
      <StatsRow label="permitted" value={
        <span style={{ color: '#166534', fontWeight: 'bold' }}>{status?.permitted ?? '…'}</span>
      } />
      <StatsRow label="denied" value={
        <span style={{ color: '#991b1b', fontWeight: 'bold' }}>{status?.denied ?? '…'}</span>
      } />
      <div style={s.divider} />
      <p style={s.hint}>
        Consumer identity is read from the client X.509 certificate CN.
        No self-reported header — the TLS layer enforces who the caller is.
        Permit/Deny decision from AuthzForce (XACML).
      </p>
    </div>
  )
}

function CertConsumerCard({ stats }: { stats: CertConsumerStats | null }) {
  return (
    <div style={{ ...s.pepCard, borderColor: '#a5f3fc' }}>
      <div style={s.cardTitle}>cert-consumer</div>
      <div style={s.cardSubtitle}>mTLS REST client · transport: {stats?.transport ?? 'rest-mtls'}</div>
      <StatsRow label="messages received" value={stats?.msgCount ?? '…'} />
      <StatsRow label="denied count"      value={stats?.deniedCount ?? '…'} />
      <StatsRow label="last received"     value={stats ? formatTime(stats.lastReceivedAt) : '…'} />
      <StatsRow label="last denied at"    value={stats?.lastDeniedAt ? formatTime(stats.lastDeniedAt) : '—'} />
      <div style={s.divider} />
      <p style={s.hint}>
        cert-consumer issues its own X.509 certificate at startup (CN=cert-consumer)
        and presents it on every request to cert-rest-authz. The proxy reads the CN
        and checks AuthzForce — no X-Consumer-Name header is used.
      </p>
    </div>
  )
}

// ── Top-level view ────────────────────────────────────────────────────────────

export function SecurityView() {
  const { data: caInfo }         = usePolling<CAInfo>(fetchCAInfo, 30_000)
  const { data: certAuthzStatus } = usePolling<CertRestAuthzStatus>(fetchCertRestAuthzStatus, 3_000)
  const { data: certStats }       = usePolling<CertConsumerStats>(fetchCertConsumerStats, 3_000)

  return (
    <div style={s.wrap}>

      {/* ── CA Certificate ──────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>X.509 Certificate Authority</h2>
        <p style={s.intro}>
          The Arrowhead CA (<code>core/cmd/ca</code>, port 8086) generates a self-signed
          ECDSA P-256 root certificate at startup. Every service in this experiment
          obtains a leaf certificate from the CA at startup. All TLS connections are
          verified against this root.
        </p>
        <CAInfoPanel info={caInfo} />
      </section>

      {/* ── Certificate Issuance Demo ────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>Certificate Issuance Demo</h2>
        <p style={s.intro}>
          Issue a leaf certificate for any system name. This demonstrates the
          <code> POST /ca/certificate/issue</code> flow that all services use at startup.
          Certificates include a DNS SAN for the system name so Go 1.15+ hostname
          verification works when connecting to <code>&lt;systemName&gt;:&lt;port&gt;</code>.
        </p>
        <CertIssueDemo />
      </section>

      {/* ── TLS Transport Summary ────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>TLS Transport Security</h2>
        <p style={s.intro}>
          All three transport paths in experiment-7 use TLS. Kafka and RabbitMQ use
          certificates provisioned by <strong>cert-provisioner</strong> into a shared
          volume. Go services request certificates directly from the CA via HTTP at startup.
        </p>
        <TLSTransportTable />
      </section>

      {/* ── mTLS PEP Live Status ─────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>mTLS PEP — Live Status</h2>
        <p style={s.intro}>
          cert-rest-authz is the Policy Enforcement Point for the REST transport.
          Unlike experiment-6&apos;s rest-authz (which trusted the self-reported
          <code> X-Consumer-Name</code> header), cert-rest-authz reads consumer
          identity from the client X.509 certificate CN — enforced at the TLS layer.
        </p>
        <div style={s.cardRow}>
          <CertRestAuthzCard status={certAuthzStatus} />
          <CertConsumerCard  stats={certStats} />
        </div>
      </section>

      {/* ── Architecture note ────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>X.509 Architecture Notes</h2>
        <pre style={s.diagram}>{`
  Trust anchor: Arrowhead CA (ephemeral, in-memory, rotates on restart)
  ════════════════════════════════════════════════════════════════════════

  Startup flow (all Go services):
      service  ──GET /ca/info──►  CA (port 8086)   ← fetch root cert PEM
      service  ──POST /ca/certificate/issue──►  CA  ← get own leaf cert
      service  builds TLS config with root pool + own cert

  cert-provisioner (runs once, writes to /certs volume):
      provisioner  ──POST /ca/certificate/issue──►  CA  ← kafka cert
      provisioner  ──POST /ca/certificate/issue──►  CA  ← rabbitmq cert
      provisioner writes  kafka.crt, kafka.key, rabbitmq.crt, rabbitmq.key, ca.crt

  Identity enforcement (REST path):
      cert-consumer  ──mTLS──►  cert-rest-authz:9098
                                  reads CN from peer cert
                                  POST AuthzForce /pdp (CN, service, "invoke")
                                  Permit → proxy to data-provider-tls:9094 (HTTPS)
                                  Deny   → 403 Forbidden

  AH5.2 conformity gap:
      · Leaf certs use CN-only naming (not sys.cloud.org.arrowhead.eu hierarchy)
      · No CRL/OCSP revocation — cert-provisioner does not refresh certs
      · Core systems communicate over plain HTTP (not mTLS)
      · XACML/ABAC replaces JWT authorization tokens
      See X509_SECURITY_ASSESSMENT.md for full analysis.
`.trim()}</pre>
      </section>

    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  wrap:       { fontFamily: 'monospace' },
  section:    { marginBottom: 32 },
  heading:    { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  intro:      { fontSize: '0.8rem', color: '#444', lineHeight: 1.6, marginBottom: 12 },
  body:       { fontSize: '0.78rem', color: '#555', marginBottom: 10 },
  hint:       { color: '#888', fontSize: '0.75rem', marginTop: 6, lineHeight: 1.5 },
  muted:      { color: '#999', fontSize: '0.78rem', padding: '8px 0' },
  errBox:     { background: '#fff5f5', border: '1px solid #fca5a5', borderRadius: 4, padding: '6px 10px', fontSize: '0.78rem', color: '#991b1b', marginTop: 8 },

  caCard:     { background: '#f0f7ff', border: '1px solid #93c5fd', borderRadius: 6, padding: '14px 18px', marginBottom: 4 },
  caHeader:   { display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 },
  caBadge:    { background: '#1d4ed8', color: '#fff', borderRadius: 4, padding: '2px 8px', fontSize: '0.72rem', fontWeight: 'bold' },
  caName:     { fontWeight: 'bold', fontSize: '0.9rem' },
  caMeta:     { fontSize: '0.7rem', color: '#555' },
  pemBlock:   { marginBottom: 8 },
  pemLabel:   { fontSize: '0.72rem', color: '#666', marginBottom: 4, display: 'flex', alignItems: 'center', gap: 8 },
  pem:        { background: '#1a1a2e', color: '#a0c4ff', padding: '10px 14px', borderRadius: 4, fontSize: '0.7rem', lineHeight: 1.5, overflowX: 'auto', margin: 0 },
  toggleBtn:  { fontFamily: 'monospace', fontSize: '0.7rem', background: 'transparent', border: '1px solid #ccc', borderRadius: 3, cursor: 'pointer', padding: '1px 6px', color: '#555' },

  demoWrap:   { background: '#faf5ff', border: '1px solid #d8b4fe', borderRadius: 6, padding: 16, maxWidth: 680 },
  row:        { display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12, flexWrap: 'wrap' },
  label:      { width: 100, fontSize: '0.78rem', color: '#555', flexShrink: 0 },
  input:      { flex: 1, fontFamily: 'monospace', fontSize: '0.8rem', border: '1px solid #ccc', borderRadius: 4, padding: '4px 8px', minWidth: 140 },
  btn:        { fontFamily: 'monospace', fontSize: '0.78rem', cursor: 'pointer', background: '#6d28d9', color: '#fff', border: 'none', borderRadius: 4, padding: '6px 14px', flexShrink: 0 },

  certResult: { background: '#fff', border: '1px solid #d8b4fe', borderRadius: 4, padding: 12, marginTop: 10 },
  certMeta:   { marginBottom: 10 },
  metaRow:    { display: 'flex', gap: 8, marginBottom: 3, fontSize: '0.78rem' },
  metaLabel:  { width: 90, color: '#888', flexShrink: 0 },
  metaValue:  { fontWeight: 'bold', color: '#333', fontFamily: 'monospace' },

  tableWrap:  { overflowX: 'auto', marginBottom: 4 },
  table:      { borderCollapse: 'collapse', fontSize: '0.78rem', width: '100%' },
  th:         { textAlign: 'left', padding: '6px 12px', borderBottom: '2px solid #ddd', color: '#555', fontWeight: 'bold', background: '#f9fafb' },
  td:         { padding: '6px 12px', borderBottom: '1px solid #f0f0f0', color: '#333' },

  cardRow:    { display: 'flex', gap: 16, flexWrap: 'wrap' },
  pepCard:    { background: '#fff', border: '1px solid', borderRadius: 6, padding: '12px 16px', flex: '1 1 260px' },
  cardTitle:  { fontWeight: 'bold', fontSize: '0.85rem', marginBottom: 2, color: '#1a1a2e' },
  cardSubtitle:{ fontSize: '0.7rem', color: '#7c3aed', marginBottom: 10 },
  stat:       { display: 'flex', justifyContent: 'space-between', gap: 8, marginBottom: 4, fontSize: '0.78rem' },
  statLabel:  { color: '#888' },
  statValue:  { fontWeight: 'bold', color: '#333', fontFamily: 'monospace' },
  divider:    { borderTop: '1px solid #f0f0f0', margin: '8px 0' },

  diagram:    { background: '#1a1a2e', color: '#a0c4ff', padding: 16, borderRadius: 6, fontSize: '0.72rem', lineHeight: 1.65, overflowX: 'auto' },
}
