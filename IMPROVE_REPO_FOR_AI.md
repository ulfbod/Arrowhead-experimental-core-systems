# IMPROVE_REPO_FOR_AI.md

This file is the self-contained execution plan for a set of structural improvements to the
ArrowheadCore repository, derived from analysis of AI-assisted development best practices.
Work through the steps in order. Tick each checkbox when the step is complete.
Context-safe: the file can be used as the sole prompt after a context reset.

---

## Why these steps exist

Six structural gaps were identified that reduce the safety and consistency of AI-assisted
work in this repo:

1. Architectural constraints live in prose (`core/CLAUDE.md`) but are not enforced by any
   failing test — a session that misreads the rules produces a violation that `go build`
   and `go test` will not catch.
2. External HTTP clients (ServiceRegistry, ConsumerAuthorization inside DynamicOrchestration)
   are concrete structs, not interfaces — no clean seam for testing or swapping.
3. Integration test coverage of SQLite paths is thin relative to unit tests.
4. `CONFORMANCE_UPDATE_PLAN.md` contains TDD recipes (test code, implementation snippets)
   that belong in actual test files, not in a planning document.
5. `core/` and `core-evol/` must be kept in sync manually — no mechanical guarantee.
6. The eight core systems have no single canonical shape document to copy from.

Seven steps address these in dependency order. Steps H and I address a seventh structural
gap: six verbatim copies of the same handler helper code (`writeJSON`, `writeError`,
`errTypeForStatus`) across handler packages, which causes pattern drift and makes
AI-assisted changes inconsistent. These slot after Step C and before Step D so that
all Phase 1 handler code uses the shared helpers from the start.

---

## Files to read before starting any step

```
CLAUDE.md                          root-level repo rules
core/CLAUDE.md                     core implementation rules and prohibitions
core/GAP_ANALYSIS.md               gap descriptions, design decisions (G-IDs)
CONFORMANCE.md                     per-system ratings and phase plan (G-IDs, no recipes)
CONFORMANCE_UPDATE_PLAN.md         TDD execution plan — Steps 22–26 (Phase 1)
EXPERIENCES.md                     documented failure modes and pre-flight checklist
```

## Standard pre-flight (run before and after every step)

```bash
cd core && go build ./... && go vet ./... && go test -race ./...
cd core-evol && go build ./... && go test -race ./...
```

---

## Step A — Write the architectural constraint test

**Goal:** Mechanically enforce the import rules in `core/CLAUDE.md` so that violations are
caught by `go test`, not by human review.

**Status:** - [x] Complete

### What to create

`core/internal/arch/arch_test.go` — a Go test that uses `os/exec` to call
`go list -json -deps ./...` from the `core/` directory, parses the import graph, and
asserts each of the following rules. Fail with a descriptive message naming the offending
package and the rule violated.

### Rules to encode

| Rule | Expression |
|---|---|
| No package outside `core/` imports `core/internal/` | Any import path containing `arrowhead/core/internal` must have an importer whose module is `arrowhead/core` |
| `model/` packages have zero internal imports | Packages matching `arrowhead/core/internal/*/model` or `arrowhead/core/internal/model` import no other `arrowhead/core/internal/` package |
| `api/` packages do not import `repository/` | Packages matching `arrowhead/core/internal/*/api` must not import any package matching `arrowhead/core/internal/*/repository` |
| `service/` packages do not import `api/` | Packages matching `arrowhead/core/internal/*/service` must not import any package matching `arrowhead/core/internal/*/api` |
| Nothing imports `cmd/` | No package imports any path containing `arrowhead/core/cmd` |

### Implementation approach

Use `os/exec` to run `go list -json -deps ./...` from the `core/` directory within the
test. Parse each JSON object (one per package) to extract `ImportPath` and `Imports`.
Build a map and assert the rules above. No external dependencies needed — `os/exec`,
`encoding/json`, and `bytes` are sufficient.

### Completion criteria

- [ ] `go test ./internal/arch/...` passes when rules are satisfied
- [ ] Temporarily add a forbidden import to one package → test fails with a clear message
      naming the violating package and rule → revert
- [ ] `go test -race ./...` from `core/` passes (all other tests unaffected)

---

## Step B — Write Phase 1 failing tests into actual test files

