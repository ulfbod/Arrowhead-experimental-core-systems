# Experiment 11 — Architecture Diagrams

## System overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Arrowhead Cloud (UC3)                               │
│                                                                             │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐   ┌────────────┐  │
│  │ServiceRegistry│  │Authentication│   │ConsumerAuth  │   │DynamicOrch │  │
│  │  TLS :8490   │  │  TLS :8491   │   │  TLS :8492   │   │ TLS :8493  │  │
│  └──────────────┘  └──────────────┘   └──────┬───────┘   └────────────┘  │
│                                              │                              │
│                            ┌─────────────────┘ poll /authorization/lookup  │
│                            ▼  (every SYNC_INTERVAL)                        │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                    Policy Plane (Strategy A)                          │ │
│  │                                                                       │ │
│  │  ┌──────────────────────────────┐   ┌──────────────────────────────┐ │ │
│  │  │  PAP :9405                   │   │  PIP :9406                   │ │ │
│  │  │  (Policy Admin Point)        │◀──│  (Policy Info Point)         │ │ │
│  │  │                              │   │                              │ │ │
│  │  │  Stores native policies.     │   │  Caches ConsumerAuth grants. │ │ │
│  │  │  On Create/Delete:           │   │  Version increments only on  │ │ │
│  │  │    triggerPush() → AF        │   │  content change.             │ │ │
│  │  │  Background loop:            │   │                              │ │ │
│  │  │    SyncFromPIP() when        │   │  GET /grants                 │ │ │
│  │  │    PIP version changes       │   │  GET /grants?subject=X       │ │ │
│  │  │                              │   │  GET /status                 │ │ │
│  │  │  POST   /policies            │   └──────────────────────────────┘ │ │
│  │  │  GET    /policies            │                │                   │ │
│  │  │  GET    /policies/{id}       │                │ merge + dedup     │ │
│  │  │  DELETE /policies/{id}       │                ▼                   │ │
│  │  │  GET    /status              │   ┌──────────────────────────────┐ │ │
│  │  └──────────────────────────────┘   │  AuthzForce PDP :8080        │ │ │
│  │             │                       │  XACML PolicySet             │ │ │
│  │             └──────────────────────▶│  (urn:arrowhead:exp11:pap)   │ │ │
│  │                   SetPolicy()       └──────────────────────────────┘ │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                    ▲                                        │
│                    XACML Decide    │                                        │
│  ┌─────────────────────────────────┴───────────────────────────────────┐   │
│  │                       Enforcement Plane (PEP)                       │   │
│  │                                                                     │   │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────┐ │   │
│  │  │ kafka-authz      │  │ topic-auth-xacml│  │ pki-rest-authz      │ │   │
│  │  │ (Kafka PEP)      │  │ (AMQP PEP)      │  │ (REST mTLS PEP)     │ │   │
│  │  │ :9101            │  │ :9090           │  │ :9208 (TLS)         │ │   │
│  │  └────────┬─────────┘  └────────┬────────┘  └──────────┬──────────┘ │   │
│  └───────────┼────────────────────-┼─────────────────────-┼────────────┘   │
│              │                     │                       │                │
│  ┌───────────▼─────────────────────▼───────────────────────▼────────────┐  │
│  │                        Data Plane (UC3)                               │  │
│  │                                                                       │  │
│  │  Robot Fleet   ──Kafka+AMQP TLS──▶  Portal & Cloud ML  ──REST mTLS──▶│  │
│  │  Sites 1/2/3                         (aggregator)        Service     │  │
│  │                                                          Partners    │  │
│  │                                                          SP1/SP2     │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Policy merge flow

```
ConsumerAuth /authorization/lookup
        │
        │  (every 10 s)
        ▼
  PIP.fetchAndUpdate()
        │
        │  if content changed → version++
        ▼
  PIP grant cache (versioned)
        │
        │  PAP SyncFromPIP() detects version change
        ▼
  PAP.Push(nativePolicies, pipGrants)
        │
        │  1. Include native Permit rules
        │  2. Add PIP grants not already covered (dedup by subject+resource)
        │  3. Build XACML PolicySet
        ▼
  AuthzForce.SetPolicy(domainID, xacmlXML)
```

## Timing comparison: Strategy A vs B

```
Strategy A (experiment-11 — Hybrid)
────────────────────────────────────────────────────────────
Event: ConsumerAuth grant added
  t=0s   ConsumerAuth grant stored
  t≤10s  PIP detects change (poll interval)
  t≤10s  PAP SyncFromPIP() triggered
  t≤10s  AuthzForce updated → authorization takes effect

Event: PAP-native policy added/deleted
  t=0s   PAP policy stored + triggerPush()
  t~0s   AuthzForce updated → authorization takes effect (instant)


Strategy B (experiment-10 — PAP-only)
────────────────────────────────────────────────────────────
Event: Any policy change
  t=0s   PAP policy stored + direct push
  t~0s   AuthzForce updated → authorization takes effect (instant)

  (ConsumerAuth is only used for DynamicOrch — not as a policy source)
```

## Two-source policy effect table

| Subject/Resource | PAP-native | PIP grant | AuthzForce effect |
|---|---|---|---|
| sp1/svc | Permit | (any) | Permit |
| sp1/svc | Deny | (any) | **Deny** (native takes precedence, but Deny excluded from XACML grant list — effective Deny because no Permit rule exists) |
| sp1/svc | — | present | Permit (from PIP) |
| sp1/svc | — | — | Deny (NotApplicable → default Deny) |
| sp1/svc | Permit | same sp1/svc | Permit (deduplicated — one rule) |
