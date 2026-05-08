# support/CLAUDE.md

## Purpose

This directory contains **shared Go services and libraries** used by one or more experiments.
These are production-quality support modules — held to a higher standard than experiment-local code.

---

## Module Stability

| Module | Status | Used by |
|---|---|---|
| `authzforce` | **Stable** — library only, no main | kafka-authz, policy-sync, rest-authz, topic-auth-xacml |
| `policy-sync` | **Stable** | experiment-5, experiment-6 |
| `topic-auth-xacml` | **Stable** | experiment-5, experiment-6 |
| `kafka-authz` | **Stable** | experiment-5, experiment-6 |
| `rest-authz` | **Stable** | experiment-6 |
| `authzforce-server` | Stable | experiment-5, experiment-6 (AuthzForce Docker wrapper) |
| `message-broker` | Stable | experiment-2, experiment-3, experiment-4 |
| `topic-auth-http` | Stable | experiment-3, experiment-4 |
| `topic-auth-sync` | Stable | experiment-3 (core service); not wired in experiments 4–6 |

**topic-auth-sync is the architectural centrepiece of experiment-3.** It polls ConsumerAuthorization and reconciles RabbitMQ users and topic permissions. It has a dedicated Dockerfile, health checks, and dashboard integration in experiment-3. Experiment-4 has a Dockerfile for it but does not wire it into its stack. It was superseded by the XACML/AuthzForce approach from experiment-5 onward. Do not wire it into experiments 5+. Do not remove it while experiment-3 exists.

---

## Module Dependency Graph

```
authzforce  (library — no dependencies on other support modules)
    ↑
    ├── kafka-authz
    ├── policy-sync
    ├── rest-authz
    └── topic-auth-xacml
```

All other modules (`authzforce-server`, `message-broker`, `topic-auth-http`, `topic-auth-sync`) are standalone — no intra-support dependencies.

Replace directives in each go.mod wire the local `authzforce` library:
```
replace arrowhead/authzforce => ../authzforce
```
Do not change this pattern. If `authzforce` is moved, update all four dependents.

---

## Critical Invariant: Kafka Consumer Startup Order

**Services that start before their Kafka producer MUST use a partition reader, not a consumer group.**

Consumer groups commit offsets and will miss messages produced before the consumer joins the group.
A partition reader (reading from offset 0 or the earliest available) replays all messages on startup
regardless of order.

Violating this invariant causes silent data loss at startup — the consumer appears healthy but never
receives the first N messages.

This applies to: `kafka-authz`, `data-provider`, and any future Kafka consumer in experiments.

---

## Build

Each module is built independently from its own directory:

```bash
cd support/<module>
go build ./...
go test ./...
```

There is no top-level build for support/. Use `go work` from the repo root to build across modules:

```bash
# from repo root
go build ./...
go test ./...
```

### Test coverage

| Module | Test file(s) | What is covered |
|---|---|---|
| `authzforce-server` | `server_test.go` | `parseGrants`, `parseXACMLRequest`, `parseExternalID`; HTTP handlers: list domains, put policy, PDP (Permit/Deny), /health |
| `rest-authz` | `cache_test.go`, `server_test.go` | TTL cache behaviour; /health, /auth/check (permit/deny/missing fields), proxy (forward/403/401), stats counters |

When adding a new support module, add a corresponding `_test.go` covering its exported functions and HTTP handlers.

---

## API Specifications

Four support services expose HTTP APIs that experiment code depends on.
**Always read the relevant SPEC.md before writing TypeScript types, test assertions,
or dashboard API calls against these services.** Never infer field names from first
principles — use the spec (see EXP-009 in `EXPERIENCES.md`).

| Service | Spec file |
|---|---|
| `authzforce-server` | [`authzforce-server/SPEC.md`](authzforce-server/SPEC.md) |
| `kafka-authz` | [`kafka-authz/SPEC.md`](kafka-authz/SPEC.md) |
| `policy-sync` | [`policy-sync/SPEC.md`](policy-sync/SPEC.md) |
| `rest-authz` | [`rest-authz/SPEC.md`](rest-authz/SPEC.md) |

---

## Environment Variables Reference

[`support/README.md`](README.md) documents every environment variable, default value, and endpoint
for each support service. Read it before writing or modifying any `docker-compose.yml`,
Dockerfile, or service configuration that wires a support module.

---

## Adding a New Support Module

1. Create `support/<name>/go.mod` with module name `arrowhead/<name>`
2. If it depends on `authzforce`, add the replace directive
3. Add the module to `go.work` at the repo root
4. Update the stability table in this file