**Goal:** Move TDD recipes out of `CONFORMANCE_UPDATE_PLAN.md` and into the actual test
files where they belong. Tests must fail before implementation (for the right reason, not
a compile error) and pass after.

**Status:** - [ ] Complete

**Source:** For the full test body of each function listed below, read the TDD cycle
sections in `CONFORMANCE_UPDATE_PLAN.md` Steps 22–26. This step is purely about placing
those tests in the correct files.

### Step 22 tests → `core/internal/api/ah5_handler_test.go`

- [ ] `TestSystemRevokeUsesTokenIdentity` — fake Auth server returns `systemName`; revoke
      without `?name=` uses token identity; system is gone afterward
- [ ] `TestSystemRevokeWithoutBearerReturns401` — no header, no `?name=` → 401
- [ ] `TestSystemRevokeAuthUnreachableReturns401` — Auth URL unreachable → 401 (fail-closed)

**Expected compile failure:** `NewAH5Handler` does not yet accept an `authURL` parameter.
Fix: add a second `authURL string` parameter with empty default behaviour (pass `""` in all
existing call sites) so the file compiles. Tests then fail at runtime with wrong status codes.

### Step 23 tests → `core/internal/blacklist/api/handler_test.go`

- [ ] `TestLookupRequiresBearerWhenAuthConfigured` — with authURL set: no header → 401,
      valid header → 200
- [ ] `TestCheckRequiresBearerWhenAuthConfigured` — same pattern for `/check/{name}`
- [ ] `TestMgmtQueryModeActives` — `{"mode":"ACTIVES"}` returns only active entries
- [ ] `TestMgmtQueryModeAll` — `{"mode":"ALL"}` returns all entries
- [ ] `TestMgmtQueryModeInactives` — `{"mode":"INACTIVES"}` returns only inactive entries
- [ ] `TestMgmtQueryModeInvalid` — `{"mode":"BOGUS"}` returns 400

**Expected compile failure:** `NewBlacklistHandler` does not yet accept `authURL`.
Same fix: add parameter with empty-string no-op behaviour.

### Step 24 tests

→ `core/internal/orchestration/dynamic/service/orchestrator_test.go`:
- [ ] `TestOrchestrationResultHasCloudIdentifier` — result contains `CloudIdentifier == "LOCAL"`
- [ ] `TestOrchestrationResultHasExclusiveUntilWhenLocked` — locked provider → non-empty
      RFC3339 `ExclusiveUntil`
- [ ] `TestOrchestrationResultNoExclusiveUntilWhenUnlocked` — no lock → `ExclusiveUntil` empty

→ `core/internal/orchestration/simplestore/service/orchestrator_test.go` and
  `core/internal/orchestration/flexiblestore/service/orchestrator_test.go`:
- [ ] `TestOrchestrationResultHasCloudIdentifier` (same assertion, each orchestrator)

→ All three orchestrator handler test files:
- [ ] `TestOrchestrationResultForwardsInterfaces` — SR response includes `interfaces`;
      result includes same slice

**Expected failure:** `OrchestrationResult` has no `CloudIdentifier`, `ExclusiveUntil`,
or `Interfaces` fields → compile error. Fix: add the three fields (zero-value acceptable
for now) so tests compile and then fail at runtime with wrong values.

### Step 25 tests

→ `core/internal/orchestration/dynamic/api/handler_test.go` and
  `core/internal/orchestration/simplestore/api/handler_test.go`:
- [ ] `TestAllowInterclouReturns501` — request with `"ALLOW_INTERCLOUD": true` → 501
- [ ] `TestOnlyInterclouReturns501` — request with `"ONLY_INTERCLOUD": true` → 501
- [ ] `TestLocalOrchestrationUnaffectedByInterclouChange` — no intercloud flag → 200
      (regression guard; should already pass; confirm it still does after implementation)

**Expected failure:** both tests return 200 (flags currently silently ignored).

### Step 26 tests → `core/internal/authentication/api/handler_test.go`

- [ ] `TestLoginMissingPasswordFieldReturns400` — `{"credentials":{"token":"x"}}` → 400
- [ ] `TestLoginNonObjectCredentialsReturns400` — `{"credentials":"plainstring"}` → 400
- [ ] `TestLoginNullCredentialsReturns400` — `{"credentials":null}` → 400
- [ ] `TestLoginValidCredentialsObjectSucceeds` — `{"credentials":{"password":"..."}}` → 200
      (regression guard; must remain passing after implementation)

