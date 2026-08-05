# Architecture

See [core/DIAGRAMS.md](core/DIAGRAMS.md) for Mermaid architecture and sequence diagrams.

This repository is divided into four areas.

---

## /core — Arrowhead 5 Core Systems

Six independent, spec-compliant implementations of the Arrowhead 5 core systems, each running as its own binary on its own port.

- Governed by `core/SPEC.md`
- All in-memory, no external dependencies
- Independently buildable and testable
- No dependency on `experiments/`

See [`core/CLAUDE.md`](core/CLAUDE.md) for implementation rules.

### Systems and Ports

| System | Port | Package |
|---|---|---|
| ServiceRegistry | 8080 | `cmd/serviceregistry` |
| Authentication | 8081 | `cmd/authentication` |
| ConsumerAuthorization | 8082 | `cmd/consumerauth` |
| DynamicOrchestration | 8083 | `cmd/dynamicorch` |
| SimpleStoreOrchestration | 8084 | `cmd/simplestoreorch` |
| FlexibleStoreOrchestration | 8085 | `cmd/flexiblestoreorch` |

### Inter-System Communication

```
DynamicOrchestration ──HTTP──▶ ServiceRegistry        (query)
DynamicOrchestration ──HTTP──▶ ConsumerAuthorization  (verify, optional)
```

All other systems operate independently. No system imports another's Go packages — communication is HTTP only.

### API Surface

**ServiceRegistry (8080)**

| Endpoint | Method | Description |
|---|---|---|
| `/serviceregistry/register` | POST | Register a service instance |
| `/serviceregistry/query` | POST | Query registered services |
| `/serviceregistry/lookup` | GET | Query via URL params |
| `/serviceregistry/unregister` | DELETE | Remove a service instance |
| `/health` | GET | Liveness check |

**Authentication (8081)**

| Endpoint | Method | Description |
|---|---|---|
| `/authentication/identity/login` | POST | Issue an identity token |
| `/authentication/identity/logout` | DELETE | Revoke the current token |
| `/authentication/identity/verify` | GET | Check token validity |
| `/health` | GET | Liveness check |

**ConsumerAuthorization (8082)**

| Endpoint | Method | Description |
|---|---|---|
| `/authorization/grant` | POST | Create an authorization rule |
| `/authorization/revoke/{id}` | DELETE | Remove a rule |
| `/authorization/lookup` | GET | List matching rules |
| `/authorization/verify` | POST | Check if pair is authorized |
| `/authorization/token/generate` | POST | Generate authorization token |
| `/health` | GET | Liveness check |

**DynamicOrchestration (8083)**

| Endpoint | Method | Description |
|---|---|---|
| `/orchestration/dynamic` | POST | Real-time SR lookup + optional auth filter |
| `/health` | GET | Liveness check |

**SimpleStoreOrchestration (8084)**

| Endpoint | Method | Description |
|---|---|---|
| `/orchestration/simplestore` | POST | Orchestrate via stored rules |
| `/orchestration/simplestore/rules` | GET | List rules |
| `/orchestration/simplestore/rules` | POST | Create rule |
| `/orchestration/simplestore/rules/{id}` | DELETE | Delete rule |
| `/health` | GET | Liveness check |

**FlexibleStoreOrchestration (8085)**

| Endpoint | Method | Description |
|---|---|---|
| `/orchestration/flexiblestore` | POST | Orchestrate via priority rules |
| `/orchestration/flexiblestore/rules` | GET | List rules |
| `/orchestration/flexiblestore/rules` | POST | Create rule |
| `/orchestration/flexiblestore/rules/{id}` | DELETE | Delete rule |
| `/health` | GET | Liveness check |

### Dashboard (core/dashboard/)

A React + TypeScript browser application for monitoring and interacting with all six systems.

- Communicates with backends via HTTP only
- Does not import any Go packages
- Built separately with `npm install && npm run build`
- In development, Vite proxies API calls to all backends (no CORS required)
- When built, served by the ServiceRegistry binary at `http://localhost:8080/`

