# CLAUDE.md — ArrowheadCore

This file is the entry point for any AI-assisted session in this repository.
Read it fully before doing anything else.

---

## What this repository is

A Go implementation of the **Arrowhead 5 core systems** plus a series of
experiments that use those systems to explore policy-based IoT data-plane
authorization.

Three clearly separated areas:

| Area | Path | Standard |
|---|---|---|
| Core systems | `core/` | Strict — spec-compliant, fully tested |
| Shared support modules | `support/` | Stable — production-quality shared libraries |
| Experiments | `experiments/` | Exploratory — may be incomplete or in-progress |

**Experiment status:** Experiments 1–5 are preserved as historical reference — they document the design iterations that led to the current architecture and must not be removed. They are **not actively maintained**. Experiment-6 is the **active baseline**; new experiments (experiment-7 and beyond) should use it as their template. Do not load experiments 1–5 as reference material when working on experiment-6 or later.

---

## The one rule that governs everything

```
experiments/ ──HTTP──▶ core/
support/     ──HTTP──▶ core/   (at runtime)
```

**No code outside `core/` may import packages from `core/internal/` or `core/cmd/`.**
If an experiment needs something the core does not expose, add it to the core HTTP API.
Never reach into core internals.

---

## Before starting any task

1. **Identify which area the task is in** (core/, support/, or experiments/).
2. **Read the CLAUDE.md for that area** — each has rules specific to its context:
   - `core/` work → read [`core/CLAUDE.md`](core/CLAUDE.md) first
   - `support/` work → read [`support/CLAUDE.md`](support/CLAUDE.md) first
   - `experiments/` work → read [`experiments/CLAUDE_EXPERIMENTS.md`](experiments/CLAUDE_EXPERIMENTS.md) first
3. **Read [`EXPERIENCES.md`](EXPERIENCES.md)** — ten documented failure modes with root causes and fixes. The pre-flight checklist at the bottom applies to every experiment task.

---

## Building and testing

The repo uses a Go workspace. From the repo root, a single command covers all modules:

```bash
go build ./...
go test ./...
```

For `core/` specifically (fastest feedback cycle):

```bash
cd core
go build ./...
go test ./...
```

For a full experiment stack:

```bash
cd experiments/experiment-N
docker compose up --build
```

The `core/docker-compose.yml` starts **ServiceRegistry only** — it is not the full stack.
Full multi-service stacks are in each experiment directory.

### System tests

Each area has a `test-system.sh` script that runs a self-contained test suite and prints a PASS/FAIL summary:

| Script | Requires Docker | What it covers |
|---|---|---|
| `core/test-system.sh` | No | build + vet + unit/integration tests for all core systems |
| `experiments/experiment-5/test-system.sh` | Yes (`docker compose up -d`) | AuthzForce, policy-sync, kafka-authz, SSE stream, analytics-consumer, revocation |
| `experiments/experiment-6/test-system.sh` | Yes (`docker compose up -d`) | kafka-authz, rest-authz, policy-sync, data-provider, rest-consumer, revocation |

Run a system test from its experiment directory with the stack already up:

```bash
cd experiments/experiment-2
docker compose up -d --build
bash test-system.sh
```

---

## Source of truth hierarchy (core/ work)

When working in `core/`, authority is strictly ordered:

1. `core/SPEC.md` — behavioral contract; do not deviate
2. `core/TEST_PLAN.md` — defines correctness
3. `core/EXAMPLES.md` — expected request/response formats
4. `core/CLAUDE.md` — implementation rules and prohibitions
5. `core/GAP_ANALYSIS.md` — intentional deviations from the AH5 spec (D1–D8)

If these files conflict with anything else (including this file), they win.

---

## Cross-cutting prohibitions

- Do NOT modify `core/` files to serve an experiment's needs
- Do NOT add endpoints, fields, or behaviors not in `core/SPEC.md`
- Do NOT wire `support/topic-auth-sync` into experiments 5 or later — it is superseded by the XACML/AuthzForce approach from experiment-5 onward. It remains active in experiment-3 and must not be removed while that experiment exists.
- Do NOT hardcode ports, hostnames, or AuthzForce domain names — always use environment variables
- Do NOT use Kafka consumer group readers for services that start before their producer — use partition readers (see `EXPERIENCES.md` EXP-007)

---

## Key files at a glance

| File | Purpose |
|---|---|
| `ARCHITECTURE.md` | Full system diagram, API surface, directory tree |
| `EXPERIENCES.md` | Seven real bugs — root causes, fixes, generalised lessons |
| `REPO_RULES.md` | Short governance summary |
| `core/SPEC.md` | The authoritative API contract |
| `core/GAP_ANALYSIS.md` | Intentional spec deviations |
| `support/CLAUDE.md` | Which support modules are stable, which is dead code, Kafka invariant |
| `experiments/CLAUDE_EXPERIMENTS.md` | Experiment rules, canonical structure, shared files |
