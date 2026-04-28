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

---

## Stability expectation

Code here may be incomplete, in-progress, or abandoned.
It will not be held to the correctness standard of `core/`.
