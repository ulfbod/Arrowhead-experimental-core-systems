# Test Plan

## Goal

Verify full compliance with SPEC.md across all six Arrowhead 5 core systems.

---

## Test Structure

```
core/
├── internal/
│   ├── service/                        # ServiceRegistry service tests
│   ├── api/                            # ServiceRegistry handler tests
│   ├── authentication/
│   │   ├── service/auth_test.go        # Auth service unit tests
│   │   └── api/handler_test.go         # Auth handler tests
│   ├── consumerauth/
│   │   ├── service/auth_test.go        # ConsumerAuth service unit tests
│   │   └── api/handler_test.go         # ConsumerAuth handler tests
│   ├── orchestration/
│   │   ├── dynamic/
│   │   │   ├── service/orchestrator_test.go  # Dynamic orch service tests
│   │   │   └── api/handler_test.go           # Dynamic orch handler tests
│   │   ├── simplestore/
│   │   │   ├── service/orchestrator_test.go  # SimpleStore service tests
│   │   │   └── api/handler_test.go           # SimpleStore handler tests
│   │   └── flexiblestore/
│   │       ├── service/orchestrator_test.go  # FlexibleStore service tests
│   │       └── api/handler_test.go           # FlexibleStore handler tests
│   └── integration/
│       └── e2e_test.go                 # Cross-system end-to-end tests
```

All tests use `net/http/httptest` — no external dependencies, no real network, no database.

---

## How to Run

```bash
# All tests
go test ./...

# Single system
go test ./internal/authentication/...

# With verbose output
go test -v ./internal/integration/...

# With race detector
go test -race ./...
```

---

## Systems and Coverage

### ServiceRegistry (port 8080)

**Service layer** (`internal/service/`):
- Register: valid, missing fields, version defaulting, duplicate overwrite
- Query: exact match, interface subset match (case-insensitive), metadata subset match, version filter, no match, combined filters

**Handler layer** (`internal/api/`):
- Register: 201, 400 for each missing field, 400 for invalid JSON
- Query: 200 with results, 200 with empty list
- Health: `/health`

---

### Authentication (port 8081)

**Service layer** (`internal/authentication/service/`):
- Login: valid credentials, empty username/password, whitespace-only, no credentials configured, token uniqueness across calls
- Verify: valid token (returns systemName), unknown token, expired token (lazy-deleted on verify), verify after logout
- Logout: valid token, unknown token, already-logged-out token

Key technique: `newAuthService(-time.Second)` creates tokens that are immediately expired without sleeping.

**Handler layer** (`internal/authentication/api/`):
- POST `/authentication/login` → 201 / 400 (missing fields) / 400 (invalid JSON) / 405
- POST `/authentication/verify` → 200 (valid/invalid/expired) / 405
- POST `/authentication/logout` → 200 / 401 (missing/unknown token) / 405
- GET `/health`, `/authentication/health` → 200

---

### ConsumerAuthorization (port 8082)

**Service layer** (`internal/consumerauth/service/`):
- Grant: valid, validation (empty consumer/provider/service), duplicate rejected, same consumer different service allowed
- Revoke: valid, not found
- Lookup: no filter returns all, filter by consumer/provider/service, nil returns empty slice not nil
- Verify: authorized pair, unauthorized pair
- GenerateToken: authorized → token string, unauthorized → error

**Handler layer** (`internal/consumerauth/api/`):
- POST `/authorization/grant` → 201 / 400 (validation) / 409 (duplicate) / 400 (invalid JSON) / 405
- DELETE `/authorization/revoke/{id}` → 200 / 404 / 400 (non-integer ID) / 405
- GET `/authorization/rules` → 200 (empty) / 200 (with results, filter by consumer)
- POST `/authorization/verify` → 200 authorized / 200 unauthorized
- POST `/authorization/token` → 201 / 403
- GET `/health`, `/authorization/health` → 200

---

### DynamicOrchestration (port 8083)

**Service layer** (`internal/orchestration/dynamic/service/`):
- No-auth mode: all SR results returned as-is
- Empty SR: returns empty response
- Auth mode, all allowed: all results returned
- Auth mode, all denied: empty response
- Auth mode, partial: CA inspects providerSystemName in request body and allows selectively
- CA unreachable (fail-closed): provider excluded, no error returned
- SR unreachable: error returned
- Validation: missing requesterSystem.systemName, missing serviceDefinition
- Identity check disabled: no token needed, works normally
- Identity check enabled, empty token: `ErrIdentityRequired`
- Identity check enabled, invalid/expired token: `ErrIdentityInvalid`
- Identity check enabled, auth system unreachable: fail-closed error
- Identity check enabled, verified name overrides self-reported name for CA check

