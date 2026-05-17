# Experiment 14 — Architecture Diagrams

## Connection-Time Revocation Enforcement (D2')

### Kafka: TLS handshake → ArrowheadPrincipalBuilder

```
┌─────────────────┐     TLS handshake      ┌─────────────────────────────┐
│ Kafka Client    │ ──────────────────────▶ │ Kafka Broker                │
│ (cert: CN=foo)  │                         │                             │
│                 │                         │  ArrowheadPrincipalBuilder  │
│                 │                         │  .build(SslAuthContext)     │
│                 │                         │      │                      │
│                 │                         │      ▼                      │
│                 │                         │  extractCN(peerCert)        │
│                 │                         │      │ CN = "foo"           │
│                 │                         │      ▼                      │
│                 │     GET /pip/attributes/foo    │                      │
│                 │                 ┌───────┤ PipCertValidityChecker      │
│                 │                 ▼       │      │                      │
│                 │          ┌───────────┐  │      │                      │
│                 │          │   PIP     │  │      │                      │
│                 │          │ valid=?   │  │      │                      │
│                 │          └─────┬─────┘  │      │                      │
│                 │                │        └──────┤                      │
│                 │          valid=false           │ throw                │
│                 │◀── AuthenticationException ────┤ KafkaException       │
│  (rejected)     │                                │                      │
└─────────────────┘                          valid=true                   │
                                                   │                      │
                                                   ▼                      │
                                          KafkaPrincipal(USER, "foo")     │
                                          → kafka-authz (message-level)   │
                                                                           │
                                          ┌────────────────────────────┐  │
                                          │ kafka-authz                │◀─┘
                                          │ GET /pip/attributes/foo    │
                                          │ POST /pdp → AuthzForce     │
                                          │ Permit → consume allowed   │
                                          └────────────────────────────┘
```

### RabbitMQ: AMQP connect → topic-auth-xacml D2' pre-gate

```
┌─────────────────┐   AMQP CONNECT (mTLS)   ┌──────────────┐
│ AMQP Client     │ ──────────────────────▶  │  RabbitMQ    │
│ user=CN, pw=*** │                           │              │
└─────────────────┘                           │  POST /auth/user
                                              │ ──────────────▶ ┌──────────────────────┐
                                              │                  │ topic-auth-xacml     │
                                              │                  │                      │
                                              │                  │ 1. password check    │
                                              │                  │    wrong? → "deny"   │
                                              │                  │                      │
                                              │                  │ 2. D2' pre-gate      │
                                              │                  │    GET /pip/attr/CN  │
                                              │                  │         │            │
                                              │                  │    ┌────▼──────┐     │
                                              │                  │    │   PIP     │     │
                                              │                  │    │ valid=?   │     │
                                              │                  │    └────┬──────┘     │
                                              │                  │         │            │
                                              │                  │  valid=false         │
                                              │                  │    → "deny"          │
                                              │                  │    (PDP not called)  │
                                              │                  │                      │
                                              │   "deny"         │  valid=true          │
                                              │ ◀────────────────│    → 3. AuthzForce   │
                         connection rejected  │                  │    → "allow"/"deny"  │
                                              │                  └──────────────────────┘
                                              │
                                              │  POST /auth/vhost  (same D2' pre-gate)
                                              │ ──────────────▶ topic-auth-xacml
```

## Enforcement Layering

```
┌─────────────────────────────────────────────────────────────────────┐
│                   Enforcement Points (exp-14)                       │
│                                                                     │
│  Layer 0: PKI / TLS                                                 │
│    - RabbitMQ: fail_if_no_peer_cert (cert must be CA-signed)       │
│    - Kafka: ssl.client.auth=required (same)                         │
│    → Rejects structurally invalid/unsigned certs                   │
│                                                                     │
│  Layer 1: Connection-time cert validity (D2') ← NEW in exp-14      │
│    - RabbitMQ: topic-auth-xacml PIP pre-gate in handleUser/Vhost   │
│    - Kafka: ArrowheadPrincipalBuilder plugin                        │
│    → Rejects revoked certs (certValid=false in PIP)                │
│    → Fail-closed: PIP unreachable = denied                         │
│    → PDP (AuthzForce) never called for revoked certs               │
│                                                                     │
│  Layer 2: Policy / grant enforcement (AuthzForce)                  │
│    - RabbitMQ: topic-auth-xacml → AuthzForce after Layer 1         │
│    - Kafka: kafka-authz → PIP → AuthzForce (message-level)         │
│    → Rejects consumers without a Permit policy                     │
│                                                                     │
│  Layer 3: Proactive revocation (periodic loop)                     │
│    - topic-auth-xacml revocation loop checks PIP + PDP             │
│    - Forces disconnect on already-open connections                  │
│    → Closes connections when cert revoked mid-session              │
└─────────────────────────────────────────────────────────────────────┘
```

## Comparison: exp-13 vs exp-14

```
exp-13 flow (D2 — cert-valid forwarded to AuthzForce):
  Client → mTLS → AMQP connect → /auth/user →
    password OK → decide() → PIP → AuthzForce(certValid=false, grant=Permit)
    → AuthzForce may return Permit (policy ignores certValid) → ALLOWED

exp-14 flow (D2' — cert-valid pre-gate):
  Client → mTLS → AMQP connect → /auth/user →
    password OK → PIP(certValid?) →
      certValid=false → "deny" immediately (PDP never called)
      certValid=true  → decide() → AuthzForce → "allow"/"deny"
```