### Running

```bash
cd core
go build ./...
go test ./...
```

---

## /core-evol — ADAPI Extensions

Evolved core system variants that implement the ADAPI gRPC interfaces, extending the core's control-plane authorization to data-plane enforcement. See [`core-evol/README.md`](core-evol/README.md).

| Component | Description |
|---|---|
| `cmd/authz-pdp` | gRPC authorization PDP server (`authorize.proto`) |
| `cmd/dynamicorch-xacml` | DynamicOrchestration variant delegating to an external XACML PDP |
| `proto/authorize/` | PEP-to-PDP gRPC interface for authorization decisions |
| `proto/certlifecycle/` | Certificate lifecycle event stream from CA to PEPs |

Used by experiments 12-14.

---

## /support — Shared Support Libraries

Reusable modules shared across experiments. Each module is a standalone Go module referenced via `replace` directives by the services that use it.

| Module | Path | Description |
|---|---|---|
| `message-broker` | `support/message-broker/` | AMQP publish/subscribe wrapper (used by experiments 2–5) |
| `topic-auth-sync` | `support/topic-auth-sync/` | Syncs ConsumerAuth policies to RabbitMQ topic permissions (experiment-3) |
| `topic-auth-http` | `support/topic-auth-http/` | RabbitMQ HTTP authz backend — live CA checks + user lifecycle management (experiment-4) |
| `authzforce` | `support/authzforce/` | AuthzForce REST client + XACML 3.0 PolicySet builder (experiments 5+) |
| `policy-sync` | `support/policy-sync/` | Compiles ConsumerAuth grants into a XACML PolicySet and uploads to AuthzForce (experiments 5–6) |
| `topic-auth-xacml` | `support/topic-auth-xacml/` | RabbitMQ HTTP authz backend — delegates all decisions to AuthzForce PDP (experiments 5–14) |
| `kafka-authz` | `support/kafka-authz/` | Kafka SSE proxy PEP — authenticates streams against AuthzForce and sends `event: revoked` on policy change (experiments 5–14) |
| `rest-authz` | `support/rest-authz/` | HTTP reverse proxy PEP — forwards requests to an upstream service only when AuthzForce returns Permit (experiments 6–14) |
| `dashboard-shared` | `support/dashboard-shared/` | Canonical shared React source files (10 components/views/hooks) symlinked into experiment dashboards |

---

## /experiments — Experimental Extensions

Exploratory code built on top of the core. May include additional frontends, simulation drivers, client libraries, or analysis tools.

- Communicates with core via HTTP only — no internal package imports
- Self-contained per experiment; each subdirectory manages its own dependencies
- Not held to the strict correctness standard of `core/`

### Experiments

Experiments 1–5 are preserved as historical reference. Experiment-6 is the **active baseline**; experiments 7–14 build on it progressively.

