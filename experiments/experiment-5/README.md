# Experiment 5 — Unified Policy Projection with AuthzForce (XACML/ABAC)

This experiment extends experiment-4 by adding a second transport path (Kafka)
and centralising all authorization decisions in an **AuthzForce** XACML/ABAC
Policy Decision Point (PDP).  The same grant in ConsumerAuthorization now
propagates to both RabbitMQ/AMQP and Kafka/SSE enforcement within one
policy-sync cycle.

---

## What is new compared to experiment-4

| Concern | Experiment-4 | Experiment-5 |
|---|---|---|
| **Policy model** | Live CA checks (per-call HTTP) | XACML PolicySet in AuthzForce; compiled from CA grants |
| **Transport** | AMQP only | AMQP **+** Kafka (dual-publish) |
| **Authorization policy** | Per-transport, independently evaluated | Unified: one PolicySet governs both transports |
| **Revocation propagation** | Immediate (live CA check) | Within one sync cycle (~10 s); both transports revoked together |
| **Kafka consumers** | None | analytics-consumer via SSE (kafka-authz) |
| **Policy admin** | Implicit (live checks) | Explicit: XACML 3.0 PolicySet, versioned in AuthzForce |
| **Dashboard** | Health + Grants + Live Data | + Policy Projection tab |

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│  Arrowhead Core                                                          │
│                                                                          │
│  ServiceRegistry :8080   Authentication :8081   ConsumerAuth :8082      │
│  DynamicOrch :8083       CertAuth :8086                                  │
└───────────────────────────────────┬──────────────────────────────────────┘
                                    │ grants
                                    ▼
                             policy-sync
                          (compiles XACML PolicySet)
                                    │
                                    ▼
                          AuthzForce :8080
                      (XACML PDP/PAP — single source of truth)
                                    │
                   ┌────────────────┴────────────────┐
                   │                                  │
                   ▼                                  ▼
          topic-auth-xacml                       kafka-authz
       (RabbitMQ HTTP authz PEP)            (Kafka SSE proxy PEP)
                   │                                  │
                   ▼                                  ▼
          RabbitMQ :5672                     Kafka :9092
         (AMQP transport)                  (Kafka transport)
                   │                                  │
         ┌─────────┼─────────┐                        │
         ▼         ▼         ▼                        ▼
    consumer-1 consumer-2 consumer-3         analytics-consumer
    (AMQP)     (AMQP)     (AMQP)            (SSE / Kafka path)
                   ▲
          robot-fleet (dual-publish: AMQP + Kafka)
```

---

## Policy projection model

1. **policy-sync** polls `ConsumerAuthorization /authorization/lookup` every 10 s.
2. It compiles grants into a XACML 3.0 `PolicySet` with `deny-unless-permit`
   combining: one `Policy` element per grant.
3. The PolicySet is uploaded to AuthzForce (PAP) with an incremented version number.
4. AuthzForce atomically switches the active policy to the new version (PDP).
5. Both PEPs evaluate against the same AuthzForce domain:
   - **topic-auth-xacml**: queried on every RabbitMQ broker operation (publish, bind, topic-read)
   - **kafka-authz**: queried on SSE stream open and every 100 messages (revocation check)

Revocation is effective within `SYNC_INTERVAL` (default 10 s) on both transports
simultaneously.  This is the key difference from experiment-4's per-transport,
per-call CA checks.

---

## Services

| Service | Host port | Role |
|---|---|---|
| **serviceregistry** | 8080 | Service registration and discovery |
| **authentication** | 8081 | Bearer identity tokens |
| **consumerauth** | 8082 | Authorization grants (source of truth for policy-sync) |
| **dynamicorch** | 8083 | Orchestration gate |
| **ca** | 8086 | Certificate Authority (placeholder) |
| **rabbitmq** | 15675 | AMQP broker; management UI on 15675 |
| **authzforce** | 8180 | XACML PDP/PAP |
| **kafka** | — | Message broker (internal only) |
| **policy-sync** | 9095 | CA → XACML compiler and AuthzForce uploader |
| **topic-auth-xacml** | — | RabbitMQ HTTP authz backend (AMQP PEP) |
| **kafka-authz** | 9091 | Kafka SSE proxy (Kafka PEP) |
| **robot-fleet** | 9105→9003 | Dual-publish: AMQP + Kafka |
| **consumer-1/2/3** | — | AMQP subscribers via AHC orchestration flow |
| **analytics-consumer** | — | Kafka SSE subscriber |
| **dashboard** | 3005 | React UI |

---

## Quick Start

```bash
cd experiments/experiment-5
docker compose up --build
```

Service startup order:
```
rabbitmq + authzforce + kafka + serviceregistry + authentication + consumerauth + ca
  → dynamicorch + setup (seeds grants)
  → policy-sync (waits for authzforce + consumerauth + setup; first sync returns 200)
  → topic-auth-xacml + kafka-authz (wait for policy-sync to be healthy)
  → robot-fleet (waits for rabbitmq + kafka + topic-auth-xacml + serviceregistry)
  → consumer-1/2/3 + analytics-consumer
  → dashboard
