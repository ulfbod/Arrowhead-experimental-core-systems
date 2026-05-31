# core/PATTERNS.md — Canonical Shape for Core Systems

This document is the single reference for how every core system is structured.
Before implementing any change in `core/`, read this file and compare your approach
to the canonical shape below. Any deviation that is not documented in `GAP_ANALYSIS.md`
is a defect.

This document is consistent with `core/CLAUDE.md` and enforced mechanically by
`core/internal/arch/arch_test.go`.

---

## 1. Package Structure

Every system follows this four-layer layout:

```
internal/<system>/model/       types only
internal/<system>/repository/  storage
internal/<system>/service/     business logic
internal/<system>/api/         HTTP handlers
cmd/<system>/main.go           wiring
```

### Import rules (enforced by arch_test.go)

```
model/       imports: stdlib only (no other core/internal/ package)
             Exception: sub-system model packages may import the designated shared
             types package arrowhead/core/internal/orchestration/model.

repository/  imports: model/, stdlib

service/     imports: model/, repository/, stdlib
             May NOT import api/

api/         imports: service/, model/, arrowhead/core/internal/httputil, stdlib
             May NOT import repository/ directly

cmd/main.go  imports: api/, service/, repository/, model/, config, stdlib
```

---

## 2. External HTTP Dependency Pattern

When a system calls another core system over HTTP:

1. **Define an interface** in `internal/<system>/client/<dep>.go` describing
   what *this* system needs (not what the remote system provides):
   ```go
   type ServiceRegistryClient interface {
       LookupServices(ctx context.Context, req model.OrchestrationRequest) ([]model.OrchestrationResult, error)
   }
   ```

2. **Provide a concrete HTTP implementation** in
   `internal/<system>/client/<dep>_http.go`. All `net/http` logic lives here.

3. **The `service/` package accepts the interface**, never the concrete type:
   ```go
   func NewOrchestrator(srClient client.ServiceRegistryClient, ...) *Orchestrator
   ```

4. **`cmd/main.go` wires the concrete implementation**:
   ```go
   srClient := client.NewSRHTTPClient(srURL)
   svc := service.NewOrchestrator(srClient, ...)
   ```

5. **Tests mock the interface**, not an httptest server:
   ```go
   type fakeSR struct{ results []model.OrchestrationResult }
   func (f *fakeSR) LookupServices(...) ([]model.OrchestrationResult, error) { return f.results, nil }
   ```

**Existing examples:** `internal/orchestration/dynamic/client/` (SR + ConsumerAuth clients).

**Counter-examples (already refactored):** The old inline HTTP calls in
`orchestrator.go` that mixed HTTP logic with business logic.

---

## 3. Handler Conventions

Every handler follows this exact sequence — no exceptions:

```
1. Method check     → 405 if wrong method (use httputil.RequireMethod)
2. Decode body      → 400 on JSON error (use httputil.DecodeJSON)
3. Validate fields  → 400 on missing required field (explicit checks)
4. Delegate         → call service method
5. Map error        → use statusFor(err) with errors.Is() against sentinel errors
6. Write response   → httputil.WriteJSON or httputil.WriteError
```

**No business logic in handlers.** All logic lives in `service/`.

### Error mapping pattern

Every handler file defines a `statusFor` helper (or inline switch):

```go
func statusFor(err error) int {
    switch {
    case errors.Is(err, model.ErrNotFound):  return http.StatusNotFound
    case errors.Is(err, model.ErrLocked):    return http.StatusLocked
    case errors.Is(err, model.ErrForbidden): return http.StatusForbidden
    default:                                  return http.StatusBadRequest
    }
}
```

Use `errors.Is()`, never `==` or `err.Error()` string comparison.

### Shared handler helpers

All handlers import `arrowhead/core/internal/httputil` for:

| Function | Purpose |
|---|---|
| `httputil.WriteJSON(w, status, v, origin)` | Set Content-Type, call WriteHeader, encode JSON |
| `httputil.WriteError(w, status, msg, origin)` | Write AH5 error envelope |
| `httputil.ErrorTypeForStatus(status)` | Map HTTP status → AH5 exceptionType string |
| `httputil.RequireMethod(w, r, method)` | Return false + 405 if wrong method |
| `httputil.DecodeJSON(w, r, &v)` | Return false + 400 on decode failure |
| `httputil.ExtractBearer(r)` | Return token string or "" |

**No handler package defines its own `writeJSON`, `writeError`, or
`errTypeForStatus` functions.** Use `httputil` instead.

### AH5 error envelope (same shape for all systems)

```json
{
  "errorMessage": "human-readable description",
  "errorCode": 400,
  "exceptionType": "INVALID_PARAMETER",
  "origin": "serviceregistry"
}
```

`exceptionType` values (from `httputil.ErrorTypeForStatus`):

| Status | exceptionType |
|---|---|
| 400 | `INVALID_PARAMETER` |
| 401 | `AUTH_EXCEPTION` |
| 403 | `FORBIDDEN` |
| 404 | `DATA_NOT_FOUND` |
| 423 | `LOCKED` |
| 501 | `NOT_IMPLEMENTED` |
| other | `ARROWHEAD_EXCEPTION` |

---

## 4. Test Conventions

### Unit tests

- `api/` tests use `httptest.NewRecorder()` as the response writer and call
  handler methods directly with `httptest.NewRequest()`.
- External dependencies (other systems) are mocked via interface implementations,
  not `httptest.NewServer()`.
- Example fake:
  ```go
  type fakeSvc struct { ... }
  func (f *fakeSvc) DoThing(...) (...) { return f.result, f.err }
  ```

### Integration tests

- Live in `core/internal/integration/`.
- Wire all systems **in-process** (no Docker, no real network).
- Every system with SQLite persistence must have at least one integration test that
  exercises the SQLite path using `DB_PATH=:memory:`.

### Coverage gate

`go test -coverprofile=cover.out ./...` — all packages touched in a given step
must reach ≥ 80% line coverage.

---

## 5. Canonical Reference Implementation

**Authentication** (`internal/authentication/`) is the canonical reference system.

Justification:
- Clean four-layer structure: model → repository → service → api
- Handler follows the exact sequence above (method check → decode → validate → service → error map → write)
- Uses `errors.Is()` for sentinel mapping
- Has comprehensive unit tests covering all endpoints and error cases
- Has integration test coverage for the SQLite path

**When implementing changes to any other system, read the Authentication packages
first** and model your approach on their structure.

---

*Created: 2026-05-29. Consistent with core/CLAUDE.md. Enforced by core/internal/arch/arch_test.go.*
