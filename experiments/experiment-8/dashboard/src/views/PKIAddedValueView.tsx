// PKIAddedValueView — Experiment-8 Added Value: Arrowhead 5.2 Profile-Based PKI
//
// This view demonstrates and explains the capabilities added by experiment-8
// beyond experiment-7 (and beyond the base AH5.2 specification):
//
//   1. PKI lifecycle walkthrough — HTTP bootstrap → on → de → sy
//   2. Interactive bootstrap step — issue onboarding cert from browser
//   3. Profile enforcement table — what is allowed/rejected at each step
//   4. Access-policy lifecycle — grant → allow, revoke → deny, restore → allow
//   5. Identity-to-authorization trace — cert CN → XACML subject → Permit/Deny
//   6. Service registry integration — lookup services in experiment-8
//   7. Extensions table — experiment-8 vs AH5.2 spec comparison

import { useState, useCallback } from 'react'
import { usePolling } from '../hooks/usePolling'
import {
  fetchCAInfo,
  issueOnboardingCert,
  fetchPKIConsumerStats,
  fetchPKIRestAuthzStatus,
  fetchAuthRules,
  checkPKIRestAuthz,
  queryServiceRegistry,
} from '../api'
import type {
  CAInfo,
  IssuedCert,
  PKIConsumerStats,
  PKIRestAuthzStatus,
  LookupResponse,
  AuthCheckResult,
  ServiceQueryResponse,
} from '../types'

function formatTime(iso: string): string {
  if (!iso) return '—'
  try { return new Date(iso).toLocaleTimeString() } catch { return iso }
}

function pemPreview(pem: string): string {
  const lines = pem.trim().split('\n')
  if (lines.length <= 4) return pem
  return [lines[0], lines[1], '  …', lines[lines.length - 2], lines[lines.length - 1]].join('\n')
}

// ── Section 1: PKI Lifecycle Walkthrough ─────────────────────────────────────

function LifecycleDiagram() {
  return (
    <pre style={s.diagram}>{`
  Arrowhead 5.2 Certificate Lifecycle (as implemented in experiment-8)
  ══════════════════════════════════════════════════════════════════════

  [Bootstrap — HTTP :8087, no prior cert needed]
  Step 1:  POST /bootstrap/onboarding-cert
           body: {"systemName": "pki-consumer"}
           response: {certificate, privateKey, profile:"on"}
           cert has: OU=on, CN=pki-consumer

  [Profile issuance — mTLS :8088, prior cert required]
  Step 2:  mTLS POST /profile/device-cert        (present OU=on cert as client cert)
           response: {certificate, privateKey, profile:"de"}
           cert has: OU=de, CN=pki-consumer

  Step 3:  mTLS POST /profile/system-cert        (present OU=de cert as client cert)
           response: {certificate, privateKey, profile:"sy"}
           cert has: OU=sy, CN=pki-consumer
           ✓ This is the identity cert used for mTLS service calls

  [Service access — mTLS :9108, OU=sy required]
  Step 4:  mTLS GET https://pki-rest-authz:9108/telemetry/latest
           client cert: CN=pki-consumer, OU=sy
           PEP checks: (1) OU=sy? (2) AuthzForce Permit?
           → proxies to data-provider-tls if both pass

  Profile enforcement by profile-ca (:8088):
      OU=on cert required to get OU=de    (cannot skip to de)
      OU=de cert required to get OU=sy    (cannot skip to sy)
      Any attempt to skip a level → TLS rejection

  Profile enforcement by pki-rest-authz (:9108):
      OU=on cert presented → TLS rejection (wrong profile)
      OU=de cert presented → TLS rejection (wrong profile)
      OU=sy cert presented → AuthzForce check → Permit/Deny
`.trim()}</pre>
  )
}

// ── Section 2: Interactive Bootstrap ─────────────────────────────────────────

