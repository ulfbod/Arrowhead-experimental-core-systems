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
| `topic-auth-sync` | **Dead code** — not wired in any experiment | — |

**topic-auth-sync is dead code.** It implements a RabbitMQ topic-authorization sync mechanism that was superseded by the XACML/AuthzForce approach in experiment-5. Do not wire it into new experiments. Do not delete it without confirming it is safe to remove.

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

---

## Adding a New Support Module

1. Create `support/<name>/go.mod` with module name `arrowhead/<name>`
2. If it depends on `authzforce`, add the replace directive
3. Add the module to `go.work` at the repo root
4. Update the stability table in this file
