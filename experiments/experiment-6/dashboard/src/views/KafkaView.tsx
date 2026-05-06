// KafkaView — Kafka transport monitoring and authorization check tool.
//
// Kafka does not ship with a browser management UI comparable to RabbitMQ's.
// This view exposes the kafka-authz PEP state and an interactive authorization
// check tool so operators can inspect and test Kafka access decisions in real time.

import { useState, useCallback } from 'react'
import { usePolling } from '../hooks/usePolling'
import { fetchKafkaAuthzStatus, checkKafkaAuthz } from '../api'
import type { KafkaAuthzStatus, KafkaAuthCheckResult } from '../types'

// Well-known consumers in this experiment — offered as quick-fill shortcuts.
const KNOWN_CONSUMERS = [
  'demo-consumer-1',
  'demo-consumer-2',
  'demo-consumer-3',
  'analytics-consumer',
  'rest-consumer',
  'test-probe',
]

// ── Status card ───────────────────────────────────────────────────────────────

function StatusCard({ status }: { status: KafkaAuthzStatus | null }) {
  return (
    <div style={s.card}>
      <div style={s.cardTitle}>kafka-authz</div>
      <div style={{ ...s.transport }}>Kafka / SSE — PEP</div>
      <div style={s.stat}>
        <span style={s.statLabel}>active SSE streams</span>
        <span style={s.statValue}>{status?.activeStreams ?? '…'}</span>
      </div>
      <div style={s.stat}>
        <span style={s.statLabel}>total messages served</span>
        <span style={s.statValue}>{status?.totalServed ?? '…'}</span>
      </div>
      <p style={s.hint}>
        Each authorized consumer opens one SSE stream. The count drops to 0 when no
        consumer is connected and increments again on reconnect.
      </p>
    </div>
  )
}

// ── Authorization check tool ──────────────────────────────────────────────────