All tests use `httptest.NewServer` as fake SR, CA, and Authentication system — no real network calls.

**Handler layer** (`internal/orchestration/dynamic/api/`):
- POST `/orchestration/dynamic` → 200 (match/no-match/auth-denied) / 400 (invalid JSON) / 401 (no token when identity check on) / 401 (invalid token) / 405
- Identity check: valid token with correct systemName → 200; self-reported name overridden by verified token name
- GET `/health`, `/orchestration/dynamic/health` → 200

---

### SimpleStoreOrchestration (port 8084)

**Service layer** (`internal/orchestration/simplestore/service/`):
- CreateRule: valid, validation table (empty consumer/service/provider/serviceUri/interfaces)
- DeleteRule: valid, not found
- ListRules: empty returns `{count:0, rules:[]}`, all rules returned
- Orchestrate: match, no match (wrong service/wrong consumer), multiple rules, missing requester, missing service

**Handler layer** (`internal/orchestration/simplestore/api/`):
- POST `/orchestration/simplestore/rules` → 201 / 400 (validation table) / 400 (invalid JSON)
- GET `/orchestration/simplestore/rules` → 200
- DELETE `/orchestration/simplestore/rules/{id}` → 200 / 404 / 400 (invalid ID)
- POST `/orchestration/simplestore` → 200 (match/no-match) / 400 (invalid JSON) / 405
- GET `/health`, `/orchestration/simplestore/health` → 200

---

### FlexibleStoreOrchestration (port 8085)

**Service layer** (`internal/orchestration/flexiblestore/service/`):
- CreateRule: valid, validation table (same fields as SimpleStore)
- Priority ordering: rules inserted in reverse order, results returned lowest-number-first
- Priority zero: treated as MaxInt32 (lowest priority), appears last
- Priority in result: `priority` field propagated to orchestration result
- MetadataFilter match: rule filter ⊆ request metadata → included
- MetadataFilter no match: mismatched value → excluded
- Empty filter: rule with no metadataFilter matches any request
- Missing key: request has no metadata, rule requires key → excluded
- No match, missing requester, DeleteRule valid/not-found

**Handler layer** (`internal/orchestration/flexiblestore/api/`):
- POST `/orchestration/flexiblestore/rules` → 201 (with/without metadataFilter) / validation
- GET `/orchestration/flexiblestore/rules` → 200 (empty returns non-nil slice)
- DELETE `/orchestration/flexiblestore/rules/{id}` → 200 / 404
- POST `/orchestration/flexiblestore` → 200 (match/priority-ordering/no-match)
- GET `/health`, `/orchestration/flexiblestore/health` → 200

---

## End-to-End Integration Tests (`internal/integration/`)

All six systems wired in-process using `httptest.NewServer`. No mocking of business logic.

| Test | Flow |
|------|------|
| `TestE2EDynamicOrchNoAuth` | Register service → dynamic orchestrate (no auth) → result matches |
| `TestE2EDynamicOrchWithAuth` | Register → grant auth rule → orchestrate → authorized result |
| `TestE2EDynamicOrchNoGrant` | Register → orchestrate with auth (no grant) → empty result |
| `TestE2ESimpleStoreLifecycle` | Create rule → orchestrate → delete rule → orchestrate again (empty) |
| `TestE2EFlexibleStorePriority` | Create two rules with different priorities → results in priority order |
| `TestE2EFlexibleStoreMetadataFiltering` | Create rule with metadata filter → matching/non-matching requests |
| `TestE2EConsumerAuthLifecycle` | Grant → verify authorized → revoke → verify denied |
| `TestE2EDuplicateGrantRejected` | Grant same triple twice → second returns 409 |
| `TestE2EFullFlow` | Register → grant → dynamic orchestrate (authenticated) → verify result |
| `TestE2EUnregisterClearsOrchestration` | Register → orchestrate (match) → unregister → orchestrate (empty) |
| `TestE2EIdentityCheckBlocksWithoutToken` | `ENABLE_IDENTITY_CHECK=true` + no Authorization header → 401 |
| `TestE2EIdentityCheckAllowsWithValidToken` | Login → token → orchestrate with token + CA grant → result returned |
| `TestE2EIdentityCheckPreventsImpersonation` | Token for "consumer-app", body claims "impersonator" → verified name used → authorized |

---

## Test Principles

- **Deterministic**: no sleeps, no random ports, no wall-clock dependencies
- **In-process**: all HTTP calls use `httptest.NewServer` or `httptest.NewRecorder`
- **External test packages**: all test files use `package X_test` for black-box testing
- **Table-driven**: validation tests use `[]struct{ name; mutate/input }` pattern
- **Fail-closed**: security tests verify that unavailable auth services → no access
- **No mocking of business logic**: only external HTTP dependencies are faked
