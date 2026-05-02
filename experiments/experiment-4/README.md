# Experiment 4 — Full AHC Integration: SR + Auth + ConsumerAuth + DO

This experiment implements the full Arrowhead 5 integration described in
`AHC-INTEGRATION-PLAN.md`, building on the broker-level topic authorization
from experiment-3.

---

## What is new compared to experiment-3

| Concern | Experiment-3 | Experiment-4 |
|---|---|---|
| **Service location** | Hardcoded `AMQP_URL` env var | robot-fleet registers in ServiceRegistry; consumers discover via DynamicOrchestration |
| **Consumer identity** | None — password is the only credential | Identity token from Authentication; verified by DynamicOrchestration |
| **Authorization gate** | Broker topic-permissions only | Two independent layers: orchestration gate **+** broker topic-permissions |
| **Policy read security** | ConsumerAuth API open to all | topic-auth-sync carries Bearer token on ConsumerAuth calls |
| **Consumer flow** | `AMQP_URL` → connect directly | Auth login → DynamicOrchestration → CA token → connect to discovered endpoint |
| **Revocation** | Effective at next sync cycle (≤ 10 s) | Immediate at orchestration layer; also at next sync cycle at broker layer |
| **Core systems used** | ConsumerAuth only | ServiceRegistry + Authentication + ConsumerAuth + DynamicOrchestration + CA |

Phase 5 (mTLS via CertificateAuthority) is declared in docker-compose but not
yet implemented in the service code — the CA service runs and its `/health`
endpoint is reachable, but services do not request certificates and all
inter-system communication remains plain HTTP.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│  Arrowhead Core                                                      │
│                                                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌─────────┐ │
│  │ ServiceReg   │  │ Auth         │  │ ConsumerAuth │  │   DO    │ │
│  │ :8080        │  │ :8081        │  │ :8082        │  │  :8083  │ │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └────┬────┘ │
│         │ register        │ login/verify     │ grants        │ orch │
└─────────┼─────────────────┼──────────────────┼───────────────┼──────┘
          │                 │                  │               │
   robot-fleet          all services    topic-auth-sync   consumers
   (at startup)         (at startup)    (every 10 s)      (at startup)
                                                               │
                                        ┌──────────────────────▼──────┐
                                        │  RabbitMQ :5672             │
                                        │  exchange: arrowhead        │
                                        │  (topic + topic-auth plugin)│
                                        └──────┬──────────────────────┘
                                               │
                         ┌─────────────────────┼──────────────────────┐
                         │                     │                      │
              ┌──────────▼───┐      ┌──────────▼───┐      ┌──────────▼───┐
              │ consumer-1   │      │ consumer-2   │      │ consumer-3   │
              │ (direct AMQP)│      │ (direct AMQP)│      │ (direct AMQP)│
              └──────────────┘      └──────────────┘      └──────────────┘
                         ▲
              ┌──────────┴────────┐
              │  robot-fleet      │
              │  (AMQP publisher) │
              └───────────────────┘
```

---

## Consumer startup flow (Phases 2–4)

```
1. POST /authentication/identity/login           → Bearer token
2. POST /orchestration/dynamic  [Bearer token]   → AMQP host:port, exchange, binding key
3. POST /authorization/token/generate            → proof-of-authorization token (logged)
4. AMQP connect + subscribe                      → start receiving telemetry
   On connection drop → repeat from step 1
