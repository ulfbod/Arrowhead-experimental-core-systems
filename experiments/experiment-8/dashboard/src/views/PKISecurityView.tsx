// PKISecurityView — Arrowhead 5.2 profile-based PKI security monitoring.
//
// Shows:
//   1. Profile-CA Certificate — live info from GET /ca/info
//   2. Bootstrap Demo — issue an onboarding cert (OU=on) from the browser via HTTP
//   3. Profile Certificate Hierarchy — lo → on → de → sy explained
//   4. TLS Transport Summary — all transports and their TLS/identity model
//   5. PKI PEP Status — pki-rest-authz live counters
//   6. Security Gap Status — experiment-8 gap resolution vs experiment-7
//
// Note: profile-ca differs from core CA.
//   - HTTP :8087: bootstrap endpoint (no auth, issues OU=on certs)
//   - mTLS HTTPS :8088: profile enforcement (requires prior-profile cert)
//   - The browser can reach :8087 only — mTLS cert steps require a client cert.

import { useState, useCallback } from 'react'
import { usePolling } from '../hooks/usePolling'
import {
  fetchCAInfo,
  issueOnboardingCert,
  fetchPKIRestAuthzStatus,
  fetchPKIConsumerStats,
} from '../api'
import type { CAInfo, IssuedCert, PKIRestAuthzStatus, PKIConsumerStats } from '../types'

function formatTime(iso: string): string {
  if (!iso) return '—'
  try { return new Date(iso).toLocaleString() } catch { return iso }
}

function pemPreview(pem: string): string {
  const lines = pem.trim().split('\n')
  if (lines.length <= 4) return pem
  return [lines[0], lines[1], '  …', lines[lines.length - 2], lines[lines.length - 1]].join('\n')
}

// ── Section 1: Profile-CA Certificate ────────────────────────────────────────

function CAInfoPanel({ info }: { info: CAInfo | null }) {
  const [expanded, setExpanded] = useState(false)

  if (!info) return <div style={s.muted}>Loading profile-ca info…</div>

  return (
    <div style={s.caCard}>
      <div style={s.caHeader}>
        <span style={s.caBadge}>Profile-CA</span>
        <span style={s.caName}>{info.commonName}</span>
        <span style={s.caMeta}>AH5.2 Local Cloud CA · two-port design · profile enforcement on :8088</span>
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
        profile-ca is an Arrowhead 5.2 Local Cloud CA. It generates a self-signed root
        certificate at startup. The HTTP port (:8087) provides bootstrap access (no client
        cert required) for the first step of the PKI lifecycle. The mTLS HTTPS port (:8088)
        enforces profile ordering: issuing a device cert requires presenting an onboarding
        cert; issuing a system cert requires presenting a device cert.
      </p>
    </div>
  )
}

// ── Section 2: Bootstrap Demo ─────────────────────────────────────────────────