function BootstrapInteractive({ caInfo }: { caInfo: CAInfo | null }) {
  const [systemName, setSystemName] = useState('demo-system')
  const [result, setResult] = useState<IssuedCert | null>(null)
  const [issuing, setIssuing] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [showKey, setShowKey] = useState(false)
  const [showCert, setShowCert] = useState(false)

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
        This calls <code>POST /api/profile-ca/bootstrap/onboarding-cert</code> — the same
        plain HTTP endpoint that pki-consumer calls as step 1 of its lifecycle. The browser
        can perform this step because it requires no client certificate. Steps 2 and 3
        (device cert, system cert) require mTLS and are shown as read-only explanations below.
      </p>

      {caInfo && (
        <div style={s.caHint}>
          <strong>Trust anchor:</strong> {caInfo.commonName}
        </div>
      )}

      <div style={s.formRow}>
        <label style={s.fLabel}>System Name</label>
        <input
          value={systemName}
          onChange={e => setSystemName(e.target.value)}
          style={s.input}
          placeholder="e.g. demo-system"
          onKeyDown={e => { if (e.key === 'Enter') void issue() }}
        />
        <button onClick={() => void issue()} disabled={issuing || !systemName.trim()} style={s.greenBtn}>
          {issuing ? 'issuing…' : 'Bootstrap → get OU=on cert'}
        </button>
      </div>

      {err && <div style={s.errBox}>{err}</div>}

      {result && (
        <div style={s.certBox}>
          <div style={s.certHeaderRow}>
            <span style={{ ...s.profileBadge, background: '#fef3c7', color: '#92400e' }}>
              {result.profile} — onboarding
            </span>
            <span style={s.certNote}>OU=on · step 1 / 4 complete</span>
          </div>

          <div style={s.certRow}>
            <span style={s.certLabel}>Certificate</span>
            <button style={s.toggleBtn} onClick={() => setShowCert(c => !c)}>
              {showCert ? 'hide' : 'show PEM'}
            </button>
          </div>
          {showCert && <pre style={s.pem}>{result.certificate.trim()}</pre>}
          {!showCert && <pre style={{ ...s.pem, color: '#aaa' }}>{pemPreview(result.certificate)}</pre>}

          <div style={s.certRow}>
            <span style={s.certLabel}>Private Key</span>
            <button style={s.toggleBtn} onClick={() => setShowKey(k => !k)}>
              {showKey ? 'hide' : 'show PEM'}
            </button>
          </div>
          {showKey && <pre style={s.pem}>{result.privateKey.trim()}</pre>}
          {!showKey && <pre style={{ ...s.pem, color: '#aaa' }}>-----BEGIN EC PRIVATE KEY----- (hidden)</pre>}

          <p style={s.hint}>
            Next: use this cert as a TLS client cert to call{' '}
            <code>POST https://profile-ca:8088/profile/device-cert</code> → get OU=de.
            Then use OU=de to get OU=sy. Then use OU=sy to call pki-rest-authz.
            (Browser cannot perform the mTLS steps — pki-consumer does all four at startup.)
          </p>
        </div>
      )}

      {/* Steps 2–4 as read-only explanation */}
      <div style={s.steps}>
        <div style={s.step}>
          <span style={{ ...s.stepBadge, background: '#d1fae5', color: '#065f46' }}>Step 2</span>
          <span style={s.stepDesc}>
            mTLS <code>POST /profile/device-cert</code> (profile-ca :8088)
            — present OU=on cert → receive OU=de cert
          </span>
          <span style={s.stepNote}>browser-inaccessible (requires mTLS client cert)</span>
        </div>
        <div style={s.step}>
          <span style={{ ...s.stepBadge, background: '#ede9fe', color: '#5b21b6' }}>Step 3</span>
          <span style={s.stepDesc}>
            mTLS <code>POST /profile/system-cert</code> (profile-ca :8088)
            — present OU=de cert → receive OU=sy cert
          </span>
          <span style={s.stepNote}>browser-inaccessible (requires mTLS client cert)</span>
        </div>
        <div style={s.step}>
          <span style={{ ...s.stepBadge, background: '#c4b5fd', color: '#3730a3' }}>Step 4</span>
          <span style={s.stepDesc}>
            mTLS GET/POST to pki-rest-authz :9108
            — present OU=sy cert → AuthzForce check → data access
          </span>
          <span style={s.stepNote}>confirmed by pki-consumer live stats below</span>
        </div>
      </div>
    </div>
  )
}

// ── Section 3: Profile Enforcement Table ─────────────────────────────────────

