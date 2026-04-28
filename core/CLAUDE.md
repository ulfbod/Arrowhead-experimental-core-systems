# core/CLAUDE.md

## Purpose

This directory contains the **strict, spec-compliant Arrowhead Core Service Registry** implementation.

All code here is held to the highest standard of correctness. It is stable, minimal, and independent.

---

## Source of Truth (strict order)

1. `SPEC.md` — complete behavioral contract
2. `TEST_PLAN.md` — defines correctness
3. `EXAMPLES.md` — clarifies expected behavior

Claude MUST NOT deviate from these files.

---

## Explicit Prohibitions

The following are **strictly forbidden** in `core/`:

### UI / frontend logic
- No CORS headers or OPTIONS preflight handling
- No HTML, CSS, JavaScript, or template rendering
- No browser-specific response headers
- No static file serving

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
- No calls to external services at runtime

---

## Critical Implementation Requirement

Claude MUST implement ALL fields and behaviors defined in SPEC.md, including:

- `metadata` — stored and returned; queryable via key-value subset match
- `version` — stored and returned; defaults to 1; included in uniqueness key
- `secure`, `authenticationInfo` — stored and returned
- Interface matching — case-insensitive; ALL requested interfaces must be present
- Overwrite semantics — duplicate key `(serviceDefinition, systemName, address, port, version)` overwrites the existing entry, keeping the same `id`

Partial implementations are NOT allowed.

---

## Language Rules

- Go ONLY
- Prefer standard library
- Minimal dependencies

---

## Architecture

```
core/
├── cmd/serviceregistry/   ← entry point; wiring only
└── internal/
    ├── api/               ← HTTP handlers; no business logic
    ├── service/           ← business logic and validation
    ├── repository/        ← storage; implements Repository interface
    ├── model/             ← data types; no logic
    └── config/            ← environment configuration
```

Rules:
- No circular dependencies between packages
- No package may import `cmd/`
- `api/` must not contain business logic — delegate to `service/`
- `model/` must not import any other internal package

---

## Validation Rules

All required fields must be validated strictly. Return `400 Bad Request` with an error message for any violation:

| Field | Rule |
|---|---|
| `serviceDefinition` | non-empty string (after trimming whitespace) |
| `providerSystem` | required object |
| `providerSystem.systemName` | non-empty string |
| `providerSystem.address` | non-empty string |
| `providerSystem.port` | integer > 0 |
| `serviceUri` | non-empty string |
| `interfaces` | non-empty list |
| `version` | defaults to 1 if omitted or ≤ 0 |

---

## Query Matching Rules

All filters are ANDed. A filter with a zero value is ignored:

| Filter | Rule |
|---|---|
| `serviceDefinition` | exact string match |
| `interfaces` | service must provide ALL requested (case-insensitive) |
| `metadata` | service must contain ALL requested key-value pairs |
| `versionRequirement` | exact version match; 0 = no filter |

---

## Testing Rules

- Tests MUST reflect `TEST_PLAN.md` fully
- Use table-driven tests where multiple cases share the same shape
- No mocking of the repository — use `MemoryRepository` directly in tests
- All spec features must be covered: metadata, version, duplicates, edge cases, error handling

---

## Build Requirements

Must always pass from `core/`:

```bash
go build ./...
go test ./...
```

---

## Workflow

1. Read `SPEC.md` fully before making any change
2. Implement or update models
3. Implement or update logic
4. Write or update tests
5. Verify `go build ./...` and `go test ./...`
6. Remove dead code
