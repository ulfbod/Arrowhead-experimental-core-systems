# experiments/CLAUDE_EXPERIMENTS.md

## Purpose

This directory is for **experimental extensions** built on top of the Arrowhead Core Service Registry.

Experiments are exploratory, potentially unstable, and not held to the strict compliance standard of `core/`.

---

## Fundamental Rule: HTTP Boundary

Experiments MUST communicate with the core exclusively via its HTTP API.

```
experiments/ ──HTTP──▶ core (POST /serviceregistry/register, POST /serviceregistry/query, ...)
```

**Never** cross this boundary with code:

- Do NOT import any package from `core/internal/`
- Do NOT import `core/cmd/`
- Do NOT modify any file under `core/`

If you need something the core does not expose, add it to the core API — do not reach in.

---

## What belongs here

- Frontend UIs (web dashboards, visualizations)
- Simulation drivers or scenario runners
- Client libraries that wrap the core HTTP API
- Data collection, logging, or analysis tools
- Prototype services that register themselves with the core

---

## What does NOT belong here

- Modifications to core business logic
- Alternative implementations of the Service Registry
- Anything that must be true for the core spec to hold

---

## Structure

Each experiment should be self-contained in its own subdirectory:

```
experiments/
├── CLAUDE_EXPERIMENTS.md
├── dashboard/        ← example: a React frontend
├── scenario-runner/  ← example: a Go client that exercises the registry
└── ...
```

Each subdirectory manages its own dependencies (`go.mod`, `package.json`, etc.) and build tooling independently.

---

## Referring to the core

The core base URL is configurable. Use an environment variable:

```
CORE_URL=http://localhost:8080
```

Do not hardcode addresses.

---

## Build and test rules

- Experiments have no requirement to pass `core/`'s test suite
- Each experiment is responsible for its own build and tests
- Broken experiments MUST NOT prevent `core/` from building or testing

### Test-first approach

When adding new API functions, UI components, or service handlers, **write the tests before the implementation** and iterate until all tests pass:

1. Write tests that specify the expected contract (URLs, request bodies, response shapes, rendered elements, error handling).
2. Implement the feature.
3. Run the tests; fix either the implementation or the tests (if the spec was wrong) until all pass.

This applies clearly to: API functions, pure logic, component structure, and HTTP handler behaviour.
It applies partially to: interactive component behaviour where element structure is decided during implementation — write the structural assertions first, add interaction assertions as the design stabilises.
It applies less to: free-form content such as architecture diagrams, whose exact wording is chosen while writing.

### Unit tests

Each Go service with meaningful logic should have a `_test.go` file alongside it. Run from the service directory:

```bash
cd experiments/experiment-N/services/<service>
go test ./...
```

### System tests

Experiments that have a running Docker stack provide a `test-system.sh` script. Run it from the experiment directory with the stack already up:

```bash
cd experiments/experiment-N
docker compose up -d --build
bash test-system.sh
```

The script prints a PASS/FAIL summary for each check and exits 1 if any check fails.

---

## Stability expectation

Code here may be incomplete, in-progress, or abandoned.
It will not be held to the correctness standard of `core/`.

---

## Canonical experiment structure

A new experiment directory should follow this layout:

```
experiments/experiment-N/
├── README.md                  ← required: what the experiment demonstrates
├── docker-compose.yml         ← required: full stack definition
├── dockerfiles/               ← one Dockerfile per service
├── rabbitmq/                  ← broker config (if AMQP is used)
├── services/                  ← experiment-local Go services (own go.mod each)
└── dashboard/                 ← React + Vite UI (own package.json)
```

Support services (shared across experiments) live in `support/`. Before adding a new service
locally, check whether an equivalent already exists there.

Add each new Go service module to `go.work` at the repo root.

---

## Files shared across experiments — mirror changes manually

The following files are **duplicated** across experiments and must be kept in sync by hand:

| File | Shared between |
|---|---|
| `experiment-5/dashboard/src/` | experiment-5, experiment-6 (identical source) |
| `experiment-6/dashboard/src/` | experiment-5, experiment-6 (identical source) |

If you change dashboard logic in one of these experiments, apply the same change to the other.
There is currently no automated check for divergence.

---

## Before starting work on a specific experiment

1. Read that experiment's `README.md` — it contains the service table, port assignments,
   startup order, and architecture specific to that experiment.
2. If the experiment uses support modules, read [`support/README.md`](../support/README.md)
   for environment variable defaults and endpoint references.

---

## Pre-flight checklist

Before starting implementation in any experiment, read the pre-flight checklist in
[`/EXPERIENCES.md`](../EXPERIENCES.md). It documents seven recurring failure modes with
root causes and fixes.