function ProfileEnforcementTable() {
  const rows = [
    { cert: 'no cert (plain HTTP)', endpoint: '/bootstrap/onboarding-cert :8087', result: '✓ Permit', note: 'bootstrap; no prior cert required', color: '#f0fdf4' },
    { cert: 'OU=on cert', endpoint: '/profile/device-cert :8088',  result: '✓ Permit', note: 'correct profile for step 2',    color: '#f0fdf4' },
    { cert: 'OU=de cert', endpoint: '/profile/device-cert :8088',  result: '✗ Reject', note: 'wrong profile (too advanced)',  color: '#fff5f5' },
    { cert: 'OU=on cert', endpoint: '/profile/system-cert :8088',  result: '✗ Reject', note: 'wrong profile (skipped de)',    color: '#fff5f5' },
    { cert: 'OU=de cert', endpoint: '/profile/system-cert :8088',  result: '✓ Permit', note: 'correct profile for step 3',    color: '#f0fdf4' },
    { cert: 'OU=sy cert', endpoint: '/profile/system-cert :8088',  result: '✗ Reject', note: 'wrong profile (already at top)', color: '#fff5f5' },
    { cert: 'OU=on cert', endpoint: 'pki-rest-authz mTLS :9108',   result: '✗ Reject', note: 'TLS rejection, wrong profile',  color: '#fff5f5' },
    { cert: 'OU=de cert', endpoint: 'pki-rest-authz mTLS :9108',   result: '✗ Reject', note: 'TLS rejection, wrong profile',  color: '#fff5f5' },
    { cert: 'OU=sy cert', endpoint: 'pki-rest-authz mTLS :9108',   result: '⚖ AuthzForce', note: 'Permit if grant exists; Deny otherwise', color: '#faf5ff' },
  ]

  return (
    <div style={s.tableWrap}>
      <table style={s.table}>
        <thead>
          <tr>
            {['Client Certificate', 'Endpoint', 'Result', 'Reason'].map(h => (
              <th key={h} style={s.th}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i} style={{ background: r.color }}>
              <td style={s.td}><code>{r.cert}</code></td>
              <td style={s.td}><code>{r.endpoint}</code></td>
              <td style={{ ...s.td, fontWeight: 'bold', color: r.result.startsWith('✓') ? '#166534' : r.result.startsWith('⚖') ? '#5b21b6' : '#991b1b' }}>
                {r.result}
              </td>
              <td style={{ ...s.td, fontSize: '0.74rem', color: '#555' }}>{r.note}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ── Section 4: Access-Policy Lifecycle ───────────────────────────────────────

function PolicyLifecyclePanel({ grants }: { grants: LookupResponse | null }) {
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

  const relevantGrants = (grants?.rules ?? []).filter(r => r.serviceDefinition === service.trim())

  return (
    <div style={s.lifecycleWrap}>
      <p style={s.body}>
        The access-policy lifecycle: add a grant (Grants tab) → policy-sync propagates to
        AuthzForce within SYNC_INTERVAL → next mTLS call receives Permit. Remove the grant →
        after SYNC_INTERVAL the decision flips to Deny. Add the grant again → Permit restores.
        The live AuthzForce check below shows the current decision.
      </p>

      <div style={s.lifecycleSteps}>
        <div style={s.lcStep}>
          <span style={s.lcNum}>1</span>
          <div>
            <strong>Add grant</strong> on Grants tab<br/>
            <span style={s.lcNote}>(consumer, provider, service)</span>
          </div>
        </div>
        <div style={s.lcArrow}>→</div>
        <div style={s.lcStep}>
          <span style={s.lcNum}>2</span>
          <div>
            <strong>policy-sync</strong> compiles XACML<br/>
            <span style={s.lcNote}>(up to SYNC_INTERVAL)</span>
          </div>
        </div>
        <div style={s.lcArrow}>→</div>
        <div style={s.lcStep}>
          <span style={s.lcNum}>3</span>
          <div>
            <strong>AuthzForce</strong> stores new policy<br/>
            <span style={s.lcNote}>(domain arrowhead-exp8)</span>
          </div>
        </div>
        <div style={s.lcArrow}>→</div>
        <div style={s.lcStep}>
          <span style={s.lcNum}>4</span>
          <div>
            <strong>mTLS call</strong> → Permit<br/>
            <span style={s.lcNote}>(cert CN matches grant)</span>
          </div>
        </div>
      </div>

      <div style={s.grantHint}>
        {relevantGrants.length > 0
          ? <span style={{ color: '#166534' }}>✓ {relevantGrants.length} grant(s) for <code>{service}</code> active in ConsumerAuth</span>
          : <span style={{ color: '#991b1b' }}>✗ No grants for <code>{service}</code> — add one on the Grants tab</span>
        }
      </div>

      <div style={s.checkInline}>
        <input value={consumer} onChange={e => setConsumer(e.target.value)} style={s.inputSm} placeholder="consumer" />
        <span style={{ fontSize: '0.78rem', color: '#555' }}>→</span>
        <input value={service} onChange={e => setService(e.target.value)} style={s.inputSm} placeholder="service" />
        <button onClick={() => void check()} disabled={checking} style={s.checkBtn}>
          {checking ? '…' : 'Check AuthzForce'}
        </button>
      </div>

      {err && <div style={s.errBox}>{err}</div>}
      {result && (
        <div style={{ ...s.resultRow, borderColor: result.permit ? '#86efac' : '#fca5a5' }}>
          <span style={{ ...s.decisionBadge, background: result.permit ? '#dcfce7' : '#fee2e2', color: result.permit ? '#166534' : '#991b1b' }}>
            {result.decision}
          </span>
          <span style={s.resultNote}>
            {result.permit
              ? 'Access permitted — grant exists and policy is synced'
              : 'Access denied — no matching grant in current XACML policy'}
          </span>
        </div>
      )}
    </div>
  )
}

// ── Section 5: Identity-to-Authorization Trace ───────────────────────────────

function IdentityTracePanel({ pkiStats }: { pkiStats: PKIConsumerStats | null; pkiAuthzStatus: PKIRestAuthzStatus | null }) {
  return (
    <div style={s.traceWrap}>
      <pre style={s.traceCode}>{`
  Identity-to-Authorization trace for pki-consumer
  ──────────────────────────────────────────────────

  [1] pki-consumer at startup:
        GET  http://profile-ca:8087/ca/info
             → fetch root CA certificate PEM

  [2] pki-consumer → onboarding cert (step 1):
        POST http://profile-ca:8087/bootstrap/onboarding-cert
             body: {"systemName":"pki-consumer"}
             → cert: {OU=on, CN=pki-consumer}

  [3] pki-consumer → device cert (step 2, mTLS):
        POST https://profile-ca:8088/profile/device-cert
             client_cert: OU=on
             → cert: {OU=de, CN=pki-consumer}

  [4] pki-consumer → system cert (step 3, mTLS):
        POST https://profile-ca:8088/profile/system-cert
             client_cert: OU=de
             → cert: {OU=sy, CN=pki-consumer}   ← identity cert

  [5] pki-consumer → service call (mTLS, every POLL_INTERVAL):
        GET https://pki-rest-authz:9108/telemetry/latest
             client_cert: OU=sy, CN=pki-consumer

  [6] pki-rest-authz enforcement:
        TLS handshake:
          require_client_cert: true
          enforce_ou = "sy"
          cert.OU = "sy"  ✓
          identity = cert.CN = "pki-consumer"

        AuthzForce XACML request:
          subject.id = "pki-consumer"      (from cert CN)
          resource.id = "telemetry-rest"   (DEFAULT_SERVICE)
          action.id = "invoke"
          → Permit / Deny

        If Permit:
          proxy → data-provider-tls:9094 (HTTPS, TLS-verified)
          → return telemetry data to pki-consumer

  [7] Live evidence:
        pki-consumer msgCount: ${pkiStats?.msgCount ?? '…'}
        pki-rest-authz permitted: (see PKI REST tab)
        transport field: ${pkiStats?.transport ?? '…'}
`.trim()}</pre>
    </div>
  )
}

// ── Section 6: Service Registry Lookup ───────────────────────────────────────

function ServiceRegistryPanel() {
  const [serviceDef, setServiceDef] = useState('telemetry-rest')
  const [result, setResult] = useState<ServiceQueryResponse | null>(null)
  const [querying, setQuerying] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const SERVICES = ['telemetry-rest', 'telemetry', 'arrowhead-onboarding', 'arrowhead-profile']

  const query = useCallback(async () => {
    if (!serviceDef.trim()) return
    setQuerying(true)
    setErr(null)
    setResult(null)
    try {
      const r = await queryServiceRegistry(serviceDef.trim())
      setResult(r)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setQuerying(false)
    }
  }, [serviceDef])

  return (
    <div style={s.demoWrap}>
      <p style={s.body}>
        Query the ServiceRegistry for a service definition. In experiment-8, pki-consumer
        registers via ServiceRegistry lookup at startup; the profile-ca also registers its
        profile endpoints. This demonstrates the AH5.2 service discovery lifecycle.
      </p>
      <div style={s.formRow}>
        <label style={s.fLabel}>Service Definition</label>
        <input
          value={serviceDef}
          onChange={e => setServiceDef(e.target.value)}
          style={s.input}
          placeholder="e.g. telemetry-rest"
          list="svc-def-list"
          onKeyDown={e => { if (e.key === 'Enter') void query() }}
        />
        <datalist id="svc-def-list">
          {SERVICES.map(s => <option key={s} value={s} />)}
        </datalist>
        <button onClick={() => void query()} disabled={querying || !serviceDef.trim()} style={s.greenBtn}>
          {querying ? 'querying…' : 'Query ServiceRegistry'}
        </button>
      </div>
      <p style={s.hint}>
        Quick-fill: {SERVICES.map((svc, i) => (
          <span key={svc}>
            <button style={s.chipBtn} onClick={() => setServiceDef(svc)}>{svc}</button>
            {i < SERVICES.length - 1 && ' '}
          </span>
        ))}
      </p>

      {err && <div style={s.errBox}>{err}</div>}

      {result && (
        <div style={s.srResult}>
          {result.serviceQueryData && result.serviceQueryData.length > 0 ? (
            <>
              <div style={s.srCount}>{result.serviceQueryData.length} provider(s) found</div>
              {result.serviceQueryData.map((entry, i) => (
                <div key={i} style={s.srEntry}>
                  <div style={s.srProvider}>
                    <strong>{entry.providerSystem?.systemName ?? '?'}</strong>
                    {' · '}
                    {entry.serviceDefinition}
                    {' · '}
                    <code>{entry.interfaces?.join(', ') ?? '?'}</code>
                  </div>
                  {entry.providerSystem?.address && (
                    <div style={s.srAddr}>{entry.providerSystem.address}:{entry.providerSystem.port}</div>
                  )}
                </div>
              ))}
            </>
          ) : (
            <div style={s.muted}>No providers registered for <code>{serviceDef}</code></div>
          )}
        </div>
      )}
    </div>
  )
}

// ── Section 7: Extensions Comparison Table ────────────────────────────────────

function ExtensionsTable() {
  const rows = [
    {
      feature:    'Certificate hierarchy',
      ah5spec:    'Master → Org → Cloud CA (Section 6)',
      exp7:       'Flat single CA (core CA)',
      exp8:       'AH5.2 Local Cloud CA with lo→on→de→sy profiles (profile-ca)',
      added:      true,
    },
    {
      feature:    'Certificate profiles',
      ah5spec:    'Defined: onboarding (on), device (de), system (sy)',
      exp7:       'Not implemented; any cert from CA accepted',
      exp8:       'Fully enforced at profile-ca mTLS :8088',
      added:      true,
    },
    {
      feature:    'Profile enforcement at PEP',
      ah5spec:    'Implied (system cert required for service access)',
      exp7:       'Not enforced; any cert CN accepted by cert-rest-authz',
      exp8:       'pki-rest-authz enforces OU=sy before AuthzForce query',
      added:      true,
    },
    {
      feature:    'Bootstrap endpoint',
      ah5spec:    'HTTP bootstrap for initial cert (Section 6.2)',
      exp7:       'N/A (single flat CA)',
      exp8:       'HTTP :8087 /bootstrap/onboarding-cert (step 1)',
      added:      true,
    },
    {
      feature:    'Authorization model',
      ah5spec:    'JWT bearer tokens (Section 9)',
      exp7:       'XACML/AuthzForce (intentional deviation, GAP G3)',
      exp8:       'XACML/AuthzForce (same; identity input improved)',
      added:      false,
    },
    {
      feature:    'Identity source',
      ah5spec:    'X.509 cert CN (implied)',
      exp7:       'cert CN from client cert (any CA-issued cert)',
      exp8:       'cert CN from OU=sy system cert only',
      added:      true,
    },
    {
      feature:    'Service discovery',
      ah5spec:    'ServiceRegistry + Orchestrator (Section 5)',
      exp7:       'ServiceRegistry + static config',
      exp8:       'ServiceRegistry query demonstrated in this tab',
      added:      true,
    },
    {
      feature:    'Core system mTLS',
      ah5spec:    'All inter-system communication via mTLS (Section 8)',
      exp7:       'Optional TLS_PORT; plain HTTP retained for healthchecks',
      exp8:       'Same as exp-7 (partial resolution)',
      added:      false,
    },
    {
      feature:    'Certificate revocation',
      ah5spec:    'CRL required (Section 6.4)',
      exp7:       'In-memory CRL via core CA',
      exp8:       'In-memory (profile-ca); no browser-accessible revoke endpoint',
      added:      false,
    },
  ]

  return (
    <div style={s.tableWrap}>
      <table style={s.table}>
        <thead>
          <tr>
            {['Feature', 'AH5.2 Spec', 'Experiment-7', 'Experiment-8', 'Added'].map(h => (
              <th key={h} style={s.th}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i} style={{ background: r.added ? '#f0fdf4' : '#fafafa' }}>
              <td style={{ ...s.td, fontWeight: 'bold', whiteSpace: 'nowrap' }}>{r.feature}</td>
              <td style={{ ...s.td, fontSize: '0.73rem', color: '#444' }}>{r.ah5spec}</td>
              <td style={{ ...s.td, fontSize: '0.73rem', color: '#666' }}>{r.exp7}</td>
              <td style={{ ...s.td, fontSize: '0.73rem', color: '#166534', fontWeight: r.added ? 'bold' : 'normal' }}>{r.exp8}</td>
              <td style={{ ...s.td, textAlign: 'center' }}>{r.added ? '★' : '·'}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p style={s.hint}>★ = new or significantly improved in experiment-8 relative to experiment-7</p>
    </div>
  )
}

// ── Top-level view ────────────────────────────────────────────────────────────

export function PKIAddedValueView() {
  const { data: caInfo }          = usePolling<CAInfo>(fetchCAInfo, 30_000)
  const { data: pkiStats }        = usePolling<PKIConsumerStats>(fetchPKIConsumerStats, 3_000)
  const { data: pkiAuthzStatus }  = usePolling<PKIRestAuthzStatus>(fetchPKIRestAuthzStatus, 3_000)
  const { data: grants }          = usePolling<LookupResponse>(fetchAuthRules, 5_000)

  return (
    <div style={s.wrap}>

      {/* ── Intro ─────────────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>Experiment-8 Added Value — Arrowhead 5.2 Profile-Based PKI</h2>
        <p style={s.intro}>
          Experiment-8 advances experiment-7 by implementing the Arrowhead 5.2 Local Cloud CA
          with full <strong>profile-based certificate hierarchy</strong> (lo → on → de → sy).
          Identity is no longer just <em>any cert issued by the CA</em> — it is specifically a
          <em> system certificate</em> (OU=sy) obtained by climbing the full lifecycle ladder.
          Profile enforcement happens at two layers: the CA (when issuing) and the PEP (when accessing).
        </p>

        {/* Live evidence banner */}
        <div style={s.evidenceBanner}>
          <div style={s.evidenceRow}>
            <span style={s.evidenceLabel}>pki-consumer messages received</span>
            <span style={s.evidenceValue}>{pkiStats?.msgCount ?? '…'}</span>
          </div>
          <div style={s.evidenceRow}>
            <span style={s.evidenceLabel}>pki-rest-authz permitted</span>
            <span style={s.evidenceValue}>{pkiAuthzStatus?.permitted ?? '…'}</span>
          </div>
          <div style={s.evidenceRow}>
            <span style={s.evidenceLabel}>pki-rest-authz denied</span>
            <span style={s.evidenceValue}>{pkiAuthzStatus?.denied ?? '…'}</span>
          </div>
          <div style={s.evidenceRow}>
            <span style={s.evidenceLabel}>last message</span>
            <span style={s.evidenceValue}>{pkiStats ? formatTime(pkiStats.lastReceivedAt) : '…'}</span>
          </div>
        </div>
      </section>

      {/* ── 1. PKI Lifecycle ──────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>1. PKI Lifecycle Walkthrough</h3>
        <p style={s.body}>
          The Arrowhead 5.2 certificate lifecycle as implemented in experiment-8.
          Steps are sequential and enforced — no step can be skipped.
        </p>
        <LifecycleDiagram />
      </section>

      {/* ── 2. Bootstrap Demo ─────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>2. Interactive Bootstrap (Step 1)</h3>
        <BootstrapInteractive caInfo={caInfo} />
      </section>

      {/* ── 3. Profile Enforcement Table ──────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>3. Profile Enforcement — Allowed and Rejected Cases</h3>
        <p style={s.body}>
          Which certificate profiles are accepted at each endpoint. Profile-ca enforces
          sequential ladder climbing. pki-rest-authz enforces OU=sy for service access.
        </p>
        <ProfileEnforcementTable />
      </section>

      {/* ── 4. Access-Policy Lifecycle ────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>4. Access-Policy Lifecycle: Grant → Allow → Revoke → Deny → Restore → Allow</h3>
        <PolicyLifecyclePanel grants={grants} />
      </section>

      {/* ── 5. Identity Trace ─────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>5. Identity-to-Authorization Trace</h3>
        <p style={s.body}>
          How pki-consumer&apos;s certificate identity flows through the system to produce
          an authorization decision. This is the full chain from PKI lifecycle to data access.
        </p>
        <IdentityTracePanel pkiStats={pkiStats} pkiAuthzStatus={pkiAuthzStatus} />
      </section>

      {/* ── 6. Service Registry ───────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>6. Service Registry Lookup</h3>
        <p style={s.body}>
          Query the Arrowhead ServiceRegistry for service providers. In experiment-8,
          pki-consumer performs service lookup at startup before its first mTLS call.
        </p>
        <ServiceRegistryPanel />
      </section>

      {/* ── 7. Extensions Comparison ──────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>7. Experiment-8 vs AH5.2 Spec — Feature Comparison</h3>
        <ExtensionsTable />
      </section>

    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  wrap:         { fontFamily: 'monospace' },
  section:      { marginBottom: 36 },
  heading:      { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  subheading:   { fontSize: '0.85rem', fontWeight: 'bold', color: '#333', marginBottom: 8 },
  intro:        { fontSize: '0.8rem', color: '#444', lineHeight: 1.6, marginBottom: 12 },
  body:         { fontSize: '0.78rem', color: '#555', marginBottom: 10 },
  hint:         { color: '#888', fontSize: '0.75rem', marginTop: 6, lineHeight: 1.5 },
  muted:        { color: '#999', fontSize: '0.78rem', padding: '8px 0' },
  errBox:       { background: '#fff5f5', border: '1px solid #fca5a5', borderRadius: 4, padding: '6px 10px', fontSize: '0.78rem', color: '#991b1b', marginTop: 8 },

  diagram:      { background: '#0d2d20', color: '#6ee7b7', padding: 16, borderRadius: 6, fontSize: '0.72rem', lineHeight: 1.65, overflowX: 'auto' },

  evidenceBanner: {
    display: 'flex', gap: 24, flexWrap: 'wrap',
    background: '#f0fdf4', border: '1px solid #6ee7b7', borderRadius: 6,
    padding: '10px 16px', marginTop: 12,
  },
  evidenceRow:  { display: 'flex', flexDirection: 'column', gap: 2 },
  evidenceLabel:{ fontSize: '0.7rem', color: '#555' },
  evidenceValue:{ fontWeight: 'bold', fontFamily: 'monospace', color: '#047857', fontSize: '1rem' },

  demoWrap:     { background: '#f0fdf4', border: '1px solid #6ee7b7', borderRadius: 6, padding: 16, maxWidth: 720 },
  formRow:      { display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8, flexWrap: 'wrap' },
  fLabel:       { width: 100, fontSize: '0.78rem', color: '#555', flexShrink: 0 },
  input:        { flex: 1, fontFamily: 'monospace', fontSize: '0.8rem', border: '1px solid #ccc', borderRadius: 4, padding: '4px 8px', minWidth: 120 },
  greenBtn:     { fontFamily: 'monospace', fontSize: '0.78rem', cursor: 'pointer', background: '#047857', color: '#fff', border: 'none', borderRadius: 4, padding: '6px 14px', flexShrink: 0 },
  caHint:       { background: '#dcfce7', borderRadius: 4, padding: '4px 10px', fontSize: '0.75rem', color: '#166534', marginBottom: 10 },

  certBox:      { background: '#fff', border: '1px solid #6ee7b7', borderRadius: 4, padding: 12, marginTop: 10 },
  certHeaderRow:{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8 },
  profileBadge: { padding: '2px 10px', borderRadius: 10, fontSize: '0.75rem', fontWeight: 'bold' },
  certNote:     { fontSize: '0.72rem', color: '#666' },
  certRow:      { display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4, fontSize: '0.72rem', color: '#666' },
  certLabel:    { width: 80, flexShrink: 0 },
  pem:          { background: '#1a1a2e', color: '#a0c4ff', padding: '8px 12px', borderRadius: 4, fontSize: '0.68rem', lineHeight: 1.5, overflowX: 'auto', margin: '4px 0' },
  toggleBtn:    { fontFamily: 'monospace', fontSize: '0.7rem', background: 'transparent', border: '1px solid #ccc', borderRadius: 3, cursor: 'pointer', padding: '1px 6px', color: '#555' },

  steps:        { marginTop: 16 },
  step:         { display: 'flex', alignItems: 'flex-start', gap: 8, marginBottom: 8, fontSize: '0.78rem' },
  stepBadge:    { padding: '2px 8px', borderRadius: 10, fontSize: '0.72rem', fontWeight: 'bold', flexShrink: 0 },
  stepDesc:     { flex: 1, color: '#333' },
  stepNote:     { fontSize: '0.7rem', color: '#888', flexShrink: 0 },

  tableWrap:    { overflowX: 'auto', marginBottom: 4 },
  table:        { borderCollapse: 'collapse', fontSize: '0.78rem', width: '100%' },
  th:           { textAlign: 'left', padding: '6px 12px', borderBottom: '2px solid #ddd', color: '#555', fontWeight: 'bold', background: '#f9fafb' },
  td:           { padding: '6px 12px', borderBottom: '1px solid #f0f0f0', color: '#333', verticalAlign: 'top' },

  lifecycleWrap:{ background: '#f9f9ff', border: '1px solid #c4b5fd', borderRadius: 6, padding: 16, maxWidth: 680 },
  lifecycleSteps:{ display: 'flex', alignItems: 'flex-start', gap: 4, flexWrap: 'wrap', marginBottom: 16 },
  lcStep:       { display: 'flex', gap: 8, alignItems: 'flex-start', flex: '1 1 120px', fontSize: '0.75rem' },
  lcNum:        { background: '#6d28d9', color: '#fff', borderRadius: '50%', width: 20, height: 20, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: '0.7rem', fontWeight: 'bold', flexShrink: 0 },
  lcArrow:      { color: '#c4b5fd', fontSize: '1.2rem', flexShrink: 0, lineHeight: '20px', marginTop: 0 },
  lcNote:       { color: '#888', fontSize: '0.7rem' },
  grantHint:    { fontSize: '0.78rem', marginBottom: 10, padding: '6px 10px', background: '#f0f0f0', borderRadius: 4 },
  checkInline:  { display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8, flexWrap: 'wrap' },
  inputSm:      { fontFamily: 'monospace', fontSize: '0.78rem', border: '1px solid #ccc', borderRadius: 4, padding: '4px 8px', width: 140 },
  checkBtn:     { fontFamily: 'monospace', fontSize: '0.78rem', cursor: 'pointer', background: '#5b21b6', color: '#fff', border: 'none', borderRadius: 4, padding: '5px 12px' },
  resultRow:    { border: '1px solid', borderRadius: 6, padding: '8px 12px', marginTop: 4, display: 'flex', alignItems: 'center', gap: 8 },
  decisionBadge:{ padding: '2px 12px', borderRadius: 12, fontSize: '0.8rem', fontWeight: 'bold' },
  resultNote:   { fontSize: '0.75rem', color: '#555' },

  traceWrap:    { background: '#0d2d20', border: '1px solid #065f46', borderRadius: 6, overflowX: 'auto', maxWidth: 760 },
  traceCode:    { color: '#6ee7b7', padding: 16, fontSize: '0.72rem', lineHeight: 1.65, margin: 0 },

  srResult:     { background: '#fff', border: '1px solid #6ee7b7', borderRadius: 4, padding: 12, marginTop: 8 },
  srCount:      { fontSize: '0.75rem', color: '#047857', marginBottom: 8, fontWeight: 'bold' },
  srEntry:      { borderBottom: '1px solid #e0e0e0', padding: '6px 0', marginBottom: 4 },
  srProvider:   { fontSize: '0.78rem', color: '#333' },
  srAddr:       { fontSize: '0.72rem', color: '#888', marginTop: 2 },

  chipBtn:      { fontFamily: 'monospace', fontSize: '0.72rem', cursor: 'pointer', background: '#dcfce7', color: '#065f46', border: 'none', borderRadius: 3, padding: '1px 6px' },
}
