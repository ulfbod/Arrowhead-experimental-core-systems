# core/CLAUDE.md

## Purpose

This directory contains the **six Arrowhead 5 (AH5) core systems**, each as an independent Go binary, plus a shared browser dashboard.

All Go code here is held to the highest standard of correctness. It is stable, minimal, and independent.

---

## Source of Truth (strict order)

1. `SPEC.md` — complete behavioral contract for all six systems
2. `TEST_PLAN.md` — defines correctness (per-system scenarios)
3. `TESTING.md` — how to run tests, key techniques, known limitations
4. `EXAMPLES.md` — clarifies expected behavior
5. `GAP_ANALYSIS.md` — documents deviations, design decisions, and ambiguities

Claude MUST NOT deviate from these files.

---

## Systems

| System | Binary | Port | Package prefix |
|---|---|---|---|
| ServiceRegistry | `cmd/serviceregistry` | 8080 | `internal/` |
| Authentication | `cmd/authentication` | 8081 | `internal/authentication/` |
| ConsumerAuthorization | `cmd/consumerauth` | 8082 | `internal/consumerauth/` |
| DynamicOrchestration | `cmd/dynamicorch` | 8083 | `internal/orchestration/dynamic/` |
| SimpleStoreOrchestration | `cmd/simplestoreorch` | 8084 | `internal/orchestration/simplestore/` |
| FlexibleStoreOrchestration | `cmd/flexiblestoreorch` | 8085 | `internal/orchestration/flexiblestore/` |
| CertificateAuthority | `cmd/ca` | 8086 | `internal/ca/` |

Shared orchestration types: `internal/orchestration/model/`

---

## Explicit Prohibitions

The following are **strictly forbidden** in `core/` Go code:

### UI / frontend logic in Go

- No CORS headers or OPTIONS preflight handling in Go handlers
- No HTML, CSS, or JavaScript template rendering in Go handlers
- No browser-specific response headers beyond Content-Type

**Exception — dashboard static serving:**
`cmd/serviceregistry/main.go` MAY serve the pre-built dashboard files from
`dashboard/dist/`. This is a file-serving mechanism only; no frontend logic lives in Go code.

### Experimental features

- No speculative endpoints or behaviors
- No feature flags
- No code that is "in progress" or "to be decided"

### API changes not in SPEC.md

- No new endpoints beyond those defined in SPEC.md
- No new request or response fields beyond those defined in SPEC.md
- No change to HTTP methods, status codes, or paths without a SPEC.md update
- No removal of fields or endpoints that SPEC.md requires

### External coupling

- No imports from `../experiments/`
- No dependency on code outside `core/`
- No runtime calls to external services, except:
  - DynamicOrchestration calling ServiceRegistry and ConsumerAuthorization (documented in SPEC.md)

---

## Dashboard Rules

The dashboard (`core/dashboard/`) is a React + TypeScript + Vite application.

Rules:
- It MUST only communicate with the backends via HTTP (the spec-defined APIs)
- It MUST NOT import any Go packages
- It is built independently with `npm install && npm run build`
- The built output (`dashboard/dist/`) is served by the ServiceRegistry binary when present
- In development, Vite proxies all system API calls to their respective ports

The dashboard is **not** required for `go build` or `go test` to pass.

---

## Architecture Rules

- No circular dependencies between packages
- No package may import `cmd/`
- `api/` packages must not contain business logic — delegate to `service/`
- `model/` packages must not import any other internal package
- `dashboard/` must not import any Go package
- Systems communicate only via HTTP, never via Go package imports

---

## Validation Rules

### ServiceRegistry

All required fields must be validated strictly. Return `400` for any violation:

| Field | Rule |
|---|---|
| `serviceDefinition` | non-empty string |
| `providerSystem` | required object |
| `providerSystem.systemName` | non-empty string |
| `providerSystem.address` | non-empty string |
| `providerSystem.port` | integer > 0 |
| `serviceUri` | non-empty string |
| `interfaces` | non-empty list |
| `version` | defaults to 1 if omitted or ≤ 0 |

### ConsumerAuthorization

| Field | Rule |
|---|---|
| `consumerSystemName` | non-empty string |
| `providerSystemName` | non-empty string |
| `serviceDefinition` | non-empty string |

### SimpleStore / FlexibleStore rules

| Field | Rule |
|---|---|
| `consumerSystemName` | non-empty string |
| `serviceDefinition` | non-empty string |
| `provider.systemName` | non-empty string |
| `provider.address` | non-empty string |
| `provider.port` | integer > 0 |
| `serviceUri` | non-empty string |
| `interfaces` | non-empty list |

---

## Query / Matching Rules

### ServiceRegistry query

All filters ANDed. Zero value = no filter:

| Filter | Rule |
|---|---|
| `serviceDefinition` | exact string match |
| `interfaces` | service must provide ALL requested (case-insensitive) |
| `metadata` | service must contain ALL requested key-value pairs |
| `versionRequirement` | exact version match; 0 = no filter |

### FlexibleStore orchestration

1. `consumerSystemName` == `requesterSystem.systemName`
2. `serviceDefinition` == `requestedService.serviceDefinition`
3. Rule's `metadataFilter` ⊆ request's `requestedService.metadata`
4. Results sorted by `priority` ascending (`0` treated as `MaxInt`)

---

## Language Rules

- Go ONLY for backend
- TypeScript + React for dashboard
- Prefer standard library (Go); minimal npm dependencies (dashboard)

---

## Build Requirements

Must always pass from `core/`:

```bash
go build ./...
go test ./...
```

Dashboard build (optional, run from `core/dashboard/`):

```bash
npm install
npm run build
```

---

## Workflow

1. Read `SPEC.md` fully before making any change
2. Implement or update models
3. **Write or update tests first** — specify the expected behaviour (status codes, response shapes, error cases) before writing the implementation
4. Implement or update logic until all tests pass
5. Verify `go build ./...` and `go test ./...`
6. Remove dead code