**Expected failure:** first three tests return 401 instead of 400.

### Completion criteria for Step B

- [ ] All tests above compile (struct fields added where needed; constructor signatures
      updated with no-op defaults)
- [ ] All new tests fail at runtime for the correct reason (wrong status code, not panic)
- [ ] `TestLocalOrchestrationUnaffectedByInterclouChange` and
      `TestLoginValidCredentialsObjectSucceeds` pass (they are regression guards)
- [ ] `go test -race ./...` from `core/` reports only the new intentional failures

---

## Step C — Write `core/PATTERNS.md`

**Goal:** A short reference document describing the canonical shape of one core system.
Every future step copies this shape rather than deriving the pattern independently.

**Status:** - [ ] Complete

### What to write

Create `core/PATTERNS.md` containing:

1. **Package structure** — the four layers every system must have and what each may import:

   ```
   internal/<system>/model/      — types only; imports: stdlib + no other internal package
   internal/<system>/repository/ — storage; imports: model/
   internal/<system>/service/    — business logic; imports: model/, repository/
   internal/<system>/api/        — HTTP handlers; imports: service/, model/
   cmd/<system>/main.go          — wiring; imports: api/, service/, repository/, model/
   ```

2. **External HTTP dependency pattern** — whenever a system calls another system over HTTP:
   - Define a Go interface in `internal/<system>/client/<dep>.go` describing what the system
     needs (not what the remote system provides)
   - Provide a concrete HTTP implementation in `internal/<system>/client/<dep>_http.go`
   - The `service/` package accepts the interface, never the concrete type
   - `cmd/main.go` wires the concrete implementation
   - Existing example: `BlacklistClient` interface (to be added in Step E2/G42)
   - Counter-example (to be refactored): the SR and ConsumerAuth HTTP calls embedded
     directly in `internal/orchestration/dynamic/service/orchestrator.go`

3. **Handler conventions** — every handler follows: method check → decode body → validate →
   delegate to service → map error to status code → write JSON. No business logic in handlers.

4. **Test conventions** — unit tests use `httptest.NewRecorder()` + `httptest.NewServer()`
   for fake dependencies. Integration tests in `core/internal/integration/` wire all
   systems in-process. SQLite paths must be covered by at least one integration test per
   system (use `DB_PATH=:memory:`).

5. **Canonical reference implementation** — nominate the system whose current code most
   closely matches this shape. Read that system's packages before implementing changes to
   any other system.

### Completion criteria

- [ ] `core/PATTERNS.md` created and covers all five sections above
- [ ] Canonical reference system named and justified
- [ ] Reviewed against `core/CLAUDE.md` for consistency (no contradictions)

---

## Step H — Extract shared handler helpers to `core/internal/httputil/`

**Prerequisite:** Step C complete (`core/PATTERNS.md` written — it will reference this package).

**Status:** - [ ] Complete

**Goal:** Replace six verbatim copies of `writeJSON`, `writeError`, and `errTypeForStatus`
with a single shared package. Eliminates the most common source of handler pattern drift:
when an AI session reads one handler file and copies its local helpers, it may propagate
a subtly different version of the same function.

### What to create

**`core/internal/httputil/respond.go`**

Exported functions (all handlers in core/ will use these):

| Function | Signature | Purpose |
|---|---|---|
| `WriteJSON` | `(w http.ResponseWriter, status int, v any, origin string)` | Set `Content-Type`, call `WriteHeader`, encode JSON |
| `WriteError` | `(w http.ResponseWriter, status int, msg, origin string)` | Write AH5 error envelope using `ErrorTypeForStatus` |
| `ErrorTypeForStatus` | `(status int) string` | Map HTTP status → AH5 `exceptionType` string |
| `RequireMethod` | `(w http.ResponseWriter, r *http.Request, method string) bool` | Return false + 405 if method doesn't match |
| `DecodeJSON` | `(w http.ResponseWriter, r *http.Request, v any) bool` | Return false + 400 on decode failure |
| `ExtractBearer` | `(r *http.Request) string` | Return token string or `""` |