```

DynamicOrchestration internally:
- Verifies the Bearer token via Authentication (ENABLE_IDENTITY_CHECK=true)
- Queries ServiceRegistry for `telemetry` providers
- Checks ConsumerAuthorization for each provider (ENABLE_AUTH=true)
- Returns only authorized providers

---

## Services

| Service | Port | Role |
|---|---|---|
| **serviceregistry** | 8080 | Stores service registrations; queried by DynamicOrchestration |
| **authentication** | 8081 | Issues and verifies Bearer identity tokens |
| **consumerauth** | 8082 | Stores authorization grants; polled by topic-auth-sync |
| **dynamicorch** | 8083 | Orchestration gate: verify identity + query SR + check CA |
| **ca** | 8086 | Certificate Authority (Phase 5 placeholder — mTLS not yet wired) |
| **rabbitmq** | 5672 / 15674 | AMQP broker; management UI on 15674 |
| **topic-auth-sync** | 9090 | Reconciles RabbitMQ users and topic permissions from ConsumerAuth |
| **robot-fleet** | 9104→9003 | Publishes synthetic telemetry; registers in ServiceRegistry at startup |
| **consumer-1/2/3** | — | Subscribe via AHC orchestration flow |

---

## Quick Start

```bash
cd experiments/experiment-4
docker compose up --build
```

Service startup order:
```
rabbitmq + serviceregistry + authentication + consumerauth + ca
  → dynamicorch (needs SR + auth + consumerauth)
  → setup (seeds grants into consumerauth)
  → topic-auth-sync (needs setup + authentication)
  → robot-fleet (registers in SR; needs topic-auth-sync + authentication)
  → consumer-1/2/3 (orchestrate via DO; needs robot-fleet registered)
```

Open the dashboard at **http://localhost:3004**.

RabbitMQ management UI: **http://localhost:15674** (admin/admin)

---

## Key Concepts Demonstrated

### Dual-layer authorization

Grant revocation is effective **immediately** at the orchestration layer (DynamicOrchestration
returns empty results) and additionally at the broker layer (topic-auth-sync removes the
RabbitMQ user within ≤ 10 s). A consumer cannot reconnect even if it has cached credentials,
because the orchestration gate refuses to supply the endpoint.

### Identity verification

DynamicOrchestration verifies the consumer's Bearer token via Authentication before answering.
A compromised consumer cannot impersonate another — the verified identity from the token
replaces the self-reported `requesterSystem.systemName` for all authorization checks.

### Dynamic service discovery

Consumers learn the AMQP host, port, exchange name, and binding key from the orchestration
response. Adding a new robot-fleet instance requires only registering it in ServiceRegistry;
consumers pick it up without any configuration change.

### Authorization token (Phase 4)

After orchestration, each consumer calls `POST /authorization/token/generate` on
ConsumerAuthorization and receives a proof-of-authorization token. This token is logged and
could be presented to the provider in a future experiment where providers independently verify
consumer authorization without a live call to ConsumerAuth.

---

## Verifying the Dual-Layer Enforcement

**Remove a grant and observe immediate orchestration denial:**
```bash
# Find the grant id for demo-consumer-2
curl http://localhost:8082/authorization/lookup

# Revoke it
curl -X DELETE http://localhost:8082/authorization/revoke/{id}

# Within 5 s consumer-2 logs:
#   "no authorized providers — grant may be missing"
```

**Restore the grant and observe reconnection:**
```bash
curl -X POST http://localhost:8082/authorization/grant \
  -H 'Content-Type: application/json' \
  -d '{"consumerSystemName":"demo-consumer-2","providerSystemName":"robot-fleet","serviceDefinition":"telemetry"}'

# consumer-2 will re-authenticate, re-orchestrate, and resume receiving messages.
```

---

## Directory Structure

```
experiment-4/
├── docker-compose.yml
├── dockerfiles/
│   ├── core.Dockerfile              # shared Dockerfile for all core binaries
│   ├── consumer-direct.Dockerfile   # builds experiment-4 consumer
│   ├── robot-fleet.Dockerfile       # builds experiment-4 robot-fleet
│   ├── topic-auth-sync.Dockerfile   # builds support/topic-auth-sync
│   └── dashboard.Dockerfile
├── rabbitmq/
│   ├── rabbitmq.conf
│   └── enabled_plugins
├── services/
│   ├── consumer-direct/             # Go: full AHC orchestration flow consumer
│   └── robot-fleet/                 # Go: AMQP publisher + SR registration
├── AHC-INTEGRATION-PLAN.md          # the plan that was implemented here
├── DIAGRAMS.md                      # component and sequence diagrams
└── README.md
```

See [DIAGRAMS.md](DIAGRAMS.md) for Mermaid component and sequence diagrams.
