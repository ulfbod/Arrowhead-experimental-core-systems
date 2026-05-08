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

Each `test-system.sh` begins with a **Pre-flight smoke-check** section that runs before any application-level tests. Smoke-check failures call `smoke_fail` which exits immediately (exit 2) so that downstream cascade failures do not obscure the root cause.

Each `test-system.sh` sources the shared assertion library at the top:

```bash
PASS=0
FAIL=0
source "$(dirname "$0")/../test-lib.sh"
```

The library (`experiments/test-lib.sh`) provides: `pass`, `fail`, `check_eq`, `http_code`, `http_body`, `smoke_fail`, `smoke_http`, `assert_http`, `assert_contains`, `assert_not_contains`, `assert_json_field`, `assert_json_value`, `assert_json_gt`. Use these — do NOT copy-paste inline implementations. The key rule: **never use `echo "$x" | grep -q` as a test assertion** — use `assert_contains`/`assert_json_value` instead, which use `[[ ]]` matching to avoid the SSE false-negative trap (EXP-004, EXP-006).

Smoke-check responsibilities per experiment type:
- All experiments: core services reachable, message broker management API reachable, primary PEP service healthy, grants seeded in ConsumerAuth.
- XACML experiments (5, 6): additionally waits up to 30 s for `policy-sync` to report `synced=true` and verifies `domainExternalId` matches the experiment's `AUTHZFORCE_DOMAIN` — a mismatch causes every auth check to return Deny silently (see `EXPERIENCES.md` EXP-001).

When adding a new experiment:
1. Define `smoke_fail` and `smoke_http` helpers (see any existing `test-system.sh`).
2. Add a `=== Pre-flight: smoke-check ===` section before section 1.
3. Include at minimum: one representative core service, the primary PEP, the message broker, and (for XACML stacks) the policy-sync domain check.

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

Add each new Go service module to `go.work` — see **Go workspace** section below.

---

## Go workspace (go.work)

The repository uses a Go workspace (`go.work` at the root) that spans every Go module:
`core/`, all `support/<module>/` directories, and every
`experiments/experiment-N/services/<service>/` directory.

This lets `go build ./...` and `go test ./...` from the **repo root** build and test the
entire codebase in one step without manual `replace` directives between modules.

**When you create a new Go service, you must register it or the workspace build silently
fails to resolve it:**

```bash
# From the repo root — run once per new service directory:
go work use ./experiments/experiment-N/services/<service-name>
```

To confirm the current module list:

```bash
go work edit -json | jq '.Use[].DiskPath'
```

### Current workspace modules

| Group | Modules |
|---|---|
| Core | `core` |
| Experiment 1 | `experiment-1/example-client`, `experiment-1/service` |
| Experiment 2 | `experiment-2/services/consumer`, `edge-adapter`, `robot-fleet`, `robot-simulator`, `tests` |
| Experiment 3 | `experiment-3/services/consumer-direct` |
| Experiment 4 | `experiment-4/services/consumer-direct`, `robot-fleet` |
| Experiment 5 | `experiment-5/services/analytics-consumer`, `consumer-direct`, `robot-fleet` |
| Experiment 6 | `experiment-6/services/data-provider`, `rest-consumer` |
| Support | `authzforce`, `authzforce-server`, `kafka-authz`, `message-broker`, `policy-sync`, `rest-authz`, `topic-auth-http`, `topic-auth-sync`, `topic-auth-xacml` |

**Experiment status:** Experiment-6 is the **active baseline** for new development. Experiments 1–5 are **historical reference** — preserved to document the design iterations that led to experiment-6, but not actively maintained and not suitable templates for new experiments. When building experiment-7 or later, treat experiment-6 as the authoritative starting point; ignore earlier experiments unless specifically tracing a design decision.

**Note:** a service that has no unit tests and is only built inside Docker does not need
to be in `go.work`. Adding it anyway is harmless; omitting it is only fine if you never
want `go build ./...` from the root to cover it.

---

## Adding a new experiment — step-by-step checklist

Use this when creating experiment-N. Work through the items in order; each group can cause
hard-to-diagnose failures if skipped.

### Directory and documentation

- [ ] Create `experiments/experiment-N/`
- [ ] Write `README.md`: one-paragraph description, service table (name | port | what it does), startup order, architecture note
- [ ] Document port assignments — pick numbers that do not collide with existing experiments

### Docker stack

- [ ] Create `docker-compose.yml` defining the full stack
- [ ] Use environment variables for **all** ports, hostnames, domain names, and URLs — never hardcode (checked by pre-flight checklist in `EXPERIENCES.md`)
- [ ] If using XACML/AuthzForce: set `AUTHZFORCE_DOMAIN` identically across `policy-sync`, `kafka-authz`, `rest-authz`, and `topic-auth-xacml`; use a name unique to this experiment (e.g. `arrowhead-exp7`) to avoid collisions (see `EXPERIENCES.md` EXP-001)
- [ ] Create `dockerfiles/` with one Dockerfile per service; use `--build` with every `docker compose up`

