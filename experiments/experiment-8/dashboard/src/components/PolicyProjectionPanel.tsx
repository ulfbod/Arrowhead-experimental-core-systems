// Shows the AuthzForce policy state and the per-transport enforcement status.

import { usePolling } from '../hooks/usePolling'
import { fetchPolicySyncStatus, fetchKafkaAuthzStatus, fetchPKIRestAuthzStatus } from '../api'

function formatTime(iso: string): string {
  if (!iso) return '—'
  try { return new Date(iso).toLocaleTimeString() } catch { return iso }
}

export function PolicyProjectionPanel() {
  const { data: syncStatus, error: syncErr, stale: syncStale } =
    usePolling(fetchPolicySyncStatus, 5_000)
  const { data: kafkaStatus } =
    usePolling(fetchKafkaAuthzStatus, 5_000)
  const { data: restStatus } =
    usePolling(fetchPKIRestAuthzStatus, 5_000)

  const syncOk = syncStatus?.synced ?? false

  return (
    <div style={s.wrap}>

      {/* ── Policy-sync state ─────────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>
          AuthzForce Policy State
          {syncStale && <span style={s.stale}> (stale)</span>}
        </h2>
        {syncErr && !syncStale && <p style={s.err}>{syncErr}</p>}

        <div style={s.statusRow}>
          <span style={{ ...s.badge, background: syncOk ? '#e8f5e9' : '#fff3e0', color: syncOk ? '#388e3c' : '#f57c00' }}>
            {syncOk ? 'synced' : 'pending'}
          </span>
          <span style={s.meta}>
            version {syncStatus?.version ?? '…'}
            {' · '}
            {syncStatus?.grants ?? '…'} grant{syncStatus?.grants !== 1 ? 's' : ''}
            {' · '}
            last synced {formatTime(syncStatus?.lastSyncedAt ?? '')}
            {syncStatus?.syncInterval && (
              <>{' · '}interval {syncStatus.syncInterval}</>
            )}
          </span>
        </div>
        {syncStatus?.error && <p style={s.err}>Last error: {syncStatus.error}</p>}

        <p style={s.hint}>
          policy-sync fetches CA grants every SYNC_INTERVAL (configurable on Config tab),
          compiles them into a XACML 3.0 PolicySet, and uploads it to AuthzForce (domain: arrowhead-exp8).
          All three PEPs query the same AuthzForce domain for enforcement decisions.
        </p>
      </section>

      {/* ── Transport enforcement ─────────────────────────────────────────── */}
      <section style={s.section}>
        <h2 style={s.heading}>Transport Enforcement</h2>

        <div style={s.transportGrid}>
          {/* AMQP path */}
          <div style={s.transportCard}>
            <div style={s.transportTitle}>AMQP / RabbitMQ</div>
            <div style={s.transportBody}>
              <div style={s.row}>
                <span style={s.rowLabel}>PEP</span>
                <span style={s.rowValue}>topic-auth-xacml</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>protocol</span>
                <span style={s.rowValue}>AMQP-INSECURE-JSON</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>enforcement</span>
                <span style={s.rowValue}>per broker operation</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>cache TTL</span>
                <span style={s.rowValue}>0s (live AuthzForce query)</span>
              </div>
              <div style={s.divider} />
              <div style={s.row}>
                <span style={s.rowLabel}>management UI</span>
                <a
                  href={`http://${window.location.hostname}:15677`}
                  target="_blank"
                  rel="noopener noreferrer"
                  style={s.link}
                >
                  {window.location.hostname}:15677 ↗
                </a>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>credentials</span>
                <span style={s.rowValue}>admin / admin</span>
              </div>
            </div>
          </div>

          {/* Kafka path */}
          <div style={s.transportCard}>
            <div style={s.transportTitle}>Kafka / SSE</div>
            <div style={s.transportBody}>
              <div style={s.row}>
                <span style={s.rowLabel}>PEP</span>
                <span style={s.rowValue}>kafka-authz</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>protocol</span>
                <span style={s.rowValue}>KAFKA-INSECURE-JSON (SSE)</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>enforcement</span>
                <span style={s.rowValue}>on connect + every 100 msgs</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>active streams</span>
                <span style={s.rowValue}>{kafkaStatus?.activeStreams ?? '…'}</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>total served</span>
                <span style={s.rowValue}>{kafkaStatus?.totalServed ?? '…'}</span>
              </div>
              <div style={s.divider} />
              <div style={s.row}>
                <span style={s.rowLabel}>browser UI</span>
                <span style={{ ...s.rowValue, color: '#7c3aed' }}>→ Kafka tab</span>
              </div>
            </div>
          </div>

          {/* REST path */}
          <div style={{ ...s.transportCard, borderColor: '#fde68a' }}>
            <div style={{ ...s.transportTitle, color: '#6d28d9' }}>REST / mTLS (PKI)</div>
            <div style={s.transportBody}>
              <div style={s.row}>
                <span style={s.rowLabel}>PEP</span>
                <span style={s.rowValue}>pki-rest-authz</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>identity</span>
                <span style={s.rowValue}>cert CN (OU=sy required)</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>enforcement</span>
                <span style={s.rowValue}>per request</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>sync delay</span>
                <span style={s.rowValue}>up to SYNC_INTERVAL</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>requests</span>
                <span style={s.rowValue}>{restStatus?.requestsTotal ?? '…'}</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>permitted</span>
                <span style={s.rowValue}>{restStatus?.permitted ?? '…'}</span>
              </div>
              <div style={s.row}>
                <span style={s.rowLabel}>denied</span>
                <span style={s.rowValue}>{restStatus?.denied ?? '…'}</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* ── Unified policy diagram ────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>Policy Projection Model</h3>
        <pre style={s.diagram}>{`
  ConsumerAuthorization (CA)   ←  pki-consumer (CN=pki-consumer, OU=sy)
        │ grants/revokes
        ▼
  policy-sync ──► AuthzForce PDP/PAP  (XACML 3.0 · domain: arrowhead-exp8)
                       │
           ┌───────────┼───────────────┐
           │           │               │
           ▼           ▼               ▼
  topic-auth-xacml  kafka-authz    pki-rest-authz
  (RabbitMQ AMQP)   (Kafka SSE)   (mTLS proxy)
           │           │               │  identity = cert CN
           ▼           ▼               ▼  cert must have OU=sy (AH5.2 system profile)
  Consumer-1/2/3  analytics-consumer  pki-consumer
  (AMQP)          (SSE / Kafka)       (mTLS REST, PKI lifecycle cert)
`.trim()}</pre>
        <p style={s.hint}>
          Sync-delay caveat: REST enforcement lags CA by up to SYNC_INTERVAL.
          pki-consumer identity is derived from its system certificate CN (OU=sy),
          obtained via the full on→de→sy PKI lifecycle at startup.
        </p>
      </section>
    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  wrap:          { fontFamily: 'monospace' },
  section:       { marginBottom: 28 },
  heading:       { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  subheading:    { fontSize: '0.85rem', fontWeight: 'bold', color: '#333', marginBottom: 8 },
  err:           { color: '#f44336', fontSize: '0.8rem' },
  stale:         { color: '#ff9800', fontWeight: 'normal', fontSize: '0.75rem' },
  hint:          { color: '#888', fontSize: '0.75rem', marginTop: 6 },
  statusRow:     { display: 'flex', alignItems: 'center', gap: 12, marginBottom: 8 },
  badge:         { padding: '2px 10px', borderRadius: 12, fontSize: '0.75rem', fontWeight: 'bold' },
  meta:          { fontSize: '0.78rem', color: '#555' },
  transportGrid: { display: 'flex', gap: 16, flexWrap: 'wrap' },
  transportCard: {
    flex: '1 1 240px', background: '#fff', border: '1px solid #c4b5fd',
    borderRadius: 6, padding: '12px 16px',
  },
  divider: { borderTop: '1px solid #eee', margin: '8px 0' },
  link:    { color: '#5b6af0', textDecoration: 'none', fontFamily: 'monospace', fontSize: '0.78rem', fontWeight: 'bold' },
  transportTitle: { fontWeight: 'bold', fontSize: '0.85rem', marginBottom: 10, color: '#1a1a2e' },
  transportBody:  {},
  row:           { display: 'flex', justifyContent: 'space-between', gap: 8, marginBottom: 4, fontSize: '0.78rem' },
  rowLabel:      { color: '#888' },
  rowValue:      { fontWeight: 'bold', color: '#333', textAlign: 'right' },
  diagram:       {
    background: '#f8f8f8', border: '1px solid #ddd', borderRadius: 4,
    padding: 16, fontSize: '0.75rem', lineHeight: 1.6, overflowX: 'auto',
  },
}