The AH5 error envelope shape (same across all systems):
```json
{"errorMessage":"...","errorCode":400,"exceptionType":"INVALID_PARAMETER","origin":"..."}
```

`ErrorTypeForStatus` maps:
- 400 → `"INVALID_PARAMETER"`
- 401 → `"AUTH_EXCEPTION"`
- 403 → `"FORBIDDEN"`
- 404 → `"DATA_NOT_FOUND"`
- 423 → `"LOCKED"`
- 501 → `"NOT_IMPLEMENTED"`
- all others → `"ARROWHEAD_EXCEPTION"`

### What to change

For each of the six handler packages — `internal/api`, `internal/authentication/api`,
`internal/consumerauth/api`, `internal/orchestration/dynamic/api`,
`internal/orchestration/simplestore/api`, `internal/orchestration/flexiblestore/api`,
`internal/blacklist/api` — do all of the following:

1. Delete the local `writeJSON`, `writeError`, `errTypeForStatus` (and any alias like
   `writeErrorResponse`) functions
2. Replace all call sites with the corresponding `httputil.WriteJSON`, `httputil.WriteError`,
   `httputil.RequireMethod`, `httputil.DecodeJSON`, `httputil.ExtractBearer` calls
3. Remove any now-unused imports

### Arch test update

Add one rule to `core/internal/arch/arch_test.go`:
> `api/` packages MAY import `arrowhead/core/internal/httputil` (allowlisted exception to
> the existing "api/ does not import repository/" rule; httputil is not a repository).

Alternatively: the existing rules do not forbid `api/ → httputil` (httputil is not
`repository/` or `service/`), so no rule change may be needed — verify.

### Completion criteria

- [ ] `core/internal/httputil/respond.go` created with all six exported functions
- [ ] `core/internal/httputil/respond_test.go` created — unit tests for `ErrorTypeForStatus`
      (all mapped status codes) and `WriteError` (verify JSON shape and status code)
- [ ] All six handler packages compile with no local `writeJSON`/`writeError` definitions
- [ ] `go test -race ./...` from `core/` passes
- [ ] `core/internal/arch/arch_test.go` still passes (verify `httputil` import is not
      blocked by any arch rule)
- [ ] `core/PATTERNS.md` updated to reference `httputil` in the handler conventions section

---

## Step I — Consistent sentinel error mapping in all handlers

**Prerequisite:** Step H complete (all handlers use `httputil.WriteError`).

**Status:** - [ ] Complete

**Goal:** Every handler error branch uses `errors.Is()` against the package's sentinel
errors to determine the correct HTTP status code. No error is mapped via string comparison
or hardcoded fallback to 400. This makes the pattern unambiguous for future AI-assisted
steps that add new sentinel errors.

### Current state

Most handlers check errors like this:
```go
result, err := svc.DoSomething(req)
if err != nil {
    httputil.WriteError(w, http.StatusBadRequest, err.Error(), origin)
    return
}
```

This maps every service error to 400 regardless of type. The correct pattern is:
```go
result, err := svc.DoSomething(req)
if err != nil {
    httputil.WriteError(w, statusFor(err), err.Error(), origin)
    return
}
```

Where `statusFor` is a handler-local function (or inline switch) using `errors.Is`:
```go
func statusFor(err error) int {
    switch {
    case errors.Is(err, model.ErrNotFound):   return http.StatusNotFound
    case errors.Is(err, model.ErrLocked):     return http.StatusLocked
    case errors.Is(err, model.ErrForbidden):  return http.StatusForbidden
    default:                                   return http.StatusBadRequest
    }
}
```

### What to change

For each handler package:

1. Identify all sentinel errors defined in that system's `model/` or `service/` packages
2. Identify every `if err != nil` branch in the handler and determine the correct status
   code for each sentinel error
3. Replace hardcoded status codes with `errors.Is()` switches
4. Verify no handler maps the same sentinel error to different status codes in different
   places

### Scope

Only fix handlers that currently use string comparison or blind 400 fallback. Do not
change handlers that already use `errors.Is()` correctly (ServiceRegistry and CA are
closest to correct — read those first as a reference).

### Completion criteria

- [ ] Every `if err != nil` branch in every handler resolves status code via `errors.Is()`
      against at least the sentinel errors that system defines