### Go services

For each new Go service:

- [ ] Create `services/<name>/go.mod` with `module arrowhead/<name>`
- [ ] If it imports a support module, add the replace directive:
  ```
  replace arrowhead/<dep> => ../../../../support/<dep>
  ```
- [ ] **Register in `go.work`** (easy to miss — causes silent workspace build failures):
  ```bash
  go work use ./experiments/experiment-N/services/<name>
  ```
- [ ] Verify: `go build ./...` from repo root succeeds with no "cannot find module" errors

### Dashboard (if applicable)

- [ ] If sharing source files with experiments 5–6, symlink from `support/dashboard-shared/`; run `bash support/dashboard-shared/check-dashboard-shared.sh` (expect 20/20 PASS, plus N new symlinks)
- [ ] If the dashboard contains symlinks, set `build.context: ../..` in `docker-compose.yml` and use the three-step Dockerfile pattern to resolve them (see `EXPERIENCES.md` EXP-010)
- [ ] Set `package.json` build script to `tsc -p tsconfig.app.json` (not bare `tsc`) to exclude test files (see `EXPERIENCES.md` EXP-008)

### test-system.sh

- [ ] Source `../test-lib.sh` at the top — do **not** copy helpers inline
- [ ] Add a `=== Pre-flight: smoke-check ===` section using `smoke_http` for each critical service
- [ ] For XACML stacks: add a `policy-sync` smoke-check that waits up to 30 s for `synced=true` and asserts the correct `domainExternalId`
- [ ] Use `assert_json_value` / `assert_contains` / `assert_json_gt` for all assertions — never `echo "$x" | grep -q` (see `EXPERIENCES.md` EXP-004/EXP-006)

### Final verification

- [ ] `docker compose up -d --build` → `bash test-system.sh` → all PASS
- [ ] `go build ./...` and `go test ./...` from repo root → no errors
- [ ] If a new support module was added: update the stability table in `support/CLAUDE.md` and the module overview in `support/README.md`

---

## Files shared across experiments — symlinked via support/dashboard-shared/

Ten source files are shared between the experiment-5 and experiment-6 dashboards.
They live canonically in `support/dashboard-shared/` and are **symlinked** into both
experiment `dashboard/src/` directories:

| Canonical file | Symlinked from |
|---|---|
| `support/dashboard-shared/main.tsx` | `experiment-5/dashboard/src/main.tsx`, `experiment-6/dashboard/src/main.tsx` |
| `support/dashboard-shared/hooks/usePolling.ts` | both experiments |
| `support/dashboard-shared/config/context.tsx` | both experiments |
| `support/dashboard-shared/config/defaults.ts` | both experiments |
| `support/dashboard-shared/config/types.ts` | both experiments |
| `support/dashboard-shared/components/StatusDot.tsx` | both experiments |
| `support/dashboard-shared/views/HealthView.tsx` | both experiments |
| `support/dashboard-shared/views/GrantsView.tsx` | both experiments |
| `support/dashboard-shared/views/PolicyView.tsx` | both experiments |
| `support/dashboard-shared/views/LiveDataView.tsx` | both experiments |

**Rule: always edit the canonical file in `support/dashboard-shared/`.** Never edit
the symlinks directly — they are resolved by the filesystem and by Docker at build time.

To verify all symlinks are intact and no content has drifted:

```bash
bash support/dashboard-shared/check-dashboard-shared.sh
```

**Docker build context:** both dashboard Dockerfiles use `context: ../..` (repo root)
so that `support/dashboard-shared/` is within the build context. Do not change this
back to `context: .` — the COPY of shared files would fail with "forbidden path
outside build context".

**Docker symlink resolution:** Docker copies relative symlinks as-is; inside the
container the relative paths are dangling. The Dockerfiles handle this explicitly:
after copying the dashboard, they copy `support/dashboard-shared/` to a temp path,
remove all dangling symlinks in `src/`, and copy in the real files. See EXP-010 in
`EXPERIENCES.md`.

**Development (npm run dev):** symlinks are resolved normally by the filesystem; no
special setup needed.

---

## Before starting work on a specific experiment

If building a **new experiment**, use experiment-6 as the template — not any earlier experiment.

1. Read that experiment's `README.md` — it contains the service table, port assignments,
   startup order, and architecture specific to that experiment.
2. If the experiment uses support modules, read [`support/README.md`](../support/README.md)
   for environment variable defaults and endpoint references.

---

## Pre-flight checklist

Before starting implementation in any experiment, read the pre-flight checklist in
[`/EXPERIENCES.md`](../EXPERIENCES.md). It documents seven recurring failure modes with
root causes and fixes.
