# Experiment 6 — Triple-Transport Policy Projection with AuthzForce (XACML/ABAC)

This experiment extends experiment-5 by adding a **third transport path (REST/HTTP)**
governed by the same AuthzForce XACML policy as AMQP and Kafka.  A single grant in
ConsumerAuthorization now propagates to all three transports within one policy-sync cycle.

The experiment also introduces a **runtime-configurable `SYNC_INTERVAL`** so the sync-delay
caveat — the window between revocation in CA and enforcement in all PEPs — can be observed
and controlled interactively from the browser UI.

---

## What is new compared to experiment-5

| Concern | Experiment-5 | Experiment-6 |
|---|---|---|
| **Transport** | AMQP + Kafka | AMQP + Kafka + **REST** |
| **PEP count** | 2 (topic-auth-xacml, kafka-authz) | 3 (+ **rest-authz**) |
| **REST authorization** | Not present | rest-authz → AuthzForce (same PDP) |
| **REST data service** | Not present | data-provider (Kafka consumer + REST API) |
| **REST consumer** | Not present | rest-consumer (polls via rest-authz) |
| **SYNC_INTERVAL** | Fixed in compose env | Runtime-configurable via Config tab |
| **Sync-delay caveat** | Mentioned in docs | Demonstrated interactively |

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
                          (SYNC_INTERVAL configurable at runtime)
                                    │
                                    ▼
                          AuthzForce :8080
                      (XACML PDP/PAP — single source of truth)
                                    │
              ┌─────────────────────┼────────────────────┐
              │                     │                     │
              ▼                     ▼                     ▼
     topic-auth-xacml          kafka-authz           rest-authz
  (RabbitMQ HTTP authz PEP)  (Kafka SSE PEP)    (HTTP reverse proxy PEP)
              │                     │                     │
              ▼                     ▼                     ▼
     RabbitMQ :5672          Kafka :9092          data-provider :9094
    (AMQP transport)       (Kafka transport)     (Kafka consumer + REST API)
              │                     │                     │
    ┌─────────┼────────┐            │                     │
    ▼         ▼        ▼            ▼                     ▼
consumer-1 consumer-2 consumer-3  analytics-consumer  rest-consumer
(AMQP)     (AMQP)     (AMQP)     (SSE / Kafka path)  (REST path)
                ▲
       robot-fleet (dual-publish: AMQP + Kafka)
```

---

## Policy projection model

1. **policy-sync** polls `ConsumerAuthorization /authorization/lookup` every `SYNC_INTERVAL`
   (default 10 s, runtime-configurable via dashboard Config tab → POST /config).
2. It compiles grants into a XACML 3.0 `PolicySet` with `deny-unless-permit`
   combining: one `Policy` element per grant.
3. The PolicySet is uploaded to AuthzForce (PAP) with an incremented version number.
4. All three PEPs evaluate against the same AuthzForce domain (`arrowhead-exp6`):
   - **topic-auth-xacml**: queried on every RabbitMQ broker operation
   - **kafka-authz**: queried on SSE stream open and every 100 messages
   - **rest-authz**: queried on every HTTP request (using `X-Consumer-Name` header)

### Sync-delay caveat (REST-specific)

REST enforcement lags CA by up to `SYNC_INTERVAL`.  A revoked grant continues to
produce Permit decisions until policy-sync uploads the next PolicySet version.
This is visible in the dashboard: `rest-consumer/stats.deniedCount` increases only
after the sync cycle completes, while `analytics-consumer` receives an `event: revoked`
SSE message at the same time.

---

## Services

| Service | Host port | Role |
|---|---|---|
| **serviceregistry** | 8080 | Service registration and discovery |
| **authentication** | 8081 | Bearer identity tokens |
| **consumerauth** | 8082 | Authorization grants (source of truth for policy-sync) |
| **dynamicorch** | 8083 | Orchestration gate |
| **ca** | 8086 | Certificate Authority (placeholder) |
| **rabbitmq** | 15676 | AMQP broker; management UI on 15676 |
| **authzforce** | 8186 | XACML PDP/PAP |
| **kafka** | — | Message broker (internal only) |
| **policy-sync** | — | CA → XACML compiler and AuthzForce uploader |
| **topic-auth-xacml** | — | RabbitMQ HTTP authz backend (AMQP PEP) |
| **kafka-authz** | 9091 | Kafka SSE proxy (Kafka PEP) |
| **rest-authz** | 9093 | HTTP reverse proxy (REST PEP) |
| **robot-fleet** | 9106→9003 | Dual-publish: AMQP + Kafka |
| **data-provider** | — | Kafka consumer + REST API (upstream of rest-authz) |
| **consumer-1/2/3** | — | AMQP subscribers via AHC orchestration flow |
| **analytics-consumer** | — | Kafka SSE subscriber |
| **rest-consumer** | — | REST subscriber via rest-authz |
| **dashboard** | 3006 | React UI |

---

## Quick Start

```bash
cd experiments/experiment-6
docker compose up --build
```

Service startup order:
```
rabbitmq + authzforce + kafka + serviceregistry + authentication + consumerauth + ca
  → dynamicorch + setup (seeds grants)
  → policy-sync (waits for authzforce + consumerauth + setup; first sync returns 200)
  → topic-auth-xacml + kafka-authz + data-provider
  → rest-authz (waits for data-provider + policy-sync)
  → robot-fleet + analytics-consumer + rest-consumer
  → consumer-1/2/3
  → dashboard