- [ ] No handler maps an error via `err.Error()` string comparison
- [ ] Adding a new sentinel error to a service package requires only adding one `case` in
      the handler's `statusFor` switch — verified by inspection
- [ ] `go test -race ./...` from `core/` passes
- [ ] No test status code expectations changed (existing tests already expect the correct
      codes; this step makes the implementation match what tests already assert)

---

## Step D — Shrink `CONFORMANCE_UPDATE_PLAN.md`

**Prerequisite:** Step B complete (failing tests now live in actual test files).

**Goal:** Remove TDD recipe sections from `CONFORMANCE_UPDATE_PLAN.md`; retain only
constraint, reason, affected files, coverage gate, and completion criteria per step.
The plan becomes a governance document, not a recipe book.

**Status:** - [ ] Complete

### What to remove from Steps 22–26

For each step, delete:
- The `### TDD cycle N.M` subsections containing test function bodies and implementation
  snippets (these now live in the actual test files from Step B)
- The `**Expected failure:**` paragraphs (already served their purpose)
- The `**Implementation notes:**` code blocks (belong in the implementation, not the plan)

### What to keep

For each step, retain:
- The gap reference (G-ID), one-sentence description, and "Why now" rationale
- The file list (files to modify in core/ and core-evol/)
- Any new environment variable table
- The `### System test` description (prose, no code)
- The `### Coverage check` bash commands
- The `### Documentation updates` checklist
- The `### Completion criteria` checklist

### Completion criteria

- [ ] Steps 22–26 in `CONFORMANCE_UPDATE_PLAN.md` retain constraint + file list +
      coverage gate + completion criteria but contain no test function bodies or
      implementation code snippets
- [ ] All information removed was already captured in Step B's test files
- [ ] `CONFORMANCE_UPDATE_PLAN.md` line count reduced substantially from 5857

---

## Step E — Execute Phase 1 (Steps 22–26)

**Prerequisite:** Steps A, B, C, H, I complete. Pre-flight green.

**Protocol for each sub-step:**
1. Read the step in `CONFORMANCE_UPDATE_PLAN.md` (constraints, files, coverage gate)
2. Read `core/PATTERNS.md` for the canonical shape
3. Implement until all pre-written failing tests pass
4. Run coverage check; reach ≥ 80% on touched packages
5. Apply documentation updates listed in the step
6. Run full pre-flight; tick completion criteria in `CONFORMANCE_UPDATE_PLAN.md`
7. Tick the checkbox below

---

### Step E1 — G11: System revoke derives identity from verified token

**Status:** - [ ] Complete

**Core change:** `core/internal/api/ah5_handler.go` `handleSystemRevoke` — extract
`Authorization: Bearer <token>`, call `SR_AUTH_URL/authentication/identity/verify/<token>`,
use `systemName` from response. Fall back to `?name=` query param only when no header
present (deprecated path). Return 401 on missing header, failed verification, or
unreachable auth server (fail-closed).

**New env var:** `SR_AUTH_URL` (default `http://localhost:8081`) — add to
`core/cmd/serviceregistry/main.go` and pass to `NewAH5Handler`.

**core-evol:** Not applicable.

**Docs to update after:** `core/GAP_ANALYSIS.md` (mark G11 resolved), `core/SPEC.md`,
`CONFORMANCE.md`, `README.md`.

---

### Step E2 — G41: Blacklist Bearer enforcement and mode enum

**Status:** - [ ] Complete

**Core changes:**
- `core/internal/blacklist/api/handler.go` — add Bearer middleware to `handleLookup` and
  `handleCheck`; call `BLACKLIST_AUTH_URL/authentication/identity/verify/<token>` when
  env var is set; skip check when unset (dev mode)
- `core/internal/blacklist/service/blacklist.go` — accept `Mode string` in query; map
  `ALL`→nil, `ACTIVES`→true, `INACTIVES`→false; return error for unknown values
- Blacklist query request model — replace `Active *bool` with `Mode string`

**New env var:** `BLACKLIST_AUTH_URL` (unset = no auth check) — add to
`core/cmd/blacklist/main.go` and pass to `NewBlacklistHandler`.

**core-evol:** Not applicable.

**Docs to update after:** `core/GAP_ANALYSIS.md` (mark G41 resolved), `core/SPEC.md`,
`CONFORMANCE.md`, `README.md`.