function BootstrapDemo() {
  const [systemName, setSystemName] = useState('my-new-system')
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
      const cert = await issueOnboardingCert(systemName.trim())
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
        Issues step 1 of the PKI lifecycle: an <strong>onboarding certificate</strong> (OU=on)
        via the plain HTTP bootstrap endpoint. This is the only cert step accessible from the
        browser. Steps 2–4 (device cert, system cert) require mTLS and cannot be initiated
        from the browser — they are performed by pki-consumer at startup automatically.
      </p>
      <div style={s.row}>
        <label style={s.label}>System Name</label>
        <input value={systemName} onChange={e => setSystemName(e.target.value)} style={s.input}
          placeholder="e.g. my-new-system" onKeyDown={e => { if (e.key === 'Enter') void issue() }} />
      </div>
      <div style={{ ...s.row, marginTop: 4 }}>
        <button onClick={() => void issue()} disabled={issuing || !systemName.trim()} style={s.btn}>
          {issuing ? 'issuing…' : 'Issue Onboarding Cert (step 1 / 4)'}
        </button>
      </div>

      {err && <div style={s.errBox}>{err}</div>}

      {result && (
        <div style={s.certResult}>
          <div style={s.certMeta}>
            <div style={s.metaRow}>
              <span style={s.metaLabel}>profile</span>
              <span style={{ ...s.metaValue, background: '#fef3c7', padding: '1px 8px', borderRadius: 10 }}>
                {result.profile} (onboarding)
              </span>
            </div>
            <div style={s.metaRow}>
              <span style={s.metaLabel}>OU</span>
              <span style={s.metaValue}>on</span>
            </div>
          </div>
          <div style={s.pemLabel}>Certificate (PEM)</div>
          <pre style={s.pem}>{result.certificate.trim()}</pre>
          <div style={s.pemLabel}>
            Private Key (EC)
            <button style={s.toggleBtn} onClick={() => setShowKey(k => !k)}>
              {showKey ? 'hide' : 'show'}
            </button>
          </div>
          {showKey
            ? <pre style={s.pem}>{result.privateKey.trim()}</pre>
            : <pre style={{ ...s.pem, color: '#aaa' }}>-----BEGIN EC PRIVATE KEY----- (hidden)</pre>}
          <p style={s.hint}>
            This onboarding cert (OU=on) can now be used to request a device cert (step 3)
            via <code>POST /profile/device-cert</code> on mTLS port :8088. The browser cannot
            perform this step — pki-consumer does it automatically at startup.
          </p>
        </div>
      )}
    </div>
  )
}

// ── Section 3: Profile Certificate Hierarchy ──────────────────────────────────