```

Open the dashboard at **http://localhost:3005**.

AuthzForce API: **http://localhost:8180/authzforce-ce/domains**

---

## Key Concepts Demonstrated

### Unified policy projection

A single grant in ConsumerAuthorization produces a single XACML Rule in
AuthzForce.  When that grant is revoked, **both** the AMQP path and the Kafka
path begin denying access within one sync cycle — without any per-transport
configuration change.

### XACML 3.0 PolicySet structure

```xml
<PolicySet PolicyCombiningAlgId="deny-unless-permit">
  <!-- one Policy per grant -->
  <Policy RuleCombiningAlgId="deny-unless-permit">
    <Target>
      <AnyOf><!-- subject: consumerSystemName --></AnyOf>
      <AnyOf><!-- resource: serviceDefinition --></AnyOf>
    </Target>
    <Rule Effect="Permit"/>
  </Policy>
  <!-- ... -->
</PolicySet>
```

### Kafka SSE revocation

`kafka-authz` re-checks AuthzForce every 100 messages.  If the decision
changes to Deny, it sends an `event: revoked` SSE message and closes the
stream.  The `analytics-consumer` disconnects and retries with exponential
back-off.

### Dual-transport publish

`robot-fleet` publishes each telemetry message to both transports
simultaneously via a `PublishFn` callback:
- AMQP routing key: `telemetry.{robotId}`
- Kafka topic: `arrowhead.telemetry`, key: `telemetry.{robotId}`

---

## Verifying Policy Projection

**Revoke analytics-consumer and observe Kafka denial:**
```bash
# Find the grant id for analytics-consumer
curl http://localhost:8082/authorization/lookup

# Revoke it
curl -X DELETE http://localhost:8082/authorization/revoke/{id}

# Wait ~10 s for policy-sync cycle.
# analytics-consumer logs "policy revoked — disconnecting"
# and retries after back-off.
# AMQP consumers are unaffected (their grants still exist).
```

**Revoke demo-consumer-2 and observe both paths:**
```bash
curl -X DELETE http://localhost:8082/authorization/revoke/{id}

# Within ~10 s:
# - consumer-2 (AMQP): denied by topic-auth-xacml on next broker operation
# - analytics-consumer (Kafka): not affected (different grant)
```

**Restore and observe reconnection:**
```bash
curl -X POST http://localhost:8082/authorization/grant \
  -H 'Content-Type: application/json' \
  -d '{"consumerSystemName":"analytics-consumer","providerSystemName":"robot-fleet","serviceDefinition":"telemetry"}'
# Within ~10 s analytics-consumer reconnects and resumes streaming.
```

---

## Directory Structure

```
experiment-5/
├── docker-compose.yml
├── dockerfiles/
│   ├── core.Dockerfile
│   ├── consumer-direct.Dockerfile
│   ├── robot-fleet.Dockerfile
│   ├── analytics-consumer.Dockerfile
│   ├── topic-auth-xacml.Dockerfile
│   ├── policy-sync.Dockerfile
│   ├── kafka-authz.Dockerfile
│   └── dashboard.Dockerfile
├── rabbitmq/
│   ├── rabbitmq.conf                  # points to topic-auth-xacml
│   └── enabled_plugins
├── services/
│   ├── consumer-direct/               # Go: AMQP consumer (AHC orchestration flow)
│   ├── robot-fleet/                   # Go: dual-publish AMQP+Kafka, SR registration
│   └── analytics-consumer/            # Go: Kafka SSE consumer
└── dashboard/                         # React + Vite + nginx
```

Support services (shared across experiments):
```
support/
├── authzforce/                        # Go: AuthzForce REST client + XACML policy builder
├── policy-sync/                       # Go: CA → XACML → AuthzForce compiler
├── topic-auth-xacml/                  # Go: RabbitMQ HTTP authz backend → AuthzForce
└── kafka-authz/                       # Go: Kafka SSE proxy → AuthzForce
```

See [DIAGRAMS.md](DIAGRAMS.md) for Mermaid component and sequence diagrams.

---

> **Note:** The dashboard source (`dashboard/src/`) is identical to
> `../experiment-6/dashboard/src/`. Changes to dashboard logic must be
> mirrored manually to the other experiment.