---

### Step E3 — G40: OrchestrationResult missing spec-defined fields

**Status:** - [ ] Complete

**Core changes:**
- `core/internal/orchestration/model/types.go` — add to `OrchestrationResult`:
  `CloudIdentifier string`, `ExclusiveUntil string`, `Interfaces []string`
  (json tags: `cloudIdentifier,omitempty`, `exclusiveUntil,omitempty`, `interfaces,omitempty`)
- All three orchestrator `service/orchestrator.go` files — set `CloudIdentifier = "LOCAL"`;
  forward `Interfaces` from the SR response; for DynamicOrch only: check LockStore for an
  active lock on the provider and set `ExclusiveUntil` to `lock.ExpiresAt.Format(time.RFC3339)`
- `core-evol/internal/orchestration/types.go` — add same three fields
- `core-evol/internal/orchestration/service.go` — same population logic

**Docs to update after:** `core/GAP_ANALYSIS.md` (mark G40 result-fields portion resolved),
`core/SPEC.md`, `CONFORMANCE.md`, `core/EXAMPLES.md`.

---

### Step E4 — G25: Intercloud flags return 501

**Status:** - [ ] Complete

**Core changes:**
- `core/internal/orchestration/model/` — add `ErrInterclouNotSupported` sentinel error
- `core/internal/orchestration/dynamic/service/orchestrator.go` and
  `core/internal/orchestration/simplestore/service/orchestrator.go` — after parsing
  `orchestrationFlags`, if `ALLOW_INTERCLOUD` or `ONLY_INTERCLOUD` is true, return
  `nil, ErrInterclouNotSupported`
- Both handler files — map `ErrInterclouNotSupported` → `http.StatusNotImplemented`
- `core-evol/internal/orchestration/service.go` and `handler.go` — same pattern

**Docs to update after:** `core/GAP_ANALYSIS.md` (update G25 — intercloud portion resolved),
`core/SPEC.md`, `CONFORMANCE.md`, `core/EXAMPLES.md`.

---

### Step E5 — G43: Credentials validated as structured object

**Status:** - [ ] Complete

**Core changes:**
- `core/internal/authentication/model/types.go` — define `Credentials struct { Password string }`
  with `UnmarshalJSON` that rejects non-object JSON (anything whose first non-whitespace byte
  is not `{`); add `ErrMissingPassword` sentinel
- In `LoginRequest`, change `Credentials` field type from `string` (or `interface{}`) to
  `Credentials`
- `core/internal/authentication/service/auth.go` — check `req.Credentials.Password == ""`
  before bcrypt; return `ErrMissingPassword`
- `core/internal/authentication/api/handler.go` — map `ErrMissingPassword` → 400

**core-evol:** Not applicable.

**Experiment impact — BREAKING:** Experiments 4, 5, 7, and 13 all send credentials as a
plain string (`"credentials":"secret"`). After this step they will receive 400. Update
the `authLoginRequest` struct in each affected file as part of this step:

```
experiments/experiment-4/services/consumer-direct/main.go
experiments/experiment-4/services/robot-fleet/main.go
experiments/experiment-5/services/consumer-direct/main.go
experiments/experiment-5/services/robot-fleet/main.go
experiments/experiment-7/services/consumer-direct-tls/main.go
experiments/experiment-7/services/robot-fleet-tls/main.go
experiments/experiment-13/services/robot-fleet-tls/main.go
```

In each file, change the `Credentials` field from `string` to a struct:
```go
type authLoginRequest struct {
    SystemName  string            `json:"systemName"`
    Credentials authCredentials   `json:"credentials"`
}
type authCredentials struct {
    Password string `json:"password"`
}
```
And update the call site where the struct is populated.

**Docs to update after:** `core/GAP_ANALYSIS.md` (mark G43 resolved), `core/SPEC.md`,
`CONFORMANCE.md`, `core/EXAMPLES.md`.

---

## Step F — Add EXPERIENCES.md entry for core/core-evol sync risk

**Status:** - [ ] Complete

**Goal:** Document the synchronization failure mode so future sessions have a named,
searchable record of the risk and its mitigation.

**What to add** as a new entry in `EXPERIENCES.md`:

- **Symptom:** A conformance step is applied to `core/` and passes all tests there, but
  the equivalent change is not applied to `core-evol/`. Behaviour diverges at runtime in
  experiments that use `dynamicorch-xacml`.
- **Root cause:** `core/` and `core-evol/` are separate Go modules with structurally
  similar orchestration code. There is no shared package and no CI check that requires
  both to be updated together.
- **Pre-flight rule:** Any step that modifies orchestration logic, handler behaviour,
  request/response model, or general management in `core/` must be followed immediately
  by the equivalent change in `core-evol/`. Mark a step incomplete until both pass
  `go test -race ./...`.
- **Detection:** `CONFORMANCE_UPDATE_PLAN.md` Step N's completion criteria always include
  a `cd core-evol && go test -race ./...` gate for orchestration-touching steps.

---

## Step G — Refactor existing external HTTP clients to interfaces (prerequisite for G42)

**Prerequisite:** Steps A–F, H, I complete. This step must be done before implementing G42
(Blacklist enforcement in core systems), which is Phase 2 work.

**Status:** - [ ] Complete

**Goal:** Apply the pattern described in `core/PATTERNS.md` (external HTTP dependency
pattern) retroactively to the two existing concrete HTTP clients in DynamicOrchestration,
so that when G42 adds a third external dependency (BlacklistClient), all three are
consistent.

### What to create

**`core/internal/orchestration/dynamic/client/sr.go`**
```
ServiceRegistryClient interface {
    LookupServices(ctx, req) ([]OrchestrationResult, error)
}
```
Concrete implementation in `client/sr_http.go` using the existing SR HTTP call logic
extracted from `orchestrator.go`.

**`core/internal/orchestration/dynamic/client/consumerauth.go`**
```
ConsumerAuthClient interface {
    IsAuthorized(ctx, consumer, provider, service string) (bool, error)
}
```
Concrete implementation in `client/consumerauth_http.go` using the existing ConsumerAuth
HTTP call logic extracted from `orchestrator.go`.

### What to change

- `core/internal/orchestration/dynamic/service/orchestrator.go` — replace embedded HTTP
  call logic with calls to the two interfaces; accept them via constructor
- `core/cmd/dynamicorch/main.go` — wire concrete `_http.go` implementations
- All existing orchestrator tests — replace fake HTTP servers with mock implementations
  of the two interfaces (simpler and faster than httptest servers)

### Completion criteria

- [ ] `ServiceRegistryClient` and `ConsumerAuthClient` interfaces defined in `client/`
- [ ] Concrete HTTP implementations satisfy the interfaces
- [ ] `DynamicOrchestrator` accepts both via constructor; no `net/http` import in
      `service/orchestrator.go` (all HTTP is in `client/`)
- [ ] All existing orchestrator tests pass with mock implementations
- [ ] `core/internal/arch/arch_test.go` still passes (no new rule violations)
- [ ] `go test -race ./...` from `core/` passes

---

## Final verification

When all steps above are ticked, run:

```bash
# Full build and test
cd core && go build ./... && go vet ./... && go test -race ./...
cd core-evol && go build ./... && go test -race ./...
go build ./...   # workspace root

# Architecture constraint test
cd core && go test ./internal/arch/...

# Coverage on all packages touched in Phase 1
cd core && go test -coverprofile=cover.out ./... && go tool cover -func=cover.out
```

Then confirm:
- [ ] `core/internal/arch/arch_test.go` passes with zero rule violations
- [ ] `core/internal/httputil/respond.go` exists; no handler package defines its own `writeJSON`/`writeError`
- [ ] All handler `err != nil` branches resolve status via `errors.Is()` (Step I)
- [ ] All Phase 1 gaps (G11, G41, G40, G25, G43) marked resolved in `core/GAP_ANALYSIS.md`
- [ ] `CONFORMANCE.md` ratings updated for affected systems
- [ ] `CONFORMANCE_UPDATE_PLAN.md` Steps 22–26 completion criteria all ticked
- [ ] `core/PATTERNS.md` exists and is consistent with the implemented code
- [ ] `EXPERIENCES.md` has the core/core-evol sync entry
- [ ] `README.md` has `SR_AUTH_URL` and `BLACKLIST_AUTH_URL` in the configuration table

---

*Created: 2026-05-29*