function AuthCheckTool() {
  const [consumer, setConsumer] = useState('analytics-consumer')
  const [service,  setService]  = useState('telemetry')
  const [result,   setResult]   = useState<KafkaAuthCheckResult | null>(null)
  const [checking, setChecking] = useState(false)
  const [err,      setErr]      = useState<string | null>(null)

  const check = useCallback(async () => {
    if (!consumer.trim() || !service.trim()) return
    setChecking(true)
    setErr(null)
    setResult(null)
    try {
      const r = await checkKafkaAuthz(consumer.trim(), service.trim())
      setResult(r)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setChecking(false)
    }
  }, [consumer, service])

  return (
    <div style={s.checkWrap}>
      <div style={s.checkRow}>
        <label style={s.label}>Consumer</label>
        <input
          value={consumer}
          onChange={e => setConsumer(e.target.value)}
          style={s.input}
          placeholder="consumer name"
          list="known-consumers"
        />
        <datalist id="known-consumers">
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
        onClick={check}
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
          <span style={s.resultMeta}>
            {result.consumer} → {result.service}
          </span>
          <p style={s.hint}>
            {result.permit
              ? 'A SUBSCRIBE to this service is permitted. The consumer can open an SSE stream.'
              : 'Access denied. No matching grant in AuthzForce. Revoke/add a grant and check again.'}
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

// ── Architecture diagram ──────────────────────────────────────────────────────

function KafkaDiagram() {
  return (
    <pre style={s.diagram}>{`
  analytics-consumer
  (or any authorized consumer)
        │
        │  GET /stream/{consumerName}?service=telemetry
        │  (Server-Sent Events)
        ▼
  ┌─────────────────────────────┐
  │  kafka-authz  (PEP)         │
  │  port 9091                  │
  │                             │
  │  1. POST AuthzForce /pdp    │──► AuthzForce PDP
  │     XACML decision          │◄── Permit / Deny
  │                             │
  │  2a. Deny  → 403 Forbidden  │
  │  2b. Permit → subscribe to  │
  │      Kafka topic            │
  └─────────────────────────────┘
        │  (Permit path)
        │  partition reader, offset 0
        ▼
  Kafka broker  (arrowhead.telemetry)
  ← robot-fleet publishes here
        │
        │  SSE event: data: {...}
        ▼
  analytics-consumer receives telemetry

  Re-check: every 100 messages kafka-authz re-queries
  AuthzForce. A revoked grant causes a "revoked" SSE
  event and stream teardown on the next 100-message mark.
`.trim()}</pre>
  )
}

// ── Top-level view ────────────────────────────────────────────────────────────

export function KafkaView() {
  const { data: kafkaStatus } = usePolling<KafkaAuthzStatus>(fetchKafkaAuthzStatus, 3_000)

  return (
    <div style={s.wrap}>

      <section style={s.section}>
        <h2 style={s.heading}>Kafka Transport Monitor</h2>
        <p style={s.intro}>
          Kafka does not ship with a browser management UI. This view exposes
          the <strong>kafka-authz</strong> PEP state and an interactive authorization
          check tool. kafka-authz is the Policy Enforcement Point that sits between
          SSE consumers and the Kafka broker — it queries AuthzForce on connect
          and again every 100 messages to enforce the active XACML policy.
        </p>
      </section>

      {/* ── Live status ─────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>Live Status</h3>
        <div style={s.cardRow}>
          <StatusCard status={kafkaStatus} />
        </div>
      </section>

      {/* ── Auth check tool ──────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>Authorization Check</h3>
        <p style={s.body}>
          Ask AuthzForce (via kafka-authz) whether a given consumer is currently
          permitted to subscribe to a service. This reflects the live XACML policy —
          the same decision an SSE connect attempt would receive right now.
        </p>
        <AuthCheckTool />
      </section>

      {/* ── Architecture ─────────────────────────────────────────────────────── */}
      <section style={s.section}>
        <h3 style={s.subheading}>Kafka Path Architecture</h3>
        <KafkaDiagram />
      </section>

    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  wrap:         { fontFamily: 'monospace' },
  section:      { marginBottom: 28 },
  heading:      { fontSize: '0.9rem', marginBottom: 8, color: '#555' },
  subheading:   { fontSize: '0.85rem', fontWeight: 'bold', color: '#333', marginBottom: 8 },
  intro:        { fontSize: '0.8rem', color: '#444', lineHeight: 1.6, marginBottom: 0 },
  body:         { fontSize: '0.78rem', color: '#555', marginBottom: 10 },
  hint:         { color: '#888', fontSize: '0.75rem', marginTop: 6 },

  cardRow:      { display: 'flex', gap: 16, flexWrap: 'wrap' },
  card:         {
    background: '#fff', border: '1px solid #a5f3fc', borderRadius: 6,
    padding: '12px 16px', minWidth: 240, flex: '0 1 320px',
  },
  cardTitle:    { fontWeight: 'bold', fontSize: '0.85rem', marginBottom: 4, color: '#1a1a2e' },
  transport:    { fontSize: '0.7rem', color: '#0891b2', marginBottom: 10 },
  stat:         { display: 'flex', justifyContent: 'space-between', gap: 8, marginBottom: 4, fontSize: '0.78rem' },
  statLabel:    { color: '#888' },
  statValue:    { fontWeight: 'bold', color: '#333', fontFamily: 'monospace' },

  checkWrap:    { background: '#f9f9ff', border: '1px solid #e0e0f0', borderRadius: 6, padding: 16, maxWidth: 480 },
  checkRow:     { display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 },
  label:        { width: 70, fontSize: '0.78rem', color: '#555', flexShrink: 0 },
  input:        {
    flex: 1, fontFamily: 'monospace', fontSize: '0.8rem',
    border: '1px solid #ccc', borderRadius: 4, padding: '4px 8px',
  },
  btn:          {
    fontFamily: 'monospace', fontSize: '0.8rem', cursor: 'pointer',
    background: '#1a1a2e', color: '#fff', border: 'none',
    borderRadius: 4, padding: '6px 16px', marginBottom: 10,
  },
  resultBox:    {
    border: '1px solid', borderRadius: 6, padding: '10px 14px',
    marginTop: 8, marginBottom: 6,
  },
  decisionBadge: {
    display: 'inline-block', padding: '2px 12px', borderRadius: 12,
    fontSize: '0.8rem', fontWeight: 'bold', marginBottom: 6,
  },
  resultMeta:   { fontSize: '0.75rem', color: '#555', marginLeft: 10 },
  errBox:       { background: '#fff5f5', border: '1px solid #fca5a5', borderRadius: 4, padding: '6px 10px', fontSize: '0.78rem', color: '#991b1b' },
  chipBtn:      {
    fontFamily: 'monospace', fontSize: '0.72rem', cursor: 'pointer',
    background: '#ede9fe', color: '#5b21b6', border: 'none',
    borderRadius: 3, padding: '1px 6px',
  },
  diagram:      {
    background: '#f8f8f8', border: '1px solid #ddd', borderRadius: 4,
    padding: 16, fontSize: '0.75rem', lineHeight: 1.6, overflowX: 'auto',
  },
}