function ProfileHierarchyTable() {
  const profiles = [
    {
      step: '0',
      profile: 'lo',
      ou: '(none)',
      name: 'Local Cloud Root',
      issuer: 'self-signed',
      access: 'sign all profile certs',
      color: '#fef3c7',
    },
    {
      step: '1',
      profile: 'on',
      ou: 'on',
      name: 'Onboarding',
      issuer: 'Local Cloud Root',
      access: 'bootstrap HTTP :8087 (no prior cert)',
      color: '#dbeafe',
    },
    {
      step: '2',
      profile: 'de',
      ou: 'de',
      name: 'Device',
      issuer: 'Local Cloud Root',
      access: 'mTLS :8088, present OU=on cert',
      color: '#d1fae5',
    },
    {
      step: '3',
      profile: 'sy',
      ou: 'sy',
      name: 'System',
      issuer: 'Local Cloud Root',
      access: 'mTLS :8088, present OU=de cert',
      color: '#ede9fe',
    },
  ]

  return (
    <div style={s.tableWrap}>
      <table style={s.table}>
        <thead>
          <tr>
            {['Step', 'Profile', 'OU', 'Certificate Name', 'Issued by', 'How to obtain'].map(h => (
              <th key={h} style={s.th}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {profiles.map(p => (
            <tr key={p.profile} style={{ background: p.color }}>
              <td style={s.td}>{p.step}</td>
              <td style={s.td}><code>{p.profile}</code></td>
              <td style={s.td}><code>{p.ou}</code></td>
              <td style={s.td}><strong>{p.name}</strong></td>
              <td style={s.td}>{p.issuer}</td>
              <td style={{ ...s.td, fontSize: '0.74rem' }}>{p.access}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p style={s.hint}>
        Profile enforcement: profile-ca rejects a request for profile N if the client does
        not present a valid cert at profile N−1. This prevents skipping lifecycle steps.
        pki-rest-authz additionally rejects any client that does not present a OU=sy cert —
        onboarding and device certs are not accepted as service identities.
      </p>
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
    identity:  'AMQP credentials (username/password over AMQPS)',
    color:     '#fef3c7',
  },
  {
    transport: 'Kafka',
    protocol:  'Kafka SSL (port 9092)',
    tlsMode:   'Server TLS',
    identity:  'PKCS12 keystore / JKS truststore via cert-provisioner',
    color:     '#d1fae5',
  },
  {
    transport: 'REST (pki-rest-authz)',
    protocol:  'HTTPS mTLS (port 9108)',
    tlsMode:   'mTLS — client cert required, OU=sy enforced',
    identity:  'Consumer identity = cert CN; profile-ca AH5.2 system cert (OU=sy)',
    color:     '#ede9fe',
  },
  {
    transport: 'profile-ca bootstrap',
    protocol:  'HTTP (port 8087)',
    tlsMode:   'No TLS — bootstrap only',
    identity:  'No client cert; issues OU=on certificates',
    color:     '#dbeafe',
  },
  {
    transport: 'profile-ca profile issuance',
    protocol:  'HTTPS mTLS (port 8088)',
    tlsMode:   'mTLS — prior-profile cert required',
    identity:  'Verified at TLS layer: OU=on for device cert, OU=de for system cert',
    color:     '#fae8ff',
  },
  {
    transport: 'Dashboard → data-provider-tls',
    protocol:  'HTTPS nginx proxy (port 9094)',
    tlsMode:   'TLS verified',
    identity:  'nginx verifies server cert against profile-ca /certs/ca.crt',
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
    </div>
  )
}

// ── Section 5: mTLS PEP Live Status ──────────────────────────────────────────

function StatsRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div style={s.stat}>
      <span style={s.statLabel}>{label}</span>
      <span style={s.statValue}>{value}</span>
    </div>
  )
}

function PKIRestAuthzCard({ status }: { status: PKIRestAuthzStatus | null }) {
  return (
    <div style={{ ...s.pepCard, borderColor: '#c4b5fd' }}>
      <div style={s.cardTitle}>pki-rest-authz</div>
      <div style={s.cardSubtitle}>mTLS reverse-proxy PEP (port 9108) · OU=sy required</div>
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
        The Arrowhead 5.2 system profile (OU=sy) is enforced — onboarding and
        device profile certs are rejected before the AuthzForce check.
      </p>
    </div>
  )
}

function PKIConsumerCard({ stats }: { stats: PKIConsumerStats | null }) {
  return (
    <div style={{ ...s.pepCard, borderColor: '#6ee7b7' }}>
      <div style={s.cardTitle}>pki-consumer</div>
      <div style={s.cardSubtitle}>mTLS REST client · transport: {stats?.transport ?? 'rest-mtls-pki'}</div>
      <StatsRow label="messages received" value={stats?.msgCount ?? '…'} />
      <StatsRow label="denied count"      value={stats?.deniedCount ?? '…'} />
      <StatsRow label="last received"     value={stats ? formatTime(stats.lastReceivedAt) : '…'} />
      <StatsRow label="last denied at"    value={stats?.lastDeniedAt ? formatTime(stats.lastDeniedAt) : '—'} />
      <div style={s.divider} />
      <p style={s.hint}>
        Completes on→de→sy lifecycle at startup. Presents system cert (OU=sy) on
        every request. A non-zero messages-received count confirms the full PKI
        lifecycle and AuthzForce authorization path work end-to-end.
      </p>
    </div>
  )
}

// ── Section 6: Gap Status ─────────────────────────────────────────────────────

interface GapRow {
  id:       string
  title:    string
  status:   'resolved' | 'partial' | 'new' | 'documented'
  detail:   string
}

const GAPS: GapRow[] = [
  {
    id:     '8.1',
    title:  'Flat single CA — no AH5.2 profile hierarchy',
    status: 'resolved',
    detail: 'profile-ca implements the AH5.2 Local Cloud CA with lo→on→de→sy profile ' +
            'enforcement. Clients must climb the ladder: onboarding cert → device cert → ' +
            'system cert. Each step requires presenting the previous profile cert via mTLS.',
  },
  {
    id:     '8.2',
    title:  'Profile identity not enforced at PEP level',
    status: 'resolved',
    detail: 'pki-rest-authz enforces OU=sy at the TLS layer. Clients presenting OU=on or ' +
            'OU=de certificates receive a TLS rejection before any AuthzForce check. ' +
            'Only OU=sy (system-level identity) reaches the authorization decision.',
  },
  {
    id:     '8.3',
    title:  'Bootstrap endpoint accessible without TLS',
    status: 'documented',
    detail: 'profile-ca HTTP :8087 (bootstrap) is intentionally plain HTTP — this is the ' +
            'start of the lifecycle and by definition requires no prior cert. AH5.2 allows ' +
            'HTTP for the initial bootstrap step.',
  },
  {
    id:     '8.4',
    title:  'Core systems — optional mTLS via TLS_PORT (inherited from exp-7)',
    status: 'partial',
    detail: 'All four core systems expose an optional HTTPS+mTLS listener via TLS_PORT. ' +
            'Plain HTTP is retained on PORT for healthchecks and Docker bootstrap. ' +
            'Full AH5 compliance would require removing plain HTTP entirely.',
  },
  {
    id:     '8.5',
    title:  'Kafka and RabbitMQ use server-only TLS',
    status: 'documented',
    detail: 'Kafka: KAFKA_SSL_CLIENT_AUTH=none. RabbitMQ: AMQP credential auth over AMQPS. ' +
            'These are pragmatic design choices for the messaging layer.',
  },
  {
    id:     '8.6',
    title:  'Authorization model diverges from AH5 JWT spec',
    status: 'documented',
    detail: 'XACML/AuthzForce is used instead of JWT bearer tokens (see GAP_ANALYSIS.md G3). ' +
            'Intentional architectural choice — XACML enables richer policy expression.',
  },
  {
    id:     '8.7',
    title:  'profile-ca revocation state is in-memory',
    status: 'documented',
    detail: 'Certificate revocation state resets on CA restart (ephemeral). Production ' +
            'deployment would require persistent storage for the revocation list (G5 in GAP_ANALYSIS).',
  },
]

const statusLabel: Record<GapRow['status'], string> = {
  resolved:   '✓ Resolved',
  partial:    '~ Partial',
  new:        '★ New in exp-8',
  documented: '— Documented gap',
}
const statusColor: Record<GapRow['status'], string> = {
  resolved:   '#166534',
  partial:    '#92400e',
  new:        '#1d4ed8',
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
            <tr key={g.id} style={{
              background: g.status === 'resolved' ? '#f0fdf4'
                        : g.status === 'new'      ? '#eff6ff'
                        : g.status === 'partial'  ? '#fffbeb'
                        : '#f9fafb',
            }}>
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

export function PKISecurityView() {
  const { data: caInfo }          = usePolling<CAInfo>(fetchCAInfo, 30_000)
  const { data: pkiAuthzStatus }  = usePolling<PKIRestAuthzStatus>(fetchPKIRestAuthzStatus, 3_000)
  const { data: pkiStats }        = usePolling<PKIConsumerStats>(fetchPKIConsumerStats, 3_000)

  return (
    <div style={s.wrap}>

      {/* ── Profile-CA Certificate ───────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>Profile-CA Certificate Authority (AH5.2)</h2>
        <p style={s.intro}>
          profile-ca is an Arrowhead 5.2 Local Cloud CA (<code>core/cmd/profile-ca</code>).
          It implements the four-level profile hierarchy (lo, on, de, sy) and enforces
          profile ordering via mTLS on its :8088 port. The :8087 HTTP bootstrap port
          allows the initial onboarding cert request without any prior certificate.
        </p>
        <CAInfoPanel info={caInfo} />
      </section>

      {/* ── Bootstrap Demo ───────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>Bootstrap Demo — Issue Onboarding Cert (Step 1/4)</h2>
        <p style={s.intro}>
          The browser can reach the profile-ca HTTP bootstrap endpoint. Issue an onboarding
          certificate (OU=on) here. Steps 2–4 (device cert and system cert) require mTLS
          and are performed by pki-consumer automatically at startup.
        </p>
        <BootstrapDemo />
      </section>

      {/* ── Profile Hierarchy ────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>Arrowhead 5.2 Certificate Profile Hierarchy</h2>
        <p style={s.intro}>
          The AH5.2 Local Cloud PKI defines four certificate profiles. Each profile
          corresponds to a trust level in the system. Profiles must be acquired in order —
          skipping is not possible because each step requires the previous profile cert as
          a client certificate.
        </p>
        <ProfileHierarchyTable />
      </section>

      {/* ── TLS Transport Summary ────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>TLS Transport Security</h2>
        <p style={s.intro}>
          All three experiment-8 transport paths use TLS. pki-rest-authz adds profile
          enforcement to the mTLS layer — only OU=sy (system) certificates are accepted.
          Kafka and RabbitMQ use certificates provisioned by cert-provisioner. profile-ca
          provides the root CA cert mounted into the nginx proxy for server cert verification.
        </p>
        <TLSTransportTable />
      </section>

      {/* ── mTLS PEP Live Status ─────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>mTLS PEP — Live Status</h2>
        <p style={s.intro}>
          pki-rest-authz enforces both profile level (OU=sy required) and authorization
          policy (AuthzForce XACML decision). pki-consumer demonstrates end-to-end
          operation: lifecycle completion → system cert → successful mTLS calls.
        </p>
        <div style={s.cardRow}>
          <PKIRestAuthzCard status={pkiAuthzStatus} />
          <PKIConsumerCard  stats={pkiStats}        />
        </div>
      </section>

      {/* ── Gap Status ───────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>Security Gap Status — Experiment 8</h2>
        <p style={s.intro}>
          Security gaps tracked for this experiment. Gaps 8.1 and 8.2 are new resolutions
          that advance beyond experiment-7&apos;s flat PKI model. Remaining gaps are
          intentional architectural choices documented in <code>core/GAP_ANALYSIS.md</code>.
        </p>
        <GapStatusTable />
      </section>

      {/* ── Architecture note ────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>Profile-CA Architecture</h2>
        <pre style={s.diagram}>{`
  Arrowhead 5.2 Local Cloud CA (profile-ca)
  ══════════════════════════════════════════════════════════════════════

  Two-port design:
      :8087 HTTP   — bootstrap (no client cert required)
      :8088 HTTPS  — profile enforcement (client cert required, OU enforced)

  Certificate lifecycle (on→de→sy):
      Step 1: POST http://profile-ca:8087/bootstrap/onboarding-cert
              → returns cert with OU=on
              (no prior cert needed)

      Step 2: mTLS POST https://profile-ca:8088/profile/device-cert
              client presents OU=on cert → server issues OU=de cert

      Step 3: mTLS POST https://profile-ca:8088/profile/system-cert
              client presents OU=de cert → server issues OU=sy cert

  pki-consumer performs all three steps at startup, then:
      Step 4: Use OU=sy cert to present to pki-rest-authz:9108 (mTLS)

  Profile enforcement at pki-rest-authz (port 9108):
      TLS handshake: require client cert with OU=sy
      Reject OU=on / OU=de → TLS alert (before AuthzForce query)
      Accept OU=sy  → POST AuthzForce /pdp → Permit/Deny

  Arrowhead 5.2 naming (Local Cloud scope):
      CN = systemName.cloudName.operatorName.arrowhead.eu  (when cloudName set)
      OU = sy (system profile)
      DNS SAN includes both bare name and hierarchical name

  Browser-accessible steps:
      GET  /api/profile-ca/ca/info               → root cert PEM
      POST /api/profile-ca/bootstrap/onboarding-cert → OU=on cert (step 1 only)
      (steps 2-4 require mTLS, cannot be initiated from the browser)
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

  caCard:      { background: '#f0fdf4', border: '1px solid #6ee7b7', borderRadius: 6, padding: '14px 18px', marginBottom: 4 },
  caHeader:    { display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10, flexWrap: 'wrap' },
  caBadge:     { background: '#047857', color: '#fff', borderRadius: 4, padding: '2px 8px', fontSize: '0.72rem', fontWeight: 'bold' },
  caName:      { fontWeight: 'bold', fontSize: '0.9rem' },
  caMeta:      { fontSize: '0.7rem', color: '#555' },
  pemBlock:    { marginBottom: 8 },
  pemLabel:    { fontSize: '0.72rem', color: '#666', marginBottom: 4, display: 'flex', alignItems: 'center', gap: 8 },
  pem:         { background: '#1a1a2e', color: '#a0c4ff', padding: '10px 14px', borderRadius: 4, fontSize: '0.7rem', lineHeight: 1.5, overflowX: 'auto', margin: 0 },
  toggleBtn:   { fontFamily: 'monospace', fontSize: '0.7rem', background: 'transparent', border: '1px solid #ccc', borderRadius: 3, cursor: 'pointer', padding: '1px 6px', color: '#555' },

  demoWrap:    { background: '#f0fdf4', border: '1px solid #6ee7b7', borderRadius: 6, padding: 16, maxWidth: 680 },
  row:         { display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8, flexWrap: 'wrap' },
  label:       { width: 100, fontSize: '0.78rem', color: '#555', flexShrink: 0 },
  input:       { flex: 1, fontFamily: 'monospace', fontSize: '0.8rem', border: '1px solid #ccc', borderRadius: 4, padding: '4px 8px', minWidth: 120 },
  btn:         { fontFamily: 'monospace', fontSize: '0.78rem', cursor: 'pointer', background: '#047857', color: '#fff', border: 'none', borderRadius: 4, padding: '6px 14px', flexShrink: 0 },

  certResult:  { background: '#fff', border: '1px solid #6ee7b7', borderRadius: 4, padding: 12, marginTop: 10 },
  certMeta:    { marginBottom: 10 },
  metaRow:     { display: 'flex', gap: 8, marginBottom: 3, fontSize: '0.78rem', alignItems: 'center' },
  metaLabel:   { width: 90, color: '#888', flexShrink: 0 },
  metaValue:   { fontWeight: 'bold', color: '#333', fontFamily: 'monospace' },

  tableWrap:   { overflowX: 'auto', marginBottom: 4 },
  table:       { borderCollapse: 'collapse', fontSize: '0.78rem', width: '100%' },
  th:          { textAlign: 'left', padding: '6px 12px', borderBottom: '2px solid #ddd', color: '#555', fontWeight: 'bold', background: '#f9fafb' },
  td:          { padding: '6px 12px', borderBottom: '1px solid #f0f0f0', color: '#333', verticalAlign: 'top' },

  cardRow:     { display: 'flex', gap: 16, flexWrap: 'wrap' },
  pepCard:     { background: '#fff', border: '1px solid', borderRadius: 6, padding: '12px 16px', flex: '1 1 260px' },
  cardTitle:   { fontWeight: 'bold', fontSize: '0.85rem', marginBottom: 2, color: '#1a1a2e' },
  cardSubtitle:{ fontSize: '0.7rem', color: '#047857', marginBottom: 10 },
  stat:        { display: 'flex', justifyContent: 'space-between', gap: 8, marginBottom: 4, fontSize: '0.78rem' },
  statLabel:   { color: '#888' },
  statValue:   { fontWeight: 'bold', color: '#333', fontFamily: 'monospace' },
  divider:     { borderTop: '1px solid #f0f0f0', margin: '8px 0' },

  diagram:     { background: '#1a1a2e', color: '#a0c4ff', padding: 16, borderRadius: 6, fontSize: '0.72rem', lineHeight: 1.65, overflowX: 'auto' },
}