```

Open the dashboard at **http://localhost:3006**.

AuthzForce API: **http://localhost:8186/authzforce-ce/domains**

---

## Key Concepts Demonstrated

### Triple-transport unified policy

A single grant in ConsumerAuthorization produces a single XACML Rule in AuthzForce.
Revoking it propagates to **all three transports** within one sync cycle:
- AMQP: denied on the next broker operation
- Kafka: `event: revoked` SSE + re-check on next 100-message boundary
- REST: 403 Forbidden on the next request after the sync cycle

### Runtime-configurable SYNC_INTERVAL

The Config tab sends `POST /api/policy-sync/config {"syncInterval":"Ns"}` to policy-sync,
which updates an `atomic.Int64` and applies it on the next sleep iteration — no restart needed.
Set a long interval to widen the sync-delay window and observe REST consumers continuing to
receive data after a grant revocation.

### REST identity via X-Consumer-Name header

`rest-authz` reads the consumer identity from the `X-Consumer-Name` HTTP header (or
`?consumer=` query parameter).  This keeps rest-authz independent of the Authentication
service — the consumer must self-identify, and the trust boundary is at the network layer
(internal Docker network).

### Data flow for REST path

```
rest-consumer
  → GET /telemetry/latest (X-Consumer-Name: rest-consumer)
  → rest-authz :9093
      → POST /pdp/request to AuthzForce (XACML decide)
      → Permit → proxy to data-provider :9094/telemetry/latest
      → 200 OK with latest Kafka message
rest-consumer records msgCount++
```

---

## Verifying Policy Projection

**Demonstrate REST authorization:**
```bash
# Permitted consumer
curl -H 'X-Consumer-Name: rest-consumer' http://localhost:9093/telemetry/latest

# Unknown consumer (no grant)
curl -H 'X-Consumer-Name: unauthorized' http://localhost:9093/telemetry/latest
# → 403 {"error":"not authorized"}
```

**Revoke rest-consumer and observe sync delay:**
```bash
# Find the grant id for rest-consumer
curl http://localhost:8082/authorization/lookup

# Revoke it
curl -X DELETE http://localhost:8082/authorization/revoke/{id}

# Set a long sync interval to extend the window
curl -X POST http://localhost:9095/config \
  -H 'Content-Type: application/json' \
  -d '{"syncInterval":"60s"}'

# REST consumer continues receiving 200 OK for up to 60 s
# After sync cycle: starts receiving 403 Forbidden
```

**Explicit AuthzForce check:**
```bash
curl -X POST http://localhost:9093/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"rest-consumer","service":"telemetry-rest"}'
# → {"consumer":"rest-consumer","service":"telemetry-rest","permit":true,"decision":"Permit"}
```

---

## Directory Structure

```
experiment-6/
├── docker-compose.yml
├── dockerfiles/
│   ├── core.Dockerfile
│   ├── authzforce-server.Dockerfile
│   ├── policy-sync.Dockerfile
│   ├── topic-auth-xacml.Dockerfile
│   ├── kafka-authz.Dockerfile
│   ├── rest-authz.Dockerfile
│   ├── robot-fleet.Dockerfile
│   ├── consumer-direct.Dockerfile
│   ├── analytics-consumer.Dockerfile
│   ├── data-provider.Dockerfile
│   ├── rest-consumer.Dockerfile
│   └── dashboard.Dockerfile
├── rabbitmq/
│   ├── rabbitmq.conf               # points to topic-auth-xacml
│   └── enabled_plugins
├── services/
│   ├── data-provider/              # Go: Kafka consumer + REST API
│   └── rest-consumer/              # Go: REST client via rest-authz
└── dashboard/                      # React + Vite + nginx
```

Support services (shared with experiment-5):
```
support/
├── authzforce/                     # Go: AuthzForce REST client + XACML policy builder
├── policy-sync/                    # Go: CA → XACML → AuthzForce compiler (+ /config endpoint)
├── topic-auth-xacml/               # Go: RabbitMQ HTTP authz backend → AuthzForce
├── kafka-authz/                    # Go: Kafka SSE proxy → AuthzForce
└── rest-authz/                     # Go: HTTP reverse proxy PEP → AuthzForce (new in exp-6)
```

See [DIAGRAMS.md](DIAGRAMS.md) for Mermaid component and sequence diagrams.

---

> **Note:** The shared dashboard files (`dashboard/src/`) are symlinked from
> `support/dashboard-shared/`. To change shared dashboard logic, edit the
> canonical file in `support/dashboard-shared/` — never edit the symlinks
> directly. Run `bash support/dashboard-shared/check-dashboard-shared.sh` to
> verify all symlinks are intact.
