// SecurityView — X.509 / mTLS security monitoring for experiment-7.
//
// Shows:
//   1. CA Certificate — live info from GET /ca/info
//   2. Certificate Issuance Demo — issue and inspect a leaf cert via POST /ca/certificate/issue
//   3. Certificate Revocation — revoke a cert and inspect the CRL via GET /ca/crl
//   4. TLS Transport Summary — which transports are TLS-secured and how
//   5. mTLS PEP Status — cert-rest-authz counters; identity-from-CN explanation
//   6. Security Gap Status — remaining documented gaps and resolved items

import { useState, useCallback } from 'react'
import { usePolling } from '../hooks/usePolling'
import {
  fetchCAInfo,
  issueCert,
  revokeCert,
  fetchCRL,
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
        When <code>cloudName</code> and <code>operatorName</code> are provided in the
        issue request, the CN follows the AH5 hierarchical format:
        <code> systemName.cloudName.operatorName.arrowhead.eu</code>.
      </p>
    </div>
  )
}

// ── Section 2: Certificate Issuance Demo ─────────────────────────────────────

function CertIssueDemo({ onIssued }: { onIssued?: (cert: IssuedCert) => void }) {
  const [systemName, setSystemName] = useState('my-system')
  const [cloudName, setCloudName]   = useState('')
  const [operatorName, setOperatorName] = useState('')
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
      const cert = await issueCert(systemName.trim(), cloudName.trim() || undefined, operatorName.trim() || undefined)
      setResult(cert)
      onIssued?.(cert)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setIssuing(false)
    }
  }, [systemName, cloudName, operatorName, onIssued])

  const hierarchicalCN = cloudName.trim() && operatorName.trim()
    ? `${systemName.trim()}.${cloudName.trim()}.${operatorName.trim()}.arrowhead.eu`
    : systemName.trim()

  return (
    <div style={s.demoWrap}>
      <p style={s.body}>
        Issue a leaf certificate. When <strong>Cloud Name</strong> and <strong>Operator Name</strong> are
        set, the CN follows the AH5 hierarchical format and both the bare name and hierarchical name
        appear as DNS SANs (enabling both Docker hostname and AH5-conformant TLS verification).
      </p>
      <div style={s.row}>
        <label style={s.label}>System Name</label>
        <input value={systemName} onChange={e => setSystemName(e.target.value)} style={s.input}
          placeholder="e.g. my-service" onKeyDown={e => { if (e.key === 'Enter') void issue() }} />
      </div>
      <div style={s.row}>
        <label style={s.label}>Cloud Name</label>
        <input value={cloudName} onChange={e => setCloudName(e.target.value)} style={s.input}
          placeholder="optional, e.g. testcloud" />
        <label style={{ ...s.label, width: 'auto', marginLeft: 8 }}>Operator Name</label>
        <input value={operatorName} onChange={e => setOperatorName(e.target.value)} style={s.input}
          placeholder="optional, e.g. testorg" />
      </div>
      {(cloudName.trim() || operatorName.trim()) && (
        <div style={s.cnPreview}>CN will be: <code>{hierarchicalCN}</code></div>
      )}
      <div style={{ ...s.row, marginTop: 8 }}>
        <button onClick={() => void issue()} disabled={issuing || !systemName.trim()} style={s.btn}>
          {issuing ? 'issuing…' : 'Issue Certificate'}
        </button>
      </div>

      {err && <div style={s.errBox}>{err}</div>}

      {result && (
        <div style={s.certResult}>
          <div style={s.certMeta}>
            <div style={s.metaRow}><span style={s.metaLabel}>systemName</span><span style={s.metaValue}>{result.systemName}</span></div>
            <div style={s.metaRow}><span style={s.metaLabel}>CN (Subject)</span><span style={s.metaValue}>{hierarchicalCN}</span></div>
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

// ── Section 3: Certificate Revocation ────────────────────────────────────────

function CertRevocationPanel() {
  const [certPEM, setCertPEM] = useState('')
  const [revoking, setRevoking] = useState(false)
  const [revokeErr, setRevokeErr] = useState<string | null>(null)
  const [revokedName, setRevokedName] = useState<string | null>(null)

  const [crl, setCRL] = useState<string | null>(null)
  const [loadingCRL, setLoadingCRL] = useState(false)
  const [crlErr, setCrlErr] = useState<string | null>(null)

  const doRevoke = useCallback(async () => {
    if (!certPEM.trim()) return
    setRevoking(true)
    setRevokeErr(null)
    setRevokedName(null)
    try {
      const resp = await revokeCert(certPEM.trim())
      setRevokedName(resp.systemName)
    } catch (e) {
      setRevokeErr(e instanceof Error ? e.message : String(e))
    } finally {
      setRevoking(false)
    }
  }, [certPEM])

  const doFetchCRL = useCallback(async () => {
    setLoadingCRL(true)
    setCrlErr(null)
    try {
      const pem = await fetchCRL()
      setCRL(pem)
    } catch (e) {
      setCrlErr(e instanceof Error ? e.message : String(e))
    } finally {
      setLoadingCRL(false)
    }
  }, [])

  return (
    <div style={s.demoWrap}>
      <p style={s.body}>
        Revoke a certificate via <code>POST /ca/certificate/revoke</code>. After revocation,{' '}
        <code>POST /ca/certificate/verify</code> returns <code>valid: false</code> for that cert.
        The CRL (<code>GET /ca/crl</code>) lists all revoked certificate serials, signed by the CA.
      </p>

      {/* Revoke */}
      <div style={s.subSection}>
        <div style={s.subLabel}>Revoke a Certificate</div>
        <textarea
          value={certPEM}
          onChange={e => setCertPEM(e.target.value)}
          style={s.textarea}
          placeholder="Paste PEM certificate here (issue one above, then paste it)"
          rows={5}
        />
        <div style={{ ...s.row, marginTop: 6 }}>
          <button onClick={() => void doRevoke()} disabled={revoking || !certPEM.trim()} style={s.btn}>
            {revoking ? 'revoking…' : 'Revoke Certificate'}
          </button>
        </div>
        {revokeErr && <div style={s.errBox}>{revokeErr}</div>}
        {revokedName && (
          <div style={s.okBox}>
            Revoked: <strong>{revokedName}</strong> — certificate is now on the CRL.
          </div>
        )}
      </div>

      {/* CRL */}
      <div style={{ ...s.subSection, marginTop: 16 }}>
        <div style={s.subLabel}>Certificate Revocation List (CRL)</div>
        <div style={{ ...s.row, marginTop: 6 }}>
          <button onClick={() => void doFetchCRL()} disabled={loadingCRL} style={s.btn}>
            {loadingCRL ? 'fetching…' : 'Fetch CRL'}
          </button>
        </div>
        {crlErr && <div style={s.errBox}>{crlErr}</div>}
        {crl && <pre style={s.pem}>{crl.trim()}</pre>}
      </div>
    </div>
  )
}

// ── Section 4: TLS Transport Summary ─────────────────────────────────────────

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
    identity:  'Clients authenticate via AMQP credentials (username/password)',
    color:     '#fef3c7',
  },
  {
    transport: 'Kafka',
    protocol:  'Kafka SSL (port 9092)',
    tlsMode:   'Server TLS (client cert optional)',
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
  {
    transport: 'Dashboard → data-provider-tls',
    protocol:  'HTTPS nginx proxy (port 9094)',
    tlsMode:   'TLS verified (fixed)',
    identity:  'nginx verifies server cert against /certs/ca.crt (Gap 4.8 resolved)',
    color:     '#dcfce7',
  },
]

function TLSTransportTable() {
  return (
    <div style={s.tableWrap}>
      <table style={s.table}>
        <thead>
          <tr>
            {['Transport', 'Protocol', 'TLS Mode', 'Identity / Notes'].map(h => (
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
        startup and writes certs (including <code>ca.crt</code>) to the shared <code>/certs</code> volume
        used by Kafka, RabbitMQ, and the dashboard nginx proxy.
      </p>
    </div>
  )
}

// ── Section 5: mTLS PEP Status ────────────────────────────────────────────────

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

// ── Section 6: Gap Status ─────────────────────────────────────────────────────

interface GapRow {
  id:       string
  title:    string
  status:   'resolved' | 'partial' | 'documented'
  detail:   string
}

const GAPS: GapRow[] = [
  {
    id:     '4.2',
    title:  'Certificate naming non-conformance',
    status: 'resolved',
    detail: 'CA now accepts optional cloudName and operatorName. When set, CN follows ' +
            'systemName.cloudName.operatorName.arrowhead.eu and both the bare name and ' +
            'hierarchical name appear as DNS SANs.',
  },
  {
    id:     '4.4',
    title:  'No X.509-level CRL/revocation',
    status: 'resolved',
    detail: 'POST /ca/certificate/revoke records certs in an in-memory revocation list. ' +
            'GET /ca/crl returns a CA-signed CRL. VerifyCert now checks revocation status. ' +
            'Note: revocation state is in-memory and lost on CA restart (GAP_ANALYSIS G5).',
  },
  {
    id:     '4.8',
    title:  'nginx proxy disables TLS verification for data-provider-tls',
    status: 'resolved',
    detail: 'cert-provisioner writes ca.crt to the shared /certs volume. The dashboard ' +
            'container now mounts this volume; nginx uses proxy_ssl_verify on with ' +
            '/certs/ca.crt as the trusted certificate.',
  },
  {
    id:     '4.1',
    title:  'Core systems remain on plain HTTP',
    status: 'documented',
    detail: 'Documented in GAP_ANALYSIS.md G4. Adding mTLS to all four core systems ' +
            'requires significant architectural changes and is out of scope for this experiment.',
  },
  {
    id:     '4.5',
    title:  'Kafka and RabbitMQ use server-only TLS',
    status: 'documented',
    detail: 'Kafka: KAFKA_SSL_CLIENT_AUTH=none. RabbitMQ: AMQP credential auth over AMQPS. ' +
            'These are pragmatic design choices for the messaging layer.',
  },
  {
    id:     '4.6',
    title:  'Authorization model diverges from AH5 JWT spec',
    status: 'documented',
    detail: 'Documented in GAP_ANALYSIS.md G3. XACML/AuthzForce is used instead of JWT ' +
            'bearer tokens. This is an intentional architectural choice for this experiment.',
  },
  {
    id:     '4.7',
    title:  'CA is not part of the AH5 specification',
    status: 'documented',
    detail: 'Documented in GAP_ANALYSIS.md G9. The CA is a custom extension added for ' +
            'experiment-2 to enable certificate-based identity. AH5 defines a hierarchical ' +
            'PKI (Master→Org→Cloud CA) which is not implemented.',
  },
]

const statusLabel: Record<GapRow['status'], string> = {
  resolved:   '✓ Resolved',
  partial:    '~ Partial',
  documented: '— Documented gap',
}
const statusColor: Record<GapRow['status'], string> = {
  resolved:   '#166534',
  partial:    '#92400e',
  documented: '#6b7280',
}

function GapStatusTable() {
  return (
    <div style={s.tableWrap}>
      <table style={s.table}>
        <thead>
          <tr>
            {['Gap', 'Title', 'Status', 'Notes'].map(h => (
              <th key={h} style={s.th}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {GAPS.map(g => (
            <tr key={g.id} style={{ background: g.status === 'resolved' ? '#f0fdf4' : g.status === 'partial' ? '#fffbeb' : '#f9fafb' }}>
              <td style={s.td}><code>{g.id}</code></td>
              <td style={s.td}><strong>{g.title}</strong></td>
              <td style={{ ...s.td, color: statusColor[g.status], fontWeight: 'bold', whiteSpace: 'nowrap' }}>
                {statusLabel[g.status]}
              </td>
              <td style={{ ...s.td, fontSize: '0.74rem', color: '#555' }}>{g.detail}</td>
            </tr>
          ))}
        </tbody>
      </table>
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
        <h2 style={s.heading}>Certificate Issuance</h2>
        <p style={s.intro}>
          Issue a leaf certificate. The optional <code>cloudName</code> and <code>operatorName</code> fields
          enable AH5-conformant hierarchical naming. Both the bare system name and the hierarchical
          name appear as DNS SANs so TLS hostname verification works in Docker and in AH5-named environments.
        </p>
        <CertIssueDemo />
      </section>

      {/* ── Certificate Revocation ───────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>Certificate Revocation (CRL)</h2>
        <p style={s.intro}>
          The CA maintains an in-memory revocation list. Revoked certs are rejected by{' '}
          <code>POST /ca/certificate/verify</code>. <code>GET /ca/crl</code> returns a
          CA-signed Certificate Revocation List in PEM format. Note: the revocation list
          is ephemeral (in-memory) — it resets when the CA restarts.
        </p>
        <CertRevocationPanel />
      </section>

      {/* ── TLS Transport Summary ────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>TLS Transport Security</h2>
        <p style={s.intro}>
          All three experiment-7 transport paths use TLS. Kafka and RabbitMQ use
          certificates provisioned by <strong>cert-provisioner</strong> into a shared
          volume. Go services request certificates directly from the CA via HTTP at startup.
          The dashboard nginx proxy now verifies the data-provider-tls server certificate
          against the shared CA cert (Gap 4.8 resolved).
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

      {/* ── Gap Status ───────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>X509_SECURITY_ASSESSMENT.md — Gap Status</h2>
        <p style={s.intro}>
          Gaps identified in <code>X509_SECURITY_ASSESSMENT.md</code>. Resolved items
          reflect code changes in this session; documented gaps are intentional
          architectural choices recorded in <code>core/GAP_ANALYSIS.md</code>.
        </p>
        <GapStatusTable />
      </section>

      {/* ── Architecture note ────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>X.509 Architecture</h2>
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
      dashboard container mounts /certs:ro → nginx uses ca.crt to verify data-provider-tls

  Identity enforcement (REST path):
      cert-consumer  ──mTLS──►  cert-rest-authz:9098
                                  reads CN from peer cert
                                  POST AuthzForce /pdp (CN, service, "invoke")
                                  Permit → proxy to data-provider-tls:9094 (HTTPS, verified)
                                  Deny   → 403 Forbidden

  Certificate naming:
      Default:     CN = systemName,  DNS SAN = [systemName]
      AH5-conformant (cloudName + operatorName set):
                   CN = systemName.cloudName.operatorName.arrowhead.eu
                   DNS SAN = [systemName, systemName.cloudName.operatorName.arrowhead.eu]

  Revocation:
      POST /ca/certificate/revoke → in-memory revocation list
      GET  /ca/crl                → CA-signed CRL (PEM, valid 24h)
      POST /ca/certificate/verify → checks chain + revocation

  Remaining documented gaps (see GAP_ANALYSIS.md):
      · Core systems communicate over plain HTTP (G4)
      · XACML/ABAC replaces JWT authorization tokens (G3)
      · Flat single self-signed CA (not hierarchical AH5 PKI) (G9)
      · Revocation state is in-memory (G5 applies to CA too)
`.trim()}</pre>
      </section>

    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  wrap:        { fontFamily: 'monospace' },
  section:     { marginBottom: 32 },
  heading:     { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  intro:       { fontSize: '0.8rem', color: '#444', lineHeight: 1.6, marginBottom: 12 },
  body:        { fontSize: '0.78rem', color: '#555', marginBottom: 10 },
  hint:        { color: '#888', fontSize: '0.75rem', marginTop: 6, lineHeight: 1.5 },
  muted:       { color: '#999', fontSize: '0.78rem', padding: '8px 0' },
  errBox:      { background: '#fff5f5', border: '1px solid #fca5a5', borderRadius: 4, padding: '6px 10px', fontSize: '0.78rem', color: '#991b1b', marginTop: 8 },
  okBox:       { background: '#f0fdf4', border: '1px solid #86efac', borderRadius: 4, padding: '6px 10px', fontSize: '0.78rem', color: '#166534', marginTop: 8 },

  caCard:      { background: '#f0f7ff', border: '1px solid #93c5fd', borderRadius: 6, padding: '14px 18px', marginBottom: 4 },
  caHeader:    { display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 },
  caBadge:     { background: '#1d4ed8', color: '#fff', borderRadius: 4, padding: '2px 8px', fontSize: '0.72rem', fontWeight: 'bold' },
  caName:      { fontWeight: 'bold', fontSize: '0.9rem' },
  caMeta:      { fontSize: '0.7rem', color: '#555' },
  pemBlock:    { marginBottom: 8 },
  pemLabel:    { fontSize: '0.72rem', color: '#666', marginBottom: 4, display: 'flex', alignItems: 'center', gap: 8 },
  pem:         { background: '#1a1a2e', color: '#a0c4ff', padding: '10px 14px', borderRadius: 4, fontSize: '0.7rem', lineHeight: 1.5, overflowX: 'auto', margin: 0 },
  toggleBtn:   { fontFamily: 'monospace', fontSize: '0.7rem', background: 'transparent', border: '1px solid #ccc', borderRadius: 3, cursor: 'pointer', padding: '1px 6px', color: '#555' },

  demoWrap:    { background: '#faf5ff', border: '1px solid #d8b4fe', borderRadius: 6, padding: 16, maxWidth: 720 },
  subSection:  { marginTop: 4 },
  subLabel:    { fontSize: '0.78rem', fontWeight: 'bold', color: '#444', marginBottom: 6 },
  row:         { display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8, flexWrap: 'wrap' },
  label:       { width: 100, fontSize: '0.78rem', color: '#555', flexShrink: 0 },
  input:       { flex: 1, fontFamily: 'monospace', fontSize: '0.8rem', border: '1px solid #ccc', borderRadius: 4, padding: '4px 8px', minWidth: 120 },
  textarea:    { width: '100%', fontFamily: 'monospace', fontSize: '0.72rem', border: '1px solid #ccc', borderRadius: 4, padding: '6px 8px', resize: 'vertical', boxSizing: 'border-box' as const },
  btn:         { fontFamily: 'monospace', fontSize: '0.78rem', cursor: 'pointer', background: '#6d28d9', color: '#fff', border: 'none', borderRadius: 4, padding: '6px 14px', flexShrink: 0 },
  cnPreview:   { fontSize: '0.75rem', color: '#1d4ed8', marginBottom: 4 },

  certResult:  { background: '#fff', border: '1px solid #d8b4fe', borderRadius: 4, padding: 12, marginTop: 10 },
  certMeta:    { marginBottom: 10 },
  metaRow:     { display: 'flex', gap: 8, marginBottom: 3, fontSize: '0.78rem' },
  metaLabel:   { width: 90, color: '#888', flexShrink: 0 },
  metaValue:   { fontWeight: 'bold', color: '#333', fontFamily: 'monospace' },

  tableWrap:   { overflowX: 'auto', marginBottom: 4 },
  table:       { borderCollapse: 'collapse', fontSize: '0.78rem', width: '100%' },
  th:          { textAlign: 'left', padding: '6px 12px', borderBottom: '2px solid #ddd', color: '#555', fontWeight: 'bold', background: '#f9fafb' },
  td:          { padding: '6px 12px', borderBottom: '1px solid #f0f0f0', color: '#333', verticalAlign: 'top' },

  cardRow:     { display: 'flex', gap: 16, flexWrap: 'wrap' },
  pepCard:     { background: '#fff', border: '1px solid', borderRadius: 6, padding: '12px 16px', flex: '1 1 260px' },
  cardTitle:   { fontWeight: 'bold', fontSize: '0.85rem', marginBottom: 2, color: '#1a1a2e' },
  cardSubtitle:{ fontSize: '0.7rem', color: '#7c3aed', marginBottom: 10 },
  stat:        { display: 'flex', justifyContent: 'space-between', gap: 8, marginBottom: 4, fontSize: '0.78rem' },
  statLabel:   { color: '#888' },
  statValue:   { fontWeight: 'bold', color: '#333', fontFamily: 'monospace' },
  divider:     { borderTop: '1px solid #f0f0f0', margin: '8px 0' },

  diagram:     { background: '#1a1a2e', color: '#a0c4ff', padding: 16, borderRadius: 6, fontSize: '0.72rem', lineHeight: 1.65, overflowX: 'auto' },
}
