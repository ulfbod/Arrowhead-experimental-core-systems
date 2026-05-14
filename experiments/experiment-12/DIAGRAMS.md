# Experiment 12 — Architecture Diagrams

## System overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Arrowhead Cloud (UC3)                               │
│                                                                             │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐                    │
│  │ServiceRegistry│  │Authentication│   │ConsumerAuth  │  (AH5 spec only —  │
│  │  TLS :8490   │  │  TLS :8491   │   │  TLS :8492   │   not called for   │
│  └──────────────┘  └──────────────┘   └──────────────┘   authorization)   │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │         DynamicOrch-XACML  :8083  (core-evol — Approach B)          │  │
│  │                                                                      │  │
│  │  1. Validate request                                                 │  │
│  │  2. POST /serviceregistry/query → all providers of service          │  │
│  │  3. AuthzForce.Decide(domain, consumer, service, "consume")         │  │
│  │     Permit → return all providers                                   │  │
│  │     Deny/error → return [] (fail-closed)                            │  │
│  └──────────────────────────┬───────────────────────────────────────────┘  │
│                             │ single XACML call                             │
│                             ▼                                               │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                    Policy Plane (Strategy B — exp-10)                 │ │
│  │                                                                       │ │
│  │  ┌──────────────────────────────┐   ┌──────────────────────────────┐ │ │
│  │  │  PAP :9505                   │   │  PIP :9506                   │ │ │
│  │  │  (Policy Admin Point)        │   │  (Policy Info Point)         │ │ │
│  │  │                              │   │                              │ │ │
│  │  │  Single source of truth.     │   │  Subject attribute registry  │ │ │
│  │  │  On Create/Delete:           │   │  (cert level per system).    │ │ │
│  │  │    triggerPush() → AF        │   │  Not used for policy source  │ │ │
│  │  │    (instant — no sync lag)   │   │  in this experiment.         │ │ │
│  │  │                              │   └──────────────────────────────┘ │ │
│  │  │  POST   /policies            │                                    │ │
│  │  │  GET    /policies            │                                    │ │
│  │  │  GET    /policies/{id}       │                                    │ │
│  │  │  DELETE /policies/{id}       │                                    │ │
│  │  │  GET    /status              │                                    │ │
│  │  └──────────────────────────────┘                                    │ │
│  │             │ SetPolicy()                                             │ │
│  │             ▼                                                         │ │
│  │  ┌──────────────────────────────┐                                    │ │
│  │  │  AuthzForce PDP :8080        │                                    │ │
│  │  │  XACML PolicySet             │                                    │ │
│  │  │  (urn:arrowhead:exp12:pap)   │                                    │ │
│  │  └──────────────────────────────┘                                    │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                       ▲                                                     │
│         XACML Decide  │                                                     │
│  ┌────────────────────┴──────────────────────────────────────────────────┐  │
│  │                       Enforcement Plane (PEP)                         │  │
│  │                                                                       │  │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────┐  │  │
│  │  │ kafka-authz      │  │ topic-auth-xacml│  │ pki-rest-authz      │  │  │
│  │  │ (Kafka PEP)      │  │ (AMQP PEP)      │  │ (REST mTLS PEP)     │  │  │
│  │  │ :9101            │  │ :9090           │  │ :9208 (TLS)         │  │  │
│  │  └────────┬─────────┘  └────────┬────────┘  └──────────┬──────────┘  │  │
│  └───────────┼────────────────────-┼─────────────────────-┼─────────────┘  │
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

## Orchestration flow — Approach B vs A

```
Approach B (experiment-12 — DynamicOrch-XACML)
────────────────────────────────────────────────────────────
Consumer: POST /orchestration/dynamic {requester, service}

  DynamicOrch-XACML:
    1. POST /serviceregistry/query {serviceDefinition}
       → N providers from SR

    2. AuthzForce.Decide(domain, consumer, service, "consume")
       → single XACML call (not per-provider)

    3a. Permit → return all N providers
    3b. Deny   → return [] (fail-closed)
    3c. Error  → return [] (fail-closed)

  Total external calls: 1 (SR) + 1 (AF) = 2

AH5 / Approach A (experiment-10 — DynamicOrch core)
────────────────────────────────────────────────────────────
Consumer: POST /orchestration/dynamic {requester, service}

  DynamicOrchestrator:
    1. POST /serviceregistry/query {serviceDefinition}
       → N providers from SR

    2. For each provider:
         POST /authorization/verify {consumer, provider, service}
         → boolean authorized

    3. Filter: keep only authorized providers
    4. Return filtered list

  Total external calls: 1 (SR) + N (CA) = N+1
  (N grows with the number of providers)
```

## Authority unification

```
Experiment-10 (split authority):
────────────────────────────────────────────────────────
  Orchestration decision: ConsumerAuthorization grants
  Enforcement decision:   AuthzForce XACML policies

  → Two systems to update per policy change.
  → Risk: PAP revokes, CA grants remain → stale endpoints served.

Experiment-12 (unified authority):
────────────────────────────────────────────────────────
  Orchestration decision: AuthzForce XACML policies (via DynamicOrch-XACML)
  Enforcement decision:   AuthzForce XACML policies (via PEPs)

  → One system to update: PAP.
  → Revocation is atomic: PAP DELETE → AuthzForce updated → both planes deny.
```

## Policy effect table

| Subject | Resource | PAP policy | DynamicOrch-XACML | kafka-authz / rest-authz |
|---|---|---|---|---|
| portal-cloud-ml | telemetry | Permit | Returns providers | Permit |
| service-partner-1 | telemetry-rest | Permit | Returns providers | Permit |
| unauthorized | any | — (NotApplicable) | Returns [] | Deny |
| (any) | (any) after PAP delete | — | Returns [] | Deny |

## Fail-closed behaviour

```
AuthzForce unavailable:
  DynamicOrch-XACML.Orchestrate → empty list (Deny equivalent)
  PEPs                          → Deny (each has its own AF client)

ServiceRegistry unavailable:
  DynamicOrch-XACML.Orchestrate → error 500

AuthzForce responds with Deny:
  DynamicOrch-XACML.Orchestrate → empty list (no error returned to caller)
```