| Experiment | Description |
|---|---|
| [experiment-1](experiments/experiment-1/) | Interactive browser demo: register services, grant authorization, orchestrate |
| [experiment-2](experiments/experiment-2/) | Virtual local cloud with AMQP data plane: robot-fleet → RabbitMQ → edge-adapter → orchestrated consumers |
| [experiment-3](experiments/experiment-3/) | Direct AMQP subscriptions with broker-level topic authorization sourced from ConsumerAuth |
| [experiment-4](experiments/experiment-4/) | Geo-distributed consumers over AMQP: dual-layer authorization via `topic-auth-http` (live CA checks) + RabbitMQ user lifecycle management |
| [experiment-5](experiments/experiment-5/) | Unified XACML/ABAC policy projection across AMQP and Kafka: `policy-sync` compiles CA grants into a XACML PolicySet; one AuthzForce PDP governs both `topic-auth-xacml` (AMQP) and `kafka-authz` (Kafka SSE) |
| [experiment-6](experiments/experiment-6/) | Triple-transport policy projection (AMQP + Kafka + REST) with runtime-configurable `SYNC_INTERVAL`; active baseline for all later experiments |
| [experiment-7](experiments/experiment-7/) | X.509/TLS extension: REST consumers identified by cert CN; mTLS across all transport paths |
| [experiment-8](experiments/experiment-8/) | Arrowhead 5.2 profile-based PKI with enforced certificate hierarchy and compliance assessment |
| [experiment-9](experiments/experiment-9/) | UC3 "Lawn Mowing as a Service": multi-site robot fleets publish over Kafka + AMQP; Portal & Cloud ML aggregates streams; Service Partners consume via mTLS REST proxy PEP |
| [experiment-10](experiments/experiment-10/) | UC3 with classical PAP/PIP/PDP access-control architecture; eliminates sync delay by separating policy administration, information, and decision points |
| [experiment-11](experiments/experiment-11/) | Hybrid PAP/PIP/PDP (Strategy A): two policy sources merged into a single XACML PolicySet at push time |
| [experiment-12](experiments/experiment-12/) | DynamicOrchestration-XACML (Approach B): gRPC PDP interface replaces ConsumerAuthorization for orchestration decisions |
| [experiment-13](experiments/experiment-13/) | PKI identity unification: cert CN as XACML subject on all paths; cert-level ABAC attributes; CertificateLifecycle gRPC stream auto-populates PIP |
| [experiment-14](experiments/experiment-14/) | Connection-time certificate revocation: Kafka `ArrowheadPrincipalBuilder` plugin and RabbitMQ `topic-auth-xacml` pre-gate both reject revoked clients before the PDP is consulted |

See [`experiments/CLAUDE_EXPERIMENTS.md`](experiments/CLAUDE_EXPERIMENTS.md) for rules.

---

## Boundary Rule

```
experiments/ ──HTTP──▶ core/
dashboard/   ──HTTP──▶ core/   (served from same binary)
```

No code in `experiments/` or `dashboard/` may import packages from `core/internal/`.

---

## Repository Structure

