# Architecture

See [DIAGRAMS.md](DIAGRAMS.md) for Mermaid architecture and sequence diagrams.

This repository is divided into two clearly separated areas.

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

## /support — Shared Support Libraries

Reusable modules shared across experiments. Each module is a standalone Go module referenced via `replace` directives by the services that use it.

| Module | Path | Description |
|---|---|---|
| `message-broker` | `support/message-broker/` | AMQP publish/subscribe wrapper (used by experiment-2 and experiment-3) |
| `topic-auth-sync` | `support/topic-auth-sync/` | Syncs Arrowhead ConsumerAuth policies to RabbitMQ topic permissions (used by experiment-3) |

---

## /experiments — Experimental Extensions

Exploratory code built on top of the core. May include additional frontends, simulation drivers, client libraries, or analysis tools.

- Communicates with core via HTTP only — no internal package imports
- Self-contained per experiment; each subdirectory manages its own dependencies
- Not held to the strict correctness standard of `core/`

### Experiments

| Experiment | Description |
|---|---|
| [experiment-1](experiments/experiment-1/) | Interactive browser demo: register services, grant authorization, orchestrate |
| [experiment-2](experiments/experiment-2/) | Virtual local cloud with AMQP data plane: robot-fleet → RabbitMQ → edge-adapter → orchestrated consumers |
| [experiment-3](experiments/experiment-3/) | Direct AMQP subscriptions with broker-level topic authorization sourced from ConsumerAuth |

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
├── support/
│   ├── message-broker/              # AMQP publish/subscribe library
│   └── topic-auth-sync/             # ConsumerAuth → RabbitMQ topic-permission sync
│       ├── go.mod
│       ├── main.go                  # env config, health server, run loop
│       ├── policy.go                # ConsumerAuth rules → RabbitMQ permission patterns
│       ├── rmqapi.go                # RabbitMQ Management API client
│       └── sync.go                  # reconciliation logic
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
    └── experiment-3/
        ├── docker-compose.yml
        ├── dockerfiles/
        ├── rabbitmq/                # rabbitmq.conf + enabled_plugins
        └── services/
            └── consumer-direct/     # direct AMQP subscriber
```
