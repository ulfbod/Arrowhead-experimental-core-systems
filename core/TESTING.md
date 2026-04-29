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

DynamicOrchestration makes real HTTP calls to ServiceRegistry and ConsumerAuthorization. Tests spin up `httptest.NewServer` instances that return controlled JSON:

```go
sr := httptest.NewServer(http.HandlerFunc(srResponse("sensor-1", "sensor-2")))
defer sr.Close()
ca := httptest.NewServer(http.HandlerFunc(caAlwaysAuthorized()))
defer ca.Close()

orch := service.NewDynamicOrchestrator(sr.URL, ca.URL, true)
resp, err := orch.Orchestrate(req)
```

### Fail-closed security

When ConsumerAuthorization is unreachable, DynamicOrchestration excludes the provider rather than allowing access:

```go
// CA pointed at a closed port
orch := service.NewDynamicOrchestrator(sr.URL, "http://127.0.0.1:1", true)
resp, _ := orch.Orchestrate(req)
// len(resp.Response) == 0  — fail closed
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
- **No authentication integration with orchestration**: The Authentication system issues tokens for system login, but DynamicOrchestration's auth check is against ConsumerAuthorization (grant rules), not Authentication tokens. This separation is per SPEC.md; a full cross-system token-gated flow is not tested.