```
/
├── ARCHITECTURE.md
├── README.md
├── REPO_RULES.md
├── LICENSE
├── .gitignore
│
├── core/
│   ├── CLAUDE.md
│   ├── SPEC.md
│   ├── GAP_ANALYSIS.md
│   ├── TEST_PLAN.md
│   ├── EXAMPLES.md
│   ├── docker-compose.yml
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/
│   │   ├── serviceregistry/
│   │   ├── authentication/
│   │   ├── consumerauth/
│   │   ├── dynamicorch/
│   │   ├── simplestoreorch/
│   │   └── flexiblestoreorch/
│   ├── internal/
│   │   ├── api/                    ← ServiceRegistry HTTP handlers
│   │   ├── config/                 ← ServiceRegistry config
│   │   ├── model/                  ← ServiceRegistry models
│   │   ├── repository/             ← ServiceRegistry storage
│   │   ├── service/                ← ServiceRegistry business logic
│   │   ├── authentication/
│   │   │   ├── api/
│   │   │   ├── model/
│   │   │   └── service/
│   │   ├── consumerauth/
│   │   │   ├── api/
│   │   │   ├── model/
│   │   │   └── service/
│   │   └── orchestration/
│   │       ├── model/              ← shared orchestration types
│   │       ├── dynamic/
│   │       │   ├── api/
│   │       │   └── service/
│   │       ├── simplestore/
│   │       │   ├── api/
│   │       │   ├── model/
│   │       │   └── service/
│   │       └── flexiblestore/
│   │           ├── api/
│   │           ├── model/
│   │           └── service/
│   └── dashboard/
│       ├── package.json
│       ├── vite.config.ts
│       ├── index.html
│       └── src/
│           ├── App.tsx
│           ├── types.ts
│           ├── api.ts
│           └── components/
│               ├── SystemStatus.tsx
│               ├── MetricsBar.tsx
│               ├── RegisterForm.tsx
│               ├── ServiceTable.tsx
│               ├── ServiceDetail.tsx
│               ├── AuthRulesPanel.tsx
│               └── OrchestrationPanel.tsx
│
├── core-evol/
│   ├── go.mod
│   ├── cmd/                         # Evolved core binaries (dynamicorch-xacml, etc.)
│   ├── internal/                    # Evolved core internals
│   └── proto/                       # Protobuf definitions
│       ├── authorize/               # gRPC PDP interface (authorize.proto)
│       └── certlifecycle/           # gRPC cert event stream (certlifecycle.proto)
│
├── support/
│   ├── message-broker/              # AMQP publish/subscribe library
│   ├── topic-auth-sync/             # ConsumerAuth → RabbitMQ topic-permission sync (experiment-3)
│   ├── topic-auth-http/             # RabbitMQ HTTP authz backend, live CA checks (experiment-4)
│   ├── authzforce/                  # AuthzForce REST client + XACML PolicySet builder
│   ├── policy-sync/                 # CA → XACML → AuthzForce compiler (experiments 5–6)
│   ├── topic-auth-xacml/            # RabbitMQ HTTP authz backend → AuthzForce PDP (experiments 5–14)
│   ├── kafka-authz/                 # Kafka SSE proxy PEP → AuthzForce PDP (experiments 5–14)
│   ├── rest-authz/                  # HTTP reverse proxy PEP → AuthzForce PDP (experiments 6–14)
│   └── dashboard-shared/            # Shared React source files symlinked into experiment dashboards
│
└── experiments/
    ├── CLAUDE_EXPERIMENTS.md
    ├── experiment-1/
    ├── experiment-2/
    │   ├── docker-compose.yml
    │   ├── dockerfiles/
    │   ├── services/
    │   │   ├── robot-simulator/
    │   │   ├── edge-adapter/
    │   │   └── consumer/
    │   ├── dashboard/               # React dashboard (nginx-served in Docker)
    │   └── tests/
    ├── experiment-3/
    │   ├── docker-compose.yml
    │   ├── dockerfiles/
    │   ├── rabbitmq/                # rabbitmq.conf + enabled_plugins
    │   └── services/
    │       └── consumer-direct/     # direct AMQP subscriber
    ├── experiment-4/
    │   ├── docker-compose.yml
    │   ├── dockerfiles/
    │   ├── rabbitmq/
    │   └── services/
    │       ├── robot-fleet/         # AMQP publisher + SR registration
    │       └── consumer-direct/     # AMQP consumer via AHC orchestration flow
    ├── experiment-5/
    │   ├── docker-compose.yml
    │   ├── dockerfiles/
    │   ├── rabbitmq/
    │   ├── authzforce/              # AuthzForce config
    │   └── services/
    │       ├── robot-fleet/         # dual-publish AMQP + Kafka
    │       ├── consumer-direct/     # AMQP consumer via AHC orchestration flow
    │       └── analytics-consumer/  # Kafka SSE consumer via kafka-authz
    ├── experiment-6/
    │   ├── docker-compose.yml
    │   ├── dockerfiles/
    │   ├── rabbitmq/
    │   └── services/
    │       ├── data-provider/       # Kafka consumer + REST API (upstream of rest-authz)
    │       └── rest-consumer/       # REST subscriber polling via rest-authz
    ├── experiment-7/                # X.509/TLS: REST consumers identified by cert CN
    ├── experiment-8/                # AH 5.2 profile-based PKI, compliance assessment
    ├── experiment-9/                # UC3 Lawn Mowing: multi-site fleets + mTLS REST proxy PEP
    ├── experiment-10/               # UC3 with PAP/PIP/PDP separation
    ├── experiment-11/               # Hybrid PAP/PIP/PDP: two policy sources merged at push time
    ├── experiment-12/               # DynamicOrchestration-XACML: gRPC PDP replaces ConsumerAuth
    ├── experiment-13/               # PKI identity unification: cert CN as XACML subject
    └── experiment-14/               # Connection-time cert revocation on Kafka + RabbitMQ
```
