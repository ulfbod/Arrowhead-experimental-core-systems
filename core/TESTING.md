# Testing

## Quick Start

```bash
cd core/
go test ./...
```

All tests are self-contained. No database, no running servers, no environment variables required.

---

## Test Layout

```
internal/
├── service/                          ServiceRegistry service
├── api/                              ServiceRegistry handler
├── authentication/service/           Authentication service
├── authentication/api/               Authentication handler
├── consumerauth/service/             ConsumerAuthorization service
├── consumerauth/api/                 ConsumerAuthorization handler
├── orchestration/dynamic/service/    DynamicOrchestration service
├── orchestration/dynamic/api/        DynamicOrchestration handler
├── orchestration/simplestore/service/
├── orchestration/simplestore/api/
├── orchestration/flexiblestore/service/
├── orchestration/flexiblestore/api/
└── integration/                      Cross-system end-to-end tests
```

Each system has two test layers:

- **Service tests** — business logic in isolation, using an in-memory repository
- **Handler tests** — HTTP behavior: status codes, request parsing, response encoding

The `integration/` package wires all six real handlers together in a single test binary.

---

## Running Specific Tests

```bash
# One system
go test ./internal/authentication/...

# One layer
go test ./internal/orchestration/flexiblestore/service/

# One test by name
go test -run TestOrchestratePriorityOrdering ./internal/orchestration/flexiblestore/service/

# Verbose output
go test -v ./internal/integration/

# Race detector
go test -race ./...
```

---

## Key Test Scenarios

### Expired token without sleeping

The Authentication service accepts a `tokenDuration` parameter. Tests pass `-time.Second` to make tokens immediately expired:

```go
svc := service.NewAuthService(repo, -time.Second)
// svc.Login(...) → token with ExpiresAt already in the past
// svc.Verify(token) → "token expired", lazy-deletes the entry
```

### Fake upstream services for DynamicOrchestration

DynamicOrchestration makes real HTTP calls to ServiceRegistry, ConsumerAuthorization, and (when identity check is on) the Authentication system. Tests spin up `httptest.NewServer` instances that return controlled JSON:

```go
sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1", "sensor-2")))
defer sr.Close()
ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
defer ca.Close()
authSys := httptest.NewServer(http.HandlerFunc(fakeAuthSys(true, "consumer-app")))
defer authSys.Close()

orch := service.NewDynamicOrchestrator(sr.URL, ca.URL, authSys.URL, true, true)
resp, err := orch.Orchestrate(req, "some-token")
```

### Identity verification and impersonation prevention

When `ENABLE_IDENTITY_CHECK=true`, the orchestrator calls the Authentication system to verify the Bearer token and uses the returned `systemName` — ignoring the self-reported `requesterSystem.systemName`. Tests verify this by having the CA only authorize "consumer-app" while the request body claims "impersonator":

```go
// Token verifies as "consumer-app"; body says "impersonator"
// CA only grants "consumer-app" → result is returned (verified name wins)
orch := service.NewDynamicOrchestrator(sr.URL, ca.URL, authSys.URL, true, true)
resp, err := orch.Orchestrate(reqWithImpersonatorName, "consumer-app-token")
// len(resp.Response) == 1
```

### Fail-closed security

When ConsumerAuthorization is unreachable, DynamicOrchestration excludes the provider rather than allowing access. The same applies when the Authentication system is unreachable with `ENABLE_IDENTITY_CHECK=true`:

```go
// CA pointed at a closed port
orch := service.NewDynamicOrchestrator(sr.URL, "http://127.0.0.1:1", "", true, false)
resp, _ := orch.Orchestrate(req, "")
// len(resp.Response) == 0  — fail closed

// Auth system pointed at a closed port
orch2 := service.NewDynamicOrchestrator(sr.URL, "", "http://127.0.0.1:1", false, true)
_, err := orch2.Orchestrate(req, "some-token")
// err != nil  — fail closed
```

### Duplicate grant rejection (409)

```go
postJSON(t, h, "/authorization/grant", body)          // 201
w := postJSON(t, h, "/authorization/grant", body)     // 409
```

### FlexibleStore priority ordering

Priority `0` is treated as `MaxInt32` (lowest). Rules inserted in any order are returned sorted lowest-number-first:

```go
// Insert: priority 3, then 1, then 2
// Orchestrate result order: 1 → 2 → 3
```

### MetadataFilter subset semantics

A rule's `metadataFilter` must be a subset of the request's `requestedService.metadata`:

| Rule filter | Request metadata | Match? |
|---|---|---|
| `{"region": "eu"}` | `{"region": "eu", "unit": "celsius"}` | yes |
| `{"region": "eu"}` | `{"region": "us"}` | no |
| `{}` (empty) | anything | yes |
| `{"region": "eu"}` | `{}` (no metadata) | no |

---

## End-to-End Tests

`internal/integration/e2e_test.go` wires all six systems in-process:

```
TestE2EFullFlow
  Register service in SR
  Grant authorization in CA
  DynamicOrch queries SR + checks CA → returns authorized provider

TestE2EUnregisterClearsOrchestration
  Register → orchestrate (1 result)
  Unregister → orchestrate (0 results)

TestE2EFlexibleStoreMetadataFiltering
  Create rule with metadataFilter {"region": "eu"}
  Request with {"region": "eu"} → 1 result
  Request with {"region": "us"} → 0 results

TestE2EDuplicateGrantRejected
  Grant rule → 201
  Grant same rule again → 409
```

No process boundaries are crossed. All HTTP calls go through `httptest.NewServer`; there are no real ports allocated.

---

## Known Limitations

- **No TLS testing**: All tests use plain HTTP. TLS/mTLS is out of scope for the current implementation.
- **No persistence testing**: All repositories are in-memory. Restart behavior is not tested.
- **No concurrent load testing**: Tests verify correctness, not throughput or race conditions under load (though `go test -race` catches data races).
- **Credential verification is a stub**: `ENABLE_IDENTITY_CHECK` prevents name spoofing but not token theft — any system can log in as any `systemName` since credential verification is not implemented (see GAP_ANALYSIS.md G2). The E2E identity tests cover the orchestration mechanics, not the security strength of the credential check.
