# AH5 Conformance Update Plan

**Status:** Phases 1–5 complete (Steps 1–56, E1–E5)  
**Scope:** `core/`, `core-evol/`, all active experiments  
**Source of truth for gaps:** `core/GAP_ANALYSIS.md` (gaps G1–G53)  
**Source of truth for API spec:** `core/SPEC.md` and `core/GAP_ANALYSIS.md`  
**Conformance assessment:** `CONFORMANCE.md`

---

## 1. Purpose

This plan brings the Go implementation of Arrowhead 5 core systems into closer conformance with the official AH5 specification at https://aitia-iiot.github.io/ah5-docs-java-spring/ while keeping all active experiments working. It also adds SQLite-backed persistence to replace the current in-memory-only state.

The plan is divided into nine self-contained steps. Each step can be executed independently once its prerequisites are satisfied. Each step follows a strict TDD cycle: write failing tests first, then implement, then confirm the full suite passes.

---

## 2. How to use this document

Each step is self-contained. Before starting any step, read:
1. `core/CLAUDE.md` — implementation rules and prohibitions
2. `core/GAP_ANALYSIS.md` — gap descriptions and design decisions
3. `EXPERIENCES.md` — known failure modes and the pre-flight checklist

### TDD cycle (mandatory for every behaviour change)

```
1. Write the failing test
2. Run: go test -run <TestName> ./...
3. Confirm it fails for the right reason (wrong output, not compile error)
4. Implement the minimum code to pass the test
5. Run: go test -run <TestName> ./...  — must be green
6. Run: go test -race ./...            — full regression, must be green
7. Run: bash core/test-system.sh       — system test, must be green
```

Never skip step 3. A test that fails to compile is not a failing test — fix the
compilation first, then confirm the runtime failure.

### TDD exception

Tests that rename existing test data (e.g. updating `"d1"` to `"D1"` to comply
with naming conventions) modify existing passing tests. These are not TDD cycles —
they are refactors. Apply them immediately before the TDD cycle that introduces the
validator that would reject the old names.

---

## 3. Coverage standard

**Target: ≥ 80% statement coverage on all packages modified by a step.**

Measure after a step's implementation is complete:

```bash
cd core
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

To inspect a specific package:

```bash
go test -coverprofile=coverage.out ./internal/service/...
go tool cover -func=coverage.out | grep -v "100.0%"
```

To generate an HTML report for manual inspection:

```bash
go tool cover -html=coverage.out -o coverage.html
```

If a package is below 80%, add targeted tests before marking the step complete.
Coverage is checked per-step, not globally — do not wait until Step 9 to measure.

---

## 4. Repository orientation

```
core/
  cmd/                        — one binary per system
    serviceregistry/          — port 8080
    authentication/           — port 8081
    consumerauth/             — port 8082  (path prefix: /authorization)
    dynamicorch/              — port 8083
    simplestoreorch/          — port 8084
    flexiblestoreorch/        — port 8085
    ca/                       — port 8086
  internal/
    api/                      — ServiceRegistry HTTP handlers (legacy + AH5)
      handler.go              — legacy endpoints
      ah5_handler.go          — AH5 device/system/service-discovery + mgmt
    model/
      types.go                — legacy types
      ah5_types.go            — AH5 Device, AH5System, AH5ServiceInstance, etc.
    repository/
      memory.go               — legacy in-memory store
      ah5_memory.go           — AH5 in-memory stores (AH5Store struct)
    service/
      registry.go             — legacy registry business logic
      ah5_registry.go         — AH5 registry business logic
    authentication/           — Authentication subsystem (api, model, service, repo)
    consumerauth/             — ConsumerAuthorization subsystem
    orchestration/
      dynamic/                — DynamicOrchestration
      simplestore/            — SimpleStoreOrchestration
      flexiblestore/          — FlexibleStoreOrchestration
    ca/                       — CertificateAuthority subsystem
    integration/
      e2e_test.go             — in-process E2E test wiring all systems together
  test-system.sh              — build + vet + staticcheck + all tests (no Docker)

core-evol/
  cmd/
    authz-pdp/                — gRPC PDP adapter (port 9550)
    dynamicorch-xacml/        — XACML-backed DynamicOrch (port 8083)
  internal/
    orchestration/            — orchestration logic + CADecider (ConsumerAuth client)
  proto/                      — authorize.proto, certlifecycle.proto

experiments/
  experiment-{9,13,14}/
    docker-compose.yml        — stack definition + seed container
    test-system.sh            — Docker-based experiment system test
    services/                 — experiment-specific Go services
```

### Key invariant (from CLAUDE.md)

No code outside `core/` may import packages from `core/internal/`. All
cross-system communication happens via HTTP only.

---

## 5. Pre-flight check (run before every step)

```bash
cd core
go build ./...   # must produce no errors
go vet ./...     # must produce no warnings
go test -race ./...  # all tests must pass
```

If any of these fail before you start, investigate and fix before proceeding.

---

## Step 1 — Token security

**Gaps addressed:**
- **G3** — Authentication and ConsumerAuthorization tokens use `hex(time.Now().UnixNano())`, which is predictable and forgeable. Replace with cryptographically random UUID v4.

**Why first:** Completely independent of all other changes. Zero risk. Highest security value per line changed.

**Prerequisites:** Pre-flight check passes.

**Files to modify:**
- `core/internal/authentication/service/auth.go`
- `core/internal/consumerauth/service/auth.go`

**Test files:**
- `core/internal/authentication/service/auth_test.go`
- `core/internal/consumerauth/service/auth_test.go`

---

### TDD cycle 1.1 — Identity tokens are UUID v4

**Write this failing test first** in `authentication/service/auth_test.go`:

```go
func TestLoginTokenIsUUIDv4(t *testing.T) {
    svc := newService(time.Hour)
    resp, err := svc.Login(model.LoginRequest{SystemName: "TestSystem"})
    if err != nil {
        t.Fatalf("Login failed: %v", err)
    }
    uuidRe := regexp.MustCompile(
        `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
    )
    if !uuidRe.MatchString(resp.Token) {
        t.Errorf("token %q is not a UUID v4", resp.Token)
    }
}

func TestLoginTokensAreUnique(t *testing.T) {
    svc := newService(time.Hour)
    r1, _ := svc.Login(model.LoginRequest{SystemName: "S"})
    r2, _ := svc.Login(model.LoginRequest{SystemName: "S"})
    if r1.Token == r2.Token {
        t.Error("two Login calls produced identical tokens")
    }
}
```

Add `"regexp"` to the import block.

**Expected failure:** `token "17e8a3f2c4..." is not a UUID v4`

**Implementation:** In `authentication/service/auth.go`, replace the token generator:

```go
import "crypto/rand"

func generateToken() string {
    var b [16]byte
    _, _ = rand.Read(b[:])
    b[6] = (b[6] & 0x0f) | 0x40  // version 4
    b[8] = (b[8] & 0x3f) | 0x80  // variant bits
    return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
        b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
```

Remove the `time.Now().UnixNano()` call. Keep `"time"` import (still used for token expiry).

---

### TDD cycle 1.2 — Authorization tokens are UUID v4

**Write this failing test first** in `consumerauth/service/auth_test.go`:

```go
func TestGenerateTokenIsUUIDv4(t *testing.T) {
    svc := newService()
    svc.Grant(model.GrantRequest{
        ConsumerSystemName: "Consumer1",
        ProviderSystemName: "Provider1",
        ServiceDefinition:  "temperature",
    })
    resp, err := svc.GenerateToken(model.TokenRequest{
        ConsumerSystemName: "Consumer1",
        ProviderSystemName: "Provider1",
        ServiceDefinition:  "temperature",
    })
    if err != nil {
        t.Fatalf("GenerateToken failed: %v", err)
    }
    uuidRe := regexp.MustCompile(
        `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
    )
    if !uuidRe.MatchString(resp.Token) {
        t.Errorf("token %q is not a UUID v4", resp.Token)
    }
}
```

**Implementation:** In `consumerauth/service/auth.go`, replace the `GenerateToken` body to use the same `crypto/rand` UUID generator as authentication. Remove `time.Now().UnixNano()` and the string concatenation with system names.

---

### System test

No new system test entry needed — the E2E test in `core/internal/integration/e2e_test.go`
already calls Login and passes the token to subsequent calls. After implementation,
confirm the token returned by Login in that test is opaque to callers (the E2E test
does not parse token format, so it passes regardless of format change).

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/authentication/service/... \
    ./internal/consumerauth/service/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on both packages.

### Completion criteria

- [x] `TestLoginTokenIsUUIDv4` passes
- [x] `TestLoginTokensAreUnique` passes
- [x] `TestGenerateTokenIsUUIDv4` passes
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on modified packages

---

## Step 2 — AH5 ServiceRegistry data model: composite ID and version normalisation

**Gaps addressed:**
- **G13** — `AH5ServiceInstance.InstanceId` is an auto-increment integer. Spec requires composite string `SystemName|ServiceDefinitionName|version`.
- **G14** — Empty `version` field stored as-is. Spec requires normalisation to `1.0.0`.

**Why together:** The composite ID embeds the normalised version. Implementing normalisation first ensures the ID is constructed correctly.

**Prerequisites:** Step 1 complete. Pre-flight check passes.

**Files to modify:**
- `core/internal/repository/ah5_memory.go`
- `core/internal/service/ah5_registry.go`

**Test files:**
- `core/internal/service/ah5_registry_test.go`
- `core/internal/api/ah5_handler_test.go`

---

### TDD cycle 2.1 — Version normalisation: empty string becomes `1.0.0`

**Write this failing test first** in `ah5_registry_test.go`:

```go
func TestRegisterServiceNormalisesEmptyVersion(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    req := model.ServiceRegistrationRequest{
        SystemName:            "Provider1",
        ServiceDefinitionName: "temperature",
        Version:               "",
    }
    resp, err := svc.RegisterService(req)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if resp.Version != "1.0.0" {
        t.Errorf("expected version 1.0.0, got %q", resp.Version)
    }
}

func TestRegisterServicePreservesExplicitVersion(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    req := model.ServiceRegistrationRequest{
        SystemName:            "Provider1",
        ServiceDefinitionName: "temperature",
        Version:               "2.3.1",
    }
    resp, _ := svc.RegisterService(req)
    if resp.Version != "2.3.1" {
        t.Errorf("expected version 2.3.1, got %q", resp.Version)
    }
}

func TestRegisterSystemNormalisesEmptyVersion(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    req := model.SystemRegistrationRequest{Name: "Provider1", Version: ""}
    resp, _ := svc.RegisterSystem(req)
    if resp.Version != "1.0.0" {
        t.Errorf("expected version 1.0.0, got %q", resp.Version)
    }
}
```

**Expected failure:** `expected version 1.0.0, got ""`

**Implementation:** In `ah5_registry.go`, add:

```go
func normaliseVersion(v string) string {
    if strings.TrimSpace(v) == "" {
        return "1.0.0"
    }
    return v
}
```

Call `req.Version = normaliseVersion(req.Version)` in `RegisterService` and `RegisterSystem` before passing to the store. Do NOT apply in management `CreateServiceInstances` or `UpdateServiceInstances`.

---

### TDD cycle 2.2 — Composite ServiceInstanceID format

**Write this failing test first** in `ah5_registry_test.go`:

```go
func TestServiceInstanceIDIsComposite(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    req := model.ServiceRegistrationRequest{
        SystemName:            "Provider1",
        ServiceDefinitionName: "temperature",
        Version:               "1.0.0",
    }
    resp, _ := svc.RegisterService(req)
    want := "Provider1|temperature|1.0.0"
    if resp.InstanceId != want {
        t.Errorf("expected instanceId %q, got %q", want, resp.InstanceId)
    }
}

func TestServiceInstanceIDStableOnUpsert(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    req := model.ServiceRegistrationRequest{
        SystemName:            "Provider1",
        ServiceDefinitionName: "temperature",
        Version:               "1.0.0",
    }
    r1, _ := svc.RegisterService(req)
    r2, _ := svc.RegisterService(req)  // upsert
    if r1.InstanceId != r2.InstanceId {
        t.Errorf("instanceId changed on upsert: %q → %q", r1.InstanceId, r2.InstanceId)
    }
}

func TestServiceRevokeByCompositeID(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    req := model.ServiceRegistrationRequest{
        SystemName: "Provider1", ServiceDefinitionName: "temperature", Version: "1.0.0",
    }
    inst, _ := svc.RegisterService(req)
    ok := svc.RevokeService(inst.InstanceId)
    if !ok {
        t.Error("expected RevokeService to return true for existing instance")
    }
    results, _ := svc.LookupServices(model.ServiceLookupRequest{
        InstanceIds: []string{inst.InstanceId},
    })
    if len(results) != 0 {
        t.Error("revoked instance still appears in lookup")
    }
}
```

**Expected failure:** `expected instanceId "Provider1|temperature|1.0.0", got "1"` (or similar integer string)

**Implementation:**

In `ah5_memory.go`:
1. Remove the `counter atomic.Int64` field from `AH5Store`.
2. Add:
   ```go
   func compositeServiceID(systemName, serviceDefName, version string) string {
       return systemName + "|" + serviceDefName + "|" + version
   }
   ```
3. In `SaveServiceInstance` and `CreateServiceInstance`, replace `fmt.Sprintf("%d", s.counter.Add(1))` with `compositeServiceID(req.SystemName, req.ServiceDefinitionName, req.Version)`.
4. Remove `"fmt"` and `"sync/atomic"` imports if they become unused.

In `ah5_handler.go`, in the `handleServiceRevoke` handler, add URL-decoding of the path parameter (pipe characters must be percent-encoded by clients):

```go
import "net/url"

rawID := strings.TrimPrefix(r.URL.Path, "/serviceregistry/service-discovery/revoke/")
id, err := url.PathUnescape(rawID)
if err != nil || id == "" {
    writeError(w, http.StatusBadRequest, "instanceId required in path")
    return
}
```

---

### TDD cycle 2.3 — Handler: revoke accepts URL-encoded composite ID

**Write this failing test first** in `ah5_handler_test.go`:

```go
func TestAH5ServiceRevokeByCompositeID(t *testing.T) {
    h, _ := newAH5TestHandler()
    // Register a service
    body := `{"systemName":"Provider1","serviceDefinitionName":"temperature","version":"1.0.0"}`
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, httptest.NewRequest("POST",
        "/serviceregistry/service-discovery/register", strings.NewReader(body)))
    if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
        t.Fatalf("register failed: %d", rr.Code)
    }

    // Revoke using URL-encoded composite ID
    encodedID := url.PathEscape("Provider1|temperature|1.0.0")
    rr2 := httptest.NewRecorder()
    h.ServeHTTP(rr2, httptest.NewRequest("DELETE",
        "/serviceregistry/service-discovery/revoke/"+encodedID, nil))
    if rr2.Code != http.StatusOK {
        t.Errorf("expected 200, got %d: %s", rr2.Code, rr2.Body.String())
    }
}
```

### System test addition

In `core/internal/integration/e2e_test.go`, verify that the AH5 service registration
round-trip returns a composite string instanceId. Locate the AH5 registration
section and add an assertion:

```go
// After AH5 service registration:
if !strings.Contains(inst.InstanceId, "|") {
    t.Errorf("AH5 instanceId should be composite string, got %q", inst.InstanceId)
}
```

### Coverage check

```bash
go test -coverprofile=coverage.out \
    ./internal/repository/... ./internal/service/... ./internal/api/...
go tool cover -func=coverage.out | grep -E "ah5|total"
```

Target: ≥ 80% on `ah5_memory.go`, `ah5_registry.go`, `ah5_handler.go`.

### Completion criteria

- [x] All TDD cycle tests pass
- [x] `go test -race ./...` passes
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] `AH5ServiceInstance.InstanceId` is composite string in all responses
- [x] Empty `version` normalises to `"1.0.0"` in service and system registration
- [x] Coverage ≥ 80% on modified packages

---

## Step 3 — AH5 ServiceRegistry query features: `alivesAt` and 423 Locked

**Gaps addressed:**
- **G17** — `alivesAt` field in service lookup requests is not evaluated. Expired services should be excluded.
- **G18** — `DELETE /serviceregistry/device-discovery/revoke/{name}` returns 200/204 unconditionally. Should return 423 if the device has registered dependent systems.

**Why together:** Both are purely additive to the AH5 SR query and delete paths. Neither conflicts with Step 2 changes.

**Prerequisites:** Step 2 complete. Pre-flight check passes.

**Files to modify:**
- `core/internal/model/ah5_types.go` — add `AlivesAt` to `ServiceLookupRequest`
- `core/internal/service/ah5_registry.go` — alivesAt filter; `ErrLocked`; `RevokeDevice` signature
- `core/internal/repository/ah5_memory.go` — `HasDependentSystems` method
- `core/internal/api/ah5_handler.go` — 423 response mapping; updated `RevokeDevice` call

---

### TDD cycle 3.1 — alivesAt excludes expired services

**Write this failing test first** in `ah5_registry_test.go`:

```go
func TestAlivesAtExcludesExpiredService(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
    svc.RegisterService(model.ServiceRegistrationRequest{
        SystemName: "P1", ServiceDefinitionName: "svc", Version: "1.0.0",
        ExpiresAt: past,
    })
    now := time.Now().UTC().Format(time.RFC3339)
    results, _ := svc.LookupServices(model.ServiceLookupRequest{AlivesAt: now})
    if len(results) != 0 {
        t.Errorf("expected 0 results (expired), got %d", len(results))
    }
}

func TestAlivesAtIncludesLiveService(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
    svc.RegisterService(model.ServiceRegistrationRequest{
        SystemName: "P1", ServiceDefinitionName: "svc", Version: "1.0.0",
        ExpiresAt: future,
    })
    now := time.Now().UTC().Format(time.RFC3339)
    results, _ := svc.LookupServices(model.ServiceLookupRequest{AlivesAt: now})
    if len(results) != 1 {
        t.Errorf("expected 1 result (live), got %d", len(results))
    }
}

func TestAlivesAtIncludesServiceWithNoExpiry(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    svc.RegisterService(model.ServiceRegistrationRequest{
        SystemName: "P1", ServiceDefinitionName: "svc", Version: "1.0.0",
    })
    now := time.Now().UTC().Format(time.RFC3339)
    results, _ := svc.LookupServices(model.ServiceLookupRequest{AlivesAt: now})
    if len(results) != 1 {
        t.Errorf("expected 1 result (no expiry = immortal), got %d", len(results))
    }
}
```

**Expected failure:** `expected 0 results (expired), got 1`

**Implementation:** Add `AlivesAt string` to `model.ServiceLookupRequest`. In `ah5_registry.go`, after all existing filters in `LookupServices`:

```go
if req.AlivesAt != "" {
    alivesAt, err := time.Parse(time.RFC3339, req.AlivesAt)
    if err == nil {
        var alive []model.AH5ServiceInstance
        for _, inst := range matched {
            if inst.ExpiresAt == "" {
                alive = append(alive, inst)
                continue
            }
            exp, perr := time.Parse(time.RFC3339, inst.ExpiresAt)
            if perr != nil || !exp.Before(alivesAt) {
                alive = append(alive, inst)
            }
        }
        matched = alive
    }
}
```

Add `"time"` to `ah5_registry.go` imports.

---

### TDD cycle 3.2 — 423 Locked when device has dependent systems

**Write this failing test first** in `ah5_registry_test.go`:

```go
func TestRevokeDeviceLockedWhenSystemDependent(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    // Register device
    svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "GW01"})
    // Register system referencing device
    svc.RegisterSystem(model.SystemRegistrationRequest{Name: "Sensor1", DeviceName: "GW01"})
    // Attempt to revoke device
    _, err := svc.RevokeDevice("GW01")
    if !errors.Is(err, service.ErrLocked) {
        t.Errorf("expected ErrLocked, got %v", err)
    }
}

func TestRevokeDeviceSucceedsWithNoDependent(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "GW02"})
    ok, err := svc.RevokeDevice("GW02")
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
    if !ok {
        t.Error("expected true (device found and removed)")
    }
}
```

**Expected failure:** `expected ErrLocked, got <nil>` (current implementation returns bool only and never errors)

Note: `RevokeDevice` currently returns `bool`. This test requires changing it to `(bool, error)`. The test will initially fail to compile — add the signature change first, then confirm the runtime assertion fails.

**Implementation:**

In `ah5_registry.go`:
```go
var ErrLocked = errors.New("entity has dependents and cannot be deleted")

func (s *AH5RegistryService) RevokeDevice(name string) (bool, error) {
    if s.store.HasDependentSystems(name) {
        return false, ErrLocked
    }
    return s.store.DeleteDevice(name), nil
}
```

In `ah5_memory.go`, add:
```go
func (s *AH5Store) HasDependentSystems(deviceName string) bool {
    s.mu.RLock()
    defer s.mu.RUnlock()
    for _, sys := range s.systems {
        if sys.Device != nil && sys.Device.Name == deviceName {
            return true
        }
    }
    return false
}
```

Add `"errors"` to `ah5_registry.go` imports.

Update existing tests `TestDeviceRevokeFound` and `TestDeviceRevokeNotFound` to handle `(bool, error)`.

---

### TDD cycle 3.3 — Handler returns 423 for locked device

**Write this failing test first** in `ah5_handler_test.go`:

```go
func TestAH5DeviceRevoke423WhenSystemDependent(t *testing.T) {
    h, _ := newAH5TestHandler()
    // Register device
    h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST",
        "/serviceregistry/device-discovery/register",
        strings.NewReader(`{"name":"GW01"}`)))
    // Register system referencing device
    h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST",
        "/serviceregistry/system-discovery/register",
        strings.NewReader(`{"name":"Sensor1","deviceName":"GW01","addresses":["192.0.2.1"]}`)))
    // Attempt revoke
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, httptest.NewRequest("DELETE",
        "/serviceregistry/device-discovery/revoke/GW01", nil))
    if rr.Code != http.StatusLocked {
        t.Errorf("expected 423, got %d", rr.Code)
    }
}
```

**Expected failure:** `expected 423, got 200`

**Implementation:** In `ah5_handler.go`, update `handleDeviceRevoke` to handle `(bool, error)` from `RevokeDevice`. Add:
```go
import "errors"

ok, err := h.svc.RevokeDevice(name)
if errors.Is(err, service.ErrLocked) {
    writeError(w, http.StatusLocked, err.Error())
    return
}
```

`http.StatusLocked` = 423, defined in Go standard library.

### System test addition

In `core/test-system.sh`, after the existing unit test step, add a comment block:

```bash
# Note: alivesAt filtering and 423 Locked are covered by unit tests in
# internal/service/ah5_registry_test.go and internal/api/ah5_handler_test.go.
# The E2E integration test in internal/integration/e2e_test.go covers the full
# stack path.
```

Add a test to `e2e_test.go` that registers a device, registers a system referencing it,
attempts device deletion, and asserts 423.

### Coverage check

```bash
go test -coverprofile=coverage.out \
    ./internal/service/... ./internal/repository/... ./internal/api/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `ah5_registry.go`, `ah5_memory.go`, `ah5_handler.go`.

### Completion criteria

- [x] All TDD cycle tests pass
- [x] `go test -race ./...` passes
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] E2E test covers 423 case
- [x] Coverage ≥ 80% on modified packages

---

## Step 4 — AH5 naming convention validation

**Gaps addressed:**
- **G19** — AH5 registration endpoints accept any non-empty name. Spec enforces strict naming: SystemName=PascalCase, DeviceName=UPPER_SNAKE_CASE, ServiceDefinitionName=camelCase, InterfaceTemplateName=snake_case, max 63 chars each.

**Prerequisites:** Steps 1–3 complete. Pre-flight check passes.

**Files to modify:**
- `core/internal/api/validate.go` (new file in package `api`)
- `core/internal/api/ah5_handler.go`
- `core/internal/api/ah5_handler_test.go` — rename non-conformant test data
- `core/internal/service/ah5_registry_test.go` — rename non-conformant test data

**Important: before writing the first failing test**, rename all non-conformant names in existing AH5 test files. This is a refactor, not a TDD cycle:

| Old (non-conformant) | New (conformant) | Type |
|----------------------|------------------|------|
| `"d1"`, `"d2"`, `"rem"` | `"D1"`, `"D2"`, `"Rem"` | DeviceName |
| `"gw-1"`, `"gw-2"` | `"Gw1"`, `"Gw2"` | SystemName |
| `"sensor-system"`, `"my-system"` | `"SensorSystem"`, `"MySystem"` | SystemName |
| `"prov-1"`, `"p"` | `"Prov1"`, `"P1"` | SystemName |
| `"s1"`, `"s2"` | `"Svc1"`, `"Svc2"` | ServiceDefinitionName (note: must be camelCase starting with lowercase — `"temperature"`, `"humidity"`, `"svc1"` are valid) |

After renaming, confirm: `go test ./...` — all tests must still pass before proceeding.

---

### TDD cycle 4.1 — SystemName validation

**Write this failing test first** in `ah5_handler_test.go`:

```go
func TestAH5SystemRegister_InvalidNameLowerStart(t *testing.T) {
    h, _ := newAH5TestHandler()
    body := `{"name":"mySystem","addresses":["192.0.2.1"]}`
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, httptest.NewRequest("POST",
        "/serviceregistry/system-discovery/register", strings.NewReader(body)))
    if rr.Code != http.StatusBadRequest {
        t.Errorf("expected 400 for non-PascalCase SystemName, got %d", rr.Code)
    }
}

func TestAH5SystemRegister_ValidName(t *testing.T) {
    h, _ := newAH5TestHandler()
    body := `{"name":"MySystem","addresses":["192.0.2.1"]}`
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, httptest.NewRequest("POST",
        "/serviceregistry/system-discovery/register", strings.NewReader(body)))
    if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
        t.Errorf("expected 200/201 for valid SystemName, got %d: %s",
            rr.Code, rr.Body.String())
    }
}
```

**Expected failure:** `expected 400 for non-PascalCase SystemName, got 201`

---

### TDD cycle 4.2 — DeviceName validation

```go
func TestAH5DeviceRegister_InvalidNameLowercase(t *testing.T) {
    h, _ := newAH5TestHandler()
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, httptest.NewRequest("POST",
        "/serviceregistry/device-discovery/register",
        strings.NewReader(`{"name":"gw01"}`)))
    if rr.Code != http.StatusBadRequest {
        t.Errorf("expected 400 for lowercase DeviceName, got %d", rr.Code)
    }
}

func TestAH5DeviceRegister_InvalidNameTrailingUnderscore(t *testing.T) {
    h, _ := newAH5TestHandler()
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, httptest.NewRequest("POST",
        "/serviceregistry/device-discovery/register",
        strings.NewReader(`{"name":"GW01_"}`)))
    if rr.Code != http.StatusBadRequest {
        t.Errorf("expected 400 for trailing underscore, got %d", rr.Code)
    }
}

func TestAH5DeviceRegister_ValidName(t *testing.T) {
    h, _ := newAH5TestHandler()
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, httptest.NewRequest("POST",
        "/serviceregistry/device-discovery/register",
        strings.NewReader(`{"name":"GW01"}`)))
    if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
        t.Errorf("expected 200/201, got %d", rr.Code)
    }
}
```

---

### TDD cycle 4.3 — ServiceDefinitionName validation

```go
func TestAH5ServiceRegister_InvalidServiceDefNameUpperStart(t *testing.T) {
    h, _ := newAH5TestHandler()
    body := `{"systemName":"Provider1","serviceDefinitionName":"Temperature","version":"1.0.0"}`
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, httptest.NewRequest("POST",
        "/serviceregistry/service-discovery/register", strings.NewReader(body)))
    if rr.Code != http.StatusBadRequest {
        t.Errorf("expected 400 for UpperCase ServiceDefinitionName, got %d", rr.Code)
    }
}
```

**Implementation:** Create `core/internal/api/validate.go`:

```go
package api

import "regexp"

var (
    reSystemName            = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,62}$`)
    reDeviceName            = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,61}[A-Z0-9]$`)
    reServiceDefinitionName = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,62}$`)
    reInterfaceTemplateName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
)

func validateSystemName(name string) string {
    if !reSystemName.MatchString(name) {
        return "systemName must be PascalCase (^[A-Z][A-Za-z0-9]{0,62}$), got: " + name
    }
    return ""
}
func validateDeviceName(name string) string {
    if !reDeviceName.MatchString(name) {
        return "deviceName must be UPPER_SNAKE_CASE with no trailing underscore " +
            "(^[A-Z][A-Z0-9_]{0,61}[A-Z0-9]$), got: " + name
    }
    return ""
}
func validateServiceDefinitionName(name string) string {
    if !reServiceDefinitionName.MatchString(name) {
        return "serviceDefinitionName must be camelCase (^[a-z][A-Za-z0-9]{0,62}$), got: " + name
    }
    return ""
}
func validateInterfaceTemplateName(name string) string {
    if !reInterfaceTemplateName.MatchString(name) {
        return "interfaceTemplateName must be snake_case (^[a-z][a-z0-9_]{0,62}$), got: " + name
    }
    return ""
}
```

Wire into AH5 register/create handlers. Revoke and lookup handlers do not re-validate names (they accept pre-existing names).

**Edge case to document:** `reDeviceName` requires ≥ 2 characters (anchor `[A-Z]` + end `[A-Z0-9]`). A single-character device name is rejected. Add a `// NOTE: single-char device names rejected per AH5 spec regex` comment in `validate.go` and record in `GAP_ANALYSIS.md`.

### Coverage check

```bash
go test -coverprofile=coverage.out ./internal/api/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `validate.go` and `ah5_handler.go`.

### Completion criteria

- [x] All existing tests still pass after test-data rename
- [x] All TDD cycle tests for validation pass
- [x] `go test -race ./...` passes
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on modified packages

---

## Step 5 — AH5 ServiceRegistry metadata query operators

**Gaps addressed:**
- **G16** — Metadata lookups accept only exact key-value strings. Spec defines six operators: `EQUALS_TO`, `NOT_EQUALS_TO`, `LESS_THAN_OR_EQUALS_TO`, `GREATER_THAN_OR_EQUALS_TO`, `CONTAINS`, `NOT_CONTAINS`. Also supports boolean shorthand `{"key": true}`.

**Prerequisites:** Steps 1–4 complete. Pre-flight check passes.

**Files to modify:**
- `core/internal/model/ah5_types.go`
- `core/internal/service/ah5_registry.go`

---

### TDD cycle 5.1 — EQUALS_TO operator

**Write this failing test first** in `ah5_registry_test.go`:

```go
func TestMetadataOpEqualsTo(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    svc.RegisterService(model.ServiceRegistrationRequest{
        SystemName: "P1", ServiceDefinitionName: "temperature", Version: "1.0.0",
        Metadata: map[string]string{"env": "prod"},
    })
    results, _ := svc.LookupServices(model.ServiceLookupRequest{
        MetadataRequirements: map[string]model.MetadataRequirement{
            "env": {Op: model.OpEqualsTo, Value: "prod"},
        },
    })
    if len(results) != 1 {
        t.Errorf("EQUALS_TO: expected 1 match, got %d", len(results))
    }
    results2, _ := svc.LookupServices(model.ServiceLookupRequest{
        MetadataRequirements: map[string]model.MetadataRequirement{
            "env": {Op: model.OpEqualsTo, Value: "staging"},
        },
    })
    if len(results2) != 0 {
        t.Errorf("EQUALS_TO non-match: expected 0, got %d", len(results2))
    }
}
```

---

### TDD cycles 5.2–5.6 — Remaining operators

Write one test per operator following the same pattern as 5.1:

| Test | Operator | Setup | Requirement | Expected |
|------|----------|-------|-------------|----------|
| `TestMetadataOpNotEqualsTo` | `NOT_EQUALS_TO` | `env=prod` | `env != staging` | 1 match |
| `TestMetadataOpContains` | `CONTAINS` | `desc=hello world` | `desc CONTAINS world` | 1 match |
| `TestMetadataOpNotContains` | `NOT_CONTAINS` | `desc=hello world` | `desc NOT_CONTAINS foo` | 1 match |
| `TestMetadataOpLessThanOrEqualTo` | `LESS_THAN_OR_EQUALS_TO` | `error=0.5` | `error ≤ 1.0` | 1 match |
| `TestMetadataOpGreaterThanOrEqualTo` | `GREATER_THAN_OR_EQUALS_TO` | `error=0.5` | `error ≥ 0.1` | 1 match |

---

### TDD cycle 5.7 — Boolean shorthand

```go
func TestMetadataShorthandBool(t *testing.T) {
    store := repository.NewAH5Store()
    svc := service.NewAH5RegistryService(store)
    svc.RegisterService(model.ServiceRegistrationRequest{
        SystemName: "P1", ServiceDefinitionName: "temperature", Version: "1.0.0",
        Metadata: map[string]string{"active": "true"},
    })
    // Shorthand: {Op: "" (unset), Value: true} treated as EQUALS_TO
    results, _ := svc.LookupServices(model.ServiceLookupRequest{
        MetadataRequirements: map[string]model.MetadataRequirement{
            "active": {Value: true},
        },
    })
    if len(results) != 1 {
        t.Errorf("bool shorthand: expected 1, got %d", len(results))
    }
}
```

---

### TDD cycle 5.8 — JSON unmarshalling: both wire forms

```go
func TestMetadataRequirementUnmarshalStructured(t *testing.T) {
    raw := `{"op":"CONTAINS","value":"world"}`
    var req model.MetadataRequirement
    if err := json.Unmarshal([]byte(raw), &req); err != nil {
        t.Fatalf("unmarshal error: %v", err)
    }
    if req.Op != model.OpContains {
        t.Errorf("expected CONTAINS, got %q", req.Op)
    }
}

func TestMetadataRequirementUnmarshalShorthand(t *testing.T) {
    raw := `"prod"`
    var req model.MetadataRequirement
    if err := json.Unmarshal([]byte(raw), &req); err != nil {
        t.Fatalf("unmarshal error: %v", err)
    }
    if req.Op != model.OpEqualsTo {
        t.Errorf("expected EQUALS_TO for shorthand, got %q", req.Op)
    }
    if req.Value != "prod" {
        t.Errorf("expected value prod, got %v", req.Value)
    }
}
```

**Implementation:**

Add to `ah5_types.go`:

```go
import "encoding/json"

type MetadataOp string

const (
    OpEqualsTo              MetadataOp = "EQUALS_TO"
    OpNotEqualsTo           MetadataOp = "NOT_EQUALS_TO"
    OpLessThanOrEqualsTo    MetadataOp = "LESS_THAN_OR_EQUALS_TO"
    OpGreaterThanOrEqualsTo MetadataOp = "GREATER_THAN_OR_EQUALS_TO"
    OpContains              MetadataOp = "CONTAINS"
    OpNotContains           MetadataOp = "NOT_CONTAINS"
)

type MetadataRequirement struct {
    Op    MetadataOp
    Value interface{}
}

type metadataRequirementWire struct {
    Op    MetadataOp  `json:"op"`
    Value interface{} `json:"value"`
}

func (m *MetadataRequirement) UnmarshalJSON(data []byte) error {
    var wire metadataRequirementWire
    if err := json.Unmarshal(data, &wire); err == nil && wire.Op != "" {
        m.Op = wire.Op
        m.Value = wire.Value
        return nil
    }
    var v interface{}
    if err := json.Unmarshal(data, &v); err != nil {
        return err
    }
    m.Op = OpEqualsTo
    m.Value = v
    return nil
}
```

Replace `Metadata map[string]string` in `ServiceLookupRequest`, `DeviceLookupRequest`, `SystemLookupRequest` with `MetadataRequirements map[string]MetadataRequirement`.

Add `matchesMetadata` to `ah5_registry.go` with operator dispatch; wire into all three lookup functions.

### Coverage check

```bash
go test -coverprofile=coverage.out \
    ./internal/model/... ./internal/service/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `ah5_types.go` and `ah5_registry.go`.

### Completion criteria

- [x] All 8 TDD cycle tests pass
- [x] `go test -race ./...` passes
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on modified packages

---

## Step 6 — ConsumerAuthorization path alignment

**Gaps addressed:**
- **G12** — ConsumerAuthorization uses path prefix `/authorization/`. Spec requires `/consumerauthorization/`.

**Impact:** This is the most experiment-visible change. Every experiment that calls ConsumerAuthorization must be updated.

**Callers to update:**

| Location | What to update |
|----------|----------------|
| `core/cmd/consumerauth/main.go` | Route prefix |
| `core/internal/consumerauth/api/handler.go` | Path prefix constant/strings |
| `core/internal/orchestration/dynamic/service/orchestrator.go` | URL for ConsumerAuth verify call |
| `core-evol/internal/orchestration/ca_decider.go` | URL for ConsumerAuth verify call |
| `experiments/experiment-{1..14}/docker-compose.yml` | Seed container `curl` calls |
| `experiments/experiment-{1..14}/test-system.sh` | All `curl` calls to ConsumerAuth |
| Any experiment Go service that calls ConsumerAuth directly | URL strings |

**Prerequisites:** Steps 1–5 complete. Pre-flight check passes on both `core/` and `core-evol/`.

---

### TDD cycle 6.1 — Handler rejects old path

```go
func TestConsumerAuthOldPathReturns404(t *testing.T) {
    // The handler should NOT be registered at the old /authorization/ prefix.
    // This test verifies the route change.
    srv := newConsumerAuthTestServer()
    resp, err := http.Post(srv.URL+"/authorization/grant", "application/json",
        strings.NewReader(`{"consumerSystemName":"C","providerSystemName":"P","serviceDefinition":"s"}`))
    if err != nil {
        t.Fatal(err)
    }
    if resp.StatusCode != http.StatusNotFound {
        t.Errorf("old path: expected 404, got %d", resp.StatusCode)
    }
}

func TestConsumerAuthNewPathAcceptsGrant(t *testing.T) {
    srv := newConsumerAuthTestServer()
    resp, err := http.Post(srv.URL+"/consumerauthorization/authorization/grant",
        "application/json",
        strings.NewReader(`{"consumerSystemName":"C","providerSystemName":"P","serviceDefinition":"s"}`))
    if err != nil {
        t.Fatal(err)
    }
    if resp.StatusCode != http.StatusCreated {
        t.Errorf("new path: expected 201, got %d", resp.StatusCode)
    }
}
```

**Expected failure:** old path test gets 201 (currently registered there); new path test gets 404.

**Implementation:** Change the path prefix in `cmd/consumerauth/main.go` and `consumerauth/api/handler.go` from `/authorization` to `/consumerauthorization/authorization`.

Note: the internal path structure (`/authorization/grant`, `/authorization/verify`, etc.) is retained as the tail — only the system-level prefix changes. The full paths become:
- `POST /consumerauthorization/authorization/grant`
- `DELETE /consumerauthorization/authorization/revoke/{id}`
- `GET /consumerauthorization/authorization/lookup`
- `POST /consumerauthorization/authorization/verify`
- `POST /consumerauthorization/authorization/token/generate`

Update `core/SPEC.md` to reflect the new paths.

---

### TDD cycle 6.2 — E2E test uses new path

In `core/internal/integration/e2e_test.go`, update all ConsumerAuthorization endpoint
strings from `/authorization/` to `/consumerauthorization/authorization/`.
Run the E2E test in isolation first to confirm it fails at the old path, then apply the path change.

---

### Experiment updates

For each experiment in `experiments/experiment-1/` through `experiment-14/`:

1. Search for `/authorization/grant`, `/authorization/revoke`, `/authorization/lookup`,
   `/authorization/verify` in `docker-compose.yml`, `test-system.sh`, and any `.go` files.
2. Replace with `/consumerauthorization/authorization/grant` etc.
3. Run `bash experiments/experiment-N/test-system.sh` (requires Docker stack up).

Also update `core-evol/internal/orchestration/ca_decider.go`:
- Find the URL construction that calls `/authorization/verify`
- Change to `/consumerauthorization/authorization/verify`

---

### System test

After all updates:

```bash
# Core tests
bash core/test-system.sh

# Experiment tests (each requires: docker compose up -d --build)
bash experiments/experiment-9/test-system.sh
bash experiments/experiment-13/test-system.sh
bash experiments/experiment-14/test-system.sh
```

### Coverage check

```bash
go test -coverprofile=coverage.out \
    ./internal/consumerauth/... \
    ./internal/orchestration/dynamic/...
go tool cover -func=coverage.out
```

Target: ≥ 80%.

### Completion criteria

- [x] `GET /authorization/grant` returns 404
- [x] `GET /consumerauthorization/authorization/grant` returns 201
- [x] E2E test passes with new path
- [x] All experiment test-system.sh pass
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] `core/SPEC.md` updated with new paths
- [x] Coverage ≥ 80% on modified packages

---

## Step 7 — Authentication API surface alignment

**Gaps addressed:**
- **G15** — Multiple wire-format divergences on Authentication endpoints:
  1. Login response field: `expiresAt` → `expirationTime`
  2. Login response missing `sysop` field (add as `false` always)
  3. Logout: `DELETE` method → `POST` method
  4. Verify: header-based token → path parameter (`GET /verify/{token}`)
  5. Verify response: `valid` → `verified`; add `loginTime` field

**Impact:** Experiment Go code parses `expiresAt` by name — must be updated before core changes, or both changed atomically.

**Prerequisites:** Steps 1–6 complete.

**Callers using `expiresAt`:**
- `experiments/experiment-13/services/robot-fleet-tls/main.go`
- Equivalent services in experiments 9 and 14
- Any test script that parses the login response

---

### TDD cycle 7.1 — Login response field names

**Write this failing test first** in `authentication/api/handler_test.go`:

```go
func TestLoginResponseHasExpirationTime(t *testing.T) {
    srv := newAuthTestServer()
    body := `{"systemName":"Sys1","credentials":{"password":"x"}}`
    resp, _ := http.Post(srv.URL+"/authentication/identity/login",
        "application/json", strings.NewReader(body))
    var m map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&m)
    if _, ok := m["expirationTime"]; !ok {
        t.Errorf("response missing expirationTime field; got keys: %v", keys(m))
    }
    if _, ok := m["expiresAt"]; ok {
        t.Error("response still has old expiresAt field")
    }
}
```

**Expected failure:** `response missing expirationTime field`

**Implementation:**
- In `authentication/model/types.go`, rename `ExpiresAt` to `ExpirationTime` in `LoginResponse` (adjust JSON tag: `json:"expirationTime"`).
- Add `Sysop bool` with `json:"sysop"` (always false for now).

**Important:** Update experiment code **in the same commit or immediately after**:
- `experiments/experiment-13/services/robot-fleet-tls/main.go` — change field reference
- Equivalent files in experiments 9 and 14

---

### TDD cycle 7.2 — Logout uses POST

```go
func TestLogoutRequiresPOST(t *testing.T) {
    srv := newAuthTestServer()
    // Login first
    loginResp, _ := http.Post(srv.URL+"/authentication/identity/login",
        "application/json",
        strings.NewReader(`{"systemName":"S","credentials":{"password":"x"}}`))
    var lr map[string]interface{}
    json.NewDecoder(loginResp.Body).Decode(&lr)
    token := lr["token"].(string)

    // DELETE should return 405 Method Not Allowed
    req, _ := http.NewRequest(http.MethodDelete,
        srv.URL+"/authentication/identity/logout", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    resp, _ := http.DefaultClient.Do(req)
    if resp.StatusCode != http.StatusMethodNotAllowed {
        t.Errorf("DELETE logout: expected 405, got %d", resp.StatusCode)
    }

    // POST should return 200 or 204
    req2, _ := http.NewRequest(http.MethodPost,
        srv.URL+"/authentication/identity/logout", nil)
    req2.Header.Set("Authorization", "Bearer "+token)
    resp2, _ := http.DefaultClient.Do(req2)
    if resp2.StatusCode != http.StatusOK && resp2.StatusCode != http.StatusNoContent {
        t.Errorf("POST logout: expected 200/204, got %d", resp2.StatusCode)
    }
}
```

**Implementation:** In `authentication/api/handler.go`, change the method guard on the logout handler from `http.MethodDelete` to `http.MethodPost`.

---

### TDD cycle 7.3 — Verify uses path parameter

```go
func TestVerifyAcceptsPathParam(t *testing.T) {
    srv := newAuthTestServer()
    loginResp, _ := http.Post(srv.URL+"/authentication/identity/login",
        "application/json",
        strings.NewReader(`{"systemName":"S","credentials":{"password":"x"}}`))
    var lr map[string]interface{}
    json.NewDecoder(loginResp.Body).Decode(&lr)
    token := lr["token"].(string)

    resp, _ := http.Get(srv.URL + "/authentication/identity/verify/" +
        url.PathEscape(token))
    if resp.StatusCode != http.StatusOK {
        t.Errorf("expected 200, got %d", resp.StatusCode)
    }
    var vr map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&vr)
    if vr["verified"] != true {
        t.Errorf("expected verified=true, got %v", vr["verified"])
    }
}
```

**Implementation:**
- Change the verify handler route from `/authentication/identity/verify` to `/authentication/identity/verify/` and extract the token from the path.
- Rename response field `valid` → `verified` in `authentication/model/types.go`.
- Add `LoginTime string` field (RFC3339 of when the token was issued — store with the token).

---

### System test

```bash
bash core/test-system.sh
bash experiments/experiment-9/test-system.sh
bash experiments/experiment-13/test-system.sh
bash experiments/experiment-14/test-system.sh
```

### Coverage check

```bash
go test -coverprofile=coverage.out ./internal/authentication/...
go tool cover -func=coverage.out
```

Target: ≥ 80%.

### Completion criteria

- [x] All TDD cycle tests pass
- [x] Login response has `expirationTime`, not `expiresAt`
- [x] Logout rejects DELETE, accepts POST
- [x] Verify accepts path parameter
- [x] Verify response has `verified`, not `valid`
- [x] All experiment services updated
- [x] All experiment test-system.sh pass
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80%

---

## Step 8 — Orchestration path alignment

**Gaps addressed:**
- **G24** — Orchestration systems use `/orchestration/dynamic`, `/orchestration/simplestore`, `/orchestration/flexiblestore`. Spec uses `/serviceorchestration/orchestration/pull` for both dynamic and simplestore (differentiated by service metadata).
- **G25** — Orchestration request missing `orchestrationFlags`. Implement `MATCHMAKING` and `ONLY_PREFERRED` flags.

**Prerequisites:** Steps 1–7 complete.

**Files to modify:**
- `core/internal/orchestration/dynamic/api/handler.go` — route + add stub flags
- `core/internal/orchestration/simplestore/api/handler.go` — route
- `core/internal/orchestration/flexiblestore/api/handler.go` — route
- `core/internal/orchestration/model/types.go` — add `OrchestrationFlags`
- `core-evol/cmd/dynamicorch-xacml/main.go` — route
- `experiments/experiment-{9..14}/docker-compose.yml` and service code that calls orchestration

---

### TDD cycle 8.1 — Dynamic orchestration accepts new path

```go
func TestDynamicOrchNewPath(t *testing.T) {
    srv := newDynamicOrchTestServer()
    body := `{"requesterSystem":{"systemName":"C","address":"h","port":1},
              "requestedService":{"serviceDefinition":"svc"},
              "orchestrationFlags":{}}`
    resp, err := http.Post(srv.URL+"/serviceorchestration/orchestration/pull",
        "application/json", strings.NewReader(body))
    if err != nil {
        t.Fatal(err)
    }
    if resp.StatusCode != http.StatusOK {
        t.Errorf("expected 200, got %d", resp.StatusCode)
    }
}

func TestDynamicOrchOldPathReturns404(t *testing.T) {
    srv := newDynamicOrchTestServer()
    resp, _ := http.Post(srv.URL+"/orchestration/dynamic",
        "application/json", strings.NewReader(`{}`))
    if resp.StatusCode != http.StatusNotFound {
        t.Errorf("old path: expected 404, got %d", resp.StatusCode)
    }
}
```

---

### TDD cycle 8.2 — `orchestrationFlags` field accepted

```go
func TestOrchestrationFlagsMATCHMAKING(t *testing.T) {
    // With MATCHMAKING=true, at most one result is returned.
    // Setup: SR mock returns two providers.
    // Expect: orchestration response contains exactly one.
    // (Test implementation depends on how the SR is mocked in handler tests.)
}

func TestOrchestrationFlagsONLY_PREFERRED(t *testing.T) {
    // With ONLY_PREFERRED=true and preferredProviders set,
    // only providers in the preferred list are returned.
}
```

Add `OrchestrationFlags` struct to `core/internal/orchestration/model/types.go`:

```go
type OrchestrationFlags struct {
    Matchmaking    bool `json:"MATCHMAKING"`
    OnlyPreferred  bool `json:"ONLY_PREFERRED"`
    AllowTranslation bool `json:"ALLOW_TRANSLATION"` // stub, always false
    OnlyExclusive  bool `json:"ONLY_EXCLUSIVE"`       // stub
    AllowIntercloud bool `json:"ALLOW_INTERCLOUD"`    // stub
    OnlyIntercloud  bool `json:"ONLY_INTERCLOUD"`     // stub
}
```

Implement `MATCHMAKING` (return first result only) and `ONLY_PREFERRED` (filter to `preferredProviders` list) in the dynamic orchestration service. Others accepted but ignored.

---

### System test

```bash
bash core/test-system.sh
bash experiments/experiment-9/test-system.sh
bash experiments/experiment-13/test-system.sh
bash experiments/experiment-14/test-system.sh
```

### Coverage check

```bash
go test -coverprofile=coverage.out ./internal/orchestration/...
go tool cover -func=coverage.out
```

Target: ≥ 80%.

### Completion criteria

- [x] `/serviceorchestration/orchestration/pull` accepted by all three orchestrators
- [x] Old paths return 404
- [x] `orchestrationFlags` parsed without error; MATCHMAKING and ONLY_PREFERRED functional
- [x] core-evol `dynamicorch-xacml` updated to new path
- [x] All experiment test-system.sh pass
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80%

---

## Step 9 — SQLite persistence layer

**Gaps addressed:**
- **G5** — All state is in-memory and lost on restart. Replace with SQLite-backed repositories for all six stateful systems.

**"Easy to clean" mechanism:**
- Each system's DB file path controlled by `DB_PATH` env var (default: `<system>.db` in current directory).
- Set `DB_PATH=:memory:` for tests — SQLite in-memory, zero setup, zero cleanup.
- Docker Compose: use named volumes (`serviceregistry-data`, etc.) or a bind-mount to `./data/`.
- Clean all data: `docker compose down -v` (named volumes) or `rm -rf ./data/*.db` (bind-mounts).
- Add a `reset` profile or `make reset` target that removes volumes.

**Go dependency:** Add `modernc.org/sqlite` (pure Go, no CGO) to `core/go.mod`.

**Prerequisites:** Steps 1–8 complete. Pre-flight check passes.

**Architecture:** Extract repository interfaces first (Step 9A), then add SQLite implementations (Step 9B).

---

### Step 9A — Extract repository interfaces

For each system's repository, introduce an interface that both `*Memory` and `*SQLite` will implement.

**Files to create (one per system):**

| System | Interface file | Interface name |
|--------|---------------|----------------|
| ServiceRegistry (AH5) | `repository/store.go` | `AH5StoreInterface` |
| ServiceRegistry (legacy) | `repository/registry.go` | `RegistryInterface` |
| Authentication | `authentication/repository/store.go` | `AuthStoreInterface` |
| ConsumerAuthorization | `consumerauth/repository/store.go` | `AuthzStoreInterface` |
| SimpleStore Orch | `orchestration/simplestore/repository/store.go` | `SimpleStoreInterface` |
| FlexibleStore Orch | `orchestration/flexiblestore/repository/store.go` | `FlexStoreInterface` |
| CA | `ca/` (service holds state directly — refactor to repository) | `CARespository` |

**TDD cycle 9A.1 — Interface extraction does not break any test**

After extracting interfaces, run the full test suite. No new tests are added in 9A — this is a refactor that must leave all tests green.

```bash
go build ./...
go test -race ./...
bash core/test-system.sh
```

All must pass before proceeding to 9B.

---

### Step 9B — SQLite implementations

For each repository interface, add a `sqlite.go` file in the same package implementing the interface. The `cmd/*/main.go` for each system selects the implementation based on `DB_PATH`:

```go
var store RepositoryInterface
if dbPath := os.Getenv("DB_PATH"); dbPath == "" || dbPath == ":memory:" {
    store = repository.NewMemory()   // default for tests
} else {
    var err error
    store, err = repository.NewSQLite(dbPath)
    if err != nil {
        log.Fatalf("failed to open database: %v", err)
    }
}
```

---

### TDD cycle 9B.1 — SQLite ServiceRegistry survives restart

```go
func TestSQLiteServiceRegistryPersists(t *testing.T) {
    dbPath := t.TempDir() + "/sr_test.db"

    // First instance: register a service
    store1, _ := repository.NewAH5SQLite(dbPath)
    svc1 := service.NewAH5RegistryService(store1)
    svc1.RegisterService(model.ServiceRegistrationRequest{
        SystemName: "Provider1", ServiceDefinitionName: "temperature", Version: "1.0.0",
    })
    store1.Close()

    // Second instance: data must still be there
    store2, _ := repository.NewAH5SQLite(dbPath)
    svc2 := service.NewAH5RegistryService(store2)
    results, _ := svc2.LookupServices(model.ServiceLookupRequest{
        ServiceDefinitionNames: []string{"temperature"},
    })
    if len(results) != 1 {
        t.Errorf("expected 1 persisted service, got %d", len(results))
    }
}
```

Write equivalent tests for each system:
- `TestSQLiteAuthRulePersists` — ConsumerAuthorization rule survives restart
- `TestSQLiteSimpleStoreRulePersists` — SimpleStore rule survives restart
- `TestSQLiteFlexStoreRulePersists` — FlexibleStore rule survives restart
- `TestSQLiteCACertRecordPersists` — CA issued cert record survives restart
- `TestSQLiteRevocationPersists` — CA revocation entry survives restart

---

### TDD cycle 9B.2 — `:memory:` mode is equivalent to in-memory store

All existing tests must pass unchanged when the test suite is run with SQLite in `:memory:` mode. Implement a test build tag or environment variable switch, then confirm:

```bash
DB_PATH=:memory: go test -race ./...
```

Must produce identical results to running without `DB_PATH`.

---

### TDD cycle 9B.3 — Reset: delete file gives clean state

```go
func TestSQLiteCleanOnFileDelete(t *testing.T) {
    dbPath := t.TempDir() + "/clean_test.db"
    store, _ := repository.NewAH5SQLite(dbPath)
    svc := service.NewAH5RegistryService(store)
    svc.RegisterDevice(model.DeviceRegistrationRequest{Name: "GW01"})
    store.Close()

    // Delete the file
    os.Remove(dbPath)

    // Re-open: should be empty
    store2, _ := repository.NewAH5SQLite(dbPath)
    svc2 := service.NewAH5RegistryService(store2)
    devices, _ := svc2.LookupDevices(model.DeviceLookupRequest{})
    if len(devices) != 0 {
        t.Errorf("expected empty store after file delete, got %d devices", len(devices))
    }
}
```

---

### Docker Compose integration

Add to each experiment's `docker-compose.yml`:

```yaml
volumes:
  serviceregistry-data:
  authentication-data:
  consumerauth-data:
  simplestore-data:
  flexstore-data:
  ca-data:

services:
  serviceregistry:
    environment:
      DB_PATH: /data/serviceregistry.db
    volumes:
      - serviceregistry-data:/data
```

Add a `reset` make target (or document the command) in each experiment's README:

```bash
# Full clean start:
docker compose down -v && docker compose up -d --build
```

---

### System test

```bash
bash core/test-system.sh
# Then with Docker:
bash experiments/experiment-9/test-system.sh
bash experiments/experiment-13/test-system.sh
bash experiments/experiment-14/test-system.sh
```

Also run a manual persistence test on each experiment:
1. `docker compose up -d --build` (seeds run, data populates)
2. `docker compose restart serviceregistry` (restart only core, not seeds)
3. `docker compose exec serviceregistry wget -qO- localhost:8080/serviceregistry/health`
4. `curl ... /serviceregistry/service-discovery/lookup` — must still return registered services

### Coverage check

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -5
```

Target: ≥ 80% across all packages.

### Completion criteria

- [x] Interface extraction leaves all existing tests green (Step 9A)
- [x] All six SQLite implementations compile and pass their TDD tests
- [x] `DB_PATH=:memory:` mode produces identical test results to in-memory
- [x] Restart test: services persisted before restart are visible after restart
- [x] File-delete test: empty database after file removed
- [x] `docker compose down -v` gives clean slate (verified manually)
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] All experiment test-system.sh pass
- [x] Coverage ≥ 80% across all packages

---

## 6. Documentation updates

After each step, update the relevant documentation files. After all nine steps are complete, apply the following documentation changes in full.

### `core/GAP_ANALYSIS.md`

Mark each resolved gap with a `**Status: Resolved in Step N**` line at the top of its section:

| Step | Gaps resolved |
|------|---------------|
| 1 | G6 |
| 2 | G13, G14 |
| 3 | G17, G18 |
| 4 | G19 |
| 5 | G16 |
| 6 | G12 |
| 7 | G15 (partial — response field names, logout method, verify path param) |
| 8 | G24, G25 (partial — MATCHMAKING, ONLY_PREFERRED) |
| 9 | G5 |

Add a note to G15 that `expiresAt` field name was changed to `expirationTime` — any consumer of the Authentication login API that parsed `expiresAt` must be updated.

Add DeviceName regex edge case note (≥ 2 characters required).

### `core/SPEC.md`

Update:
- ConsumerAuthorization path prefix: `/authorization` → `/consumerauthorization/authorization`
- ServiceInstanceID type: "integer" → "composite string `SystemName|ServiceDefinitionName|version`"
- Orchestration endpoint: `/orchestration/dynamic` → `/serviceorchestration/orchestration/pull`
- Authentication login response: rename `expiresAt` → `expirationTime`; add `sysop`
- Authentication verify: path parameter; rename `valid` → `verified`; add `loginTime`
- Version normalisation rule: empty string input → `1.0.0` stored
- Naming convention regexps (all four types)
- Metadata operators reference (all six)
- 423 Locked on device delete with dependents
- Persistence: `DB_PATH` environment variable, `:memory:` for tests

### `CONFORMANCE.md`

Move the following gaps from the open table to the resolved row in the phase plan:
G3, G13, G14, G15 (partial), G16, G17, G18, G19, G24, G25 (partial).

Update the per-system conformance ratings.

Add a note that G5 (no persistence) is resolved by SQLite layer.

### `EXPERIENCES.md`

Add one entry per non-obvious bug encountered during implementation (e.g., URL-encoding of `|` in composite IDs, SQLite WAL mode needed for concurrent readers, etc.). Use the existing entry format.

### `ARCHITECTURE.md`

Update:
- ConsumerAuthorization path prefix
- Orchestration paths
- Add SQLite persistence note (DB_PATH env var, volume management)
- Add `core-evol` section if not present

---

## 7. Regression matrix

Run these checks after every step, not just at the end.

| Check | Command | Requires Docker |
|-------|---------|-----------------|
| Core build | `cd core && go build ./...` | No |
| Core vet | `cd core && go vet ./...` | No |
| Core unit + E2E | `cd core && go test -race ./...` | No |
| Core system | `bash core/test-system.sh` | No |
| Core-evol build | `cd core-evol && go build ./...` | No |
| Core-evol tests | `cd core-evol && go test -race ./...` | No |
| Workspace build | `go build ./...` (repo root) | No |
| Experiment 9 | `bash experiments/experiment-9/test-system.sh` | Yes |
| Experiment 13 | `bash experiments/experiment-13/test-system.sh` | Yes |
| Experiment 14 | `bash experiments/experiment-14/test-system.sh` | Yes |

Steps 1–5 require only the "No Docker" checks.
Steps 6–9 require the full matrix including Docker.
Steps 10–21 follow the same rules: steps with no experiment impact need no Docker; steps marked "Yes" must run the full matrix.

---

## 8. Step summary

| Step | Gaps | Scope | Docker needed |
|------|------|-------|---------------|
| 1 | G6 | Token security — auth.go only | No |
| 2 | G13, G14 | Composite ID + version normalisation — AH5 SR | No |
| 3 | G17, G18 | alivesAt + 423 Locked — AH5 SR | No |
| 4 | G19 | Naming validation — AH5 handlers + test rename | No |
| 5 | G16 | Metadata operators — AH5 SR | No |
| 6 | G12 | ConsumerAuth path + all callers + experiments | Yes |
| 7 | G15 | Auth API surface + experiment Go code | Yes |
| 8 | G24, G25 | Orchestration paths + flags + core-evol | Yes |
| 9 | G5 | SQLite persistence — all six stateful systems | Yes |
| 10 | G31 | Structured error responses — all seven systems | No |
| 11 | G20 | Pagination — all query/list endpoints | No |
| 12 | G8, G15-rem | Auth completions: verify fields, change endpoint, token cleanup | No |
| 13 | G21, G2 | Authentication management: identity CRUD, sessions, credential verification | No |
| 14 | G22 | ConsumerAuth policy model overhaul: targetType, policyType, instanceId, scopedPolicies | Yes |
| 15 | G23 | ConsumerAuth token system: all authorization-token endpoints | No |
| 16 | G30 | Service instance interface model: structured ServiceInterfaceRequest | Yes |
| 17 | G7, G32 | Orchestration request/response alignment: field renames, spec typos, serviceRequirement | Yes |
| 18 | G27 | Orchestration extensions: SimpleStore mgmt API, lock management, history (also core-evol) | Yes |
| 19 | G26 | Push orchestration: subscribe/unsubscribe + push management (also core-evol) | Yes (partial — delivery stub) |
| 20 | G28 | Blacklist system: new binary, discovery + management | Yes (enforcement integration deferred) |
| 21 | G29 | GeneralManagement on all systems: /general/mgmt/logs + get-config (also core-evol) | Yes |

---

## Step 10 — Structured error responses

**Gaps addressed:**
- **G31** — All systems currently return plain-text error bodies or inconsistent JSON shapes on
  4xx/5xx responses. AH5 defines a canonical `ErrorResponse` object used on every non-2xx response
  across all systems.

### What changes

Add a shared `ErrorResponse` model and a helper function. Update every handler in all seven
systems to return a JSON `ErrorResponse` body instead of `http.Error` plain text.

**AH5 `ErrorResponse` shape:**
```json
{
  "errorMessage": "string — human-readable description",
  "errorCode":    400,
  "exceptionType": "INVALID_PARAMETER",
  "origin":       "serviceregistry.service-discovery.register"
}
```

**`ErrorType` enum** (value of `exceptionType`):
`ARROWHEAD`, `INVALID_PARAMETER`, `AUTH`, `FORBIDDEN`, `DATA_NOT_FOUND`, `TIMEOUT`,
`LOCKED`, `INTERNAL_SERVER_ERROR`, `EXTERNAL_SERVER_ERROR`

**Mapping convention:**
| HTTP status | `exceptionType` |
|---|---|
| 400 | `INVALID_PARAMETER` |
| 401 | `AUTH` |
| 403 | `FORBIDDEN` |
| 404 | `DATA_NOT_FOUND` |
| 405 | `INVALID_PARAMETER` |
| 423 | `LOCKED` |
| 500 | `INTERNAL_SERVER_ERROR` |

**`origin`** format: `<system>.<service>.<operation>` — e.g. `serviceregistry.service-discovery.register`, `authentication.identity.login`.

### TDD cycles

#### Cycle 10.1 — ErrorResponse type and WriteErrorResponse helper

**Write this failing test first** in `core/internal/api/error_test.go` *(new file)*:

```go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"arrowhead/core/internal/api"
)

func TestWriteErrorResponseShape(t *testing.T) {
	w := httptest.NewRecorder()
	api.WriteErrorResponse(w, http.StatusBadRequest, "field missing", "INVALID_PARAMETER", "serviceregistry.service-discovery.register")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body struct {
		ErrorMessage  string `json:"errorMessage"`
		ErrorCode     int    `json:"errorCode"`
		ExceptionType string `json:"exceptionType"`
		Origin        string `json:"origin"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ExceptionType != "INVALID_PARAMETER" {
		t.Errorf("exceptionType = %q, want INVALID_PARAMETER", body.ExceptionType)
	}
	if body.ErrorCode != 400 {
		t.Errorf("errorCode = %d, want 400", body.ErrorCode)
	}
	if body.Origin != "serviceregistry.service-discovery.register" {
		t.Errorf("origin = %q", body.Origin)
	}
}

func TestWriteErrorResponse404(t *testing.T) {
	w := httptest.NewRecorder()
	api.WriteErrorResponse(w, http.StatusNotFound, "not found", "DATA_NOT_FOUND", "serviceregistry.service-discovery.lookup")
	var body struct {
		ExceptionType string `json:"exceptionType"`
	}
	json.NewDecoder(w.Body).Decode(&body)
	if body.ExceptionType != "DATA_NOT_FOUND" {
		t.Errorf("exceptionType = %q, want DATA_NOT_FOUND", body.ExceptionType)
	}
}
```

**Expected failure:** `error_test.go:14: undefined: api.WriteErrorResponse`

**Implementation:** Create `core/internal/api/error.go`:

```go
package api

import "net/http"

// ErrorType constants match the AH5 ErrorResponse.exceptionType enum.
const (
	ErrTypeInvalidParameter    = "INVALID_PARAMETER"
	ErrTypeAuth                = "AUTH"
	ErrTypeForbidden           = "FORBIDDEN"
	ErrTypeDataNotFound        = "DATA_NOT_FOUND"
	ErrTypeLocked              = "LOCKED"
	ErrTypeInternalServerError = "INTERNAL_SERVER_ERROR"
)

// errorTypeForStatus maps HTTP status codes to AH5 exceptionType values.
func errorTypeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusMethodNotAllowed:
		return ErrTypeInvalidParameter
	case http.StatusUnauthorized:
		return ErrTypeAuth
	case http.StatusForbidden:
		return ErrTypeForbidden
	case http.StatusNotFound:
		return ErrTypeDataNotFound
	case http.StatusLocked:
		return ErrTypeLocked
	default:
		return ErrTypeInternalServerError
	}
}

// WriteErrorResponse writes an AH5-conformant ErrorResponse JSON body.
// If exType is empty, it is derived from status using errorTypeForStatus.
func WriteErrorResponse(w http.ResponseWriter, status int, msg, exType, origin string) {
	if exType == "" {
		exType = errorTypeForStatus(status)
	}
	type errBody struct {
		ErrorMessage  string `json:"errorMessage"`
		ErrorCode     int    `json:"errorCode"`
		ExceptionType string `json:"exceptionType"`
		Origin        string `json:"origin"`
	}
	writeJSON(w, status, errBody{
		ErrorMessage:  msg,
		ErrorCode:     status,
		ExceptionType: exType,
		Origin:        origin,
	})
}
```

---

#### Cycle 10.2 — ServiceRegistry handlers return ErrorResponse

**Write this failing test first** in `core/internal/api/handler_test.go` (add to existing file):

```go
func TestHandlerRegisterMissingFieldReturnsExceptionType(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/serviceregistry/register", map[string]any{
		"serviceDefinition": "",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var body struct {
		ExceptionType string `json:"exceptionType"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("response is not JSON: %v — body: %s", err, w.Body.String())
	}
	if body.ExceptionType != "INVALID_PARAMETER" {
		t.Errorf("exceptionType = %q, want INVALID_PARAMETER", body.ExceptionType)
	}
}
```

**Expected failure:** `handler_test.go:NN: response is not JSON: ... — body: {"error":"serviceDefinition required"}`

**Implementation:** In `core/internal/api/handler.go`, replace the local `writeError` function body to call `WriteErrorResponse`:

```go
func writeError(w http.ResponseWriter, status int, msg string) {
	WriteErrorResponse(w, status, msg, "", "serviceregistry")
}
```

Apply the same one-liner replacement to `ah5_handler.go` (origin `"serviceregistry.ah5"`) and every other handler package (each has its own `writeError`).

---

#### Cycle 10.3 — All other handler packages

Add one test to each of the following test files asserting that a 4xx response body decodes to a struct with a non-empty `exceptionType` field. Apply the `writeError` delegation pattern from Cycle 10.2 to each handler:

- `core/internal/authentication/api/handler_test.go` — test: `TestAuthLoginMissingFieldReturnsExceptionType`
- `core/internal/consumerauth/api/handler_test.go` — test: `TestConsumerAuthGrantMissingFieldReturnsExceptionType`
- `core/internal/orchestration/dynamic/api/handler_test.go` — test: `TestDynOrchBadBodyReturnsExceptionType`
- `core/internal/orchestration/simplestore/api/handler_test.go` — test: `TestSimpleStoreRulesMissingFieldReturnsExceptionType`

Pattern for each test (adapt path and handler):

```go
func TestAuthLoginMissingFieldReturnsExceptionType(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := postJSON(t, h, "/authentication/identity/login", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var body struct {
		ExceptionType string `json:"exceptionType"`
	}
	json.NewDecoder(w.Body).Decode(&body)
	if body.ExceptionType == "" {
		t.Errorf("exceptionType is empty — response: %s", w.Body.String())
	}
}
```

**Expected failure for each:** `exceptionType is empty — response: {"error":"systemName required"}`

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
    ./internal/api/... \
    ./internal/authentication/api/... \
    ./internal/consumerauth/api/... \
    ./internal/orchestration/dynamic/api/... \
    ./internal/orchestration/simplestore/api/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all modified handler packages.

### Files to modify / create

- `core/internal/model/ah5_types.go` — add `ErrorResponse`, `ErrorType` constants
- `core/internal/api/error.go` *(new)* — `WriteErrorResponse` helper
- `core/internal/api/handler.go` — replace `http.Error` calls
- `core/internal/api/ah5_handler.go` — replace `http.Error` calls
- `core/internal/authentication/api/handler.go`
- `core/internal/consumerauth/api/handler.go`
- `core/internal/orchestration/dynamic/api/handler.go`
- `core/internal/orchestration/simplestore/api/handler.go`
- `core/internal/orchestration/flexiblestore/api/handler.go`
- `core/internal/ca/api/handler.go`
- `core/SPEC.md` — add ErrorResponse section

### System test

```bash
bash core/test-system.sh
```

No Docker needed. No experiment code changes.

### Completion criteria

- [x] `ErrorResponse` type is defined with all `ErrorType` constants
- [x] `WriteErrorResponse` helper sets `Content-Type: application/json`
- [x] Every handler test that previously tested an error response now also asserts `exceptionType` field
- [x] No handler uses `http.Error` or `fmt.Fprintf(w, ...)` for error responses
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes

---

## Step 11 — Pagination on all query and list endpoints

**Gaps addressed:**
- **G20** — AH5 specifies a `PageRequest` object on all query and management list operations.
  The implementation returns the full in-memory/SQLite collection on every call.

### What changes

Add a shared `PageRequest` type and a `Paginate[T]` generic helper. Apply both to every
endpoint that returns a list. The AH5 canonical field names are `pageNumber`, `pageSize`,
`pageSortField`, `pageDirection` (from the data-model page); some API pages show `page`,
`size`, `direction`, `sortField`. Accept both naming styles by checking both names
during JSON decode; return the data-model names in responses.

**Pagination inputs:**
```json
{
  "pageNumber":   0,
  "pageSize":     20,
  "pageSortField": "createdAt",
  "pageDirection": "ASC"
}
```
Both `pageNumber` and `pageSize` must be present together or both absent. If absent, the
full collection is returned (backward-compatible). `pageDirection` values: `ASC`, `DESC`.

**Pagination wrapper for all list responses** — add `totalCount` alongside the existing `count`:
```json
{
  "entries": [...],
  "count":      10,
  "totalCount": 243
}
```
`count` = entries in this page; `totalCount` = total matching records before pagination.

### TDD cycles

#### Cycle 11.1 — PageRequest type and Paginate helper

**Write this failing test first** in `core/internal/model/paginate_test.go` *(new file)*:

```go
package model_test

import (
	"testing"

	"arrowhead/core/internal/model"
)

func TestPaginateZeroRequestReturnsAll(t *testing.T) {
	items := []string{"c", "a", "b"}
	got, total := model.Paginate(items, model.PageRequest{}, func(s string) string { return s })
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3", len(got))
	}
}

func TestPaginateFirstPage(t *testing.T) {
	items := []string{"e", "d", "c", "b", "a"}
	got, total := model.Paginate(items, model.PageRequest{PageNumber: 0, PageSize: 2, PageDirection: "ASC"}, func(s string) string { return s })
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(got) != 2 {
		t.Errorf("page len = %d, want 2", len(got))
	}
	if got[0] != "a" || got[1] != "b" {
		t.Errorf("first page = %v, want [a b]", got)
	}
}

func TestPaginateLastPage(t *testing.T) {
	items := []string{"e", "d", "c", "b", "a"}
	got, total := model.Paginate(items, model.PageRequest{PageNumber: 2, PageSize: 2, PageDirection: "ASC"}, func(s string) string { return s })
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(got) != 1 {
		t.Errorf("last page len = %d, want 1", len(got))
	}
	if got[0] != "e" {
		t.Errorf("last page[0] = %q, want e", got[0])
	}
}

func TestPaginateBeyondEnd(t *testing.T) {
	items := []string{"a", "b"}
	got, total := model.Paginate(items, model.PageRequest{PageNumber: 5, PageSize: 2, PageDirection: "ASC"}, func(s string) string { return s })
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(got) != 0 {
		t.Errorf("beyond-end page len = %d, want 0", len(got))
	}
}

func TestPaginateSortDESC(t *testing.T) {
	items := []string{"a", "b", "c"}
	got, _ := model.Paginate(items, model.PageRequest{PageNumber: 0, PageSize: 2, PageDirection: "DESC"}, func(s string) string { return s })
	if got[0] != "c" || got[1] != "b" {
		t.Errorf("DESC page = %v, want [c b]", got)
	}
}
```

**Expected failure:** `paginate_test.go:12: undefined: model.Paginate` and `undefined: model.PageRequest`

**Implementation:** Create `core/internal/model/paginate.go`:

```go
package model

import "sort"

// PageRequest carries AH5 pagination parameters.
// A zero value (PageSize == 0) means "return the full collection".
type PageRequest struct {
	PageNumber    int    `json:"pageNumber"`
	PageSize      int    `json:"pageSize"`
	PageSortField string `json:"pageSortField"`
	PageDirection string `json:"pageDirection"` // "ASC" | "DESC"
}

// Paginate sorts items by sortKey and returns the requested page together with
// the total pre-pagination count.  A zero PageRequest returns the full collection.
func Paginate[T any](items []T, req PageRequest, sortKey func(T) string) ([]T, int) {
	total := len(items)
	if total == 0 {
		return items, 0
	}
	desc := req.PageDirection == "DESC"
	sort.SliceStable(items, func(i, j int) bool {
		ki, kj := sortKey(items[i]), sortKey(items[j])
		if desc {
			return ki > kj
		}
		return ki < kj
	})
	if req.PageSize <= 0 {
		return items, total
	}
	start := req.PageNumber * req.PageSize
	if start >= total {
		return []T{}, total
	}
	end := start + req.PageSize
	if end > total {
		end = total
	}
	return items[start:end], total
}
```

---

#### Cycle 11.2 — AH5 ServiceRegistry query endpoints return totalCount

**Write this failing test first** in `core/internal/api/ah5_handler_test.go` (add to existing file):

```go
func TestAH5DeviceQueryPagination(t *testing.T) {
	h := newAH5Handler()
	// Seed 5 devices.
	for i := 0; i < 5; i++ {
		ah5Post(t, h, "/serviceregistry/device-discovery/register",
			map[string]any{"name": fmt.Sprintf("dev%d", i)})
	}
	// Query page 0, size 2.
	w := ah5Post(t, h, "/serviceregistry/device-discovery/query", map[string]any{
		"pagination": map[string]any{"pageNumber": 0, "pageSize": 2, "pageDirection": "ASC"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count      int `json:"count"`
		TotalCount int `json:"totalCount"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2", resp.Count)
	}
	if resp.TotalCount != 5 {
		t.Errorf("totalCount = %d, want 5", resp.TotalCount)
	}
}
```

Add `"fmt"` to imports if not already present.

**Expected failure:** `ah5_handler_test.go:NN: totalCount = 0, want 5` (field missing before implementation)

**Implementation:** In `core/internal/api/ah5_handler.go`, update each list/query handler to:
1. Decode an optional `"pagination"` key from the request body into a `model.PageRequest`
2. Call `model.Paginate(items, pageReq, sortKeyFn)` to get `(page, total)`
3. Return `{"entries": page, "count": len(page), "totalCount": total}` instead of `{"entries": items, "count": len(items)}`

When the request body contains no `"pagination"` key, `PageRequest` is zero-valued, and `Paginate` returns the full collection (backward-compatible).

---

#### Cycle 11.3 — Backward compat: no pagination field returns full collection

**Write this failing test first** in `core/internal/api/ah5_handler_test.go`:

```go
func TestAH5DeviceQueryNoPaginationReturnsAll(t *testing.T) {
	h := newAH5Handler()
	for i := 0; i < 3; i++ {
		ah5Post(t, h, "/serviceregistry/device-discovery/register",
			map[string]any{"name": fmt.Sprintf("d%d", i)})
	}
	w := ah5Post(t, h, "/serviceregistry/device-discovery/query", map[string]any{})
	var resp struct {
		Count      int `json:"count"`
		TotalCount int `json:"totalCount"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 3 {
		t.Errorf("count = %d, want 3", resp.Count)
	}
	if resp.TotalCount != 3 {
		t.Errorf("totalCount = %d, want 3", resp.TotalCount)
	}
}
```

**Expected failure:** `totalCount = 0, want 3` (passes automatically after Cycle 11.2 implementation since zero `PageRequest` returns full collection)

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/model/... ./internal/api/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `model` and `api` packages.

### Files to modify / create

- `core/internal/model/ah5_types.go` — add `PageRequest`, `Direction` constant
- `core/internal/model/paginate.go` *(new)* — `Paginate[T]` generic helper + tests
- `core/internal/api/ah5_handler.go` — read PageRequest, call Paginate, return totalCount
- All handler files that return list responses (ConsumerAuth, Orchestration — see respective steps)

### System test

```bash
bash core/test-system.sh
```

No Docker needed.

### Completion criteria

- [x] `Paginate[T]` passes all six unit tests
- [x] All AH5 ServiceRegistry query handlers accept `pagination` object and return `totalCount`
- [x] Requests without `pagination` field return the full collection (backward-compatible)
- [x] `count` = page size; `totalCount` = full matching set size
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes

---

## Step 12 — Authentication API completions

**Gaps addressed:**
- **G8** — `DeleteExpired()` exists but is never called; expired tokens accumulate indefinitely.
- **G15 (remaining)** — Authentication verify response is missing `expirationTime` and `sysop`
  fields. Step 7 added `verified`, `systemName`, `loginTime`; the full AH5 shape requires all five.
- **G33** — `POST /authentication/identity/change` endpoint is absent. AH5 defines it for
  in-session credential rotation.

### What changes

Three small, independent additions to `internal/authentication/`:

1. **Verify response fields** — add `expirationTime` and `sysop` to `IdentityVerifyResponse`.
   `sysop` is always `false` until Step 13 introduces the identity store.
   `expirationTime` is copied from the stored token's `ExpiresAt`.

2. **Token cleanup goroutine** — start a background goroutine in `NewAuthService` that calls
   `repo.DeleteExpired()` every `cleanupInterval` (configurable, default 5 min). Expose
   `cleanupInterval` as a constructor parameter.

3. **Change endpoint** — `POST /authentication/identity/change` accepts
   `{"systemName": "...", "credentials": {...}, "newCredentials": {...}}`. Because credential
   verification is still stubbed (G2), this endpoint validates that the system has an active
   session (token exists) and then accepts the credential update unconditionally. Returns 200
   on success, 401 if no active session exists.

### TDD cycles

#### Cycle 12.1 — Verify response includes expirationTime and sysop

**Write this failing test first** in `core/internal/authentication/api/handler_test.go`:

```go
func TestVerifyResponseIncludesExpirationTime(t *testing.T) {
	h := newTestHandler(time.Hour)
	token := loginAndGetToken(t, h, "sys-a")
	w := bearerRequest(t, h, http.MethodGet, "/authentication/identity/verify/"+token, "")
	if w.Code != http.StatusOK {
		t.Fatalf("verify failed: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		ExpirationTime string `json:"expirationTime"`
		Sysop          *bool  `json:"sysop"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ExpirationTime == "" {
		t.Error("expirationTime is empty")
	}
	if resp.Sysop == nil {
		t.Error("sysop field is absent")
	}
	if *resp.Sysop != false {
		t.Errorf("sysop = %v, want false", *resp.Sysop)
	}
}
```

**Expected failure:** `expirationTime is empty` (the current `VerifyResponse` has `expiresAt` not `expirationTime`, and lacks `sysop`)

**Implementation:**
- In `core/internal/authentication/model/types.go`, add `ExpirationTime string` and `Sysop bool` to `VerifyResponse`; remove or alias `ExpiresAt`.
- In `core/internal/authentication/service/auth.go`, populate `ExpirationTime: token.ExpiresAt.Format(time.RFC3339)` and `Sysop: false` in the `Verify` method.

---

#### Cycle 12.2 — Token cleanup goroutine

**Write this failing test first** in `core/internal/authentication/service/auth_test.go`:

```go
func TestDeleteExpiredCalledOnCleanup(t *testing.T) {
	repo := repository.NewMemoryRepository()
	// Use NewAuthServiceWithCleanup with a 10ms interval.
	svc := service.NewAuthServiceWithCleanup(repo, time.Millisecond, 10*time.Millisecond)
	// Log in and immediately expire the token.
	resp, _ := svc.Login(model.LoginRequest{SystemName: "expiry-test"})
	// Wait for cleanup to fire.
	time.Sleep(50 * time.Millisecond)
	// Verify the token is gone.
	_, err := svc.Verify(resp.Token)
	if err == nil {
		t.Error("expected expired token to be gone after cleanup")
	}
}
```

**Expected failure:** `auth_test.go:NN: undefined: service.NewAuthServiceWithCleanup`

**Implementation:**
- Add `NewAuthServiceWithCleanup(repo, tokenDuration, cleanupInterval time.Duration) *AuthService`.
- Start a goroutine inside it: `go func() { for range time.Tick(cleanupInterval) { repo.DeleteExpired() } }()`.
- Keep existing `NewAuthService` calling `NewAuthServiceWithCleanup` with a 5-minute default.

---

#### Cycle 12.3 — Change endpoint

**Write this failing test first** in `core/internal/authentication/api/handler_test.go`:

```go
func TestChangeCredentials200(t *testing.T) {
	h := newTestHandler(time.Hour)
	loginAndGetToken(t, h, "sys-b")
	w := postJSON(t, h, "/authentication/identity/change", map[string]any{
		"systemName":     "sys-b",
		"credentials":    map[string]string{"password": "old"},
		"newCredentials": map[string]string{"password": "new"},
	})
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChangeCredentials401NoSession(t *testing.T) {
	h := newTestHandler(time.Hour)
	w := postJSON(t, h, "/authentication/identity/change", map[string]any{
		"systemName":     "nobody",
		"credentials":    map[string]string{"password": "x"},
		"newCredentials": map[string]string{"password": "y"},
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
```

**Expected failure:** `handler_test.go:NN: expected 200, got 404` (route does not exist)

**Implementation:** Add `POST /authentication/identity/change` route in `core/internal/authentication/api/handler.go`. Check that a live token exists for `systemName`; return 401 if none; return 200 (no body) on success.

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
    ./internal/authentication/api/... \
    ./internal/authentication/service/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on both packages.

### Files to modify / create

- `core/internal/authentication/service/auth.go` — add `expirationTime`/`sysop` to verify
  response; add `NewAuthServiceWithCleanup`
- `core/internal/authentication/api/handler.go` — add `/authentication/identity/change` route
- `core/internal/authentication/api/handler_test.go` — all three test cycles
- `core/SPEC.md` — add `change` endpoint; update verify response shape
- `core/GAP_ANALYSIS.md` — mark G8 resolved; mark G15 fully resolved

### System test

```bash
bash core/test-system.sh
```

No Docker needed. The verify response change is additive (new fields); existing consumers
that ignore unknown fields are unaffected. Experiment code that parses the verify response
should be checked (grep `expiresAt\|expirationTime` in experiment Go files).

### Completion criteria

- [x] Verify response contains `expirationTime` (RFC3339) and `sysop: false`
- [x] Cleanup goroutine calls `DeleteExpired` on the configured interval
- [x] `POST /authentication/identity/change` returns 200 for a live-session system, 401 otherwise
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes

---

## Step 13 — Authentication management and credential verification

**Gaps addressed:**
- **G21** — Authentication management endpoints (`/authentication/mgmt/identities/*`,
  `/authentication/mgmt/sessions/*`) are absent. There is no mechanism to pre-register
  systems with credentials.
- **G2** — `POST /authentication/identity/login` accepts any non-empty systemName and issues
  a token without verifying credentials.

### What changes

This is the largest step in the second round. It adds a full identity store and wires
credential verification into the login flow.

**New identity store model:**
```json
{
  "systemName":          "string",
  "credentials":         {"password": "string"},
  "authenticationMethod": "PASSWORD",
  "sysop":               false,
  "createdBy":           "string",
  "createdAt":           "DateTime",
  "updatedAt":           "DateTime"
}
```

Credentials are stored as bcrypt hashes (Go `golang.org/x/crypto/bcrypt`, cost 12).
The `password` key of the `Credentials` map is the only supported credential field
(per AH5 `AuthenticationMethod: PASSWORD`).

**New endpoints:**
```
POST   /authentication/mgmt/identities/query   — list identities (paginated)
POST   /authentication/mgmt/identities          — create identities (bulk)
PUT    /authentication/mgmt/identities          — update credentials (bulk)
DELETE /authentication/mgmt/identities          — remove identities (?names=...)
POST   /authentication/mgmt/sessions            — query active sessions (paginated)
DELETE /authentication/mgmt/sessions            — revoke sessions (?names=...)
```

**Management response format note:** AH5 uses `identities` (not `entries`) for the list
field in identity responses, and `sessions` (not `entries`) for session responses.

**Login change:** After this step, `POST /authentication/identity/login` checks the password
against the stored bcrypt hash. If no identity record exists for the systemName, login
returns `401`. The `sysop` flag from the identity record is used in verify responses.

**Bootstrapping:** When the identity store is empty, a built-in `Sysop` identity is
auto-created at startup (password configurable via `SYSOP_PASSWORD` env var, default
`arrowhead`). This allows the management endpoints to be reached before any identities
are seeded.

**SQLite extension:** Add identity table to the authentication SQLite store created in Step 9.

### TDD cycles

#### Cycle 13.1 — Identity repository and memory implementation

**Write this failing test first** in `core/internal/authentication/repository/identity_memory_test.go` *(new file)*:

```go
package repository_test

import (
	"testing"

	"arrowhead/core/internal/authentication/repository"
)

func TestIdentityMemorySaveAndGet(t *testing.T) {
	repo := repository.NewMemoryIdentityRepository()
	repo.Save(repository.Identity{SystemName: "sys-a", PasswordHash: "hash1", Sysop: false})
	got, ok := repo.Get("sys-a")
	if !ok {
		t.Fatal("Get returned false after Save")
	}
	if got.SystemName != "sys-a" {
		t.Errorf("SystemName = %q", got.SystemName)
	}
}

func TestIdentityMemoryDelete(t *testing.T) {
	repo := repository.NewMemoryIdentityRepository()
	repo.Save(repository.Identity{SystemName: "to-delete"})
	repo.Delete("to-delete")
	_, ok := repo.Get("to-delete")
	if ok {
		t.Error("Get returned true after Delete")
	}
}

func TestIdentityMemoryAll(t *testing.T) {
	repo := repository.NewMemoryIdentityRepository()
	repo.Save(repository.Identity{SystemName: "a"})
	repo.Save(repository.Identity{SystemName: "b"})
	all := repo.All()
	if len(all) != 2 {
		t.Errorf("All() len = %d, want 2", len(all))
	}
}
```

**Expected failure:** `identity_memory_test.go:10: undefined: repository.NewMemoryIdentityRepository` and `undefined: repository.Identity`

**Implementation:** Create `core/internal/authentication/repository/identity_memory.go` with `Identity` struct, `IdentityRepository` interface (`Save`, `Get`, `Delete`, `All`), and `MemoryIdentityRepository`.

---

#### Cycle 13.2 — Management endpoints

**Write this failing test first** in `core/internal/authentication/api/handler_test.go`:

```go
func TestMgmtCreateIdentities201(t *testing.T) {
	h := newTestHandlerFull(time.Hour) // new constructor — see implementation note
	w := postJSON(t, h, "/authentication/mgmt/identities", map[string]any{
		"authenticationMethod": "PASSWORD",
		"identities": []map[string]any{
			{"systemName": "robot-1", "credentials": map[string]string{"password": "secret"}, "sysop": false},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Identities []map[string]any `json:"identities"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Identities) != 1 {
		t.Errorf("identities len = %d, want 1", len(resp.Identities))
	}
}

func TestMgmtQueryIdentities200(t *testing.T) {
	h := newTestHandlerFull(time.Hour)
	postJSON(t, h, "/authentication/mgmt/identities", map[string]any{
		"authenticationMethod": "PASSWORD",
		"identities": []map[string]any{
			{"systemName": "q-sys", "credentials": map[string]string{"password": "pw"}},
		},
	})
	w := postJSON(t, h, "/authentication/mgmt/identities/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Identities []map[string]any `json:"identities"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Identities) < 1 {
		t.Errorf("no identities in query response")
	}
}
```

**Expected failure:** `handler_test.go:NN: undefined: newTestHandlerFull` and `expected 201, got 404`

**Implementation:** Add `newTestHandlerFull` test helper that passes a `MemoryIdentityRepository` to the handler (or `NewAuthServiceFull`). Add the six management routes to `handler.go`.

---

#### Cycle 13.3 — Login with credential verification

**Write this failing test first** in `core/internal/authentication/api/handler_test.go`:

```go
func TestLoginWrongPassword401(t *testing.T) {
	h := newTestHandlerFull(time.Hour)
	// First create identity.
	postJSON(t, h, "/authentication/mgmt/identities", map[string]any{
		"authenticationMethod": "PASSWORD",
		"identities": []map[string]any{
			{"systemName": "guarded", "credentials": map[string]string{"password": "correct"}},
		},
	})
	w := postJSON(t, h, "/authentication/identity/login", map[string]any{
		"systemName":  "guarded",
		"credentials": map[string]string{"password": "wrong"},
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLoginUnknownSystem401(t *testing.T) {
	h := newTestHandlerFull(time.Hour)
	w := postJSON(t, h, "/authentication/identity/login", map[string]any{
		"systemName":  "ghost",
		"credentials": map[string]string{"password": "x"},
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBootstrapSysopIdentity(t *testing.T) {
	h := newTestHandlerFull(time.Hour)
	// Sysop identity must exist from bootstrap.
	w := postJSON(t, h, "/authentication/identity/login", map[string]any{
		"systemName":  "Sysop",
		"credentials": map[string]string{"password": "arrowhead"},
	})
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 for Sysop login, got %d: %s", w.Code, w.Body.String())
	}
}
```

**Expected failure:** `expected 401, got 201` (login currently accepts any systemName)

**Implementation:**
- Add `NewAuthServiceFull(tokenRepo, identityRepo, tokenDuration)` constructor.
- In `Login`, call `identityRepo.Get(systemName)` — return `ErrMissingSystemName` (→ 401) if not found.
- Use `bcrypt.CompareHashAndPassword` to verify; return 401 on mismatch.
- Bootstrap: if `identityRepo.All()` is empty, call `identityRepo.Save(sysopIdentity)`.

> **⚠ Regression check after Cycle 13.3**
> Implementing credential verification makes `Login` reject any system that has no identity record.
> All existing tests that call `loginAndGetToken` through the plain `newTestHandler` (which uses
> `NewAuthService`, not `NewAuthServiceFull`) continue to work because `NewAuthService` keeps the
> old unauthenticated behaviour. Tests that use `newTestHandlerFull` must pre-seed an identity
> or rely on the Sysop bootstrap. Run `go test -race ./internal/authentication/...` after this
> cycle and fix any test that unexpectedly starts returning 401.

---

#### Cycle 13.4 — Sysop flag in verify response

**Write this failing test first** in `core/internal/authentication/api/handler_test.go`:

```go
func TestVerifySysopTrue(t *testing.T) {
	h := newTestHandlerFull(time.Hour)
	// Log in as Sysop (bootstrap identity).
	w := postJSON(t, h, "/authentication/identity/login", map[string]any{
		"systemName":  "Sysop",
		"credentials": map[string]string{"password": "arrowhead"},
	})
	var loginResp struct{ Token string `json:"token"` }
	json.NewDecoder(w.Body).Decode(&loginResp)

	w2 := bearerRequest(t, h, http.MethodGet, "/authentication/identity/verify/"+loginResp.Token, "")
	var verifyResp struct{ Sysop bool `json:"sysop"` }
	json.NewDecoder(w2.Body).Decode(&verifyResp)
	if !verifyResp.Sysop {
		t.Error("sysop = false for Sysop identity, want true")
	}
}
```

**Expected failure:** `sysop = false for Sysop identity, want true`

**Implementation:** In `Verify`, look up the identity record and copy its `Sysop` flag into `VerifyResponse.Sysop`.

---

#### Cycle 13.5 — SQLite identity table

**Write this failing test first** in `core/internal/authentication/repository/identity_sqlite_test.go` *(new file)*:

```go
package repository_test

import (
	"os"
	"testing"

	"arrowhead/core/internal/authentication/repository"
)

func TestSQLiteIdentitySaveAndGet(t *testing.T) {
	f, _ := os.CreateTemp("", "auth-identity-*.db")
	f.Close()
	defer os.Remove(f.Name())

	repo, err := repository.NewSQLiteIdentityRepository(f.Name())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo.Save(repository.Identity{SystemName: "sql-sys", PasswordHash: "h", Sysop: true})
	got, ok := repo.Get("sql-sys")
	if !ok {
		t.Fatal("not found after save")
	}
	if !got.Sysop {
		t.Error("Sysop = false, want true")
	}
}
```

**Expected failure:** `identity_sqlite_test.go:NN: undefined: repository.NewSQLiteIdentityRepository`

**Implementation:** Create `core/internal/authentication/repository/identity_sqlite.go` with a `CREATE TABLE IF NOT EXISTS identities (system_name TEXT PRIMARY KEY, password_hash TEXT, sysop INTEGER, ...)` schema.

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
    ./internal/authentication/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all `authentication` subpackages.

### Files to modify / create

- `core/internal/authentication/repository/identity_memory.go` *(new)*
- `core/internal/authentication/repository/identity_sqlite.go` *(new)*
- `core/internal/authentication/repository/identity_interface.go` *(new)* — `IdentityRepository`
- `core/internal/authentication/service/auth.go` — wire identity store into login
- `core/internal/authentication/api/handler.go` — six new mgmt routes
- `core/internal/authentication/api/handler_test.go` — all five cycles
- `cmd/authentication/main.go` — pass identity repo to service, read `SYSOP_PASSWORD` env
- `core/SPEC.md` — add mgmt endpoints; update login behavior
- `core/GAP_ANALYSIS.md` — mark G2, G21 resolved

### System test

```bash
bash core/test-system.sh
```

**Docker impact:** Experiments that call `POST /authentication/identity/login` will now
fail if no identity exists. Each experiment's `setup` seed container must first create
an identity for every system that logs in. Update docker-compose.yml seed commands for
experiments 7, 9, 13, 14 (any experiment with `AUTH_URL` in its services).

Seed pattern to add before any login call:
```bash
curl -s -X POST http://authentication:8081/authentication/mgmt/identities \
  -H 'Content-Type: application/json' \
  -d '{"authenticationMethod":"PASSWORD","identities":[{"systemName":"robot-fleet-site-1","credentials":{"password":"fleet-secret"},"sysop":false}]}'
```

```bash
bash experiments/experiment-9/test-system.sh
bash experiments/experiment-13/test-system.sh
bash experiments/experiment-14/test-system.sh
```

### Completion criteria

- [x] All six mgmt endpoints respond correctly with correct response field names (`identities`, `sessions`)
- [x] Login rejects unknown systemNames with 401
- [x] Login rejects wrong passwords with 401
- [x] Login succeeds with correct bcrypt-matched credentials
- [x] Sysop bootstrap identity is created on empty-store startup
- [x] `sysop` field in verify response reflects the identity record
- [x] SQLite identity table is created and used when `DB_PATH` is set
- [x] Seed containers in all affected experiments updated
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes

---

## Step 14 — ConsumerAuthorization policy model alignment

**Gaps addressed:**
- **G22** — ConsumerAuthorization uses simple consumer–provider–serviceDefinition tuples.
  AH5 uses a provider-centric model with `AuthorizationPolicyType` (`ALL`, `WHITELIST`,
  `BLACKLIST`, `SYS_METADATA`), a composite `instanceId`, a `targetType` field
  (`SERVICE_DEF` or `EVENT_TYPE`), and per-operation `scopedPolicies`.

### What changes

This step replaces the current ConsumerAuth data model and all five endpoints. It is a
**breaking change** to the wire format. A migration path for experiments must be planned
before implementation.

**New grant request shape:**
```json
{
  "targetType":     "SERVICE_DEF",
  "target":         "temperatureService",
  "description":    "optional free text",
  "defaultPolicy":  { "policyType": "WHITELIST", "policyList": ["ConsumerApp"] },
  "scopedPolicies": { "read": { "policyType": "ALL" } }
}
```

**New grant response (and lookup entry) shape:**
```json
{
  "instanceId":        "PR|LOCAL|TemperatureProvider|SERVICE_DEF|temperatureService",
  "authorizationLevel": "PR",
  "cloud":             "LOCAL",
  "provider":          "TemperatureProvider",
  "targetType":        "SERVICE_DEF",
  "target":            "temperatureService",
  "description":       "optional free text",
  "defaultPolicy":     { "policyType": "WHITELIST", "policyList": ["ConsumerApp"] },
  "scopedPolicies":    {},
  "createdBy":         "TemperatureProvider",
  "createdAt":         "2024-01-01T00:00:00Z"
}
```

**`instanceId` construction:** `PR|LOCAL|<ProviderName>|<targetType>|<target>` (all fields
joined with `|`; pipe chars in URLs must be percent-encoded as `%7C`).

**Verify response:** AH5 specifies a plain Boolean body (`true` or `false` as JSON primitive,
not wrapped in an object). The current implementation returns `{"authorized": bool, "ruleId": int64}`.
This is a **breaking change** for any consumer that parses the verify response (check all experiment
Go services and test-system.sh scripts).

**New lookup request shape:**
```json
{
  "instanceIds":       ["PR|LOCAL|Provider|SERVICE_DEF|svc"],
  "cloudIdentifiers":  ["LOCAL"],
  "targetNames":       ["temperatureService"],
  "targetType":        "SERVICE_DEF"
}
```
At least one of `instanceIds`, `cloudIdentifiers`, `targetNames` must be provided.

**New management endpoints:**
```
POST   /consumerauthorization/authorization/mgmt/grant    — bulk grant
DELETE /consumerauthorization/authorization/mgmt/revoke   — bulk revoke (?instanceIds=...)
POST   /consumerauthorization/authorization/mgmt/query    — paginated policy query
POST   /consumerauthorization/authorization/mgmt/check    — bulk verify
```

### TDD cycles

#### Cycle 14.1 — New model types and instanceId construction

**Write this failing test first** in `core/internal/consumerauth/model/types_test.go` *(new file)*:

```go
package model_test

import (
	"testing"

	"arrowhead/core/internal/consumerauth/model"
)

func TestBuildInstanceID(t *testing.T) {
	id := model.BuildInstanceID("TemperatureProvider", "SERVICE_DEF", "temperatureService")
	want := "PR|LOCAL|TemperatureProvider|SERVICE_DEF|temperatureService"
	if id != want {
		t.Errorf("instanceId = %q, want %q", id, want)
	}
}

func TestInstanceIDURLEncoding(t *testing.T) {
	id := model.BuildInstanceID("Provider|With|Pipes", "SERVICE_DEF", "svc")
	// Pipe chars in URLs must be percent-encoded.
	encoded := model.EncodeInstanceID(id)
	if encoded == id {
		t.Error("URL-encoded instanceId should differ from raw when pipes are present")
	}
}
```

**Expected failure:** `types_test.go:NN: undefined: model.BuildInstanceID`

**Implementation:** Create `core/internal/consumerauth/model/types.go` with:
- `BuildInstanceID(provider, targetType, target string) string` returning `"PR|LOCAL|" + provider + "|" + targetType + "|" + target`
- `EncodeInstanceID(id string) string` using `url.QueryEscape` or manual `%7C` replacement
- `AuthorizationPolicyType` constants: `PolicyAll = "ALL"`, `PolicyWhitelist = "WHITELIST"`, `PolicyBlacklist = "BLACKLIST"`
- `TargetType` constants: `TargetServiceDef = "SERVICE_DEF"`, `TargetEventType = "EVENT_TYPE"`

---

#### Cycle 14.2 — Grant and revoke with instanceId

**Write this failing test first** in `core/internal/consumerauth/api/handler_test.go`:

```go
func TestGrantCreatesInstanceID(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType":    "SERVICE_DEF",
		"target":        "temperatureService",
		"defaultPolicy": map[string]any{"policyType": "WHITELIST", "policyList": []string{"ConsumerApp"}},
		"provider":      "TemperatureProvider",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		InstanceID string `json:"instanceId"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.InstanceID == "" {
		t.Error("instanceId is empty")
	}
	want := "PR|LOCAL|TemperatureProvider|SERVICE_DEF|temperatureService"
	if resp.InstanceID != want {
		t.Errorf("instanceId = %q, want %q", resp.InstanceID, want)
	}
}

func TestRevokeByInstanceID(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType": "SERVICE_DEF", "target": "svc", "provider": "P1",
		"defaultPolicy": map[string]any{"policyType": "ALL"},
	})
	var resp struct{ InstanceID string `json:"instanceId"` }
	json.NewDecoder(w.Body).Decode(&resp)

	del := deleteReq(t, h, "/consumerauthorization/authorization/revoke/"+resp.InstanceID)
	if del.Code != http.StatusOK {
		t.Errorf("revoke: expected 200, got %d", del.Code)
	}
}
```

**Expected failure:** `handler_test.go:NN: expected 201, got 400` (old grant handler expects `consumerSystemName`/`providerSystemName` shape)

**Implementation:** Replace the grant/revoke handler to accept the new request shape. Update repository interface and memory store accordingly.

> **⚠ Regression check after Cycle 14.2**
> This cycle replaces the grant/revoke request shape entirely — the old fields
> `consumerSystemName` and `providerSystemName` are removed. All existing tests in
> `handler_test.go` that send the old shape (`validGrantBody`, `grantAndGetID`, etc.) will
> now fail. **Before implementing Cycle 14.2**, update those existing tests to use the new
> shape (or delete them if superseded by the new tests). Run `go test -race ./internal/consumerauth/...`
> after each edit to the test file and confirm only the new behaviour tests fail, not
> compilation errors.

---

#### Cycle 14.3 — Lookup with at-least-one-filter validation

**Write this failing test first** in `core/internal/consumerauth/api/handler_test.go`:

```go
func TestLookupRequiresAtLeastOneFilter(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization/lookup", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLookupByTargetName(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType": "SERVICE_DEF", "target": "svc-x", "provider": "ProvX",
		"defaultPolicy": map[string]any{"policyType": "ALL"},
	})
	w := postJSON(t, h, "/consumerauthorization/authorization/lookup", map[string]any{
		"targetNames": []string{"svc-x"},
		"targetType":  "SERVICE_DEF",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
```

**Expected failure:** `handler_test.go:NN: expected 400, got 200` (current lookup accepts empty filters)

---

#### Cycle 14.4 — Verify returns plain JSON Boolean

**Write this failing test first** in `core/internal/consumerauth/api/handler_test.go`:

```go
func TestVerifyReturnsTruePlainBoolean(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/consumerauthorization/authorization/grant", map[string]any{
		"targetType": "SERVICE_DEF", "target": "svc", "provider": "P",
		"defaultPolicy": map[string]any{"policyType": "WHITELIST", "policyList": []string{"Consumer1"}},
	})
	w := postJSON(t, h, "/consumerauthorization/authorization/verify", map[string]any{
		"consumer": "Consumer1", "target": "svc", "targetType": "SERVICE_DEF",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := strings.TrimSpace(w.Body.String())
	if body != "true" {
		t.Errorf("body = %q, want plain JSON true", body)
	}
}

func TestVerifyReturnsFalsePlainBoolean(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization/verify", map[string]any{
		"consumer": "Nobody", "target": "svc", "targetType": "SERVICE_DEF",
	})
	body := strings.TrimSpace(w.Body.String())
	if body != "false" {
		t.Errorf("body = %q, want plain JSON false", body)
	}
}
```

**Expected failure:** `body = "{\"authorized\":true,\"ruleId\":1}", want plain JSON true` (current handler wraps the boolean)

**Implementation:** Replace `handleVerify` to use `json.NewEncoder(w).Encode(authorized)` where `authorized` is a plain `bool`. Add `"strings"` to imports in the test file.

---

#### Cycle 14.5 — Management endpoints

**Write this failing test first** in `core/internal/consumerauth/api/handler_test.go`:

```go
func TestMgmtGrantAndQuery(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization/mgmt/grant", map[string]any{
		"targetType": "SERVICE_DEF", "target": "mgmt-svc", "provider": "MgmtProv",
		"defaultPolicy": map[string]any{"policyType": "ALL"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("mgmt/grant: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	qw := postJSON(t, h, "/consumerauthorization/authorization/mgmt/query", map[string]any{})
	if qw.Code != http.StatusOK {
		t.Fatalf("mgmt/query: expected 200, got %d", qw.Code)
	}
}
```

**Expected failure:** `mgmt/grant: expected 201, got 404`

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/consumerauth/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all `consumerauth` subpackages.

### Files to modify / create

- `core/internal/consumerauth/model/` *(new)* — or extend `ah5_types.go`
- `core/internal/consumerauth/repository/memory.go` — replace tuple model
- `core/internal/consumerauth/repository/sqlite.go` — update schema
- `core/internal/consumerauth/service/auth.go` — policy evaluation logic
- `core/internal/consumerauth/api/handler.go` — all endpoints updated
- `cmd/consumerauth/main.go` — no structural change
- `core/SPEC.md` — full ConsumerAuth section rewrite
- `core/GAP_ANALYSIS.md` — mark G22 resolved
- Experiment docker-compose seed commands — `grant` request shape changes
- `support/policy-sync/sync.go` — lookup response parsing changes
- `support/topic-auth-http/sync.go` — same

### System test

**This step breaks all experiment stacks that call ConsumerAuth.** Update seed curl
commands in every affected docker-compose.yml and test-system.sh before running Docker tests.

```bash
bash core/test-system.sh
bash experiments/experiment-9/test-system.sh
bash experiments/experiment-13/test-system.sh
bash experiments/experiment-14/test-system.sh
```

### Completion criteria

- [x] Grant response contains `instanceId` in composite format
- [x] Revoke accepts composite `instanceId` as path parameter (URL-encoded)
- [x] Lookup requires at least one filter; returns 400 otherwise
- [x] Verify returns plain JSON Boolean (no wrapper object)
- [x] All four management endpoints work with pagination
- [x] SQLite schema updated and tested
- [x] All experiment seed commands updated
- [x] Support module lookup parsers updated
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes

---

## Step 15 — ConsumerAuthorization token system

**Gaps addressed:**
- **G23** — The `authorization-token` sub-service is missing entirely. AH5 defines six token
  variants and five endpoints for token lifecycle management.

### What changes

Add the `authorizationToken` service under `/consumerauthorization/authorization-token/`.
For an initial implementation, only `TIME_LIMITED_TOKEN` needs to be functional; the other
variants (`USAGE_LIMITED_TOKEN`, `BASE64_SELF_CONTAINED_TOKEN`, JWT variants,
`TRANSLATION_BRIDGE_TOKEN`) can be accepted but return `501 Not Implemented` with an
informative `ErrorResponse`.

**Endpoints to implement:**
```
POST   /consumerauthorization/authorization-token/generate              — generate token
GET    /consumerauthorization/authorization-token/verify/{accessToken}  — verify token
GET    /consumerauthorization/authorization-token/public-key            — public key
POST   /consumerauthorization/authorization-token/encryption-key        — register key
DELETE /consumerauthorization/authorization-token/encryption-key        — remove key
```

**Generate request:**
```json
{
  "tokenVariant": "TIME_LIMITED_TOKEN",
  "provider":     "TemperatureProvider",
  "targetType":   "SERVICE_DEF",
  "target":       "temperatureService",
  "scope":        "read"
}
```

**Generate response (`AuthorizationTokenDescriptor`):**
```json
{
  "tokenType":  "TIME_LIMITED_TOKEN",
  "targetType": "SERVICE_DEF",
  "token":      "<UUID>",
  "expiresAt":  "2024-01-01T01:00:00Z"
}
```

**Verify response:**
```json
{
  "verified":    true,
  "consumerCloud": "LOCAL",
  "consumer":    "ConsumerApp",
  "targetType":  "SERVICE_DEF",
  "target":      "temperatureService",
  "scope":       null
}
```

**Public key:** Returns 404 until a cryptographic key pair is introduced
(full JWT support is out of scope here).

**Encryption key endpoints:** Accept the request, store the key (AES key + algorithm),
return 201/200. The stored key is not used until JWT variant support is added.

### TDD cycles

#### Cycle 15.1 — Token generation (TIME_LIMITED only)

**Write this failing test first** in `core/internal/consumerauth/api/handler_test.go`:

```go
func TestGenerateTimeLimitedToken201(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization-token/generate", map[string]any{
		"tokenVariant": "TIME_LIMITED_TOKEN",
		"provider":     "TemperatureProvider",
		"targetType":   "SERVICE_DEF",
		"target":       "temperatureService",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		TokenType string `json:"tokenType"`
		Token     string `json:"token"`
		ExpiresAt string `json:"expiresAt"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Token == "" {
		t.Error("token is empty")
	}
	if resp.TokenType != "TIME_LIMITED_TOKEN" {
		t.Errorf("tokenType = %q, want TIME_LIMITED_TOKEN", resp.TokenType)
	}
}

func TestGenerateUnsupportedVariant501(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization-token/generate", map[string]any{
		"tokenVariant": "BASE64_SELF_CONTAINED_TOKEN",
		"provider":     "P", "targetType": "SERVICE_DEF", "target": "s",
	})
	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
}
```

**Expected failure:** `expected 201, got 404`

**Implementation:** Add `POST /consumerauthorization/authorization-token/generate` route. Store the generated token in a new in-memory token store keyed by UUID. Unsupported variants return 501 with `WriteErrorResponse(..., "ARROWHEAD", ...)`.

---

#### Cycle 15.2 — Token verification

**Write this failing test first** in `core/internal/consumerauth/api/handler_test.go`:

```go
func TestVerifyValidAuthToken(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization-token/generate", map[string]any{
		"tokenVariant": "TIME_LIMITED_TOKEN",
		"provider":     "P1", "targetType": "SERVICE_DEF", "target": "svc1",
	})
	var genResp struct{ Token string `json:"token"` }
	json.NewDecoder(w.Body).Decode(&genResp)

	vw := bearerRequest(t, h, http.MethodGet,
		"/consumerauthorization/authorization-token/verify/"+genResp.Token, "")
	if vw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", vw.Code, vw.Body.String())
	}
	var verResp struct{ Verified bool `json:"verified"` }
	json.NewDecoder(vw.Body).Decode(&verResp)
	if !verResp.Verified {
		t.Error("verified = false, want true")
	}
}

func TestVerifyUnknownAuthToken404(t *testing.T) {
	h := newTestHandler()
	w := bearerRequest(t, h, http.MethodGet,
		"/consumerauthorization/authorization-token/verify/no-such-token", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
```

**Expected failure:** `expected 200, got 404`

---

#### Cycle 15.3 — Encryption-key endpoints

**Write this failing test first** in `core/internal/consumerauth/api/handler_test.go`:

```go
func TestRegisterEncryptionKey201(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/consumerauthorization/authorization-token/encryption-key", map[string]any{
		"systemName": "Consumer1",
		"algorithm":  "AES",
		"key":        "base64encodedkey==",
	})
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRemoveEncryptionKey200(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/consumerauthorization/authorization-token/encryption-key", map[string]any{
		"systemName": "Consumer2", "algorithm": "AES", "key": "k",
	})
	req := httptest.NewRequest(http.MethodDelete, "/consumerauthorization/authorization-token/encryption-key?systemName=Consumer2", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
```

**Expected failure:** `expected 201, got 404`

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/consumerauth/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all `consumerauth` subpackages.

### Files to modify / create

- `core/internal/consumerauth/repository/` — add token store
- `core/internal/consumerauth/service/auth.go` — token generation and verification logic
- `core/internal/consumerauth/api/handler.go` — five new routes
- `core/SPEC.md` — add `authorization-token` section
- `core/GAP_ANALYSIS.md` — mark G23 resolved (partial — TIME_LIMITED only)

### System test

```bash
bash core/test-system.sh
```

No Docker needed. No experiment code changes.

### Completion criteria

- [x] `TIME_LIMITED_TOKEN` generate returns `AuthorizationTokenDescriptor`
- [x] Verify returns correct `AuthorizationTokenVerifyResponse`
- [x] Non-TIME_LIMITED variants return 501 with structured `ErrorResponse`
- [x] Encryption-key register/remove endpoints respond correctly
- [x] Public-key returns 404
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes

---

## Step 16 — Service instance interface model alignment

**Gaps addressed:**
- **G30** — The `interfaces` field on service instances is currently a list of arbitrary strings
  (e.g. `["HTTP-INSECURE-JSON"]`). AH5 defines a structured `ServiceInterfaceRequest`
  (`templateName`, `protocol`, `policy`, `properties`) with validation against registered
  interface templates. The `ServiceLookupRequest` also lacks the at-least-one-filter validation
  the spec requires.

### What changes

**Service registration:**
`interfaces` in `ServiceRegistrationRequest` and `ServiceCreateListRequest` changes from
`[]string` to `[]ServiceInterfaceRequest`:
```json
{
  "templateName": "http-json",
  "protocol":     "http",
  "policy":       "NONE",
  "properties":   {"operations": ["GET /data"]}
}
```
Acceptable `policy` values: the `SecurityPolicy` enum
(`NONE`, `CERT_AUTH`, `TIME_LIMITED_TOKEN_AUTH`, etc.).

**Service instance response:**
`interfaces` in `ServiceInstanceDescriptor` changes to `[]ServiceInterfaceDescriptor`
(same fields; `properties` is optional in descriptor).

**Backward-compat strategy:** Accept a string in the `templateName` field that matches
the old flat-string format (e.g. `"HTTP-INSECURE-JSON"`) and treat it as
`{"templateName": "HTTP-INSECURE-JSON", "protocol": "http", "policy": "NONE", "properties": {}}`.
This allows existing experiments to continue working without changes to their
registration calls, while new clients can use the structured form.

**ServiceLookupRequest validation:** Add the at-least-one-filter check from the spec:
at least one of `instanceIds`, `providerNames`, `serviceDefinitionNames` must be non-empty.
Return 400 otherwise.

**Interface template validation:** When an `interfaceTemplate` with the given `templateName`
exists in the store, validate that the `properties` satisfy the template's
`propertyRequirements` (check mandatory fields are present). If the template does not
exist yet (common during bootstrap), accept the interface without validation and store it.

### TDD cycles

#### Cycle 16.1 — ServiceInterfaceRequest model and backward-compat parsing

**Write this failing test first** in `core/internal/api/ah5_handler_test.go`:

```go
func TestAH5ServiceRegisterStructuredInterface(t *testing.T) {
	h := newAH5Handler()
	// Register a device and system first (required by service registration).
	ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{"name": "dev1"})
	ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{
		"name": "sys1", "address": "10.0.0.1", "port": 8080, "deviceName": "dev1",
	})
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"serviceDefinitionName": "temp-service",
		"providerSystemName":    "sys1",
		"serviceUri":            "/temp",
		"version":               "1.0.0",
		"interfaces": []map[string]any{
			{"templateName": "http-json", "protocol": "http", "policy": "NONE"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAH5ServiceRegisterFlatStringInterfaceBackwardCompat(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{"name": "dev2"})
	ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{
		"name": "sys2", "address": "10.0.0.2", "port": 8081, "deviceName": "dev2",
	})
	// Flat string interface (old style) must still be accepted.
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"serviceDefinitionName": "flat-svc",
		"providerSystemName":    "sys2",
		"serviceUri":            "/flat",
		"version":               "1.0.0",
		"interfaces":            []string{"HTTP-INSECURE-JSON"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("backward-compat: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}
```

**Expected failure:** `expected 201, got 400` (structured interface object not accepted by current parser)

**Implementation:** Update the `interfaces` field parser in `ah5_handler.go` to accept either `string` or `{templateName, protocol, policy, properties}` JSON. If string, wrap as `ServiceInterfaceDescriptor{TemplateName: s, Protocol: "http", Policy: "NONE"}`.

---

#### Cycle 16.2 — ServiceLookupRequest at-least-one-filter

**Write this failing test first** in `core/internal/api/ah5_handler_test.go`:

```go
func TestAH5ServiceLookupRequiresFilter(t *testing.T) {
	h := newAH5Handler()
	w := ah5Post(t, h, "/serviceregistry/service-discovery/lookup", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty lookup: expected 400, got %d", w.Code)
	}
}
```

**Expected failure:** `empty lookup: expected 400, got 200` (current implementation accepts empty request)

**Implementation:** In `handleServiceLookup`, check that at least one of `instanceIds`, `providerNames`, `serviceDefinitionNames` is non-empty; return 400 otherwise.

---

#### Cycle 16.3 — Template validation (mandatory properties)

**Write this failing test first** in `core/internal/api/ah5_handler_test.go`:

```go
func TestInterfaceValidationTemplateAbsent(t *testing.T) {
	h := newAH5Handler()
	ah5Post(t, h, "/serviceregistry/device-discovery/register", map[string]any{"name": "d3"})
	ah5Post(t, h, "/serviceregistry/system-discovery/register", map[string]any{
		"name": "s3", "address": "10.0.0.3", "port": 8082, "deviceName": "d3",
	})
	// No interface template registered — should be accepted without validation.
	w := ah5Post(t, h, "/serviceregistry/service-discovery/register", map[string]any{
		"serviceDefinitionName": "no-tmpl-svc",
		"providerSystemName":    "s3",
		"serviceUri":            "/x",
		"version":               "1.0.0",
		"interfaces":            []map[string]any{{"templateName": "custom", "protocol": "mqtt", "policy": "NONE"}},
	})
	if w.Code != http.StatusCreated {
		t.Errorf("absent template: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}
```

**Expected failure:** Passes immediately if template-absent path already exists (this test verifies no regression); the mandatory-property test is added alongside it.

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/api/... ./internal/model/... ./internal/service/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on modified packages.

### Files to modify / create

- `core/internal/model/ah5_types.go` — add `ServiceInterfaceRequest`, `ServiceInterfaceDescriptor`, `SecurityPolicy` enum
- `core/internal/service/ah5_registry.go` — structured interface storage + validation
- `core/internal/api/ah5_handler.go` — update lookup validation
- `core/internal/repository/ah5_memory.go` — update interface field type
- `core/internal/repository/ah5_sqlite.go` — update JSON column shape
- `core/SPEC.md` — update service registration interface field shape
- `core/GAP_ANALYSIS.md` — add and mark G30 resolved

### System test

```bash
bash core/test-system.sh
bash experiments/experiment-9/test-system.sh
bash experiments/experiment-13/test-system.sh
bash experiments/experiment-14/test-system.sh
```

**Docker impact:** Experiment robot-fleet and service-partner registration calls use flat
string interfaces. Backward-compat parsing means no code change is required in experiment
services; verify by running Docker tests.

### Completion criteria

- [x] Structured `ServiceInterfaceRequest` accepted and stored correctly
- [x] Flat-string interface (backward compat) still accepted
- [x] `policy` field rejects unknown `SecurityPolicy` values with 400
- [x] ServiceLookupRequest with no filter fields returns 400
- [x] Mandatory template property check works when template is registered
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes

---

## Step 17 — Orchestration request/response alignment

**Gaps addressed:**
- **G7** — DynamicOrchestration calls `POST /serviceregistry/query` (AH4 legacy). The AH5
  aligned endpoint is `POST /serviceregistry/service-discovery/lookup`. Migrating requires
  updating the query shape and result model.
- **G32** — `ServiceOrchestrationRequest` uses `requestedService` (our name); AH5 uses
  `serviceRequirement`. The response wraps results in our field name, not `results`. Several
  `ServiceOrchestrationResult` field names differ from the spec (including two confirmed spec typos).

### What changes

**Request field rename:** `requestedService` → `serviceRequirement` in the shared
`OrchestrationRequest` model. Keep the old name as a JSON alias for one release cycle
(decode both `requestedService` and `serviceRequirement`; encode as `serviceRequirement`).

**Response field renames (including spec typos — match exactly):**

| Current field | AH5 spec field | Notes |
|---|---|---|
| `provider` (system object) | `providerName` (SystemName string) | Simplified to string |
| `serviceDefinition` | `serviceDefinitition` | **Spec typo: double 't'** — must match |
| *(absent)* | `cloudIdentitifer` | **Spec typo: missing 'n'** |
| `serviceInstanceId` | `serviceInstanceId` | Unchanged |
| `version` | `version` | Unchanged |
| *(absent)* | `aliveUntil` | Optional; add |
| *(absent)* | `warnings` | Response-level; add as `[]string` |

The spec typos (`serviceDefinitition`, `cloudIdentitifer`) must be matched exactly in the
JSON output for wire-format conformance. Add a comment in the struct definition documenting
that these are spec-mandated typos.

**Wrap in `results`:** Rename the response list field from whatever the current name is to `results`.

**DynamicOrchestration internal lookup:** Update `dynamicorch` to call
`POST /serviceregistry/service-discovery/lookup` instead of `POST /serviceregistry/query`.
This requires translating the `OrchestrationRequest.serviceRequirement` into a
`ServiceLookupRequest` and translating the `ServiceInstanceDescriptor` response back into
`ServiceOrchestrationResult`. The `ServiceOrchestrationResult.providerName` is populated
from `serviceInstance.provider.name`.

### TDD cycles

#### Cycle 17.1 — Request dual-decode: both serviceRequirement and requestedService

**Write this failing test first** in `core/internal/orchestration/model/` *(new file `types_test.go`)*:

```go
package model_test

import (
	"encoding/json"
	"testing"

	"arrowhead/core/internal/orchestration/model"
)

func TestOrchestrationRequestDecodesServiceRequirement(t *testing.T) {
	raw := `{"requesterSystem":{"systemName":"C"},"serviceRequirement":{"serviceDefinition":"temp"}}`
	var req model.OrchestrationRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.RequestedService.ServiceDefinition != "temp" {
		t.Errorf("ServiceDefinition = %q, want temp", req.RequestedService.ServiceDefinition)
	}
}

func TestOrchestrationRequestDecodesRequestedServiceBackwardCompat(t *testing.T) {
	raw := `{"requesterSystem":{"systemName":"C"},"requestedService":{"serviceDefinition":"temp"}}`
	var req model.OrchestrationRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.RequestedService.ServiceDefinition != "temp" {
		t.Errorf("ServiceDefinition = %q, want temp", req.RequestedService.ServiceDefinition)
	}
}
```

**Expected failure:** `types_test.go:NN: ServiceDefinition = "", want temp` (the new `serviceRequirement` key isn't decoded yet)

**Implementation:** In `OrchestrationRequest`, implement a custom `UnmarshalJSON` that accepts both `serviceRequirement` and `requestedService` keys, populating the same `RequestedService` field.

---

#### Cycle 17.2 — Response field names include spec typos

**Write this failing test first** in `core/internal/orchestration/model/types_test.go`:

```go
func TestOrchestrationResultSpecTypoFieldNames(t *testing.T) {
	result := model.OrchestrationResult{
		ServiceDefinition: "temp",
		ProviderName:      "sensor-1",
	}
	data, _ := json.Marshal(result)
	var raw map[string]any
	json.Unmarshal(data, &raw)

	// Spec typo: double 't' in serviceDefinitition
	if _, ok := raw["serviceDefinitition"]; !ok {
		t.Errorf("JSON key serviceDefinitition missing (double t) — got keys: %v", keys(raw))
	}
	// Spec typo: missing 'n' in cloudIdentitifer
	if _, ok := raw["cloudIdentitifer"]; !ok {
		t.Errorf("JSON key cloudIdentitifer missing (missing n) — got keys: %v", keys(raw))
	}
	// Old key must NOT be present.
	if _, ok := raw["serviceDefinition"]; ok {
		t.Error("old key serviceDefinition must not appear in encoded output")
	}
}

func keys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
```

**Expected failure:** `JSON key serviceDefinitition missing — got keys: [serviceDefinition ...]`

**Implementation:** In `core/internal/orchestration/model/types.go`, rename the `OrchestrationResult` struct tags:
```go
// ServiceDefinitition — spec typo (double 't') is intentional, must match AH5 wire format.
ServiceDefinition string `json:"serviceDefinitition"`
// CloudIdentitifer — spec typo (missing 'n') is intentional, must match AH5 wire format.
CloudIdentifier  string `json:"cloudIdentitifer"`
```

> **⚠ Regression check after Cycle 17.2**
> Renaming JSON tags is a wire-format change. Any existing test that decodes an orchestration
> result and checks the old field names (`serviceDefinition`, `provider.systemName`) will now
> fail to decode correctly (field will be empty). Before implementing the tag rename:
> 1. Grep for `serviceDefinition` and `providerSystem` in `internal/orchestration/*/api/handler_test.go`
>    and `internal/integration/e2e_test.go`.
> 2. Update those tests to reference the new names (`serviceDefinitition`, `providerName`).
> 3. Run `go test -race ./internal/orchestration/...` and confirm only the new typo test fails.
> Also grep experiment service Go files for the old field names:
> `grep -r "serviceDefinition\|providerSystem" experiments/experiment-{9,13,14}/services/`.

---

#### Cycle 17.3 — DynamicOrch uses AH5 lookup endpoint

**Write this failing test first** in `core/internal/orchestration/dynamic/service/orchestrator_test.go`:

```go
func TestDynamicOrchCallsAH5LookupEndpoint(t *testing.T) {
	called := false
	fakeSR := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/serviceregistry/service-discovery/lookup" {
			called = true
		}
		w.Header().Set("Content-Type", "application/json")
		// Return empty AH5 lookup response.
		json.NewEncoder(w).Encode(map[string]any{"entries": []any{}, "count": 0})
	}))
	defer fakeSR.Close()

	orch := NewDynamicOrchestrator(fakeSR.URL, "", "", false, false)
	orch.Orchestrate(model.OrchestrationRequest{
		RequesterSystem: model.System{SystemName: "C"},
		RequestedService: model.ServiceRequirement{ServiceDefinition: "temp"},
	})
	if !called {
		t.Error("DynamicOrchestrator did not call AH5 lookup endpoint")
	}
}
```

**Expected failure:** `DynamicOrchestrator did not call AH5 lookup endpoint` (currently calls `/serviceregistry/query`)

**Implementation:** In `dynamicorch/service/orchestrator.go`, replace the `POST /serviceregistry/query` call with `POST /serviceregistry/service-discovery/lookup`. Translate `serviceRequirement` into a `ServiceLookupRequest{ServiceDefinitionNames: [...]}`; translate `ServiceInstanceDescriptor` response into `OrchestrationResult`.

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
    ./internal/orchestration/model/... \
    ./internal/orchestration/dynamic/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on modified packages.

### Files to modify / create

- `core/internal/orchestration/model/types.go` — rename `requestedService`, rename response fields (with typos)
- `core/internal/orchestration/dynamic/service/orchestrator.go` — switch to AH5 lookup
- `core/internal/orchestration/dynamic/api/handler.go` — update response encoding
- `core/internal/orchestration/simplestore/api/handler.go` — update response encoding
- `core/internal/orchestration/flexiblestore/api/handler.go` — update response encoding
- `core-evol/internal/orchestration/` — same field renames
- `core/SPEC.md` — update orchestration request and result shapes
- `core/GAP_ANALYSIS.md` — mark G7 resolved; document spec typos

**Experiment impact:** All experiment consumer services that parse orchestration results
must be updated if they reference the old field names. Grep for
`serviceDefinition\|provider\.systemName` in experiment Go services.

### System test

```bash
bash core/test-system.sh
bash experiments/experiment-9/test-system.sh
bash experiments/experiment-13/test-system.sh
bash experiments/experiment-14/test-system.sh
```

### Completion criteria

- [x] Both `serviceRequirement` and `requestedService` decode successfully to the same struct
- [x] Encoded response uses `results`, `serviceDefinitition`, `cloudIdentitifer` (spec typos)
- [x] DynamicOrch calls AH5 lookup endpoint; results are correctly mapped
- [x] All experiment consumer services parse the new field names
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes

---

## Step 18 — Orchestration management: SimpleStore mgmt API + lock management + history

**Gaps addressed:**
- **G27** — Lock management and orchestration history endpoints are absent; SimpleStore exposes a custom `/simplestore/rules` path instead of the AH5-defined `/serviceorchestration/orchestration/mgmt/simple-store/` paths with structured record management (UUID-based IDs, priority modification, query with filters).

### What changes

**SimpleStore management path migration:**

| Old path (custom) | New path (AH5-aligned) |
|---|---|
| `GET /serviceorchestration/orchestration/simplestore/rules` | `POST /serviceorchestration/orchestration/mgmt/simple-store/query` |
| `POST /serviceorchestration/orchestration/simplestore/rules` | `POST /serviceorchestration/orchestration/mgmt/simple-store/create` |
| `DELETE /serviceorchestration/orchestration/simplestore/rules/{id}` | `DELETE /serviceorchestration/orchestration/mgmt/simple-store/query?uuids=...` |

New additional endpoint: `POST /serviceorchestration/orchestration/mgmt/simple-store/modify-priorities`.

Record IDs change from integer to UUID. The record model gains `consumer`, `serviceDefinition`,
`serviceInstanceId`, `priority`, `createdBy`, `updatedBy`, `createdAt`, `updatedAt`.

**Retain old paths as aliases during transition** to avoid breaking the dashboard; remove
aliases in the step after all dashboard calls are updated.

**Lock management (DynamicOrchestration only):**
```
POST   /serviceorchestration/orchestration/mgmt/lock/create
POST   /serviceorchestration/orchestration/mgmt/lock/query
DELETE /serviceorchestration/orchestration/mgmt/lock/remove/{owner}
```

Lock records: `id` (int), `orchestrationJobId` (UUID), `serviceInstanceId`, `owner`,
`expiresAt`, `temporary`.

**Orchestration history (Dynamic and SimpleStore):**
```
POST /serviceorchestration/orchestration/mgmt/history/query
```

History records: `id` (UUID), `status` (PENDING/IN_PROGRESS/DONE/ERROR), `type` (PULL/PUSH),
`requesterSystem`, `targetSystem`, `serviceDefinition`, `subscriptionId`, `message`,
`createdAt`, `startedAt`, `finishedAt`.

DynamicOrchestration records a history entry on every call to `orchestration/pull`
(status DONE on success, ERROR on failure).

### TDD cycles

#### Cycle 18.1 — SimpleStore mgmt endpoints with UUID IDs

**Write this failing test first** in `core/internal/orchestration/simplestore/api/handler_test.go`:

```go
func TestSimpleStoreMgmtCreate(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/simple-store/create", validRuleBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ID == "" {
		t.Error("id is empty — expected UUID")
	}
	// Validate it looks like a UUID.
	if len(resp.ID) != 36 {
		t.Errorf("id len = %d, want 36 (UUID)", len(resp.ID))
	}
}

func TestSimpleStoreMgmtQuery(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/simple-store/create", validRuleBody)
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/simple-store/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count int `json:"count"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("count = 0 after create")
	}
}
```

**Expected failure:** `expected 201, got 404`

**Implementation:** Add `/serviceorchestration/orchestration/mgmt/simple-store/create` route. Change record ID type from `int64` to `string` (UUID). Keep old `/simplestore/rules` paths as aliases. Add `query` and `modify-priorities` endpoints.

---

#### Cycle 18.2 — Lock management

**Write this failing test first** in `core/internal/orchestration/dynamic/api/handler_test.go`:

```go
func TestLockCreate(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"owner":              "consumer-app",
		"serviceInstanceId":  "inst-1",
		"orchestrationJobId": "00000000-0000-0000-0000-000000000001",
		"temporary":          true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLockQueryExcludesExpired(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	// Create a lock that expires immediately.
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"owner": "expired-owner", "serviceInstanceId": "i", "orchestrationJobId": "oid",
		"expiresAt": "2000-01-01T00:00:00Z", // past
	})
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/query", map[string]any{})
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 0 {
		t.Errorf("count = %d, want 0 (expired lock excluded)", resp.Count)
	}
}
```

**Expected failure:** `expected 201, got 404`

---

#### Cycle 18.3 — Orchestration history

**Write this failing test first** in `core/internal/orchestration/dynamic/api/handler_test.go`:

```go
func TestHistoryRecordedOnPull(t *testing.T) {
	sr := fakeSR("sensor-1")
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	// Issue a pull request.
	postJSON(t, h, "/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]any{"systemName": "C", "address": "localhost", "port": 0},
		"serviceRequirement": map[string]any{"serviceDefinition": "temperature-service"},
	})
	// Query history.
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/history/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("history count = 0 after pull")
	}
}
```

**Expected failure:** `expected 200, got 404`

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
    ./internal/orchestration/dynamic/... \
    ./internal/orchestration/simplestore/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on modified packages.

### Files to modify / create

- `core/internal/orchestration/simplestore/` — uuid IDs, new mgmt path, new endpoints
- `core/internal/orchestration/simplestore/repository/` — update record model
- `core/internal/orchestration/dynamic/` — lock store + history store + endpoints
- `core/dashboard/src/api.ts` — update rule management API calls to new paths
- `core/SPEC.md` — SimpleStore mgmt path; add lock and history sections
- `core/GAP_ANALYSIS.md` — mark G27 resolved
- `core-evol/internal/orchestration/stores.go` *(new)* — lock, history, subscription stores + UUID helper
- `core-evol/internal/orchestration/handler.go` — add lock/history endpoints on `dynamicorch-xacml`

### System test

```bash
bash core/test-system.sh
cd core-evol && go test -race ./...
```

Dashboard API calls update requires a `npm run build` check. No Docker needed for Go tests.

### Completion criteria

- [x] SimpleStore mgmt endpoints use spec paths with UUID IDs
- [x] `modify-priorities` accepts UUID-to-priority map
- [x] Lock create/query/remove work with in-memory store
- [x] Expired locks excluded from query
- [x] History entry recorded per pull call; history query returns paginated results
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] `core-evol` (`dynamicorch-xacml`) lock and history endpoints pass `go test -race ./...`

---

## Step 19 — Push orchestration and subscriptions

**Gaps addressed:**
- **G26** — Push-style orchestration (`subscribe` / `unsubscribe`) and the push management
  endpoints are absent. Only pull-style orchestration is implemented.

### What changes

**Discovery endpoints (both Dynamic and SimpleStore):**
```
POST   /serviceorchestration/orchestration/subscribe
DELETE /serviceorchestration/orchestration/unsubscribe/{subscriptionId}
```

**Push management endpoints:**
```
POST   /serviceorchestration/orchestration/mgmt/push/subscribe    — operator subscribe on behalf
DELETE /serviceorchestration/orchestration/mgmt/push/unsubscribe  — cancel (?ids=...)
POST   /serviceorchestration/orchestration/mgmt/push/trigger      — manual trigger
POST   /serviceorchestration/orchestration/mgmt/push/query        — list subscriptions
```

**Subscription model:**
```json
{
  "id":                  "<UUID>",
  "ownerSystemName":     "ConsumerApp",
  "targetSystemName":    "ConsumerApp",
  "orchestrationRequest": { /* ServiceOrchestrationRequest */ },
  "notifyInterface":     { "protocol": "http", "properties": {} },
  "expiredAt":           "2024-01-01T01:00:00Z",
  "createdAt":           "2024-01-01T00:00:00Z"
}
```

**Subscription store:** In-memory. A background goroutine checks for provider changes in the
ServiceRegistry every `pushPollInterval` (default 30 s, configurable via env var). When the
set of matching providers changes, it fires a notification to the `notifyInterface` address.
Notification delivery is best-effort HTTP POST; failures are logged, not retried.

**Note:** Actual notification delivery to arbitrary HTTP endpoints is the complex part.
For initial implementation, subscriptions are stored and managed but notification delivery
can be a no-op stub (subscribers register successfully but receive no push notifications).
Document this as a known limitation in GAP_ANALYSIS.md.

### TDD cycles

#### Cycle 19.1 — Subscribe and unsubscribe

**Write this failing test first** in `core/internal/orchestration/dynamic/api/handler_test.go`:

```go
func TestSubscribeReturnsUUID(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	w := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", map[string]any{
		"ownerSystemName":  "consumer-app",
		"targetSystemName": "consumer-app",
		"orchestrationRequest": map[string]any{
			"requesterSystem":    map[string]any{"systemName": "consumer-app", "address": "localhost", "port": 0},
			"serviceRequirement": map[string]any{"serviceDefinition": "temperature-service"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ ID string `json:"id"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.ID) != 36 {
		t.Errorf("id = %q, not a UUID", resp.ID)
	}
}

func TestUnsubscribeNotFound204(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/unsubscribe/no-such-id", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}
```

**Expected failure:** `expected 201, got 404`

---

#### Cycle 19.2 — Push management endpoints

**Write this failing test first** in `core/internal/orchestration/dynamic/api/handler_test.go`:

```go
func TestPushMgmtSubscribeAndQuery(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	postJSON(t, h, "/serviceorchestration/orchestration/subscribe", map[string]any{
		"ownerSystemName": "C", "targetSystemName": "C",
		"orchestrationRequest": map[string]any{
			"requesterSystem":    map[string]any{"systemName": "C", "address": "localhost", "port": 0},
			"serviceRequirement": map[string]any{"serviceDefinition": "svc"},
		},
	})
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("push/query: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("push/query count = 0 after subscribe")
	}
}
```

**Expected failure:** `push/query: expected 200, got 404`

---

#### Cycle 19.3 — Trigger records PENDING history entry

**Write this failing test first** in `core/internal/orchestration/dynamic/api/handler_test.go`:

```go
func TestTriggerCreatesPendingHistoryEntry(t *testing.T) {
	sr := fakeSR()
	defer sr.Close()
	h := newTestHandler(sr.URL, "", false)
	// Subscribe first to get a valid subscriptionId.
	sw := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", map[string]any{
		"ownerSystemName": "C", "targetSystemName": "C",
		"orchestrationRequest": map[string]any{
			"requesterSystem":    map[string]any{"systemName": "C", "address": "localhost", "port": 0},
			"serviceRequirement": map[string]any{"serviceDefinition": "svc"},
		},
	})
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(sw.Body).Decode(&sub)

	// Trigger.
	tw := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/trigger",
		map[string]any{"subscriptionId": sub.ID})
	if tw.Code != http.StatusOK {
		t.Fatalf("trigger: expected 200, got %d: %s", tw.Code, tw.Body.String())
	}
	// Check history.
	hw := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/history/query", map[string]any{})
	var histResp struct{ Count int `json:"count"` }
	json.NewDecoder(hw.Body).Decode(&histResp)
	if histResp.Count < 1 {
		t.Error("trigger did not create history entry")
	}
}
```

**Expected failure:** `trigger: expected 200, got 404`

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/orchestration/dynamic/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `dynamic` orchestration package.

### Files to modify / create

- `core/internal/orchestration/dynamic/api/handler.go` — subscribe/unsubscribe + push mgmt
- `core/internal/orchestration/dynamic/service/` — subscription store
- `core/internal/orchestration/simplestore/api/handler.go` — subscribe/unsubscribe
- `core/SPEC.md` — add subscribe/unsubscribe and push mgmt sections
- `core/GAP_ANALYSIS.md` — mark G26 resolved (partial — delivery is stub)
- `core-evol/internal/orchestration/handler.go` — subscribe/unsubscribe + push mgmt on `dynamicorch-xacml`

### System test

```bash
bash core/test-system.sh
cd core-evol && go test -race ./...
```

No Docker needed. No experiment code changes.

### Completion criteria

- [x] Subscribe returns UUID; duplicate subscribe overwrites and returns 200
- [x] Unsubscribe returns 200 (found) or 204 (not found)
- [x] Push management endpoints (subscribe, unsubscribe, trigger, query) work correctly
- [x] Trigger records a PENDING history entry
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] `core-evol` (`dynamicorch-xacml`) subscribe/unsubscribe and push endpoints pass `go test -race ./...`

---

## Step 20 — Blacklist system

**Gaps addressed:**
- **G28** — The `Blacklist` core system does not exist. AH5 defines it at base path
  `/blacklist` with two discovery endpoints and three management endpoints.

### What changes

New binary `cmd/blacklist` (port 8087 — `PORT` env var). New packages
`internal/blacklist/api/`, `internal/blacklist/service/`, `internal/blacklist/repository/`.

**Discovery endpoints:**
```
GET /blacklist/lookup               — list entries applicable to the requester
GET /blacklist/check/{systemName}   — returns Boolean
```

**Management endpoints:**
```
POST   /blacklist/mgmt/query   — paginated query with filters
POST   /blacklist/mgmt/create  — bulk create entries
DELETE /blacklist/mgmt/remove  — inactivate entries (?names=...)
```

**Entry model:**
```json
{
  "systemName":  "string",
  "reason":      "string (required, max 1024 chars)",
  "expiresAt":   "DateTime (optional)",
  "active":      true,
  "createdBy":   "string",
  "createdAt":   "DateTime",
  "updatedAt":   "DateTime"
}
```

**Behavioral rules:**
- `reason` is mandatory on create; 400 if absent.
- `DELETE /blacklist/mgmt/remove` inactivates entries (`active: false`) — it does NOT delete records.
- `GET /blacklist/check/{systemName}` returns `true` only if there is an ACTIVE, non-expired entry.
- `GET /blacklist/lookup` returns all entries applicable to the calling system (same `systemName` as requester token).

**SQLite:** Add `DB_PATH` support from the start (follow Step 9 pattern).

**Integration with other systems:** Other core systems do not enforce blacklist checks in this step.
A `BLACKLIST_URL` env var and optional enforcement could be added to DynamicOrchestration
and ConsumerAuthorization in a future step.

### TDD cycles

#### Cycle 20.1 — BlacklistService and memory repository

**Write this failing test first** in `core/internal/blacklist/service/blacklist_test.go` *(new file)*:

```go
package service_test

import (
	"testing"
	"time"

	"arrowhead/core/internal/blacklist/repository"
	"arrowhead/core/internal/blacklist/service"
)

func TestIsBlacklistedTrue(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	svc.Add("bad-actor", "malicious behavior", time.Time{}, "admin")
	if !svc.IsBlacklisted("bad-actor") {
		t.Error("IsBlacklisted = false, want true")
	}
}

func TestIsBlacklistedFalse(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	if svc.IsBlacklisted("clean") {
		t.Error("IsBlacklisted = true for unknown system")
	}
}

func TestRemoveInactivatesNotDeletes(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	svc.Add("sys-a", "reason", time.Time{}, "admin")
	svc.Remove("sys-a")
	if svc.IsBlacklisted("sys-a") {
		t.Error("IsBlacklisted = true after Remove — should be inactivated")
	}
	// Entry must still exist in query results (with active: false).
	entries := svc.Query(nil)
	found := false
	for _, e := range entries {
		if e.SystemName == "sys-a" {
			found = true
			if e.Active {
				t.Error("entry active = true after Remove")
			}
		}
	}
	if !found {
		t.Error("entry not found after Remove — was deleted instead of inactivated")
	}
}

func TestIsBlacklistedExpiredEntry(t *testing.T) {
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	past := time.Now().Add(-time.Hour)
	svc.Add("expired-sys", "temp ban", past, "admin")
	if svc.IsBlacklisted("expired-sys") {
		t.Error("IsBlacklisted = true for expired entry")
	}
}
```

**Expected failure:** `blacklist_test.go:NN: undefined: repository.NewMemoryRepository` and `undefined: service.NewBlacklistService`

**Implementation:** Create `core/internal/blacklist/repository/memory.go` with `Entry` struct and `MemoryRepository`; create `core/internal/blacklist/service/blacklist.go` with `BlacklistService`.

---

#### Cycle 20.2 — Discovery endpoints

**Write this failing test first** in `core/internal/blacklist/api/handler_test.go` *(new file)*:

```go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"arrowhead/core/internal/blacklist/api"
	"arrowhead/core/internal/blacklist/repository"
	"arrowhead/core/internal/blacklist/service"
)

func newTestHandler() http.Handler {
	svc := service.NewBlacklistService(repository.NewMemoryRepository())
	return api.NewHandler(svc)
}

func getReq(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestCheckTrue(t *testing.T) {
	h := newTestHandler()
	// Seed via mgmt endpoint.
	postJSON(t, h, "/blacklist/mgmt/create", map[string]any{
		"entries": []map[string]any{{"systemName": "bad", "reason": "test"}},
	})
	w := getReq(t, h, "/blacklist/check/bad")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "true\n" && body != "true" {
		t.Errorf("body = %q, want true", body)
	}
}

func TestCheckFalse(t *testing.T) {
	h := newTestHandler()
	w := getReq(t, h, "/blacklist/check/unknown")
	if body := w.Body.String(); body != "false\n" && body != "false" {
		t.Errorf("body = %q, want false", body)
	}
}

func TestCheckExpiredEntry(t *testing.T) {
	h := newTestHandler()
	// No easy way to create expired via API — use service directly.
	repo := repository.NewMemoryRepository()
	svc := service.NewBlacklistService(repo)
	_ = time.Now() // reference to confirm time package is used
	svc.Add("exp-sys", "temp", time.Now().Add(-time.Hour), "admin")
	handler := api.NewHandler(svc)
	w := getReq(t, handler, "/blacklist/check/exp-sys")
	if body := w.Body.String(); body != "false\n" && body != "false" {
		t.Errorf("expired entry: body = %q, want false", body)
	}
}
```

**Expected failure:** `handler_test.go:NN: undefined: api.NewHandler`

**Implementation:** Create `core/internal/blacklist/api/handler.go` with `NewHandler` and five endpoints. `GET /blacklist/check/{systemName}` writes `json.NewEncoder(w).Encode(svc.IsBlacklisted(name))`.

---

#### Cycle 20.3 — Management endpoints

**Write this failing test first** in `core/internal/blacklist/api/handler_test.go`:

```go
func TestMgmtCreateMissingReason400(t *testing.T) {
	h := newTestHandler()
	w := postJSON(t, h, "/blacklist/mgmt/create", map[string]any{
		"entries": []map[string]any{{"systemName": "sys-without-reason"}},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing reason, got %d", w.Code)
	}
}

func TestMgmtRemoveInactivates(t *testing.T) {
	h := newTestHandler()
	postJSON(t, h, "/blacklist/mgmt/create", map[string]any{
		"entries": []map[string]any{{"systemName": "removable", "reason": "test"}},
	})
	req := httptest.NewRequest(http.MethodDelete, "/blacklist/mgmt/remove?names=removable", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("remove: expected 200, got %d", w.Code)
	}
	// System should no longer be blacklisted.
	cw := getReq(t, h, "/blacklist/check/removable")
	if cw.Body.String() != "false\n" && cw.Body.String() != "false" {
		t.Error("after remove, check should return false")
	}
}
```

**Expected failure:** `expected 400 for missing reason, got 404`

---

#### Cycle 20.4 — SQLite repository

**Write this failing test first** in `core/internal/blacklist/repository/sqlite_test.go` *(new file)*:

```go
package repository_test

import (
	"os"
	"testing"

	"arrowhead/core/internal/blacklist/repository"
)

func TestSQLiteBlacklistSaveAndQuery(t *testing.T) {
	f, _ := os.CreateTemp("", "blacklist-*.db")
	f.Close()
	defer os.Remove(f.Name())

	repo, err := repository.NewSQLiteRepository(f.Name())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo.Save(repository.Entry{SystemName: "sql-bad", Reason: "r", Active: true})
	entries := repo.All()
	if len(entries) != 1 {
		t.Errorf("All() len = %d, want 1", len(entries))
	}
}
```

**Expected failure:** `sqlite_test.go:NN: undefined: repository.NewSQLiteRepository`

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/blacklist/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all `blacklist` subpackages.

### Files to create

- `core/cmd/blacklist/main.go`
- `core/internal/blacklist/api/handler.go` + `handler_test.go`
- `core/internal/blacklist/service/blacklist.go` + `blacklist_test.go`
- `core/internal/blacklist/repository/memory.go` + `sqlite.go` + `interface.go`
- `core/SPEC.md` — add Blacklist section
- `core/GAP_ANALYSIS.md` — mark G28 resolved

### System test

```bash
bash core/test-system.sh
```

No Docker needed. New binary is standalone; no experiment changes required.

### Completion criteria

- [x] All five endpoints respond correctly
- [x] `check` returns `false` for expired entries
- [x] `remove` inactivates, does not delete
- [x] `reason` mandatory on create; 400 without it
- [x] SQLite repository passes all tests with `DB_PATH=:memory:`
- [x] `go build ./...` includes the new binary
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes

---

## Step 21 — GeneralManagement on all systems

**Gaps addressed:**
- **G29** — Every AH5 core system is required to expose `POST /<prefix>/general/mgmt/logs`
  and `GET /<prefix>/general/mgmt/get-config`. Neither endpoint exists on any system.

### What changes

Add a shared `generalMgmt` handler package that both endpoints delegate to. Register it
in every system's `main.go`.

**Log endpoint:** `POST /<prefix>/general/mgmt/logs`
```json
{
  "pagination": { "pageNumber": 0, "pageSize": 20 },
  "from":       "2024-01-01T00:00:00Z",
  "to":         "2024-12-31T23:59:59Z",
  "severity":   "WARN",
  "loggerStr":  "partial-logger-name"
}
```
Returns `{"entries": [...], "count": N}` where each entry has `logId`, `entryDate`, `logger`,
`severity`, `message`, `exception` (optional).

**Implementation strategy:** Adopt `log/slog` as the structured logger. A custom `slog.Handler`
writes to an in-memory ring buffer (default 1000 entries, configurable via env var). The log
endpoint reads from the ring buffer and applies filters. All existing `log.Printf` calls
in handlers are replaced with `slog.Info`/`slog.Error`/`slog.Warn`.

**Config endpoint:** `GET /<prefix>/general/mgmt/get-config?keys=KEY1,KEY2`
Returns a flat map of the requested configuration keys. Each system exposes the env vars
it reads (e.g. ServiceRegistry returns `PORT`, `DB_PATH`, `TLS_PORT`; DynamicOrchestration
also returns `SERVICE_REGISTRY_URL`, `CONSUMER_AUTH_URL`, `ENABLE_AUTH`, etc.).

### TDD cycles

#### Cycle 21.1 — Ring-buffer log handler

**Write this failing test first** in `core/internal/generalmgmt/logbuffer_test.go` *(new file)*:

```go
package generalmgmt_test

import (
	"fmt"
	"testing"
	"time"

	"arrowhead/core/internal/generalmgmt"
)

func TestRingBufferCapacity(t *testing.T) {
	buf := generalmgmt.NewLogBuffer(5)
	for i := 0; i < 10; i++ {
		buf.Append(generalmgmt.LogEntry{Message: fmt.Sprintf("msg%d", i), Severity: "INFO"})
	}
	entries := buf.All()
	if len(entries) != 5 {
		t.Errorf("ring buffer len = %d, want 5 (capacity)", len(entries))
	}
	// Most recent 5 should be msg5..msg9.
	if entries[0].Message != "msg5" {
		t.Errorf("oldest entry = %q, want msg5", entries[0].Message)
	}
}

func TestRingBufferSeverityFilter(t *testing.T) {
	buf := generalmgmt.NewLogBuffer(100)
	buf.Append(generalmgmt.LogEntry{Message: "info-msg", Severity: "INFO"})
	buf.Append(generalmgmt.LogEntry{Message: "warn-msg", Severity: "WARN"})
	buf.Append(generalmgmt.LogEntry{Message: "error-msg", Severity: "ERROR"})
	// Exact severity match: only WARN.
	entries := buf.Filter(generalmgmt.LogFilter{Severity: "WARN"})
	if len(entries) != 1 {
		t.Errorf("filtered len = %d, want 1 (exact WARN match)", len(entries))
	}
}

func TestRingBufferTimeRange(t *testing.T) {
	buf := generalmgmt.NewLogBuffer(100)
	past := time.Now().Add(-2 * time.Hour)
	present := time.Now()
	buf.Append(generalmgmt.LogEntry{Message: "old", Severity: "INFO", EntryDate: past})
	buf.Append(generalmgmt.LogEntry{Message: "new", Severity: "INFO", EntryDate: present})
	// From is RFC3339 string.
	from := time.Now().Add(-time.Hour).Format(time.RFC3339)
	entries := buf.Filter(generalmgmt.LogFilter{From: from})
	if len(entries) != 1 || entries[0].Message != "new" {
		t.Errorf("time filter: got %v, want [new]", entries)
	}
}
```

**Expected failure:** `logbuffer_test.go:NN: undefined: generalmgmt.NewLogBuffer`

**Implementation:** Create `core/internal/generalmgmt/logbuffer.go` with:
- `LogEntry` struct: `LogID`, `EntryDate time.Time`, `Logger`, `Severity`, `Message`, `Exception string` (optional)
- `LogFilter` struct: `From string` (RFC3339), `To string` (RFC3339), `Severity string`, `LoggerStr string`
- `LogBuffer` ring buffer using a fixed-size `[]LogEntry` and a `head int` pointer
- `Append(e LogEntry)` — sets `EntryDate` to `time.Now()` if zero
- `All() []LogEntry` — returns entries oldest-first
- `Filter(f LogFilter) []LogEntry` — exact `Severity` match; `From`/`To` range; `LoggerStr` substring match

---

#### Cycle 21.2 — Log endpoint registered on all systems

**Write this failing test first** in `core/internal/generalmgmt/handler_test.go` *(new file)*:

```go
package generalmgmt_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"arrowhead/core/internal/generalmgmt"
)

// newMgmtHandler creates a handler for the given system prefix.
// NewHandler signature: NewHandler(buf *LogBuffer, prefix string, config map[string]string) http.Handler
func newMgmtHandler(prefix string) http.Handler {
	buf := generalmgmt.NewLogBuffer(100)
	return generalmgmt.NewHandler(buf, prefix, map[string]string{"PORT": "8080"})
}

func TestLogEndpointReturns200(t *testing.T) {
	h := newMgmtHandler("serviceregistry")
	req := httptest.NewRequest(http.MethodPost, "/serviceregistry/general/mgmt/logs",
		bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Count int `json:"count"` }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 0 {
		t.Errorf("count = %d, want 0", resp.Count)
	}
}

func TestLogEndpointInvalidTimeRange400(t *testing.T) {
	h := newMgmtHandler("serviceregistry")
	// from > to should return 400.
	body := `{"from":"2025-01-02T00:00:00Z","to":"2025-01-01T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/serviceregistry/general/mgmt/logs",
		bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for from > to, got %d", w.Code)
	}
}
```

**Expected failure:** `handler_test.go:NN: undefined: generalmgmt.NewHandler`

**Implementation:** Create `core/internal/generalmgmt/handler.go` with `NewHandler(buf *LogBuffer, prefix string, config map[string]string) http.Handler`. The `prefix` parameter determines the URL path: `/<prefix>/general/mgmt/logs` and `/<prefix>/general/mgmt/get-config`. In each `cmd/*/main.go`, create a `LogBuffer`, configure it as the `slog` default handler, and register `generalmgmt.NewHandler` alongside the system handler.

---

#### Cycle 21.3 — Config endpoint returns requested keys

**Write this failing test first** in `core/internal/generalmgmt/handler_test.go`:

```go
func TestGetConfigReturnsRequestedKeys(t *testing.T) {
	buf := generalmgmt.NewLogBuffer(10)
	h := generalmgmt.NewHandler(buf, "serviceregistry", map[string]string{
		"PORT":    "8080",
		"DB_PATH": "/data/sr.db",
	})

	req := httptest.NewRequest(http.MethodGet,
		"/serviceregistry/general/mgmt/get-config?keys=PORT,DB_PATH,UNKNOWN", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["PORT"] != "8080" {
		t.Errorf("PORT = %q, want 8080", resp["PORT"])
	}
	if resp["DB_PATH"] != "/data/sr.db" {
		t.Errorf("DB_PATH = %q", resp["DB_PATH"])
	}
	if _, ok := resp["UNKNOWN"]; ok {
		t.Error("unknown key should not appear in response")
	}
}
```

**Expected failure:** `expected 200, got 404` (config endpoint not yet registered)

**Implementation:** In `generalmgmt/handler.go`, parse `keys=` query parameter, return only the requested keys that exist in the provided config map.

---

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/generalmgmt/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `generalmgmt` package.

### Files to modify / create

- `core/internal/generalmgmt/` *(new package)* — `logbuffer.go`, `handler.go`, `logbuffer_test.go`, `handler_test.go`
- `core/cmd/*/main.go` — all eight binaries (seven systems + Blacklist from Step 20): create `LogBuffer`, set as `slog` default, register `generalmgmt.NewHandler` alongside the system handler
- All `internal/*/api/handler.go` files — replace `log.Printf` with `slog`
- `core/SPEC.md` — add GeneralManagement section
- `core/GAP_ANALYSIS.md` — mark G29 resolved
- `core-evol/internal/generalmgmt/` *(new package)* — `logbuffer.go`, `handler.go` (same logic, separate module)
- `core-evol/cmd/dynamicorch-xacml/main.go` — wire `generalmgmt.NewHandler` (prefix `serviceorchestration/orchestration`, port 8083)

### System test

```bash
bash core/test-system.sh
cd core-evol && go build ./... && go test -race ./...
```

No Docker needed. This is purely additive.

### Completion criteria

- [x] Both endpoints registered on all eight `core/` systems (seven systems + Blacklist)
- [x] Both endpoints registered on `core-evol`'s `dynamicorch-xacml`
- [x] Log endpoint accepts all filter fields and returns paginated `LogEntry` results
- [x] Invalid time interval (from > to) returns 400
- [x] Config endpoint returns only requested keys; unknown keys are omitted
- [x] Ring buffer wraps correctly at capacity
- [x] All handler files use `slog` instead of `log.Printf`
- [x] `go test -race ./...` from `core/` passes (all existing tests still pass)
- [x] `bash core/test-system.sh` passes
- [x] `cd core-evol && go build ./... && go test -race ./...` passes

---

## 9. Second-round documentation updates

After completing Steps 10–21, apply the following documentation changes:

### `core/GAP_ANALYSIS.md`

| Step | Gaps resolved |
|------|---------------|
| 10 | G31 (error response model) |
| 11 | G20 |
| 12 | G8, G15 (fully) |
| 13 | G2, G21 |
| 14 | G22 |
| 15 | G23 (partial — TIME_LIMITED only) |
| 16 | G30 (interface model) |
| 17 | G7, G32 (orchestration response alignment) |
| 18 | G27 |
| 19 | G26 (partial — delivery stub) |
| 20 | G28 |
| 21 | G29 |

Add new entries for gaps first documented in this round:
- **G30 — ServiceInterfaceRequest not structured; ServiceLookupRequest accepts empty filter** (resolved in Step 16 — already in GAP_ANALYSIS.md)
- **G31 — Structured error responses: inconsistent/plain-text 4xx/5xx bodies** (resolved in Step 10)
- **G32 — Orchestration request/response field names diverge from spec** (resolved in Step 17, including spec typo documentation)
- **G33 — `POST /authentication/identity/change` absent** (planned for Step 12)

### `core/SPEC.md`

Add or update:
- `ErrorResponse` and `ErrorType` — new section after Overview
- Pagination — add `PageRequest` to Shared Types; update all query endpoints
- Authentication: `change` endpoint; full verify response shape; mgmt endpoints
- ConsumerAuthorization: full policy model; `authorization-token` endpoints; mgmt endpoints
- Service instance `interfaces` field — structured shape
- Orchestration request: `serviceRequirement`; response: `results`, `warnings`, spec-typo field names
- SimpleStore mgmt: spec paths; UUID IDs
- Lock management and history — new subsections under Orchestration
- Push orchestration — subscribe/unsubscribe + push mgmt subsections
- Blacklist — new system section (port 8087)
- GeneralManagement — cross-cutting section

### `CONFORMANCE.md`

Move the following gaps to resolved in the phase plan after each step completes:
G11, G20, G21, G22, G23 (partial), G26 (partial), G27, G29, G43.

---

## 10. Second-round regression matrix

| Check | Steps |
|---|---|
| `cd core && go build ./...` | All |
| `cd core && go vet ./...` | All |
| `cd core && go test -race ./...` | All |
| `bash core/test-system.sh` | All |
| `cd core-evol && go build ./...` + tests | 17, 18, 19, 21 |
| `go build ./...` (workspace root) | 17, 20 |
| Docker: experiment-9, -13, -14 | 13, 14, 17 |

---

---

# Phase 1 — Wire-compatibility fixes (Steps 22–26)

**Source:** `CONFORMANCE.md` — "Phase 1 — Quick wire-compatibility fixes"  
**Goal:** Close five low-effort, high-interoperability gaps so that an AH5-compliant client can
interact with all core systems without bespoke workarounds.  
**Prerequisite:** All Steps 1–21 pass. Pre-flight check green.

**After every Phase 1 step, update:**
- `CONFORMANCE.md` — move gap from Open → Resolved in the quick-reference table
- `core/GAP_ANALYSIS.md` — mark gap resolved, add fix summary paragraph
- `core/SPEC.md` — reflect any new/changed endpoint behaviour or fields
- `CONFORMANCE_UPDATE_PLAN.md` — tick completion criteria; note any deviations
- Any affected `README.md` or `EXAMPLES.md`

---

## Step 22 — System revoke derives identity from verified token (G11)

**Gap addressed:**
- **G11** — `DELETE /serviceregistry/system-discovery/revoke` requires `?name=`
  query parameter. AH5 spec says no query param; the server infers the name from the
  `Authorization: Bearer <token>` header identity. Credential verification is now real
  (Step 13 resolved G2), so this fix is now feasible.

**Why now:** Wire-protocol divergence that breaks every AH5-compliant client calling
system revoke. Low-effort fix that was blocked only by G2 being a stub.

**Prerequisites:** Step 13 complete. Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/api/ah5_handler.go` — `handleSystemRevoke`
- `core/cmd/serviceregistry/main.go` — read `AUTH_URL` env var; pass to AH5 handler

**Files to modify (core-evol/):** None — ServiceRegistry is a standalone core system.

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `SR_AUTH_URL` | `http://localhost:8081` | Authentication system base URL used by ServiceRegistry to verify Bearer tokens on system revoke |

---

### System test

Add to `core/internal/integration/e2e_test.go` or a new
`core/internal/integration/system_revoke_test.go`. Start Authentication with a Sysop
identity, register a system, login to get a token, then call
`DELETE /system-discovery/revoke` with `Authorization: Bearer <token>` (no `?name=`),
assert 200, and confirm the system is absent from a subsequent lookup.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/api/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/api` package.

### Documentation updates (after Step 22)

- `core/GAP_ANALYSIS.md` — mark G11 resolved; add fix summary paragraph
- `core/SPEC.md` — update `DELETE /system-discovery/revoke`: remove `?name=` as primary
  mechanism; document Bearer token identity; note `?name=` backward-compat fallback (deprecated)
- `CONFORMANCE.md` — update ServiceRegistry conformance row; move G11 to resolved in phase plan
- `README.md` — update Configuration table: add `SR_AUTH_URL`

### Completion criteria

- [x] `TestSystemRevokeUsesTokenIdentity` passes
- [x] `TestSystemRevokeWithoutBearerReturns401` passes
- [x] `TestSystemRevokeAuthUnreachableReturns401` passes
- [x] Backward-compat `?name=` fallback still works (existing tests pass without change)
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on `internal/api`

---

## Step 23 — Blacklist Bearer enforcement and mode enum (G41)

**Gap addressed:**
- **G41** — Two behavioral gaps on the Blacklist system:
  1. `GET /blacklist/lookup` and `GET /blacklist/check/{name}` do not require
     `Authorization: Bearer <token>`.
  2. `POST /blacklist/mgmt/query` accepts `active: *bool` instead of the AH5
     `mode` string enum (`ALL`, `ACTIVES`, `INACTIVES`).

**Prerequisites:** Steps 1–21 complete. Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/blacklist/model/types.go` — add `Mode` string constants; replace
  `Active *bool` with `Mode string` in `QueryRequest`
- `core/internal/blacklist/service/blacklist.go` — translate `Mode` → internal bool
  before filtering; return 400 for unknown mode values
- `core/internal/blacklist/api/handler.go` — add Bearer auth middleware on discovery
  handlers; map mode enum

**Files to modify (core-evol/):** None — Blacklist is a standalone core service.

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `BLACKLIST_AUTH_URL` | *(unset)* | Authentication system URL for Bearer verification on discovery endpoints. When unset, Bearer check is **skipped** (development/test mode). |

---

### System test

Start Blacklist with `BLACKLIST_AUTH_URL` pointing to a running Authentication instance.
Verify that `GET /blacklist/lookup` without a token returns 401, that the same endpoint
with a valid Bearer token returns 200, that `POST /blacklist/mgmt/query {"mode":"ALL"}`
returns 200, and that `{"mode":"BOGUS"}` returns 400.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/blacklist/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/blacklist/...`.

### Documentation updates (after Step 23)

- `core/GAP_ANALYSIS.md` — mark G41 resolved; add fix summary paragraph
- `core/SPEC.md` — update Blacklist section: add Bearer requirement on discovery;
  replace `active *bool` with `mode` enum in mgmt/query request
- `CONFORMANCE.md` — update Blacklist conformance rows; move G41 to resolved in phase plan
- `README.md` — add `BLACKLIST_AUTH_URL` to configuration table

### Completion criteria

- [x] `TestLookupRequiresBearerWhenAuthConfigured` passes
- [x] `TestCheckRequiresBearerWhenAuthConfigured` passes
- [x] `TestMgmtQueryModeActives` passes (and ALL, INACTIVES, BOGUS variants)
- [x] `BLACKLIST_AUTH_URL` unset → no auth check (development mode unchanged)
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on `internal/blacklist/...`

---

## Step 24 — OrchestrationResult missing spec-defined fields (G40)

**Gap addressed:**
- **G40** (result fields) — `ServiceOrchestrationResult` is missing three spec-defined fields:
  - `cloudIdentifier` — which cloud the provider belongs to
  - `exclusiveUntil` — lock expiry timestamp (non-empty when provider is locked)
  - `interfaces[]` — interface definitions forwarded from the ServiceRegistry result

**Prerequisites:** Steps 1–21 complete. Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/orchestration/model/types.go` — add three fields to `OrchestrationResult`
- `core/internal/orchestration/dynamic/service/orchestrator.go` — populate fields
- `core/internal/orchestration/simplestore/service/orchestrator.go` — populate fields
- `core/internal/orchestration/flexiblestore/service/orchestrator.go` — populate fields

**Files to modify (core-evol/):**
- `core-evol/internal/orchestration/types.go` — add same three fields
- `core-evol/internal/orchestration/service.go` — populate fields

---

### System test

Register a service with interfaces `["HTTP-INSECURE-JSON"]`, grant authorization, then
`POST /serviceorchestration/orchestration/pull`. Assert the response result contains
`"cloudIdentifier": "LOCAL"`, `"interfaces": ["HTTP-INSECURE-JSON"]`, and that
`"exclusiveUntil"` is absent or empty (no lock created).

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
    ./internal/orchestration/model/... \
    ./internal/orchestration/dynamic/service/... \
    ./internal/orchestration/simplestore/service/... \
    ./internal/orchestration/flexiblestore/service/...
go tool cover -func=coverage.out

cd core-evol
go test -coverprofile=coverage.out ./internal/orchestration/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all touched packages.

### Documentation updates (after Step 24)

- `core/GAP_ANALYSIS.md` — mark G40 result-fields portion resolved; note QoS evaluation
  still pending G35; add fix summary paragraph
- `core/SPEC.md` — add `cloudIdentifier`, `exclusiveUntil`, `interfaces[]` to
  `OrchestrationResult` shape in Orchestration section
- `CONFORMANCE.md` — update Orchestration conformance rows; move G40 (result fields) to resolved
- `core/EXAMPLES.md` — add example response showing all three fields

### Completion criteria

- [x] `TestOrchestrationResultHasCloudIdentifier` passes (all three orchestrators)
- [x] `TestOrchestrationResultHasExclusiveUntilWhenLocked` passes (DynamicOrch)
- [x] `TestOrchestrationResultNoExclusiveUntilWhenUnlocked` passes
- [x] `TestOrchestrationResultForwardsInterfaces` passes (all three orchestrators)
- [x] core-evol: same field set populated in `dynamicorch-xacml` results
- [x] `go test -race ./...` from `core/` passes
- [x] `cd core-evol && go test -race ./...` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 25 — Intercloud flags return 501 instead of silent no-op (G25)

**Gap addressed:**
- **G25** (intercloud stubs) — `ALLOW_INTERCLOUD` and `ONLY_INTERCLOUD` orchestration flags
  are parsed and accepted but silently treated as disabled. AH5 clients that set these
  flags receive a valid (but misleading) response with no indication that intercloud
  discovery was not attempted. A `501 Not Implemented` response is the correct signal.

**Prerequisites:** Step 8 complete. Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/orchestration/dynamic/service/orchestrator.go` — detect intercloud flags
- `core/internal/orchestration/dynamic/api/handler.go` — map new error to 501
- `core/internal/orchestration/simplestore/service/orchestrator.go` — same detection
- `core/internal/orchestration/simplestore/api/handler.go` — same 501 mapping

**Files to modify (core-evol/):**
- `core-evol/internal/orchestration/service.go` — detect and surface intercloud flags
- `core-evol/internal/orchestration/handler.go` — map to 501

**New sentinel error:** Define `ErrInterclouNotSupported` in
`core/internal/orchestration/model/errors.go` (or `types.go`). Each orchestrator checks
for `ALLOW_INTERCLOUD` or `ONLY_INTERCLOUD` flags after parsing, returns this error, and
each handler maps it to `http.StatusNotImplemented`.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
    ./internal/orchestration/dynamic/... \
    ./internal/orchestration/simplestore/...
go tool cover -func=coverage.out

cd core-evol
go test -coverprofile=coverage.out ./internal/orchestration/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all modified packages.

### Documentation updates (after Step 25)

- `core/GAP_ANALYSIS.md` — update G25: mark intercloud stub portion resolved; note that
  `ALLOW_INTERCLOUD`/`ONLY_INTERCLOUD` now return 501 instead of silently being no-ops
- `core/SPEC.md` — document that intercloud flags return 501
- `CONFORMANCE.md` — update Orchestration conformance rows; move G25 intercloud portion to resolved
- `core/EXAMPLES.md` — add example showing 501 response body for intercloud flags

### Completion criteria

- [x] `TestAllowInterclouReturns501` passes (DynamicOrch and SimpleStore)
- [x] `TestOnlyInterclouReturns501` passes (DynamicOrch and SimpleStore)
- [x] `TestLocalOrchestrationUnaffectedByInterclouChange` passes
- [x] core-evol: same 501 behaviour in `dynamicorch-xacml`
- [x] `go test -race ./...` from `core/` passes
- [x] `cd core-evol && go test -race ./...` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 26 — Authentication credentials validated as structured object (G43)

**Gap addressed:**
- **G43** — `POST /authentication/identity/login` accepts `credentials` as any JSON value.
  AH5 specifies `credentials` must be a JSON object with at least a `password` key
  (`{"password": "..."}`). Missing or non-object credentials should return 400.

**Prerequisites:** Step 13 complete. Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/authentication/model/types.go` — define `Credentials` struct with
  `Password string` and custom `UnmarshalJSON` that rejects non-objects
- `core/internal/authentication/service/auth.go` — use `Credentials.Password` in
  bcrypt comparison; surface 400-equivalent error if password field absent
- `core/internal/authentication/api/handler.go` — map new validation error to 400

**Files to modify (core-evol/):** None — Authentication is a standalone core service.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
    ./internal/authentication/model/... \
    ./internal/authentication/service/... \
    ./internal/authentication/api/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all three packages.

### Documentation updates (after Step 26)

- `core/GAP_ANALYSIS.md` — mark G43 resolved; add fix summary paragraph
- `core/SPEC.md` — update `POST /authentication/identity/login` request: document
  `credentials` as `{"password": "string"}`; note 400 for malformed credentials
- `CONFORMANCE.md` — update Authentication conformance row; move G43 to resolved in phase plan
- `core/EXAMPLES.md` — update login example to show structured credentials object

### Completion criteria

- [x] `TestLoginMissingPasswordFieldReturns400` passes
- [x] `TestLoginNonObjectCredentialsReturns400` passes
- [x] `TestLoginNullCredentialsReturns400` passes
- [x] `TestLoginValidCredentialsObjectSucceeds` passes
- [x] All existing login/verify/logout tests still pass (no regression)
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on `authentication/model`, `authentication/service`, `authentication/api`

---

## 11. Phase 1 — Documentation updates

After all five Phase 1 steps are complete, apply the following consolidated documentation
changes (individual step updates are listed per-step above):

### `core/GAP_ANALYSIS.md`

| Step | Gap resolved |
|------|-------------|
| 22 | G11 (system revoke identity from token) |
| 23 | G41 (Blacklist Bearer + mode enum) |
| 24 | G40 result fields (cloudIdentifier, exclusiveUntil, interfaces[]) |
| 25 | G25 intercloud stubs (ALLOW_INTERCLOUD / ONLY_INTERCLOUD → 501) |
| 26 | G43 (credentials structured object) |

### `CONFORMANCE.md`

Move all five gaps from Open → Resolved in the phase plan table. Update per-system
conformance ratings (ServiceRegistry, Blacklist, Orchestration, Authentication).

### `core/SPEC.md`

- `DELETE /system-discovery/revoke` — remove `?name=` as primary; document Bearer + deprecated fallback
- Blacklist `GET /lookup`, `GET /check/{name}` — add Bearer requirement; document `BLACKLIST_AUTH_URL`
- Blacklist `POST /mgmt/query` — replace `active *bool` with `mode` string enum
- Orchestration `OrchestrationResult` — add `cloudIdentifier`, `exclusiveUntil`, `interfaces[]`
- Orchestration request flags — document 501 for `ALLOW_INTERCLOUD`/`ONLY_INTERCLOUD`
- Authentication `POST /login` — update credentials shape to `{"password": "string"}`

### `README.md`

Add to the Configuration section:
- `SR_AUTH_URL` — ServiceRegistry auth URL for system revoke identity check
- `BLACKLIST_AUTH_URL` — Blacklist auth URL for discovery Bearer enforcement

---

## 12. Phase 1 — Regression matrix

Run after all five Phase 1 steps are complete:

| Check | Steps |
|---|---|
| `cd core && go build ./...` | All Phase 1 |
| `cd core && go vet ./...` | All Phase 1 |
| `cd core && go test -race ./...` | All Phase 1 |
| `bash core/test-system.sh` | All Phase 1 |
| `cd core-evol && go build ./... && go test -race ./...` | 24, 25 |
| `go build ./...` (workspace root) | All Phase 1 |

---

---

# Phase 2 — Functional Completeness

**Scope:** Steps 27–32. Addresses all six remaining Phase 2 gaps from `CONFORMANCE.md`.
**Order:** Steps 27 and 28 are Blockers for Production and must be done first. Steps 29–31 are independent of each other once Step 28 is done. Step 32 is the documentation sweep after all implementation steps are complete.

**TDD cycle and coverage standard** are the same as Phase 1 (see sections 2 and 3 above).

---

## Step 27 — Management access policy (G37)

**Gap addressed:**
- **G37** — All management endpoints (`/mgmt/*`) on all eight core systems are open to any
  caller on the network. AH5 specifies three access-control modes: `sysop-only`,
  `whitelist`, and `authorization`. This step implements `sysop-only` (the default and
  most critical mode): every management request must carry a valid `Authorization: Bearer`
  token whose identity resolves to the sysop account.

**Why now:** Blocker for Production. An unauthenticated management API allows any network
peer to read, create, or delete records in any running system. This is the highest-priority
remaining gap.

**Prerequisites:** Step 13 complete (Authentication management endpoints and bcrypt
credential verification exist). Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/httputil/respond.go` — add `RequireManagementAuth(w, r, authURL string) bool` helper
- `core/cmd/serviceregistry/main.go` — read `MGMT_AUTH_URL`; pass to handler
- `core/cmd/authentication/main.go` — same
- `core/cmd/consumerauth/main.go` — same
- `core/cmd/dynamicorch/main.go` — same
- `core/cmd/simplestoreorch/main.go` — same
- `core/cmd/flexiblestoreorch/main.go` — same
- `core/cmd/ca/main.go` — same
- `core/cmd/blacklist/main.go` (if exists; else `cmd/serviceregistry/main.go` hosts Blacklist) — same
- Every `api/handler.go` for each system — call `httputil.RequireManagementAuth` at the top of every mgmt route handler
- `core/internal/api/ah5_handler.go` — apply to AH5 mgmt routes
- `core/internal/generalmgmt/handler.go` — apply to all routes (all are mgmt)

**Files to modify (core-evol/):**
- `core-evol/internal/orchestration/handler.go` — apply `RequireManagementAuth` to every `/mgmt/*` route (`mgmt/push/*`, `mgmt/locks/*`, `mgmt/history/*`)

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `MGMT_AUTH_URL` | *(unset)* | When set, all `/mgmt/*` requests must carry `Authorization: Bearer <token>` verified via this Authentication system URL. Unset = open access (development/PoC mode). |

The URL is verified by calling `POST <MGMT_AUTH_URL>/authentication/identity/verify` with the
bearer token. A 200 response means the token is valid; any other response (including network
error) must reject the request with 401. This is fail-closed (same pattern as D4 and D8).

---

### System test

After setting `MGMT_AUTH_URL=http://localhost:8081` in all binaries, start the full stack.
Call `POST /serviceregistry/mgmt/system-discovery` (or any mgmt endpoint) without a token —
assert 401. Log in as sysop, extract the token, call the same endpoint with
`Authorization: Bearer <token>` — assert 200/201. Repeat for at least one mgmt endpoint on
Authentication, ConsumerAuthorization, and DynamicOrchestration.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/httputil/... ./internal/api/... \
  ./internal/authentication/api/... ./internal/consumerauth/api/... \
  ./internal/orchestration/dynamic/api/... ./internal/orchestration/simplestore/api/... \
  ./internal/orchestration/flexiblestore/api/... ./internal/generalmgmt/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on every modified package.

### Documentation updates (after Step 27)

- `core/GAP_ANALYSIS.md` — mark G37 resolved; add implementation summary (env var, fail-closed semantics, which handler files)
- `core/SPEC.md` — add `MGMT_AUTH_URL` to Configuration section; document that all `/mgmt/*` endpoints require Bearer when env var is set
- `README.md` — add `MGMT_AUTH_URL` to Configuration table (all systems)
- `CONFORMANCE.md` — move G37 from Open → Resolved; update per-system ratings (all systems gain Behavior% points)

### Completion criteria

- [x] `TestMgmtRequiresBearerWhenAuthURLSet` passes (one representative handler per system)
- [x] `TestMgmtOpenWhenAuthURLUnset` passes (default dev mode)
- [x] `TestMgmtAuthUnreachableReturns401` passes (fail-closed)
- [x] `TestMgmtValidSysopTokenSucceeds` passes
- [x] All existing management endpoint tests still pass with `MGMT_AUTH_URL` unset
- [x] `core-evol` mgmt routes return 401 without Bearer when `MGMT_AUTH_URL` set
- [x] `go test -race ./...` from `core/` passes
- [x] `cd core-evol && go test -race ./...` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 28 — Blacklist integration (G42)

**Gap addressed:**
- **G42** — The Blacklist system exists but no other system consults it. AH5 requires:
  - **ServiceRegistry** rejects `register` from a blacklisted system
  - **Orchestration** excludes blacklisted providers from results
  - **ConsumerAuthorization** rejects `grant` and `verify` for blacklisted consumers or providers

**Why now:** Blocker for Production. The Blacklist is currently decorative — a system on the
deny-list can still register, be orchestrated, and obtain grants. This step wires it in.

**Prerequisites:** Step 28 (this step) requires the Blacklist system to be running (G28
complete, Step 20) and `BlacklistClient` to be defined. Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/blacklist/client/client.go` (new file) — define `BlacklistClient` interface
  with `IsBlacklisted(ctx context.Context, name string) (bool, error)` and
  `HTTPBlacklistClient` struct implementing it
- `core/internal/api/ah5_handler.go` — call `IsBlacklisted` at the start of
  `handleServiceRegister`; return 403 if blacklisted
- `core/internal/orchestration/dynamic/service/orchestrator.go` — filter result list:
  remove any provider whose `providerName` is blacklisted
- `core/internal/orchestration/simplestore/service/orchestrator.go` — same filter
- `core/internal/orchestration/flexiblestore/service/orchestrator.go` — same filter
- `core/internal/consumerauth/service/auth.go` — call `IsBlacklisted` in `Grant` and
  `Verify`; return error (→ 403) if consumer or provider is blacklisted
- `core/cmd/serviceregistry/main.go` — read `BLACKLIST_URL`; wire `HTTPBlacklistClient`
- `core/cmd/dynamicorch/main.go` — same
- `core/cmd/simplestoreorch/main.go` — same
- `core/cmd/flexiblestoreorch/main.go` — same
- `core/cmd/consumerauth/main.go` — same

**Files to modify (core-evol/):**
- `core-evol/internal/orchestration/service.go` — add `BlacklistClient` field to
  `XACMLOrchestrator`; filter results in `Orchestrate()` method after XACML decision,
  before returning results; wire from `main.go`
- `core-evol/cmd/dynamicorch-xacml/main.go` — read `BLACKLIST_URL`; construct and inject
  `HTTPBlacklistClient`

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `BLACKLIST_URL` | *(unset)* | When set, SR register, Orchestration result filter, and ConsumerAuth grant/verify consult the Blacklist. Unset = no Blacklist checks (development/PoC mode). Fail-closed: unreachable Blacklist is treated as blacklisted. |

Fail-closed semantics: a network error or non-200 response from the Blacklist is treated as
`IsBlacklisted = true`. This is consistent with D4 and D8.

---

### System test

Start the full stack with `BLACKLIST_URL=http://localhost:XXXX`. Add a system (`sensorX`) to
the Blacklist. Then:
1. `POST /serviceregistry/service-discovery/register` with `providerSystem.systemName=sensorX` → assert 403
2. Register a different provider, then add it to the Blacklist, then call orchestration → assert `sensorX` is absent from results
3. Call `POST /consumerauthorization/authorization/grant` with a blacklisted consumer → assert 403

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
  ./internal/blacklist/client/... \
  ./internal/api/... \
  ./internal/orchestration/dynamic/service/... \
  ./internal/orchestration/simplestore/service/... \
  ./internal/orchestration/flexiblestore/service/... \
  ./internal/consumerauth/service/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on every modified package.

### Documentation updates (after Step 28)

- `core/GAP_ANALYSIS.md` — mark G42 resolved; add implementation summary (which systems, fail-closed, `BLACKLIST_URL`)
- `core/SPEC.md` — add Blacklist integration behavior to ServiceRegistry register, Orchestration pull, and ConsumerAuth grant/verify sections; document `BLACKLIST_URL`
- `README.md` — add `BLACKLIST_URL` to Configuration table
- `CONFORMANCE.md` — move G42 from Open → Resolved; update per-system ratings (SR, ConsumerAuth, Orchestration all gain Behavior% points; Blacklist Behavior% increases)

### Completion criteria

- [x] `TestRegisterBlacklistedSystemReturns403` passes
- [x] `TestOrchestrationExcludesBlacklistedProvider` passes (DynamicOrch)
- [x] `TestSimpleStoreExcludesBlacklistedProvider` passes
- [x] `TestConsumerAuthGrantBlacklistedConsumerReturns403` passes
- [x] `TestConsumerAuthGrantBlacklistedProviderReturns403` passes
- [x] `TestConsumerAuthVerifyBlacklistedReturns403` passes
- [x] `TestBlacklistUnreachableFails closed` passes (returns 403/error when Blacklist down)
- [x] `TestBlacklistURLUnsetSkipsCheck` passes (no calls to Blacklist when env var unset)
- [x] core-evol: `TestXACMLOrchestratorExcludesBlacklistedProvider` passes
- [x] `go test -race ./...` from `core/` passes
- [x] `cd core-evol && go test -race ./...` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 29 — Pagination on query/list endpoints (G20)

**Gap addressed:**
- **G20** — All query and list operations return unbounded result sets. AH5 requires a
  `pagination` object (`page`, `size`, `direction`, `sortField`) in requests and bounded,
  offset-based result pages in responses.

**Why now:** High-impact for any non-trivial deployment. Implementing pagination before bulk
endpoints (Step 30) means bulk endpoints can reuse the same helper.

**Prerequisites:** Steps 27–28 complete. Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/httputil/paginate.go` (new file) — generic `Paginate[T any](items []T, page, size int, direction string, less func(T, T) bool) []T` helper; if `size ≤ 0`, return all items (backward-compatible default)
- `core/internal/api/ah5_handler.go` — apply `Paginate` in service/system/device list handlers
- `core/internal/authentication/api/handler.go` — apply in identity/session query handlers
- `core/internal/consumerauth/api/handler.go` — apply in authorization lookup handler
- `core/internal/orchestration/dynamic/api/handler.go` — apply in push-query and lock-query handlers
- `core/internal/orchestration/simplestore/api/handler.go` — apply in rule-list handler
- `core/internal/orchestration/flexiblestore/api/handler.go` — apply in rule-list handler
- `core/internal/blacklist/api/handler.go` — apply in mgmt/query handler
- `core/internal/generalmgmt/handler.go` — apply in list handlers

**Files to modify (core-evol/):**
- `core-evol/internal/orchestration/handler.go` — apply `Paginate` in
  `mgmt/push/query`, `mgmt/locks/query`, and `mgmt/history/query` handlers. Import
  `httputil` from core module is not allowed (separate module); copy or vendor the helper
  into `core-evol/internal/httputil/paginate.go`.

**New environment variable:** None.

**Pagination request shape** (extend existing request bodies):

```json
{
  "pagination": {
    "page":       0,
    "size":       20,
    "direction":  "ASC",
    "sortField":  "id"
  }
}
```

`page` and `size` default to 0 / no-limit when absent (backward-compatible).
Response must include a `count` field (total items before pagination) alongside `data` array.

---

### System test

Register 25 services in ServiceRegistry. Call the service-discovery list endpoint with
`"pagination": {"page": 0, "size": 10}` — assert exactly 10 results returned and
`count == 25`. Call with `page: 1` — assert next 10. Call with `page: 2` — assert 5.
Call without pagination object — assert all 25 returned (backward-compatible).

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/httputil/... ./internal/api/... \
  ./internal/authentication/api/... ./internal/consumerauth/api/... \
  ./internal/orchestration/dynamic/api/... ./internal/orchestration/simplestore/api/... \
  ./internal/orchestration/flexiblestore/api/... ./internal/blacklist/api/... \
  ./internal/generalmgmt/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on every modified package.

### Documentation updates (after Step 29)

- `core/GAP_ANALYSIS.md` — mark G20 resolved; add implementation summary (`Paginate[T]` helper, backward-compatible default)
- `core/SPEC.md` — add pagination object to all query endpoint request shapes; add `count` field to all list response shapes
- `core/EXAMPLES.md` — add Example 6: paginated service list request and response
- `CONFORMANCE.md` — move G20 from Open → Resolved; update per-system ratings (all systems gain Endpoint% and Behavior% points)

### Completion criteria

- [x] `TestPaginateHelper` covers page/size/direction/sortField combinations
- [x] `TestServiceListPaginationPage0Size10Returns10Of25` passes
- [x] `TestServiceListPaginationPage2Returns5Of25` passes
- [x] `TestServiceListNoPaginationReturnsAll` passes (backward-compatible)
- [x] `TestAuthIdentityQueryPagination` passes
- [x] At least one pagination test per system handler package
- [x] `go test -race ./...` from `core/` passes
- [x] `cd core-evol && go test -race ./...` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 30 — ConsumerAuth bulk management endpoints (G38, G39)

**Gap addressed:**
- **G39** — `authorizationManagement` bulk endpoints absent: `mgmt/grant-policies`,
  `mgmt/revoke-policies`, `mgmt/query-policies`, `mgmt/check-policies`
- **G38** — `authorizationTokenManagement` bulk endpoints absent: `mgmt/generate-tokens`,
  `mgmt/revoke-tokens`, `mgmt/query-tokens`, `mgmt/add-encryption-keys`,
  `mgmt/remove-encryption-keys`

**Why now:** High-impact for operator tooling and production incident response. Both G38 and
G39 are in the same system and handler file; doing them together avoids two separate passes.

**Prerequisites:** Steps 27–29 complete (pagination helper available). Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/consumerauth/api/handler.go` — add new route handlers:
  - `POST /consumerauthorization/authorization/mgmt/grant-policies`
  - `DELETE /consumerauthorization/authorization/mgmt/revoke-policies`
  - `POST /consumerauthorization/authorization/mgmt/query-policies`
  - `POST /consumerauthorization/authorization/mgmt/check-policies`
  - `POST /consumerauthorization/authorization-token/mgmt/generate-tokens`
  - `DELETE /consumerauthorization/authorization-token/mgmt/revoke-tokens`
  - `POST /consumerauthorization/authorization-token/mgmt/query-tokens`
  - `POST /consumerauthorization/authorization-token/mgmt/add-encryption-keys`
  - `DELETE /consumerauthorization/authorization-token/mgmt/remove-encryption-keys`
- `core/internal/consumerauth/service/auth.go` — add bulk service methods if not already
  present (most delegate to existing single-item logic in a loop; `QueryPolicies` and
  `CheckPolicies` are new query paths)
- `core/internal/consumerauth/repository/memory.go` — add `QueryPolicies` and `CheckPolicies`
  repository methods with filter support

**Files to modify (core-evol/):** None — ConsumerAuthorization is a standalone core system
not present in core-evol.

**New environment variable:** None.

**Request/response shapes:**

`POST mgmt/grant-policies` body: `{"policies": [{"consumerSystemName": "...", "providerSystemName": "...", "serviceDefinition": "..."}]}`
Response: list of created policy IDs with any per-item errors.

`POST mgmt/query-policies` body: pagination object + optional filters (`consumerSystemName`, `providerSystemName`, `serviceDefinition`).

`POST mgmt/check-policies` body: list of tuples; response: same list with `authorized: bool` per tuple.

`POST mgmt/generate-tokens` body: list of `{consumerSystemName, providerSystemName, serviceDefinition}`; response: list of tokens.

---

### System test

Grant five policies via individual endpoint. Then call `POST mgmt/query-policies` — assert
all five appear. Call `POST mgmt/check-policies` with three of the five plus two
non-existent — assert three `authorized: true`, two `authorized: false`.
Call `DELETE mgmt/revoke-policies` with all five IDs — assert all revoked.
Repeat token-side: `POST mgmt/generate-tokens` with three tuples, assert three tokens;
`DELETE mgmt/revoke-tokens` with all three, assert 200.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
  ./internal/consumerauth/api/... \
  ./internal/consumerauth/service/... \
  ./internal/consumerauth/repository/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on every modified package.

### Documentation updates (after Step 30)

- `core/GAP_ANALYSIS.md` — mark G38 and G39 resolved; add implementation summary
- `core/SPEC.md` — add all nine new endpoint definitions under ConsumerAuthorization
- `CONFORMANCE.md` — move G38 and G39 from Open → Resolved; update ConsumerAuthorization rating (Endpoint% rises significantly)

### Completion criteria

- [x] `TestBulkGrantPoliciesCreatesAll` passes
- [x] `TestBulkRevokePoliciesRemovesAll` passes
- [x] `TestQueryPoliciesWithFilters` passes
- [x] `TestCheckPoliciesMixedResult` passes
- [x] `TestBulkGenerateTokensReturnsTokenList` passes
- [x] `TestBulkRevokeTokensRevokesAll` passes
- [x] `TestQueryTokensWithPagination` passes
- [x] All existing single-item endpoint tests still pass (no regression)
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 31 — Push notification HTTP delivery (G26)

**Gap addressed:**
- **G26** (delivery sub-gap) — The `mgmt/push/trigger` endpoint records a `PUSH/PENDING`
  history entry but makes no HTTP call to the subscriber's `notifyInterface` address.
  AH5 requires the orchestrator to POST the matching orchestration results to the
  subscriber's callback URL when a trigger fires.

**Why now:** High-impact for Prototyping and Production. The push model is one of the two
AH5 orchestration styles; without actual delivery the subscribe/trigger machinery is
non-functional from a client's perspective.

**Prerequisites:** Step 29 complete (pagination available for delivery response body).
Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/orchestration/dynamic/service/orchestrator.go` — add `deliverPush` private
  method: look up all active subscriptions for the triggered service, perform
  `POST <notifyInterface>` with the orchestration result payload, update history entry to
  `DELIVERED` or `FAILED` based on HTTP response. Use a goroutine per delivery to avoid
  blocking the trigger handler.
- `core/internal/orchestration/simplestore/service/orchestrator.go` — same pattern
  (SimpleStore also supports subscribe/trigger per Step 19)
- `core/internal/orchestration/dynamic/service/orchestrator_test.go` — use `httptest.Server`
  as the subscriber endpoint to assert delivery

**Files to modify (core-evol/):**
- `core-evol/internal/orchestration/service.go` — add `deliverPush` to
  `XACMLOrchestrator` trigger path; same goroutine pattern and history update

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `PUSH_DELIVERY_TIMEOUT_SECONDS` | `5` | HTTP timeout for each push notification delivery attempt |

**Delivery semantics:**
- Fire-and-forget with timeout — if the subscriber endpoint does not respond within
  `PUSH_DELIVERY_TIMEOUT_SECONDS`, mark history as `FAILED` and continue.
- No retry. A failed delivery is recorded in orchestration history as `PUSH/FAILED`.
- A successful delivery (HTTP 2xx) is recorded as `PUSH/DELIVERED`.

---

### System test

Start an `httptest.Server` (or a minimal test HTTP server) as the subscriber. Subscribe to
a service. Trigger a push. Assert that the test server received exactly one POST with the
expected orchestration result payload. Assert history shows `PUSH/DELIVERED`.
Also: trigger push with subscriber at an unreachable address — assert history shows
`PUSH/FAILED` within `PUSH_DELIVERY_TIMEOUT_SECONDS + 1` seconds.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
  ./internal/orchestration/dynamic/service/... \
  ./internal/orchestration/simplestore/service/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on both service packages.

### Documentation updates (after Step 31)

- `core/GAP_ANALYSIS.md` — mark G26 fully resolved; describe delivery semantics (goroutine,
  timeout, DELIVERED/FAILED history states, no retry)
- `core/SPEC.md` — update push trigger behavior: document that trigger now POSTs results to
  `notifyInterface`; add `PUSH_DELIVERY_TIMEOUT_SECONDS`; document DELIVERED/FAILED history
  states
- `README.md` — add `PUSH_DELIVERY_TIMEOUT_SECONDS` to DynamicOrchestration Configuration table
- `CONFORMANCE.md` — move G26 delivery from Open → Resolved; update DynamicOrchestration
  and SimpleStoreOrchestration Behavior% ratings

### Completion criteria

- [x] `TestPushTriggerDeliversToPushSubscriber` passes (real HTTP delivery via httptest.Server)
- [x] `TestPushDeliveryFailureRecordedInHistory` passes (unreachable subscriber → FAILED)
- [x] `TestPushDeliveryTimeoutRespected` passes (slow subscriber → FAILED before timeout+1s)
- [x] `TestPushTriggerDoesNotBlockHandler` passes (handler returns 200 before goroutines complete)
- [x] `TestSimpleStorePushDelivery` passes (same for SimpleStore)
- [x] core-evol: `TestXACMLOrchestratorPushDelivery` passes
- [x] `go test -race ./...` from `core/` passes
- [x] `cd core-evol && go test -race ./...` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on `orchestration/dynamic/service` and `orchestration/simplestore/service`

---

## Step 32 — Phase 2 documentation update

**Purpose:** Consolidated documentation sweep after all five implementation steps (27–31) are
complete. Ensures every authoritative document reflects the final implemented state.

**Prerequisites:** Steps 27–31 all complete and passing. Pre-flight check passes.

**No code changes.** This step is documentation only.

### `core/GAP_ANALYSIS.md`

Each of the five resolved gaps must have:
- A `**Status: Resolved in Step N**` line at the top of the gap section
- An implementation summary paragraph (env var(s), which handler/service files, key design decision)

Gaps to update: G37, G42, G20, G38, G39, G26.

### `CONFORMANCE.md`

1. Move all six gaps (G37, G42, G20, G38, G39, G26) from **Open Gaps** table → **Resolved Gaps** table with Step numbers 27–31.
2. Update **Per-System Ratings** for all affected systems:
   - ServiceRegistry: Behavior% +5 (Blacklist integration), Endpoint% +3 (pagination)
   - Authentication: Behavior% +3 (mgmt access policy)
   - ConsumerAuthorization: Endpoint% +15 (G38+G39 bulk endpoints), Behavior% +5 (G37 mgmt + G42 blacklist check)
   - DynamicOrchestration: Behavior% +8 (G37 mgmt, G42 filter, G26 delivery)
   - SimpleStoreOrchestration: Behavior% +8 (same)
   - FlexibleStoreOrchestration: Behavior% +5 (G37, G42)
   - Blacklist: Behavior% +10 (G42 integration — Blacklist actually blocks now)
   - GeneralManagement: Behavior% +5 (G37)
3. Update Phase Plan: mark Phase 2 as **Complete**.
4. Update **last updated** timestamp.

### `CONFORMANCE_UPDATE_PLAN.md`

1. Tick all completion criteria checkboxes in Steps 27–31.
2. Add Phase 2 regression matrix (see below) as section 13.

### `core/SPEC.md`

- Add `MGMT_AUTH_URL` config to all system sections (G37)
- Add `BLACKLIST_URL` config to SR, Orchestration, ConsumerAuth sections (G42)
- Add `PUSH_DELIVERY_TIMEOUT_SECONDS` to DynamicOrchestration section (G31)
- Add pagination object to all query request shapes; add `count` to all list responses (G20)
- Add nine new ConsumerAuth bulk endpoints (G38, G39)
- Update push-trigger behavior to document actual HTTP delivery (G26)

### `core/EXAMPLES.md`

- Add Example 6: paginated service list request and response
- Add Example 7: `mgmt/grant-policies` bulk request and response
- Add Example 8: `mgmt/check-policies` mixed result
- Add Example 9: push trigger — delivery confirmed in history

### `README.md`

- Add `MGMT_AUTH_URL` to the Configuration table (all systems section)
- Add `BLACKLIST_URL` to the Configuration table (SR, Orchestration, ConsumerAuth)
- Add `PUSH_DELIVERY_TIMEOUT_SECONDS` to DynamicOrchestration section

### Completion criteria

- [x] All six gaps (G37, G42, G20, G38, G39, G26) appear in the Resolved Gaps table in `CONFORMANCE.md`
- [x] None of the six gaps remain in the Open Gaps table in `CONFORMANCE.md`
- [x] Per-system ratings updated in `CONFORMANCE.md`
- [x] Phase Plan row for Phase 2 shows **Complete**
- [x] All Steps 27–31 completion criteria checkboxes are `[x]`
- [x] All new env vars documented in `core/SPEC.md` and `README.md`
- [x] All new endpoints documented in `core/SPEC.md`
- [x] `core/EXAMPLES.md` updated with at least one example per major new feature
- [x] `core/GAP_ANALYSIS.md` shows resolved status for G20, G26, G37, G38, G39, G42

---

## 13. Phase 2 — Regression matrix

Run after all Phase 2 steps are complete (Steps 27–31):

| Check | Steps |
|---|---|
| `cd core && go build ./...` | All Phase 2 |
| `cd core && go vet ./...` | All Phase 2 |
| `cd core && go test -race ./...` | All Phase 2 |
| `bash core/test-system.sh` | All Phase 2 |
| `cd core-evol && go build ./... && go test -race ./...` | 27, 28, 31 |
| `go build ./...` (workspace root) | All Phase 2 |

---

---

# Phase 3 — Advanced Conformance

**Scope:** Steps 33–39. Addresses all remaining Phase 3 gaps from `CONFORMANCE.md`.
**Order:** Step 33 is a Production blocker and must be done first. Steps 34 and 35 are independent of each other and of Step 33. Step 36 requires Step 35 (Device QoS Evaluator must exist before QoS filtering). Steps 37 and 38 are independent of all other steps. Step 38 is the highest-effort item; do it last. Step 39 is the documentation sweep after all implementation steps are complete.

**TDD cycle and coverage standard** are the same as Phase 1 (see sections 2 and 3 above).

---

## Step 33 — Registration identity enforcement (G10)

**Gap addressed:**
- **G10** — `POST /serviceregistry/system-discovery/register` and
  `POST /serviceregistry/service-discovery/register` derive the registrant name from the
  request body. AH5 requires the name to be derived from the caller's verified identity
  (Bearer token). Any caller can currently register under any name, making self-registration
  unenforceable.

**Why first:** Production blocker. All the authentication infrastructure needed (G2, G11,
G37) is already in place. This is the final gap that must close before a deployment can
enforce identity-based access.

**Prerequisites:** Steps 13 (bcrypt credential verification) and 27 (management auth)
complete. `MGMT_AUTH_URL` pattern established. Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/httputil/respond.go` — add
  `VerifyTokenIdentity(r *http.Request, authURL, claimedName string) (bool, int)` helper:
  extracts Bearer token, calls `GET <authURL>/authentication/identity/verify/<token>`,
  asserts the returned `systemName` equals `claimedName`. Returns `(true, 0)` on match,
  `(false, 401)` on missing token, `(false, 403)` on mismatch or non-sysop identity,
  `(false, 401)` on network error (fail-closed).
- `core/internal/api/ah5_handler.go` — call `httputil.VerifyTokenIdentity` at the top of
  `handleSystemRegister` (check against request `name`) and `handleServiceRegister` (check
  against `providerSystem.systemName`). Only active when `registerAuthURL` is non-empty.
- `core/cmd/serviceregistry/main.go` — read `REGISTER_AUTH_URL`; pass to `NewAH5Handler`.

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `REGISTER_AUTH_URL` | *(unset)* | When set, system and service registration require `Authorization: Bearer <token>` whose verified `systemName` matches the `name`/`providerSystem.systemName` in the request body. Unset = open registration (development/PoC mode). Fail-closed: network error → 401. |

---

### TDD cycle 33.1 — Matching token identity allows registration

**Test:** `TestRegisterSystemMatchingTokenIdentitySucceeds`

Fake Authentication server returns `{"systemName": "SensorA", "verified": true}`.
`POST /serviceregistry/system-discovery/register` with `"name": "SensorA"` and
`Authorization: Bearer <valid-token>` and `registerAuthURL` set → assert 201.

**Expected failure before implementation:** 201 without any token check.

---

### TDD cycle 33.2 — Mismatched identity returns 403

**Test:** `TestRegisterSystemMismatchedIdentityReturns403`

Fake Authentication server returns `{"systemName": "SensorA", "verified": true}`.
Request body has `"name": "SensorB"` → assert 403.

**Expected failure before implementation:** 201 (no name check).

---

### TDD cycle 33.3 — Missing Bearer returns 401

**Test:** `TestRegisterSystemMissingBearerWithAuthURLReturns401`

`REGISTER_AUTH_URL` set, no `Authorization` header → assert 401.

---

### TDD cycle 33.4 — Auth unreachable is fail-closed

**Test:** `TestRegisterSystemAuthUnreachableReturns401`

`REGISTER_AUTH_URL` points to a non-listening port → assert 401.

---

### TDD cycle 33.5 — No auth URL means open registration

**Test:** `TestRegisterSystemNoAuthURLIsOpen`

`REGISTER_AUTH_URL` unset, no token in request → assert 201 (dev-mode backward-compat).

---

### TDD cycle 33.6 — Service registration enforced identically

**Test:** `TestRegisterServiceMatchingTokenIdentitySucceeds` and
`TestRegisterServiceMismatchedIdentityReturns403`

Same pattern as 33.1–33.2, applied to `handleServiceRegister` checking
`providerSystem.systemName`.

---

### System test

Start the full stack with `REGISTER_AUTH_URL=http://localhost:8081`. Log in as `SensorA`.
Call `POST /serviceregistry/system-discovery/register` with `"name": "SensorA"` and
the token → assert 201. Call again with `"name": "SensorB"` → assert 403. Call without
any token → assert 401. Restart with `REGISTER_AUTH_URL` unset → assert 201 without
any token (backward-compatible).

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
  ./internal/httputil/... \
  ./internal/api/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on every modified package.

### Documentation updates (after Step 33)

- `core/GAP_ANALYSIS.md` — mark G10 resolved; add implementation summary
- `core/SPEC.md` — add `REGISTER_AUTH_URL` to ServiceRegistry configuration; document
  identity enforcement on register endpoints
- `README.md` — add `REGISTER_AUTH_URL` to Configuration table (ServiceRegistry)
- `CONFORMANCE.md` — move G10 from Open → Resolved; update ServiceRegistry Behavior%

### Completion criteria

- [x] `TestRegisterSystemMatchingTokenIdentitySucceeds` passes
- [x] `TestRegisterSystemMismatchedIdentityReturns403` passes
- [x] `TestRegisterSystemMissingBearerWithAuthURLReturns401` passes
- [x] `TestRegisterSystemAuthUnreachableReturns401` passes
- [x] `TestRegisterSystemNoAuthURLIsOpen` passes
- [x] `TestRegisterServiceMatchingTokenIdentitySucceeds` passes
- [x] `TestRegisterServiceMismatchedIdentityReturns403` passes
- [x] All existing ServiceRegistry handler tests still pass (no regression)
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 34 — Token variants (G23)

**Gap addressed:**
- **G23** — ConsumerAuthorization implements only `TIME_LIMITED_TOKEN`. The AH5 spec
  defines five additional variants. This step implements two feasible variants:
  `USAGE_LIMITED_TOKEN` (counter-based: each verify decrements a usage counter; token
  expires when counter reaches zero) and `BASE64_SELF_CONTAINED` (HMAC-signed JSON
  payload; verifiable without server state). JWT variants and `TRANSLATION_BRIDGE_TOKEN`
  remain `501 Not Implemented`.

**Why now:** Medium-priority for Prototyping. Independent of all other Phase 3 steps.
Implementing two variants without external dependencies is achievable at low risk.

**Prerequisites:** Step 15 complete (TIME_LIMITED_TOKEN and token endpoint scaffolding
exist). Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/consumerauth/model/types.go` — add `MaxUsageCount int` to token storage
  model; add `UsageCount int` field for tracking
- `core/internal/consumerauth/repository/memory.go` — add `IncrementUsage(token string) (int, error)` and `GetToken(token string)` methods
- `core/internal/consumerauth/repository/sqlite.go` — same, persisted
- `core/internal/consumerauth/service/auth.go` — implement `GenerateUsageLimited`
  (store token with `MaxUsageCount`, `UsageCount=0`); implement `VerifyUsageLimited`
  (increment counter, reject if `UsageCount >= MaxUsageCount`); implement
  `GenerateBase64SelfContained` (HMAC-SHA256 over JSON payload with `HMAC_SECRET` env
  var); implement `VerifyBase64SelfContained` (decode and verify HMAC without any
  repository lookup)
- `core/internal/consumerauth/api/handler.go` — route `USAGE_LIMITED_TOKEN` and
  `BASE64_SELF_CONTAINED` token types to new service methods; keep JWT variants as 501

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `HMAC_SECRET` | `arrowhead-default-secret` | 32-byte secret used to sign `BASE64_SELF_CONTAINED` tokens. Must be set to a strong random value in production. |

---

### TDD cycle 34.1 — USAGE_LIMITED token generated and counted

**Test:** `TestUsageLimitedTokenGenerated`

`POST /consumerauthorization/authorization-token/generate` with
`"tokenType": "USAGE_LIMITED_TOKEN"`, `"maxUsageCount": 3` → assert 201, token returned.

---

### TDD cycle 34.2 — USAGE_LIMITED token decrements on verify

**Test:** `TestUsageLimitedTokenDecrementsOnVerify`

Generate a token with `maxUsageCount=3`. Call
`GET /consumerauthorization/authorization-token/verify/<token>` three times → all return
200. Fourth call → assert 403.

**Expected failure before implementation:** Four calls all return 200 (no counter).

---

### TDD cycle 34.3 — Exhausted USAGE_LIMITED token is permanently invalid

**Test:** `TestUsageLimitedTokenExpiredAfterMaxUsage`

After the fourth (failing) verify call, attempt a fifth → assert 403 (not a transient error).

---

### TDD cycle 34.4 — BASE64_SELF_CONTAINED token generated

**Test:** `TestBase64SelfContainedTokenGenerated`

`POST .../generate` with `"tokenType": "BASE64_SELF_CONTAINED"` → assert 201, opaque
base64 token returned.

---

### TDD cycle 34.5 — BASE64_SELF_CONTAINED token verifiable without stored state

**Test:** `TestBase64SelfContainedTokenVerifiable`

Generate a token, then call verify → assert 200. Restart the in-memory service (discard
all stored tokens), call verify again → assert 200 (self-contained: no stored state needed).

**Expected failure before implementation:** Second verify returns 404 (token not found in store).

---

### TDD cycle 34.6 — JWT variants still return 501

**Test:** `TestJWTVariantReturns501` (regression guard)

`POST .../generate` with `"tokenType": "RSA_SHA256_JSON_WEB_TOKEN"` → assert 501.
Must still pass after implementing the two new variants.

---

### System test

With the stack running, generate a `USAGE_LIMITED_TOKEN` with `maxUsageCount=2`. Call
verify twice → both 200. Call a third time → 403. Generate a `BASE64_SELF_CONTAINED`
token; call verify → 200.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
  ./internal/consumerauth/model/... \
  ./internal/consumerauth/repository/... \
  ./internal/consumerauth/service/... \
  ./internal/consumerauth/api/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on every modified package.

### Documentation updates (after Step 34)

- `core/GAP_ANALYSIS.md` — update G23 from partial to "Resolved for USAGE_LIMITED and BASE64_SELF_CONTAINED; JWT variants remain 501"
- `core/SPEC.md` — document `maxUsageCount` field in generate request; document `HMAC_SECRET` env var; update token type table
- `core/EXAMPLES.md` — add example: USAGE_LIMITED token generation and exhaustion
- `CONFORMANCE.md` — update G23 open gap description; update ConsumerAuthorization ratings

### Completion criteria

- [x] `TestUsageLimitedTokenGenerated` passes
- [x] `TestUsageLimitedTokenDecrementsOnVerify` passes
- [x] `TestUsageLimitedTokenExpiredAfterMaxUsage` passes
- [x] `TestBase64SelfContainedTokenGenerated` passes
- [x] `TestBase64SelfContainedTokenVerifiable` passes (both before and after in-memory reset)
- [x] `TestJWTVariantReturns501` still passes (regression guard)
- [x] All existing token endpoint tests still pass
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 35 — Device QoS Evaluator (G35)

**Gap addressed:**
- **G35** — AH5 defines a Device QoS Evaluator support system that measures RTT and
  bandwidth to devices and services. DynamicOrchestration must call it when evaluating
  `qualityRequirements[]` (G40). Without this binary, Step 36 cannot be implemented.

**Why now:** Prerequisite for Step 36 (G40). The binary follows the same four-layer
package structure as all other core systems, so it is low-risk to scaffold. The minimum
viable implementation (TCP RTT probe + in-memory measurement store) is sufficient for
Step 36.

**Prerequisites:** Pre-flight check passes. `core/PATTERNS.md` read before starting.

**New files (core/):**
- `core/cmd/deviceqoseval/main.go` — wire handler, service, repository; read `PORT`
  (default 8088) and `DB_PATH`
- `core/internal/deviceqoseval/model/types.go` — `MeasurementRequest`, `MeasurementResult`,
  `QoSRecord` types
- `core/internal/deviceqoseval/repository/memory.go` — in-memory store of `QoSRecord`
- `core/internal/deviceqoseval/repository/sqlite.go` — SQLite-backed store
- `core/internal/deviceqoseval/service/evaluator.go` — `Measure(host, port string) QoSRecord`
  (TCP RTT probe using `net.DialTimeout`); `Query(filter) []QoSRecord`
- `core/internal/deviceqoseval/api/handler.go` — three endpoints (see below)

**Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/deviceqosevaluator/quality-evaluation/measure` | Trigger TCP RTT probe to `host:port`; store and return `QoSRecord` |
| `POST` | `/deviceqosevaluator/quality-evaluation/mgmt/query` | Query stored records with optional `host`, `port`, `from`, `to` filters; paginated |
| `GET` | `/deviceqosevaluator/health` | Standard health endpoint |

**`QoSRecord` fields:** `id` (UUID), `host`, `port`, `latencyMs` (int64), `measuredAt` (RFC3339), `reachable` (bool).

---

### TDD cycle 35.1 — Measure returns RTT for reachable host

**Test:** `TestMeasureLocalhostReturnsPositiveLatency`

Start a TCP listener on a random port. Call
`POST /deviceqosevaluator/quality-evaluation/measure` with `{"host": "127.0.0.1", "port": "<port>"}` → assert 200, `latencyMs >= 0`, `reachable: true`.

---

### TDD cycle 35.2 — Measure marks unreachable host

**Test:** `TestMeasureUnreachableHostReturnsRecord`

Call measure with a port that has nothing listening (use a high random port) → assert 200,
`reachable: false`, `latencyMs` is 0 or timeout value. (Not a 422 — the measurement
itself succeeded; the target was unreachable.)

**Expected failure before implementation:** 500 or panic on dial error.

---

### TDD cycle 35.3 — Mgmt query returns stored measurements

**Test:** `TestMgmtQueryReturnsMeasurements`

Trigger two measurements (one reachable, one not). Call
`POST /deviceqosevaluator/quality-evaluation/mgmt/query` with empty body → assert both
records returned.

---

### TDD cycle 35.4 — Mgmt query filters by host

**Test:** `TestMgmtQueryFilterByHost`

Trigger measurements for two different hosts. Query with `{"host": "<host1>"}` → assert
only host1 records returned.

---

### System test

Start the binary (`PORT=8088`). POST a measure to localhost:8080 (ServiceRegistry port)
→ assert 200, positive latency, reachable. POST a measure to localhost:9999 (nothing
listening) → assert reachable: false. Query mgmt → assert both records present.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
  ./internal/deviceqoseval/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on every package under `internal/deviceqoseval/`.

### Documentation updates (after Step 35)

- `core/GAP_ANALYSIS.md` — mark G35 resolved; add implementation summary (TCP RTT probe, `QoSRecord`, endpoints)
- `core/SPEC.md` — add Device QoS Evaluator section (port 8088, three endpoints, `QoSRecord` shape)
- `README.md` — add Device QoS Evaluator to the systems table and Configuration section
- `CONFORMANCE.md` — move G35 from Open → Resolved; note G40 is now unblocked

### Completion criteria

- [x] `TestMeasureLocalhostReturnsPositiveLatency` passes
- [x] `TestMeasureUnreachableHostReturnsRecord` passes
- [x] `TestMgmtQueryReturnsMeasurements` passes
- [x] `TestMgmtQueryFilterByHost` passes
- [x] `go build ./cmd/deviceqoseval` succeeds
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all `internal/deviceqoseval/` packages

---

## Step 36 — QoS filtering in orchestration (G40)

**Gap addressed:**
- **G40** — `OrchestrationRequest` has no `qualityRequirements[]` field.
  DynamicOrchestration does not call the Device QoS Evaluator. This step adds the field,
  defines the `QoSEvaluatorClient` interface, and filters orchestration candidates based
  on their measured latency.

**Why now:** Unblocked by Step 35 (Device QoS Evaluator binary exists). Step 36 is the
payoff for Step 35.

**Prerequisites:** Step 35 complete and passing. Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/orchestration/model/types.go` — add `QoSRequirement` struct
  (`maxLatencyMs int64`) and `QualityRequirements []QoSRequirement` to `OrchestrationRequest`
- `core/internal/orchestration/dynamic/client/qoseval.go` (new file) — define
  `QoSEvaluatorClient` interface with
  `Measure(ctx context.Context, host, port string) (latencyMs int64, reachable bool, err error)`;
  provide `HTTPQoSEvaluatorClient` implementation (calls `POST /deviceqosevaluator/quality-evaluation/measure`)
- `core/internal/orchestration/dynamic/service/orchestrator.go` — after SR lookup, for each
  candidate: if `QualityRequirements` is non-empty and `QOS_EVALUATOR_URL` is set, call
  `QoSEvaluatorClient.Measure`; exclude candidates where `latencyMs > maxLatencyMs` or
  `reachable == false`. Fail-open: if evaluator is unreachable, include the candidate
  (quality is additive, not a security gate)
- `core/cmd/dynamicorch/main.go` — read `QOS_EVALUATOR_URL`; construct and inject
  `HTTPQoSEvaluatorClient` (nil when unset → no filtering)

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `QOS_EVALUATOR_URL` | *(unset)* | When set, DynamicOrchestration measures each candidate's latency and filters by `qualityRequirements[].maxLatencyMs`. Unset = no QoS filtering. Fail-open: unreachable evaluator → candidate included. |

---

### TDD cycle 36.1 — Fast provider passes QoS filter

**Test:** `TestQoSFilterPassesFastProvider`

Two fake providers in SR response. Fake QoS evaluator returns 5ms for provider A, 300ms
for provider B. Request has `qualityRequirements: [{"maxLatencyMs": 50}]` → assert only
provider A in result.

**Expected failure before implementation:** both providers returned (no filtering).

---

### TDD cycle 36.2 — No requirements passes all providers

**Test:** `TestQoSFilterNoRequirementsPassesAll`

Request has empty `qualityRequirements` → all providers returned regardless of latency.
This is the backward-compatible regression guard — must pass before and after implementation.

---

### TDD cycle 36.3 — Evaluator unreachable is fail-open

**Test:** `TestQoSEvaluatorUnreachablePassesCandidate`

`QOS_EVALUATOR_URL` set to non-listening address. Request has `qualityRequirements` →
assert all candidates included (fail-open). This distinguishes QoS (additive quality
gate) from auth (security gate, which is fail-closed).

---

### TDD cycle 36.4 — Unreachable provider excluded

**Test:** `TestQoSFilterExcludesUnreachableProvider`

Fake QoS evaluator returns `reachable: false` for provider B. Request has
`qualityRequirements: [{"maxLatencyMs": 100}]` → assert provider B excluded even though
its measured latency would be within the threshold.

---

### System test

Start the full stack with `QOS_EVALUATOR_URL=http://localhost:8088`. Register two
providers. Trigger a QoS measurement for each (one fast, one slow or unreachable). Call
orchestration with `qualityRequirements: [{"maxLatencyMs": 50}]` → assert only the fast
provider is returned.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
  ./internal/orchestration/model/... \
  ./internal/orchestration/dynamic/client/... \
  ./internal/orchestration/dynamic/service/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on every modified package.

### Documentation updates (after Step 36)

- `core/GAP_ANALYSIS.md` — mark G40 fully resolved; add implementation summary
- `core/SPEC.md` — add `qualityRequirements[]` to OrchestrationRequest; add `QOS_EVALUATOR_URL` to DynamicOrchestration configuration; document fail-open semantics
- `core/EXAMPLES.md` — add example: orchestration request with `qualityRequirements` and filtered result
- `CONFORMANCE.md` — move G40 (QoS filtering) from Open → Resolved; update DynamicOrchestration ratings

### Completion criteria

- [x] `TestQoSFilterPassesFastProvider` passes
- [x] `TestQoSFilterNoRequirementsPassesAll` passes (backward-compatible regression guard)
- [x] `TestQoSEvaluatorUnreachablePassesCandidate` passes
- [x] `TestQoSFilterExcludesUnreachableProvider` passes
- [x] All existing DynamicOrchestration tests still pass
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 37 — Translation Manager (G36)

**Gap addressed:**
- **G36** — AH5 defines a Translation Manager support system for protocol and data model
  translation. DynamicOrchestration accepts `ALLOW_TRANSLATION` but treats it as a no-op.
  This step implements a minimal Translation Manager with JSON field-remapping bridges.

**Why now:** Low-priority for Research. Independent of all other Phase 3 steps. A minimal
implementation enables `ALLOW_TRANSLATION` to become functional.

**Prerequisites:** Pre-flight check passes. `core/PATTERNS.md` read before starting.

**New files (core/):**
- `core/cmd/translationmgr/main.go` — wire handler, service, repository; read `PORT`
  (default 8089) and `DB_PATH`
- `core/internal/translationmgr/model/types.go` — `Bridge` (id, sourceFormat, targetFormat,
  fieldMappings `map[string]string`, active bool), `TranslateRequest`, `TranslateResponse`
- `core/internal/translationmgr/repository/memory.go`
- `core/internal/translationmgr/repository/sqlite.go`
- `core/internal/translationmgr/service/translator.go` — `Translate(bridgeId, payload)`:
  unmarshal JSON, apply `fieldMappings` (rename keys), marshal result; `CRUD` for bridges
- `core/internal/translationmgr/api/handler.go`

**Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/translationmanager/translation/translate` | Translate payload using named bridge |
| `GET` | `/translationmanager/translation/status/{bridgeId}` | Inspect bridge status |
| `POST` | `/translationmanager/translation/mgmt/bridges` | Create bridge configuration |
| `GET` | `/translationmanager/translation/mgmt/bridges` | List all bridges (paginated) |
| `DELETE` | `/translationmanager/translation/mgmt/bridges/{id}` | Delete bridge |
| `GET` | `/translationmanager/health` | Standard health endpoint |

**DynamicOrchestration change (core/):**
- `core/internal/orchestration/dynamic/client/translationmgr.go` (new) — define
  `TranslationClient` interface; `HTTPTranslationClient` implementation
- `core/internal/orchestration/dynamic/service/orchestrator.go` — when `ALLOW_TRANSLATION`
  flag is true and `TRANSLATION_MGR_URL` is set and no compatible direct provider exists:
  attempt translation via `TranslationClient`; include translated providers in result

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `TRANSLATION_MGR_URL` | *(unset)* | When set and `ALLOW_TRANSLATION` flag is true, DynamicOrch attempts protocol translation for incompatible providers. Unset = ALLOW_TRANSLATION is a no-op. |

---

### TDD cycle 37.1 — Bridge created and translate invoked

**Test:** `TestCreateBridgeAndTranslate`

Create a bridge with `fieldMappings: {"temperature": "temp_celsius"}`. POST a payload
`{"temperature": 22.5}` to translate endpoint with that bridgeId → assert response
contains `{"temp_celsius": 22.5}` and original key absent.

---

### TDD cycle 37.2 — Unknown bridge returns 404

**Test:** `TestTranslateUnknownBridgeReturns404`

`POST /translationmanager/translation/translate` with unknown `bridgeId` → assert 404.

---

### TDD cycle 37.3 — Bridge CRUD

**Test:** `TestBridgeCRUD`

Create → list (appears) → delete → list (absent). Assert 404 on translate after deletion.

---

### TDD cycle 37.4 — DynamicOrch uses translation when ALLOW_TRANSLATION set

**Test:** `TestOrchestrationWithAllowTranslationUsesTranslationMgr`

SR returns one provider with interface `HTTP-SECURE-JSON`. Request has interface
`HTTP-INSECURE-JSON` and `ALLOW_TRANSLATION: true`. Fake translation manager returns a
translated provider. Assert translated provider in orchestration result.

**Expected failure before implementation:** Empty result (no compatible provider, translation not attempted).

---

### System test

Start Translation Manager (`PORT=8089`). Create a field-mapping bridge. Call translate
with a JSON payload — assert renamed fields. Start DynamicOrchestration with
`TRANSLATION_MGR_URL=http://localhost:8089`. Submit an orchestration request with
`ALLOW_TRANSLATION: true` and a provider with a different interface — assert result
contains the translated provider.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
  ./internal/translationmgr/... \
  ./internal/orchestration/dynamic/client/... \
  ./internal/orchestration/dynamic/service/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on every modified package.

### Documentation updates (after Step 37)

- `core/GAP_ANALYSIS.md` — mark G36 resolved; add implementation summary
- `core/SPEC.md` — add Translation Manager section (port 8089, endpoints, Bridge shape); add `TRANSLATION_MGR_URL` to DynamicOrchestration configuration; update `ALLOW_TRANSLATION` from no-op to functional
- `README.md` — add Translation Manager to systems table and Configuration section
- `CONFORMANCE.md` — move G36 from Open → Resolved; update DynamicOrchestration ratings

### Completion criteria

- [x] `TestCreateBridgeAndTranslate` passes
- [x] `TestTranslateUnknownBridgeReturns404` passes
- [x] `TestBridgeCRUD` passes
- [x] `TestOrchestrationWithAllowTranslationUsesTranslationMgr` passes
- [x] All existing DynamicOrchestration tests still pass
- [x] `go build ./cmd/translationmgr` succeeds
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 38 — MQTT communication profiles (G34)

**Gap addressed:**
- **G34** — All systems register with `interfaces: ["HTTP-INSECURE-JSON"]` only. AH5
  requires `generic_mqtt` and `generic_mqtts` profiles. This step adds an MQTT listener
  alongside the existing HTTP listener when `MQTT_BROKER_URL` is configured, and registers
  the corresponding interface in ServiceRegistry.

**Why last:** Highest-effort item. Requires a running MQTT broker (e.g., Mosquitto),
per-topic routing, request/response correlation (MQTT is message-oriented, not
request-response), and JSON payload mapping. Cross-cutting: affects all eight systems.
For research/teaching purposes this step is scoped to the minimum viable MQTT surface.

**Prerequisites:** Steps 33–37 complete. A Mosquitto broker accessible at
`MQTT_BROKER_URL`. Pre-flight check passes.

**Scope (minimum viable):**
Each system publishes its responses on a reply topic and subscribes to a request topic.
Request correlation uses a `correlationId` field in the JSON payload. A request received
on `ah5/<system>/request` is routed to the same handler logic as HTTP; the response is
published to `ah5/<system>/reply/<correlationId>`.

**Files to modify (core/):**
- `core/internal/mqttutil/broker.go` (new package) — `MQTTAdapter` wraps an
  `mqtt.Client`; `Subscribe(topic, handler)` and `Publish(topic, payload)` helpers;
  uses `github.com/eclipse/paho.mqtt.golang`
- Each `cmd/<system>/main.go` — read `MQTT_BROKER_URL`; if set, start `MQTTAdapter`
  and register the system under `HTTP-INSECURE-JSON` and `MQTT-INSECURE-JSON` in SR
- Each `internal/<system>/api/handler.go` — add `ServeHTTP`-compatible adapter for MQTT
  messages (unmarshal request from payload, call existing service method, marshal response)

**New environment variables:**

| Variable | Default | Description |
|---|---|---|
| `MQTT_BROKER_URL` | *(unset)* | MQTT broker address (e.g., `tcp://localhost:1883`). When set, each system starts an MQTT listener and registers `MQTT-INSECURE-JSON` interface. |
| `MQTT_CLIENT_ID_PREFIX` | `arrowhead` | Prefix for MQTT client IDs (appended with system name). |

---

### TDD cycle 38.1 — MQTT adapter subscribes and publishes

**Test:** `TestMQTTAdapterRoundTrip`

Use an in-process MQTT broker mock (or `paho.mqtt.golang` with `NewClient` using
`MemoryStore`). Subscribe to a topic, publish a message, assert the handler is called
with the correct payload.

---

### TDD cycle 38.2 — System registers MQTT interface when broker set

**Test:** `TestSystemRegistersMQTTInterfaceWhenBrokerSet`

Start ServiceRegistry with `MQTT_BROKER_URL` pointing to a mock broker. Assert that the
system's own SR registration includes `MQTT-INSECURE-JSON` in `interfaces`. When
`MQTT_BROKER_URL` unset → `interfaces` contains only `HTTP-INSECURE-JSON`.

---

### TDD cycle 38.3 — Health endpoint reachable via MQTT

**Test:** `TestHealthEndpointViaMQTT`

Publish a health request to `ah5/serviceregistry/request` topic with
`{"path": "/health", "correlationId": "abc"}`. Assert a response published to
`ah5/serviceregistry/reply/abc` with `{"status": "UP"}`.

---

### System test

Start a Mosquitto broker (`docker run eclipse-mosquitto`). Start all eight systems with
`MQTT_BROKER_URL=tcp://localhost:1883`. Use an MQTT client (`mosquitto_pub`/`mosquitto_sub`)
to send a health request to each system and assert a response is received on the reply
topic. Verify HTTP endpoints are unaffected.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
  ./internal/mqttutil/... \
  ./internal/api/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/mqttutil/`. Handler packages: focus on the MQTT adapter path.

### Documentation updates (after Step 38)

- `core/GAP_ANALYSIS.md` — mark G34 resolved; add implementation summary (topic scheme, correlation, `MQTT_BROKER_URL`)
- `core/SPEC.md` — add MQTT section: topic naming convention, correlation protocol, `MQTT_BROKER_URL` and `MQTT_CLIENT_ID_PREFIX` env vars; update interface registration
- `README.md` — add `MQTT_BROKER_URL` and `MQTT_CLIENT_ID_PREFIX` to Configuration table (all systems)
- `CONFORMANCE.md` — move G34 from Open → Resolved; update per-system ratings (Endpoint% gains for all systems)

### Completion criteria

- [x] `TestMQTTAdapterRoundTrip` passes
- [x] `TestSystemRegistersMQTTInterfaceWhenBrokerSet` passes
- [x] `TestHealthEndpointViaMQTT` passes
- [ ] All eight systems start successfully with `MQTT_BROKER_URL` set
- [x] All existing HTTP tests still pass (no regression from MQTT listener)
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes (broker started before test)
- [x] Coverage ≥ 80% on `internal/mqttutil/`

---

## Step 39 — Phase 3 documentation update

**Purpose:** Ensures every authoritative document reflects the final implemented state of
Phase 3. Apply after all Phase 3 implementation steps (33–38) are complete and passing.

**Prerequisites:** Steps 33–38 all complete and passing. Pre-flight check passes.

### `core/GAP_ANALYSIS.md`

For each resolved gap add (if not done per-step):
- A `**Status: Resolved in Step N**` line at the top of its section
- An implementation summary paragraph (env var(s), which files, key design decision)

Gaps to update: G10, G23, G34, G35, G36, G40.

### `CONFORMANCE.md`

1. Move all six gaps (G10, G23, G34, G35, G36, G40) from **Open Gaps** table →
   **Resolved Gaps** table with Step numbers 33–38.
2. Update **Per-System Ratings** for all affected systems.
3. Update Phase Plan: mark Phase 3 as **Complete**.
4. Update **last updated** timestamp.

### `CONFORMANCE_UPDATE_PLAN.md`

1. Tick all completion criteria checkboxes in Steps 33–38.
2. Add Phase 3 regression matrix (see below) as section 14.

### `core/SPEC.md`

- Add `REGISTER_AUTH_URL` to ServiceRegistry configuration
- Add `HMAC_SECRET` to ConsumerAuthorization configuration
- Add Device QoS Evaluator section (port 8088, three endpoints, `QoSRecord` shape)
- Add `QOS_EVALUATOR_URL` to DynamicOrchestration configuration
- Add `qualityRequirements[]` to `OrchestrationRequest`
- Add Translation Manager section (port 8089, endpoints, Bridge shape)
- Add `TRANSLATION_MGR_URL` and update `ALLOW_TRANSLATION` note in DynamicOrchestration
- Add MQTT section (topic scheme, `MQTT_BROKER_URL`, `MQTT_CLIENT_ID_PREFIX`)

### `core/EXAMPLES.md`

- Add example: `USAGE_LIMITED_TOKEN` generation and exhaustion
- Add example: orchestration request with `qualityRequirements` and filtered result
- Add example: JSON field-remapping bridge translate request and response

### `README.md`

- Add `REGISTER_AUTH_URL` (ServiceRegistry)
- Add `HMAC_SECRET` (ConsumerAuthorization)
- Add Device QoS Evaluator and Translation Manager to systems table
- Add `QOS_EVALUATOR_URL` (DynamicOrchestration)
- Add `TRANSLATION_MGR_URL` (DynamicOrchestration)
- Add `MQTT_BROKER_URL` and `MQTT_CLIENT_ID_PREFIX` (all systems)

### Completion criteria

- [x] All six gaps (G10, G23, G34, G35, G36, G40) appear in the Resolved Gaps table in `CONFORMANCE.md`
- [x] None of the six gaps remain in the Open Gaps table in `CONFORMANCE.md`
- [x] Phase Plan row for Phase 3 shows **Complete**
- [x] All Steps 33–38 completion criteria checkboxes are `[x]`
- [x] All new env vars documented in `core/SPEC.md` and `README.md`
- [x] All new endpoints documented in `core/SPEC.md`
- [x] `core/EXAMPLES.md` updated with at least one example per major new feature
- [x] `core/GAP_ANALYSIS.md` shows resolved status for G10, G23, G34, G35, G36, G40

---

## 14. Phase 3 — Regression matrix

Run after all Phase 3 steps are complete (Steps 33–38):

| Check | Steps |
|---|---|
| `cd core && go build ./...` | All Phase 3 |
| `cd core && go vet ./...` | All Phase 3 |
| `cd core && go test -race ./...` | All Phase 3 |
| `bash core/test-system.sh` | All Phase 3 |
| `cd core-evol && go build ./... && go test -race ./...` | 38 |
| `go build ./...` (workspace root) | All Phase 3 |

---

## Phase 4 — Behavioral completeness (Steps 40–49)

**Goal:** Close model-correctness and missing-CRUD gaps identified in the Phase 4/5 audit.
No new external dependencies. All packages use only the Go standard library and already-imported
modules. Each step is independent of the others except where noted.

**Order:** Step 40 first (SR PUT endpoints — unblocks management tooling). Steps 41–47 are
independent of each other. Step 42 (G46) requires no code change — tests only. Step 43 (G48)
must complete before Step 48 (G25 residual), since Step 48 adds the SimpleStore 501 that
parallels the DynamicOrch behavior added in Step 43. Step 49 is the documentation sweep.

---

## Step 40 — ServiceRegistry PUT operations for service definitions and interface templates (G44)

**Gap addressed:**
- **G44** — `PUT /serviceregistry/mgmt/service-definitions` and `PUT /serviceregistry/mgmt/interface-templates` are missing. The equivalent PUT endpoints for devices (`handleMgmtDevices`) and systems (`handleMgmtSystems`) are already implemented and follow the same pattern. Service instances also have PUT via `handleMgmtServiceInstances`.

**Why first:** Low effort — the handler and service patterns are already established. Unblocks management tooling for any client that needs to rename or update interface definitions.

**Prerequisites:** Pre-flight check passes. `core/CLAUDE.md` read before starting.

**Files to modify (core/):**
- `core/internal/api/ah5_handler.go` — add `http.MethodPut` case to `handleMgmtServiceDefs`; add `http.MethodPut` case to `handleMgmtInterfaceTemplates`
- `core/internal/service/ah5_registry.go` — add `UpdateServiceDefinitions(req model.ServiceDefinitionListRequest) (model.ServiceDefinitionListResponse, error)` and `UpdateInterfaceTemplates(req model.InterfaceTemplateListRequest) (model.InterfaceTemplateListResponse, error)` methods
- `core/internal/repository/` (memory store) — add `UpdateServiceDefinitions` and `UpdateInterfaceTemplates` methods
- `core/internal/repository/` (SQLite store, if present) — add corresponding methods
- `core/internal/api/ah5_handler_test.go` — add PUT tests for both resources

**Endpoints added:**

| Method | Path | Description |
|--------|------|-------------|
| `PUT` | `/serviceregistry/mgmt/service-definitions` | Update service definition name/metadata |
| `PUT` | `/serviceregistry/mgmt/interface-templates` | Update interface template properties |

---

### TDD cycle 40.1 — PUT service-definitions updates existing entry

**Test:** `TestMgmtServiceDefinitionsPutUpdatesEntry`

Create a service definition via POST. PUT with updated metadata. Assert response 200 and updated fields. Query via the mgmt/query endpoint and assert the stored record reflects the change.

**Expected failure before implementation:** 405 Method Not Allowed from the `default` case.

---

### TDD cycle 40.2 — PUT service-definitions with unknown name returns 404

**Test:** `TestMgmtServiceDefinitionsPutUnknownReturns404`

PUT with a `name` that does not exist in the store → assert 404 Not Found.

---

### TDD cycle 40.3 — PUT interface-templates updates existing entry

**Test:** `TestMgmtInterfaceTemplatesPutUpdatesEntry`

Create via POST. PUT with updated properties. Assert 200 and updated fields. Query and assert persisted changes.

**Expected failure before implementation:** 405 Method Not Allowed.

---

### TDD cycle 40.4 — PUT interface-templates with unknown name returns 404

**Test:** `TestMgmtInterfaceTemplatesPutUnknownReturns404`

PUT with an unknown name → assert 404.

---

### System test

Start ServiceRegistry. Create a service definition. Update it via PUT with a changed metadata field. Query all service definitions and assert the updated value is returned. Repeat for interface templates.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/api/... ./internal/service/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/api/` and `internal/service/`.

### Documentation updates (after Step 40)

- `core/GAP_ANALYSIS.md` — mark G44 fully resolved; add implementation summary noting which PUT endpoints were added and which already existed
- `core/SPEC.md` — add `PUT` row to service-definitions and interface-templates endpoint tables
- `CONFORMANCE.md` — move G44 from Open → Resolved with Step 40; update ServiceRegistry Endpoint% rating

### Completion criteria

- [x] `TestMgmtServiceDefinitionsPutUpdatesEntry` passes
- [x] `TestMgmtServiceDefinitionsPutUnknownReturns404` passes
- [x] `TestMgmtInterfaceTemplatesPutUpdatesEntry` passes
- [x] `TestMgmtInterfaceTemplatesPutUnknownReturns404` passes
- [x] All existing ServiceRegistry management tests still pass
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on modified packages

---

## Step 41 — `securityPolicy` enum validation on service registration (G45)

**Gap addressed:**
- **G45** — `POST /serviceregistry/service-discovery/register` accepts any string in the `policy` field of an `InterfaceInstance`. The `SecurityPolicy` enum (`NONE`, `CERT_AUTH`, `TIME_LIMITED_TOKEN_AUTH`, `USAGE_LIMITED_TOKEN_AUTH`, `BASE64_SELF_CONTAINED_TOKEN_AUTH`) is defined in the model but not validated on the discovery register path. An invalid policy is stored verbatim and silently propagates to downstream consumers.

**Why now:** Medium effort, independent. Prevents corrupted policy data from entering the store. Follows the same validation pattern as G19 (naming conventions) and G30 (interface names).

**Prerequisites:** Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/api/ah5_handler.go` — in the service registration handler: validate the `policy` field of each `InterfaceInstance` against the enum set; return 400 with a descriptive message if unknown
- `core/internal/model/ah5_types.go` (or equivalent) — ensure the `SecurityPolicy` constants are defined and accessible; add a `ValidSecurityPolicies` set or validation function
- `core/internal/api/ah5_handler_test.go` — add policy validation tests

**Valid policy values:** `NONE`, `CERT_AUTH`, `TIME_LIMITED_TOKEN_AUTH`, `USAGE_LIMITED_TOKEN_AUTH`, `BASE64_SELF_CONTAINED_TOKEN_AUTH`. Absent/empty policy defaults to `NONE`.

---

### TDD cycle 41.1 — Valid securityPolicy is accepted

**Test:** `TestServiceRegisterValidSecurityPolicy`

Register a service with `policy: "TIME_LIMITED_TOKEN_AUTH"` on an interface instance → assert 201 Created and the policy is stored correctly.

---

### TDD cycle 41.2 — Unknown securityPolicy returns 400

**Test:** `TestServiceRegisterInvalidSecurityPolicy`

Register a service with `policy: "NOT_A_REAL_POLICY"` → assert 400 Bad Request with an error message that names the invalid value.

---

### TDD cycle 41.3 — Absent securityPolicy defaults to NONE

**Test:** `TestServiceRegisterAbsentSecurityPolicyDefaultsToNone`

Register a service with no `policy` field → assert 201 and stored policy is `"NONE"` (or equivalent zero value). This is the backward-compatible regression guard — must pass before and after implementation.

---

### System test

Register a service with an invalid `policy` value via curl → assert 400 response with descriptive error. Register with a valid value → assert 201 and the value is returned in a subsequent query.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/api/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/api/`.

### Documentation updates (after Step 41)

- `core/GAP_ANALYSIS.md` — mark G45 resolved; list accepted enum values and default
- `core/SPEC.md` — document `securityPolicy` enum on the service register endpoint with the valid value set
- `CONFORMANCE.md` — move G45 from Open → Resolved with Step 41; update ServiceRegistry Behavior% rating

### Completion criteria

- [x] `TestServiceRegisterValidSecurityPolicy` passes
- [x] `TestServiceRegisterInvalidSecurityPolicy` passes
- [x] `TestServiceRegisterAbsentSecurityPolicyDefaultsToNone` passes (regression guard)
- [x] All existing service registration tests still pass
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on `internal/api/`

---

## Step 42 — Scoped policy evaluation in ConsumerAuth verify (G46)

**Gap addressed:**
- **G46** — GAP_ANALYSIS.md describes the scoped-policy lookup as disabled. Audit of `core/internal/consumerauth/service/auth.go:Verify` shows the lookup **is already implemented**:
  ```go
  policy := p.DefaultPolicy
  if req.Scope != "" {
      if sp, ok := p.ScopedPolicies[req.Scope]; ok {
          policy = sp
      }
  }
  if isAuthorized(req.Consumer, policy) {
      return true
  }
  ```
  No code change is required. This step adds tests that prove the behavior is correct and updates the gap status.

**Why now:** Highest-priority Phase 4 step (was marked Blocker/Production). Confirming live behavior with tests is low risk and high value. If the tests reveal a regression, this becomes an implementation step.

**Prerequisites:** Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/consumerauth/api/handler_test.go` — add scoped policy tests via the HTTP handler (integration-style)

---

### TDD cycle 42.1 — Scoped policy overrides default for matching scope

**Test:** `TestVerifyScopedPolicyOverridesDefault`

Grant policy: `defaultPolicy: DENY_ALL`, `scopedPolicies: {"write": ALLOW_ALL}`.
Verify with `scope: "write"` → assert 200 (authorized).
Verify with `scope: "read"` (no scoped entry → falls back to default DENY_ALL) → assert 403 (denied).

This test will pass immediately if the implementation is correct; it is a coverage test not a TDD cycle in the strict sense.

---

### TDD cycle 42.2 — Empty scope falls back to default policy

**Test:** `TestVerifyEmptyScopeFallsBackToDefault`

Grant with `defaultPolicy: ALLOW_ALL`. Verify with no `scope` field → assert 200 (authorized via default).

---

### TDD cycle 42.3 — Unknown scope falls back to default policy

**Test:** `TestVerifyUnknownScopeFallsBackToDefault`

Grant with `defaultPolicy: DENY_ALL`, `scopedPolicies: {"write": ALLOW_ALL}`.
Verify with `scope: "admin"` (not in map) → falls back to DENY_ALL → assert 403.

---

### System test

POST grant with scoped policies to ConsumerAuth. POST verify with matching scope in body → assert 200 authorized. POST verify with different scope → assert 403 denied. POST verify with no scope → assert 200 (uses ALLOW_ALL default).

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/consumerauth/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/consumerauth/`.

### Documentation updates (after Step 42)

- `core/GAP_ANALYSIS.md` — update G46: mark resolved; note implementation was present but untested; list confirming test names
- `CONFORMANCE.md` — move G46 from Open → Resolved with Step 42; update ConsumerAuthorization Behavior% rating (significant increase — was a Blocker)

### Completion criteria

- [x] `TestVerifyScopedPolicyOverridesDefault` passes
- [x] `TestVerifyEmptyScopeFallsBackToDefault` passes
- [x] `TestVerifyUnknownScopeFallsBackToDefault` passes
- [x] All existing ConsumerAuth tests still pass
- [x] `go test -race ./...` from `core/` passes
- [x] Coverage ≥ 80% on `internal/consumerauth/`

---

## Step 43 — `ONLY_EXCLUSIVE` flag wired to lock store in DynamicOrchestration (G48)

**Gap addressed:**
- **G48** — `OrchestrationFlags.OnlyExclusive` is parsed and stored on the request (`// stub` comment in `model/types.go`) but the filter step in `Orchestrate()` never consults it. AH5 semantics: when `ONLY_EXCLUSIVE` is true, exclude candidates whose `ExclusiveUntil` field is a future timestamp (i.e., held under an active exclusive lock by another consumer). Candidates with empty or past `ExclusiveUntil` are included.

**Why now:** Medium effort. The `LockStore` already exists in `core/internal/orchestration/dynamic/service/` and is populated by the handler. The only missing piece is wiring it into the `Orchestrate()` filter pipeline.

**Implementation approach:** The `DynamicOrchestrator` does not currently have access to the lock store (it is owned by `DynamicOrchHandler`). The cleanest approach is to define a `LockChecker` interface:

```go
type LockChecker interface {
    IsLocked(providerName string) bool
}
```

Add a `SetLockChecker(lc LockChecker)` setter on `DynamicOrchestrator`. Implement `IsLocked` on `*LockStore` by checking whether any active lock for the provider has `ExclusiveUntil` in the future. Inject from `DynamicOrchHandler`. Pass `NopLockChecker{}` in tests that don't need it.

**Prerequisites:** Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/orchestration/dynamic/service/orchestrator.go` — add `LockChecker` interface; add `lockChecker` field and `SetLockChecker` setter; add ONLY_EXCLUSIVE filter after the preferred-provider filter (Step 5)
- `core/internal/orchestration/dynamic/service/lock_store.go` — implement `IsLocked(providerName string) bool` on `*LockStore`; define `NopLockChecker{}`
- `core/internal/orchestration/dynamic/api/handler.go` — call `orch.SetLockChecker(h.locks)` after constructing the orchestrator
- `core/internal/orchestration/dynamic/service/orchestrator_test.go` — add ONLY_EXCLUSIVE filter tests

---

### TDD cycle 43.1 — ONLY_EXCLUSIVE excludes a locked provider

**Test:** `TestOrchestrationOnlyExclusiveFiltersLockedProvider`

SR response: two providers. Provider A has `ExclusiveUntil` set to a future RFC3339 timestamp (locked). Provider B has empty `ExclusiveUntil`. Request: `ONLY_EXCLUSIVE: true`. Assert only provider B is in results.

**Expected failure before implementation:** Both providers returned (flag not evaluated).

---

### TDD cycle 43.2 — ONLY_EXCLUSIVE passes a provider with no lock

**Test:** `TestOrchestrationOnlyExclusivePassesUnlockedProvider`

Provider has empty `ExclusiveUntil`. Request `ONLY_EXCLUSIVE: true`. Assert provider is included.

---

### TDD cycle 43.3 — Without ONLY_EXCLUSIVE flag locked providers are included

**Test:** `TestOrchestrationWithoutOnlyExclusiveIncludesLockedProvider`

Provider with future `ExclusiveUntil`. Request `ONLY_EXCLUSIVE: false` (or omitted). Assert provider is included (backward-compatible regression guard).

---

### TDD cycle 43.4 — Expired lock treated as unlocked

**Test:** `TestOrchestrationOnlyExclusiveExpiredLockPassesProvider`

Provider with `ExclusiveUntil` set to a timestamp in the past. Request `ONLY_EXCLUSIVE: true`. Assert provider is included (lock expired → not locked).

---

### System test

Start DynamicOrchestration. Register two providers. Lock one via `POST /serviceorchestration/orchestration/mgmt/lock`. Call orchestration with `ONLY_EXCLUSIVE: true`. Assert only the unlocked provider is returned.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/orchestration/dynamic/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all modified packages.

### Documentation updates (after Step 43)

- `core/GAP_ANALYSIS.md` — mark G48 resolved; describe `ExclusiveUntil` timestamp comparison semantics and the `LockChecker` interface
- `core/SPEC.md` — document `ONLY_EXCLUSIVE` flag semantics in OrchestrationFlags section
- `CONFORMANCE.md` — move G48 from Open → Resolved with Step 43; update DynamicOrchestration Behavior% rating

### Completion criteria

- [x] `TestOrchestrationOnlyExclusiveFiltersLockedProvider` passes
- [x] `TestOrchestrationOnlyExclusivePassesUnlockedProvider` passes
- [x] `TestOrchestrationWithoutOnlyExclusiveIncludesLockedProvider` passes (regression guard)
- [x] `TestOrchestrationOnlyExclusiveExpiredLockPassesProvider` passes
- [x] All existing DynamicOrchestration tests still pass
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 44 — Orchestration history query filtering (G49)

**Gap addressed:**
- **G49** — `POST /serviceorchestration/orchestration/mgmt/history/query` accepts any JSON body but always returns the full history. In a non-trivial deployment the store grows unboundedly and every query returns the complete set. This step adds filtering by `requesterSystemName`, `serviceDefinition`, `status`, and date range (`from`/`to`).

**Why now:** Medium effort, independent of all other steps. No new dependencies — filtering is pure in-memory slice logic. Directly improves sysop experience.

**Prerequisites:** Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/orchestration/dynamic/service/history_store.go` — add `HistoryQueryFilter` struct; update `query()` to accept a filter and apply it: exact match on string fields (empty = no filter); RFC3339 `from`/`to` bounds on `StartedAt`
- `core/internal/orchestration/dynamic/api/handler.go` — decode filter fields from the POST body and pass to `hist.query()`
- `core/internal/orchestration/dynamic/service/history_store_test.go` (new file, or append to existing) — add filter unit tests

**New filter fields (in the POST body):**

| Field | Type | Description |
|-------|------|-------------|
| `requesterSystemName` | string | Exact match on `RequesterSystemName`; empty = no filter |
| `serviceDefinition` | string | Exact match on `ServiceDefinition`; empty = no filter |
| `status` | string | Exact match on `Status` (`DONE`, `ERROR`, `PENDING`, `DELIVERED`, `FAILED`); empty = no filter |
| `from` | string (RFC3339) | Inclusive lower bound on `StartedAt`; absent = no lower bound |
| `to` | string (RFC3339) | Inclusive upper bound on `StartedAt`; absent = no upper bound |

---

### TDD cycle 44.1 — Filter by requesterSystemName returns only matching entries

**Test:** `TestHistoryQueryFilterByRequester`

Add three history entries with `RequesterSystemName` `"alpha"`, `"beta"`, `"alpha"`. Query with `requesterSystemName: "alpha"` → assert two entries returned, both from alpha.

**Expected failure before implementation:** All three entries returned (filter ignored).

---

### TDD cycle 44.2 — Filter by status returns only matching entries

**Test:** `TestHistoryQueryFilterByStatus`

Add entries with status `DONE` and `ERROR`. Query with `status: "ERROR"` → assert only ERROR entries returned.

---

### TDD cycle 44.3 — Filter by date range returns entries within bounds

**Test:** `TestHistoryQueryFilterByDateRange`

Add entries at known timestamps (use fixed `StartedAt` values). Query with `from` and `to` that include only a middle subset → assert only that subset returned.

---

### TDD cycle 44.4 — Empty filter body returns all entries (regression guard)

**Test:** `TestHistoryQueryNoFilterReturnsAll`

Add several entries. POST with an empty JSON body `{}` → assert all entries returned. Must pass before and after implementation.

---

### System test

Run orchestration requests from two different consumers (`requesterSystem.systemName` = `"Alpha"` and `"Beta"`). POST history query with `requesterSystemName: "Alpha"` → assert only Alpha's entries appear.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/orchestration/dynamic/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all modified packages.

### Documentation updates (after Step 44)

- `core/GAP_ANALYSIS.md` — mark G49 resolved; list the filter fields and semantics
- `core/SPEC.md` — add history query filter fields to the DynamicOrchestration history endpoint description
- `CONFORMANCE.md` — move G49 from Open → Resolved with Step 44; update DynamicOrchestration Behavior% rating

### Completion criteria

- [x] `TestHistoryQueryFilterByRequester` passes
- [x] `TestHistoryQueryFilterByStatus` passes
- [x] `TestHistoryQueryFilterByDateRange` passes
- [x] `TestHistoryQueryNoFilterReturnsAll` passes (regression guard)
- [x] All existing DynamicOrchestration history tests still pass
- [x] `go test -race ./...` from `core/` passes
- [x] Coverage ≥ 80% on all modified packages

---

## Step 45 — Blacklist expired-entry auto-purge (G50)

**Gap addressed:**
- **G50** — Blacklist entries whose `expiresAt` timestamp has passed accumulate in the store indefinitely. A query with `mode: ALL` returns expired entries alongside active ones, increasing noise in sysop audits. This step adds a background goroutine that periodically removes entries whose `expiresAt` is non-zero and in the past.

**Why now:** Low priority, low effort. The background-goroutine pattern is established (see G8 — expired token cleanup in Authentication). No new dependencies.

**Prerequisites:** Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/blacklist/` (service layer) — add `PurgeExpired()` method that removes entries where `expiresAt` is non-zero and before `time.Now()`
- `core/cmd/blacklist/main.go` — start a goroutine after server is ready that calls `svc.PurgeExpired()` on a ticker driven by `BLACKLIST_PURGE_INTERVAL_SECONDS`; ticker is not started when the env var is `"0"`
- `core/internal/blacklist/` (handler or service test) — add purge tests

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `BLACKLIST_PURGE_INTERVAL_SECONDS` | `3600` | Interval between expired-entry purge sweeps. Set to `0` to disable auto-purge. |

**Purge semantics:** Hard delete (remove the record). This is consistent with the G50 intent of preventing unbounded accumulation. Soft-inactive entries (those `DELETE`d via the management endpoint) remain; only those with a non-zero, past `expiresAt` are removed.

---

### TDD cycle 45.1 — PurgeExpired removes expired entries

**Test:** `TestBlacklistPurgeExpiredRemovesExpiredEntry`

Add an entry with `expiresAt` set to 1 second in the past. Call `PurgeExpired()`. Assert the entry is no longer returned by a subsequent query.

**Expected failure before implementation:** Entry still present after call.

---

### TDD cycle 45.2 — PurgeExpired keeps entries with future expiry

**Test:** `TestBlacklistPurgeExpiredKeepsFutureEntry`

Add an entry with `expiresAt` set 1 hour in the future. Call `PurgeExpired()`. Assert the entry is still present.

---

### TDD cycle 45.3 — PurgeExpired keeps permanent entries (no expiresAt)

**Test:** `TestBlacklistPurgeExpiredKeepsPermanentEntry`

Add an entry with no `expiresAt` (permanent blacklist). Call `PurgeExpired()`. Assert the entry is still present.

---

### System test

Start Blacklist with `BLACKLIST_PURGE_INTERVAL_SECONDS=1`. Add an entry with `expiresAt` 2 seconds in the future. Wait 4 seconds. Query → assert the entry is no longer present.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/blacklist/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/blacklist/`.

### Documentation updates (after Step 45)

- `core/GAP_ANALYSIS.md` — mark G50 resolved; note purge semantics (hard delete, expiry-only, soft-deleted entries unaffected)
- `core/SPEC.md` — add `BLACKLIST_PURGE_INTERVAL_SECONDS` to Blacklist configuration table
- `README.md` — add `BLACKLIST_PURGE_INTERVAL_SECONDS` to Configuration section
- `CONFORMANCE.md` — move G50 from Open → Resolved with Step 45; update Blacklist Behavior% rating

### Completion criteria

- [x] `TestBlacklistPurgeExpiredRemovesExpiredEntry` passes
- [x] `TestBlacklistPurgeExpiredKeepsFutureEntry` passes
- [x] `TestBlacklistPurgeExpiredKeepsPermanentEntry` passes
- [x] All existing Blacklist tests still pass
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] `BLACKLIST_PURGE_INTERVAL_SECONDS` documented in `core/SPEC.md` and `README.md`
- [x] Coverage ≥ 80% on `internal/blacklist/`

---

## Step 46 — SimpleStore full rule update endpoint (G51)

**Gap addressed:**
- **G51** — There is no endpoint to update the full content of an existing SimpleStore rule. `POST /serviceorchestration/orchestration/mgmt/simple-store/create` creates rules and `PUT mgmt/simple-store/modify-priorities` reorders them, but no PUT exists to update provider, consumer, service definition, service URI, or interfaces in place. Operators must delete and recreate rules, losing the stable rule UUID.

**Why now:** Medium effort. Follows the same CRUD pattern as ConsumerAuth policy management. The `handleRuleByID` handler already dispatches on DELETE; extending it to PUT is straightforward.

**Prerequisites:** Pre-flight check passes.

**New endpoint:**

| Method | Path | Description |
|--------|------|-------------|
| `PUT` | `/serviceorchestration/orchestration/mgmt/simple-store/rules/{id}` | Replace all fields of an existing rule; preserves the rule's UUID |

**Files to modify (core/):**
- `core/internal/orchestration/simplestore/api/handler.go` — in `handleRuleByID`: add `http.MethodPut` case; decode full `UpdateRuleRequest` body; call `orch.UpdateRule(id, req)`; return 200 with updated rule
- `core/internal/orchestration/simplestore/` (service layer) — add `UpdateRule(id string, req UpdateRuleRequest) (StoreRule, error)` method; return `ErrNotFound` if ID unknown; apply same field validation as `CreateRule`
- `core/internal/orchestration/simplestore/` (repository) — add `Update` method
- `core/internal/orchestration/simplestore/api/handler_test.go` — add update tests

---

### TDD cycle 46.1 — PUT rule updates all fields and returns 200

**Test:** `TestSimpleStoreUpdateRuleUpdatesFields`

Create a rule. PUT to `/mgmt/simple-store/rules/{id}` with all fields changed (different provider address, service URI, interfaces list). Assert 200 and all fields updated in the response. Query rules list and assert the stored rule reflects the changes.

**Expected failure before implementation:** 405 Method Not Allowed.

---

### TDD cycle 46.2 — PUT rule with unknown ID returns 404

**Test:** `TestSimpleStoreUpdateRuleUnknownIDReturns404`

PUT to `/mgmt/simple-store/rules/nonexistent-id` → assert 404 Not Found.

---

### TDD cycle 46.3 — PUT rule preserves the rule UUID

**Test:** `TestSimpleStoreUpdateRulePreservesID`

Create a rule, record its ID. PUT update with changed fields. Assert the response `id` field matches the original ID. Query rules list and assert only one rule exists (update, not create).

---

### System test

Create a SimpleStore rule. PUT updated provider fields. Query rules → assert the stored rule reflects all updated fields with the same ID and the list has the same number of rules as before.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/orchestration/simplestore/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/orchestration/simplestore/`.

### Documentation updates (after Step 46)

- `core/GAP_ANALYSIS.md` — mark G51 resolved
- `core/SPEC.md` — add `PUT /mgmt/simple-store/rules/{id}` to SimpleStore endpoint table; document request body shape and UUID-preservation semantics
- `CONFORMANCE.md` — move G51 from Open → Resolved with Step 46; update SimpleStoreOrchestration Endpoint% and Behavior% ratings

### Completion criteria

- [x] `TestSimpleStoreUpdateRuleUpdatesFields` passes
- [x] `TestSimpleStoreUpdateRuleUnknownIDReturns404` passes
- [x] `TestSimpleStoreUpdateRulePreservesID` passes
- [x] All existing SimpleStore tests still pass
- [x] `go test -race ./...` from `core/` passes
- [x] `bash core/test-system.sh` passes
- [x] Coverage ≥ 80% on `internal/orchestration/simplestore/`

---

## Step 47 — Authentication identity creation naming convention (G52)

**Gap addressed:**
- **G52** — `POST /authentication/mgmt/identities` creates identity records without validating `systemName` against the PascalCase convention (`^[A-Z][A-Za-z0-9]{0,62}$`). G19 established this constraint for the ServiceRegistry; an identity system that accepts names the SR would reject creates a latent inconsistency. A system that cannot register in SR can still obtain tokens from Authentication.

**Why now:** Low effort. The PascalCase regex is already used in `core/internal/api/validate.go` (or `ah5_handler.go`). This step reuses it without cross-package imports — either inline the regex or move the validator to `httputil`.

**Prerequisites:** Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/authentication/api/handler.go` — in `mgmtIdentitiesCreate`: for each identity in the batch, validate `systemName` against `^[A-Z][A-Za-z0-9]{0,62}$`; return 400 if any name is invalid, before creating any identity (atomic rejection)
- `core/internal/authentication/api/handler_test.go` — add naming validation tests

**Implementation note:** Do not import `internal/api` from `internal/authentication`. Inline the regex (`regexp.MustCompile(...)`) or add a `ValidatePascalCase` helper to `core/internal/httputil/` (shared across systems).

---

### TDD cycle 47.1 — Valid PascalCase name is accepted

**Test:** `TestAuthMgmtIdentitiesCreateValidName`

POST `mgmt/identities` with `systemName: "MySystem"` → assert 201 Created and the identity record is returned.

---

### TDD cycle 47.2 — Invalid system name returns 400

**Test:** `TestAuthMgmtIdentitiesCreateInvalidName`

POST with `systemName: "mySystem"` (lowercase start) → assert 400. POST with `systemName: ""` → assert 400. POST with `systemName: "my system"` (space) → assert 400.

---

### TDD cycle 47.3 — Batch create is atomic: any invalid name rejects the whole batch

**Test:** `TestAuthMgmtIdentitiesCreateBatchAtomicRejection`

POST batch with `[{"systemName": "ValidSystem"}, {"systemName": "invalid"}]` → assert 400 and no identities are created (query returns empty list after the failed request).

---

### System test

POST `mgmt/identities` with `systemName: "invalid"` → assert 400 and descriptive error. POST with `systemName: "ValidSystem"` → assert 201. Query identities → assert only the valid one is present.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/authentication/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/authentication/`.

### Documentation updates (after Step 47)

- `core/GAP_ANALYSIS.md` — mark G52 resolved; state the regex and note the atomic batch rejection policy
- `core/SPEC.md` — document naming convention requirement for `POST /authentication/mgmt/identities`
- `CONFORMANCE.md` — move G52 from Open → Resolved with Step 47; update Authentication Behavior% rating

### Completion criteria

- [x] `TestAuthMgmtIdentitiesCreateValidName` passes
- [x] `TestAuthMgmtIdentitiesCreateInvalidName` passes
- [x] `TestAuthMgmtIdentitiesCreateBatchAtomicRejection` passes
- [x] All existing Authentication management tests still pass
- [x] `go test -race ./...` from `core/` passes
- [x] Coverage ≥ 80% on `internal/authentication/`

---

## Step 48 — `ONLY_EXCLUSIVE` stub behavior in SimpleStoreOrchestration (G25 residual)

**Gap addressed:**
- **G25** (residual) — After Step 43 implements `ONLY_EXCLUSIVE` filtering in DynamicOrchestration, SimpleStoreOrchestration still silently ignores the flag. SimpleStore has no lock store. Per the intercloud precedent (G25/E4), returning 200 while ignoring a semantically significant flag is misleading. This step makes SimpleStore return `501 Not Implemented` when `ONLY_EXCLUSIVE` is true, consistent with how `ALLOW_INTERCLOUD` is handled.

**Why after Step 43:** Step 43 establishes the reference behavior for ONLY_EXCLUSIVE in DynamicOrch. This step mirrors it for SimpleStore. Together they fully close G25 and G48.

**Prerequisites:** Step 43 complete. Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/orchestration/simplestore/service/orchestrator.go` — check `req.OrchestrationFlags.OnlyExclusive`; if true, return an error that maps to 501 (same error type used for intercloud flags)
- `core/internal/orchestration/simplestore/api/handler_test.go` — add 501 test

---

### TDD cycle 48.1 — ONLY_EXCLUSIVE in SimpleStore orchestration returns 501

**Test:** `TestSimpleStoreOrchestrationOnlyExclusiveReturns501`

POST `/serviceorchestration/orchestration/pull` with `orchestrationFlags: {"ONLY_EXCLUSIVE": true}` → assert 501 Not Implemented with a descriptive error body.

**Expected failure before implementation:** 200 with full results (flag ignored).

---

### TDD cycle 48.2 — SimpleStore orchestration without ONLY_EXCLUSIVE succeeds (regression guard)

**Test:** `TestSimpleStoreOrchestrationWithoutOnlyExclusiveSucceeds`

Normal orchestration request (no ONLY_EXCLUSIVE flag) with a matching rule → assert 200 with results. Must pass before and after implementation.

---

### System test

POST orchestration to SimpleStore with `orchestrationFlags: {"ONLY_EXCLUSIVE": true}` via curl → assert 501 response body contains a descriptive error message explaining that ONLY_EXCLUSIVE is not supported in SimpleStore.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/orchestration/simplestore/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/orchestration/simplestore/`.

### Documentation updates (after Step 48)

- `core/GAP_ANALYSIS.md` — update G25: mark residual fully closed; note DynamicOrch implements ONLY_EXCLUSIVE (Step 43) and SimpleStore returns 501 (Step 48)
- `core/SPEC.md` — document ONLY_EXCLUSIVE behavior for each orchestrator: DynamicOrch = filter by lock, SimpleStore = 501
- `CONFORMANCE.md` — update G25 to Resolved; update SimpleStoreOrchestration ratings

### Completion criteria

- [x] `TestSimpleStoreOrchestrationOnlyExclusiveReturns501` passes
- [x] `TestSimpleStoreOrchestrationWithoutOnlyExclusiveSucceeds` passes (regression guard)
- [x] All existing SimpleStore orchestration tests still pass
- [x] `go test -race ./...` from `core/` passes
- [x] Coverage ≥ 80% on `internal/orchestration/simplestore/`

---

## Step 49 — Phase 4 documentation update

**Purpose:** Ensures every authoritative document reflects the final implemented state of Phase 4. Apply after all Phase 4 implementation steps (40–48) are complete and passing.

**Prerequisites:** Steps 40–48 all complete and passing. Pre-flight check passes.

### `core/GAP_ANALYSIS.md`

For each resolved gap add (if not done per-step):
- A `**Status: Resolved in Step N**` line at the top of its section
- An implementation summary paragraph (design decision, relevant env var if any, key files)

Gaps to update: G25 (residual), G44, G45, G46, G48, G49, G50, G51, G52.

### `CONFORMANCE.md`

1. Move all nine gaps (G25 residual, G44–G46, G48–G52) from **Open Gaps** table → **Resolved Gaps** table with Step numbers 40–48.
2. Update **Per-System Ratings** for all affected systems to match the Phase 4 projected values.
3. Update Phase Plan: mark Phase 4 as **Complete**.
4. Update **last updated** timestamp.

### `CONFORMANCE_UPDATE_PLAN.md`

1. Tick all completion criteria checkboxes in Steps 40–48.
2. Add Phase 4 regression matrix (section 15).

### `core/SPEC.md`

- Add `PUT` rows to service-definitions and interface-templates endpoint tables (Step 40)
- Document `securityPolicy` enum values on the service register endpoint (Step 41)
- Add `scope` field to the ConsumerAuth verify request body description (Step 42)
- Document `ONLY_EXCLUSIVE` flag semantics for DynamicOrch and SimpleStore (Steps 43, 48)
- Add history query filter fields to DynamicOrch history endpoint (Step 44)
- Add `BLACKLIST_PURGE_INTERVAL_SECONDS` to Blacklist configuration table (Step 45)
- Add `PUT /mgmt/simple-store/rules/{id}` to SimpleStore endpoint table (Step 46)
- Document PascalCase naming requirement for Authentication identity creation (Step 47)

### `core/EXAMPLES.md`

- Add example: SR `PUT /mgmt/service-definitions` update request and response
- Add example: ConsumerAuth verify with `scope` field and scoped policy matching
- Add example: DynamicOrch request with `ONLY_EXCLUSIVE: true` and filtered result
- Add example: Orchestration history query with `requesterSystemName` filter

### `README.md`

- Add `BLACKLIST_PURGE_INTERVAL_SECONDS` to Blacklist Configuration table

### Completion criteria

- [ ] All nine gaps (G25 residual, G44–G46, G48–G52) appear in the Resolved Gaps table in `CONFORMANCE.md`
- [ ] None of the nine gaps remain in the Open Gaps table in `CONFORMANCE.md`
- [ ] Phase Plan row for Phase 4 shows **Complete**
- [ ] All Steps 40–48 completion criteria checkboxes are `[x]`
- [ ] All new env vars documented in `core/SPEC.md` and `README.md`
- [ ] All new endpoints documented in `core/SPEC.md`
- [ ] `core/EXAMPLES.md` updated with at least one example per major Phase 4 change
- [ ] `core/GAP_ANALYSIS.md` shows resolved status for all nine gaps

---

## 15. Phase 4 — Regression matrix

Run after all Phase 4 steps are complete (Steps 40–48):

| Check | Steps |
|---|---|
| `cd core && go build ./...` | All Phase 4 |
| `cd core && go vet ./...` | All Phase 4 |
| `cd core && go test -race ./...` | All Phase 4 |
| `bash core/test-system.sh` | All Phase 4 |
| `go build ./...` (workspace root) | All Phase 4 |

---

## Phase 5 — Full protocol compliance (Steps 50–56) — **COMPLETE**

**Goal:** Reach ≥90% across all dimensions for every spec-defined system. Covers high-effort
crypto, transport, and protocol gaps. Each step has significant design complexity or external
considerations; prerequisite management is required.

**Status:** All steps completed. All tests pass with race detection (`go test -race ./...` from `core/`).

**No new external Go dependencies added in this phase.** All crypto uses Go stdlib
(`crypto/rsa`, `crypto/sha256`, `crypto/sha512`, `encoding/pem`, `crypto/rand`) or
already-imported modules (`golang.org/x/crypto` is already in `core/go.mod`).
MQTTS uses the existing `paho.mqtt.golang` client's TLS support.

**Order:**
- Step 50 (G6) first — builds directly on the `VerifyTokenIdentity` pattern from G10 (Phase 3)
  and uses the same `httputil` helper; no other Phase 5 step depends on it.
- Step 51 (G47) — JWT crypto; independent; establishes the RSA key pair used by G6
  only if TRANSLATION_BRIDGE variant needs JWT signing (it does).
- Step 52 (G4) — mTLS enforcement; independent; all binaries already have optional TLS;
  this step adds the `HTTPS_ONLY` mode and a shared `tlsutil.Serve` helper.
- Step 53 (G53) — QoS dimensions; independent; extends DeviceQoSEvaluator and orchestration model.
- Step 54 (G26) — auto-push; independent; extends DynamicOrchestration with SR polling goroutine.
- Step 55 (G34) — MQTTS; independent; extends mqttutil with TLS option.
- Step 56 — documentation sweep; apply after Steps 50–55 all pass.

---

## Step 50 — ConsumerAuthorization token relay requires Authentication identity (G6)

**Gap addressed:**
- **G6** — `POST /consumerauthorization/authorization-token/generate` does not require or
  validate a prior identity token from the Authentication system. Any caller can request a
  token for any `consumer` without proving that identity. The DynamicOrchestration identity
  check (`ENABLE_IDENTITY_CHECK`) provides partial coupling, but the ConsumerAuth token endpoint
  itself is open.

**Why first:** High priority. Builds directly on the `VerifyTokenIdentity` pattern already
implemented in `core/internal/httputil/respond.go` for G10 (Step 33). The code change is
small and follows an established pattern.

**Design:** Add `TOKEN_AUTH_URL` env var to ConsumerAuthorization. When set, `handleTokenGenerate`
extracts the `Authorization: Bearer <token>` header, calls
`GET <TOKEN_AUTH_URL>/authentication/identity/verify/<token>`, and asserts the returned
`systemName` matches `req.Consumer`. Fail-closed: missing token → 401; name mismatch → 403;
auth system unreachable → 503.

This mirrors the `REGISTER_AUTH_URL` pattern from G10 exactly: open (development mode) when
the env var is unset; enforced when set.

**Prerequisites:** Pre-flight check passes. `core/CLAUDE.md` read before starting.

**Files to modify (core/):**
- `core/internal/consumerauth/api/handler.go` — add `tokenAuthURL string` field to `Handler`;
  update `NewHandler` to accept it; in `handleTokenGenerate`: when `tokenAuthURL != ""`, call
  `httputil.VerifyTokenIdentity(r, tokenAuthURL)` and assert result matches `req.Consumer`
- `core/cmd/consumerauth/main.go` — read `TOKEN_AUTH_URL` env var; pass to `NewHandler`
- `core/internal/consumerauth/api/handler_test.go` — add token relay tests

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `TOKEN_AUTH_URL` | *(unset)* | When set, `POST /authorization-token/generate` requires a valid Bearer token whose `systemName` matches the request `consumer` field. Unset = open access (development mode). |

---

### TDD cycle 50.1 — Token generation without TOKEN_AUTH_URL succeeds unconditionally

**Test:** `TestTokenGenerateNoAuthURLSucceeds`

No `TOKEN_AUTH_URL` set. POST generate with no Authorization header → assert 201 Created.
This is the backward-compatible regression guard — must pass before and after implementation.

---

### TDD cycle 50.2 — Token generation with TOKEN_AUTH_URL requires Bearer token

**Test:** `TestTokenGenerateAuthURLRequiresBearer`

`TOKEN_AUTH_URL` set to a mock auth server. POST generate with no Authorization header →
assert 401 Unauthorized.

**Expected failure before implementation:** 201 (token generated without auth check).

---

### TDD cycle 50.3 — Token generation succeeds when identity matches consumer

**Test:** `TestTokenGenerateIdentityMatchesConsumer`

Mock auth server returns `{"systemName": "ConsumerA"}` for the provided token. POST generate
with `consumer: "ConsumerA"` and matching Bearer token → assert 201 Created.

---

### TDD cycle 50.4 — Token generation fails when identity mismatches consumer

**Test:** `TestTokenGenerateIdentityMismatchRejects`

Mock auth server returns `{"systemName": "ConsumerA"}`. POST generate with `consumer: "ConsumerB"`
→ assert 403 Forbidden.

---

### TDD cycle 50.5 — Auth system unreachable returns 503

**Test:** `TestTokenGenerateAuthUnreachableReturns503`

`TOKEN_AUTH_URL` set to a non-listening address. POST generate with any Bearer token →
assert 503 Service Unavailable.

---

### System test

Start ConsumerAuth with `TOKEN_AUTH_URL=http://localhost:8081`. Start Authentication.
Create an identity `ConsumerA`. Login to get a token. Generate an authorization token with
`consumer: "ConsumerA"` and Bearer token → assert 201. Generate with `consumer: "ConsumerB"`
and same token → assert 403.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/consumerauth/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/consumerauth/`.

### Documentation updates (after Step 50)

- `core/GAP_ANALYSIS.md` — update G6: mark resolved; describe TOKEN_AUTH_URL; note remaining
  gap completion (full relay now enforced when env var is set)
- `core/SPEC.md` — add `TOKEN_AUTH_URL` to ConsumerAuthorization configuration table;
  document 401/403/503 responses on the token generate endpoint
- `README.md` — add `TOKEN_AUTH_URL` to ConsumerAuthorization configuration section
- `CONFORMANCE.md` — move G6 from Partial → Resolved with Step 50; update ConsumerAuthorization ratings

### Completion criteria

- [ ] `TestTokenGenerateNoAuthURLSucceeds` passes (regression guard)
- [ ] `TestTokenGenerateAuthURLRequiresBearer` passes
- [ ] `TestTokenGenerateIdentityMatchesConsumer` passes
- [ ] `TestTokenGenerateIdentityMismatchRejects` passes
- [ ] `TestTokenGenerateAuthUnreachableReturns503` passes
- [ ] All existing ConsumerAuth token tests still pass
- [ ] `go test -race ./...` from `core/` passes
- [ ] `bash core/test-system.sh` passes
- [ ] Coverage ≥ 80% on `internal/consumerauth/`

---

## Step 51 — JWT token variants: RSA signing infrastructure (G47)

**Gap addressed:**
- **G47** — `RSA_SHA256_JSON_WEB_TOKEN`, `RSA_SHA512_JSON_WEB_TOKEN`, and
  `TRANSLATION_BRIDGE_TOKEN` return 501. The `/authorization-token/public-key` endpoint returns
  404. No RSA key pair is managed at startup.

**Design:** Use Go stdlib only — `crypto/rsa`, `crypto/sha256`, `crypto/sha512`, `encoding/pem`,
`crypto/rand`. No external JWT library added.

JWT format (manual encoding, RFC 7519 compliant):
```
base64url(header) + "." + base64url(payload) + "." + base64url(signature)
```
- Header: `{"alg":"RS256","typ":"JWT"}` or `{"alg":"RS512","typ":"JWT"}`
- Payload: standard claims (`iss`, `sub`, `aud`, `exp`) + AH5 fields
  (`consumer`, `provider`, `target`, `targetType`, `scope`)
- Signature: `RSASSA-PKCS1-v1_5` over SHA-256/SHA-512 of `header.payload`

RSA key management:
- `JWT_PRIVATE_KEY_FILE` env var: load RSA private key from PEM file at startup
- If unset: generate an ephemeral RSA-2048 key pair at startup (not persisted)
- Single key pair serves both RSA_SHA256 and RSA_SHA512 variants (algorithm differs, key is the same)

`TRANSLATION_BRIDGE_TOKEN`: RSA_SHA256 JWT with additional payload fields:
`bridgeId string`, `fromInterface string`, `toInterface string`.
Request body must include these fields (extend `TokenGenerateRequest`).

`GET /authorization-token/public-key`: returns `{"publicKey": "<PEM string>"}` with the
RSA public key, enabling providers to verify tokens without calling this server.

**Prerequisites:** Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/consumerauth/service/auth.go` — add RSA key pair field to `AuthService`;
  add `InitJWTKeyPair(privateKeyPEM []byte)` and `GenerateEphemeralKeyPair()` helpers;
  add `RSA_SHA256`, `RSA_SHA512`, and `TRANSLATION_BRIDGE` cases to `GenerateAuthToken`;
  add `VerifyJWTAuthToken(accessToken string)` path in `VerifyAuthToken`
- `core/internal/consumerauth/model/types.go` — extend `TokenGenerateRequest` with
  `BridgeID`, `FromInterface`, `ToInterface` fields for TRANSLATION_BRIDGE variant;
  add `TokenVariantRSA256`, `TokenVariantRSA512`, `TokenVariantTranslationBridge` constants
- `core/internal/consumerauth/api/handler.go` — update `handleTokenPublicKey` to return PEM;
  update `statusFor` to map new error types
- `core/cmd/consumerauth/main.go` — read `JWT_PRIVATE_KEY_FILE`; call `InitJWTKeyPair` or
  `GenerateEphemeralKeyPair`
- `core/internal/consumerauth/service/auth_test.go` and `handler_test.go` — add JWT tests

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `JWT_PRIVATE_KEY_FILE` | *(unset)* | Path to PEM-encoded RSA private key for JWT signing. When unset, an ephemeral RSA-2048 key pair is generated at startup (tokens are valid only for the current process lifetime). |

---

### TDD cycle 51.1 — RSA_SHA256 token generates a valid JWT

**Test:** `TestGenerateRSA256Token`

Generate token with variant `RSA_SHA256_JSON_WEB_TOKEN`. Assert response contains a token
with three `.`-separated base64url sections. Decode header → assert `{"alg":"RS256","typ":"JWT"}`.
Decode payload → assert `consumer`, `provider`, `exp` fields present and `exp` is in the future.

**Expected failure before implementation:** 501 Not Implemented.

---

### TDD cycle 51.2 — RSA_SHA256 token verifies successfully

**Test:** `TestVerifyRSA256Token`

Generate RSA_SHA256 token. Call `GET /authorization-token/verify/<token>` → assert 200 and
correct `consumer`/`provider` fields in response.

---

### TDD cycle 51.3 — RSA_SHA512 token generates with RS512 header

**Test:** `TestGenerateRSA512Token`

Generate with `RSA_SHA512_JSON_WEB_TOKEN`. Decode header → assert `"alg":"RS512"`. Verify
token → assert 200.

---

### TDD cycle 51.4 — public-key endpoint returns PEM

**Test:** `TestPublicKeyEndpointReturnsPEM`

`GET /authorization-token/public-key` → assert 200, body contains `"publicKey"` field with
string starting `"-----BEGIN PUBLIC KEY-----"` or `"-----BEGIN RSA PUBLIC KEY-----"`.

**Expected failure before implementation:** 404 Not Found.

---

### TDD cycle 51.5 — TRANSLATION_BRIDGE token includes bridge fields

**Test:** `TestGenerateTranslationBridgeToken`

Generate with variant `TRANSLATION_BRIDGE_TOKEN`, `bridgeId: "bridge-1"`,
`fromInterface: "HTTP-INSECURE-JSON"`, `toInterface: "MQTT-INSECURE-JSON"`. Decode payload →
assert `bridgeId`, `fromInterface`, `toInterface` present. Verify → assert 200.

---

### TDD cycle 51.6 — Tampered JWT fails verification

**Test:** `TestTamperedJWTFailsVerification`

Generate RSA_SHA256 token. Modify the payload section (re-base64url without resigning).
Verify → assert 404 (invalid token).

---

### System test

Start ConsumerAuth (optionally with `JWT_PRIVATE_KEY_FILE`). Generate RSA_SHA256 token via
API. Call public-key endpoint and retrieve PEM. Manually verify the JWT signature using
the returned public key (e.g., with `openssl` or a small Go snippet).

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/consumerauth/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/consumerauth/`.

### Documentation updates (after Step 51)

- `core/GAP_ANALYSIS.md` — mark G47 resolved; describe key management (ephemeral vs file-backed);
  list JWT variants implemented; note TRANSLATION_BRIDGE_TOKEN fields
- `core/SPEC.md` — add `JWT_PRIVATE_KEY_FILE` to ConsumerAuthorization configuration;
  add `RSA_SHA256_JSON_WEB_TOKEN`, `RSA_SHA512_JSON_WEB_TOKEN`, `TRANSLATION_BRIDGE_TOKEN`
  to token variant table; document `GET /authorization-token/public-key` response shape;
  document `TokenGenerateRequest` bridge fields
- `core/EXAMPLES.md` — add example: RSA_SHA256 token generation, public-key fetch, verify
- `CONFORMANCE.md` — move G47 from Open → Resolved with Step 51;
  update ConsumerAuthorization ratings (significant Endpoint% and Model% gain)

### Completion criteria

- [ ] `TestGenerateRSA256Token` passes
- [ ] `TestVerifyRSA256Token` passes
- [ ] `TestGenerateRSA512Token` passes
- [ ] `TestPublicKeyEndpointReturnsPEM` passes
- [ ] `TestGenerateTranslationBridgeToken` passes
- [ ] `TestTamperedJWTFailsVerification` passes
- [ ] All existing ConsumerAuth token tests still pass (TIME_LIMITED, USAGE_LIMITED, BASE64_SELF_CONTAINED)
- [ ] `go test -race ./...` from `core/` passes
- [ ] `bash core/test-system.sh` passes
- [ ] Coverage ≥ 80% on `internal/consumerauth/`

---

## Step 52 — mTLS default enforcement (G4)

**Gap addressed:**
- **G4** — All systems support optional mTLS via `TLS_PORT`/`TLS_CERT_FILE`/`TLS_KEY_FILE`/
  `TLS_CA_FILE` (experiment-7 pattern). However, plain HTTP remains the *primary* listener;
  TLS is additive. AH5 production deployments require TLS as the default transport.

**Design:** Add `HTTPS_ONLY` env var. When `true`:
1. The TLS listener (on `TLS_PORT`) becomes the sole full-service listener.
2. The plain HTTP listener (on `PORT`) serves *only* `/health` — all other paths return 451
   (Unavailable For Legal Reasons) or 308 Redirect to the HTTPS port.
3. If `HTTPS_ONLY=true` but `TLS_PORT` is unset or `TLS_CERT_FILE`/`TLS_KEY_FILE` are missing,
   the binary logs a warning and continues with plain HTTP (fail-safe, not fail-closed — avoids
   breaking development environments).

Abstracted into a shared `tlsutil.ServeHTTPS(plainAddr, tlsAddr, handler, tlsCfg, httpsOnly bool)`
helper to avoid duplicating this logic across all 9+ binaries.

**Prerequisites:** Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/tlsutil/tlsutil.go` — add `ServeHTTPS(plainAddr, tlsAddr string, handler http.Handler, tlsCfg *tls.Config, httpsOnly bool) error` function;
  when `httpsOnly && tlsCfg != nil`: serve health-only mux on `plainAddr` in goroutine, block on TLS;
  when `!httpsOnly || tlsCfg == nil`: start optional TLS in goroutine (if configured), block on plain HTTP
- `core/internal/tlsutil/tlsutil_test.go` — add tests for `ServeHTTPS` logic
- Each `core/cmd/*/main.go` — replace the current dual-listener pattern with a call to
  `tlsutil.ServeHTTPS(plainAddr, tlsAddr, handler, tlsCfg, httpsOnly)`

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `HTTPS_ONLY` | `false` | When `true` and `TLS_PORT`/`TLS_CERT_FILE`/`TLS_KEY_FILE` are set, the plain HTTP listener serves only `/health`; all full-service traffic must use the TLS port. Silently ignored when TLS is not fully configured. |

---

### TDD cycle 52.1 — ServeHTTPS with HTTPS_ONLY=false starts plain HTTP (regression guard)

**Test:** `TestServeHTTPSFalseStartsPlainHTTP`

Call `ServeHTTPS` with `httpsOnly=false` and `tlsCfg=nil`. Confirm the plain HTTP path is
taken (observable via the handler or an in-process listener). This is the existing behaviour;
must pass before and after implementation.

---

### TDD cycle 52.2 — HTTPS_ONLY=true without TLS config falls back to plain HTTP with warning

**Test:** `TestServeHTTPSOnlyWithoutTLSFallsBack`

`httpsOnly=true`, `tlsCfg=nil`. Assert plain HTTP listener is used (fail-safe behaviour).
No panic or fatal exit.

---

### TDD cycle 52.3 — HTTPS_ONLY=true with TLS config restricts plain HTTP to health only

**Test:** `TestServeHTTPSOnlyHealthOnlyOnPlainPort`

`httpsOnly=true`, `tlsCfg` non-nil (use test TLS config). Start servers. Plain HTTP request
to `/health` → 200. Plain HTTP request to `/serviceregistry/service-discovery/register` →
non-200 (451 or 308). TLS request to full endpoint → 200.

This test requires a test TLS certificate; use a self-signed cert generated in test setup
(`tlsutil_test.go` can generate one via `crypto/tls.Generate`).

---

### System test

Generate a self-signed CA + server cert + client cert (script in `core/test-system.sh` or
inline). Start ServiceRegistry with `HTTPS_ONLY=true`, `TLS_PORT=8480`, `TLS_CERT_FILE=...`,
`TLS_KEY_FILE=...`, `TLS_CA_FILE=...`. Assert plain HTTP `/health` returns 200. Assert plain
HTTP service-discovery endpoint returns non-200. Assert mTLS service-discovery call with
valid client cert returns 200.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/tlsutil/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/tlsutil/`.

### Documentation updates (after Step 52)

- `core/GAP_ANALYSIS.md` — update G4: mark resolved; describe `HTTPS_ONLY` semantics and
  fail-safe fallback; note CA still on plain HTTP (bootstrapping constraint remains)
- `core/SPEC.md` — add `HTTPS_ONLY` to all system configuration tables; document
  health-only plain HTTP fallback when `HTTPS_ONLY=true`
- `README.md` — add `HTTPS_ONLY` to Configuration section
- `CONFORMANCE.md` — move G4 from Partial → Resolved with Step 52;
  update all system ratings (Behavior% gain across all systems)

### Completion criteria

- [ ] `TestServeHTTPSFalseStartsPlainHTTP` passes (regression guard)
- [ ] `TestServeHTTPSOnlyWithoutTLSFallsBack` passes
- [ ] `TestServeHTTPSOnlyHealthOnlyOnPlainPort` passes
- [ ] All existing TLS tests still pass
- [ ] All existing non-TLS tests still pass (no regression from `main.go` changes)
- [ ] `go test -race ./...` from `core/` passes
- [ ] `bash core/test-system.sh` passes
- [ ] Coverage ≥ 80% on `internal/tlsutil/`

---

## Step 53 — QoS full model: bandwidth, jitter, packet-loss dimensions (G53)

**Gap addressed:**
- **G53** — `QoSRecord` has only `latencyMs` and `reachable`. AH5 defines `qualityRequirements`
  that include bandwidth and jitter. Consumers cannot specify bandwidth or jitter requirements.

**Design:**
- **Jitter** (stddev of RTT): measure 5 TCP RTT samples; compute standard deviation of the
  latency values in milliseconds. Inexpensive — reuses existing TCP dial logic.
- **Bandwidth** (approximate): open TCP connection; send 64 KB of data and measure throughput
  in bytes/second. `BandwidthBps` is an approximation suitable for research/teaching; not a
  substitute for iPerf.
- **PacketLoss** (percentage): attempt N probes; count failures; `PacketLoss = failures/N * 100`.
- **Probe timeout**: `QOS_PROBE_TIMEOUT_SECONDS` env var (default 5 s) limits each individual
  probe. Total measurement time is bounded by `PROBE_SAMPLES * QOS_PROBE_TIMEOUT_SECONDS`.

DynamicOrchestration filter extension: when `maxBandwidthBps`, `maxJitterMs`, or
`maxPacketLoss` are set in a `QoSRequirement`, the QoS evaluator filter applies them in
addition to `maxLatencyMs`.

**Prerequisites:** Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/deviceqoseval/model/types.go` — add `BandwidthBps int64`, `JitterMs int64`,
  `PacketLoss float64` to `QoSRecord`; add `MaxBandwidthBps int64`, `MaxJitterMs int64`,
  `MaxPacketLoss float64` to `QoSRequirement` in `orchestration/model/types.go`
- `core/internal/deviceqoseval/service/evaluator.go` — update `Measure` to run 5-sample RTT
  (for jitter), bandwidth probe, and packet-loss count; read `QOS_PROBE_TIMEOUT_SECONDS`
- `core/internal/orchestration/dynamic/service/orchestrator.go` — extend QoS filter to check
  `BandwidthBps`, `JitterMs`, `PacketLoss` against the new `QoSRequirement` thresholds
- `core/internal/deviceqoseval/` (tests) — add multi-dimension measurement tests

**New environment variable:**

| Variable | Default | Description |
|---|---|---|
| `QOS_PROBE_TIMEOUT_SECONDS` | `5` | Timeout for each individual QoS probe (RTT dial, bandwidth transfer). Total measurement time ≤ `5 × QOS_PROBE_TIMEOUT_SECONDS`. |

---

### TDD cycle 53.1 — QoS record includes jitter and bandwidth fields

**Test:** `TestQoSRecordHasJitterAndBandwidthFields`

Use an in-process TCP echo server. Call `Measure`. Assert returned `QoSRecord` has non-zero
`JitterMs` (≥ 0), `BandwidthBps` (> 0 for reachable host), and `PacketLoss` (≥ 0.0).

**Expected failure before implementation:** `QoSRecord` has no such fields → compile error.

---

### TDD cycle 53.2 — Jitter is zero for consistently fast loopback

**Test:** `TestQoSJitterLoopbackIsLow`

Measure 5 RTT samples against loopback (127.0.0.1). Assert `JitterMs < 10` (loopback is
consistent; stddev should be near zero in a test environment).

---

### TDD cycle 53.3 — OrchestrationRequest accepts maxBandwidthBps requirement

**Test:** `TestOrchestrationQoSBandwidthRequirement`

Two fake providers. QoS evaluator returns `BandwidthBps: 1000000` for provider A,
`BandwidthBps: 100` for provider B. Request has `qualityRequirements: [{"maxBandwidthBps": 500000}]`
— this means minimum bandwidth is 500 KB/s. Assert only provider A in results.

Note: `maxBandwidthBps` in the requirement is a *minimum* threshold for inclusion (counter-intuitive
naming from the spec); document this explicitly.

---

### TDD cycle 53.4 — No QoS requirements still passes all providers (regression guard)

**Test:** `TestOrchestrationQoSNoRequirementsPassesAll`

Existing test — must still pass after model changes. No `qualityRequirements` → all providers
included regardless of QoS values.

---

### System test

Start DeviceQoSEvaluator. Trigger a measurement to a fast endpoint (e.g., localhost) and a
slow/unreachable endpoint. Query the measurement records → assert `jitterMs`, `bandwidthBps`,
`packetLoss` fields are present and populated. Submit an orchestration request with
`qualityRequirements: [{"maxBandwidthBps": 50000}]` → assert only the fast provider is returned.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out \
  ./internal/deviceqoseval/... \
  ./internal/orchestration/dynamic/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all modified packages.

### Documentation updates (after Step 53)

- `core/GAP_ANALYSIS.md` — mark G53 resolved; describe jitter (5-sample RTT stddev), bandwidth
  (TCP throughput), packet-loss (N-probe failure rate), and the maxBandwidthBps minimum-threshold semantics
- `core/SPEC.md` — add `BandwidthBps`, `JitterMs`, `PacketLoss` to `QoSRecord`; add
  `MaxBandwidthBps`, `MaxJitterMs`, `MaxPacketLoss` to `QoSRequirement`; add
  `QOS_PROBE_TIMEOUT_SECONDS` to DeviceQoSEvaluator configuration; document minimum-threshold semantics
- `core/EXAMPLES.md` — add example: orchestration with `maxBandwidthBps` requirement
- `CONFORMANCE.md` — move G53 from Open → Resolved with Step 53;
  update DeviceQoSEvaluator ratings (all three dimensions gain)

### Completion criteria

- [ ] `TestQoSRecordHasJitterAndBandwidthFields` passes
- [ ] `TestQoSJitterLoopbackIsLow` passes
- [ ] `TestOrchestrationQoSBandwidthRequirement` passes
- [ ] `TestOrchestrationQoSNoRequirementsPassesAll` passes (regression guard)
- [ ] All existing QoS evaluator tests still pass
- [ ] `go test -race ./...` from `core/` passes
- [ ] `bash core/test-system.sh` passes
- [ ] Coverage ≥ 80% on all modified packages

---

## Step 54 — Provider-change auto-push delivery (G26 residual)

**Gap addressed:**
- **G26** (residual) — Push orchestration subscriptions are in place (Step 19) and manual
  trigger delivery works (Step 31). The remaining sub-gap: no automatic delivery when the
  SR provider set changes. Triggers must be fired manually via `mgmt/push/trigger`.

**Design:**
Add a background goroutine to `DynamicOrchHandler` that polls the ServiceRegistry for each
active subscription and fires push triggers automatically when the provider set changes.

Poll strategy:
1. On every tick (interval = `PUSH_POLL_INTERVAL_SECONDS`), for each subscription in
   `SubscriptionStore`:
   - POST `SR_POLL_URL/serviceregistry/service-discovery/lookup` with `serviceDefinition = sub.TargetSystemName` (or a new `ServiceDefinition` field on `Subscription` — see below)
   - Compare the returned provider set (sorted list of provider names) against the last-known
     set stored in a local `map[subscriptionID][]string`
   - If changed: call `TriggerPush(sub)` on the orchestrator
2. If SR is unreachable, skip this tick silently (fail-open — do not remove subscriptions).

**Model change:** Add `ServiceDefinition string` to `CreateSubscriptionRequest` and `Subscription`
so the poller knows what service to poll for. This is backward-compatible: existing subscriptions
with empty `ServiceDefinition` are skipped by the poller.

**Prerequisites:** Step 31 complete (push delivery in place). Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/orchestration/dynamic/service/subscription_store.go` — add `ServiceDefinition`
  to `Subscription` and `CreateSubscriptionRequest`
- `core/internal/orchestration/dynamic/api/handler.go` — add `srPollURL string` field;
  start `startAutoPushPoller(ctx)` goroutine in handler constructor when
  `PUSH_POLL_INTERVAL_SECONDS > 0` and `SR_POLL_URL` is set; implement polling loop
- `core/cmd/dynamicorch/main.go` — read `SR_POLL_URL` and `PUSH_POLL_INTERVAL_SECONDS`;
  pass to handler
- `core/internal/orchestration/dynamic/api/handler_test.go` — add auto-push poller tests

**New environment variables:**

| Variable | Default | Description |
|---|---|---|
| `SR_POLL_URL` | *(unset)* | ServiceRegistry base URL for auto-push polling. When unset, auto-push polling is disabled. |
| `PUSH_POLL_INTERVAL_SECONDS` | `30` | Interval between provider-change poll cycles. Ignored when `SR_POLL_URL` is unset. |

---

### TDD cycle 54.1 — Poller fires trigger when provider set changes

**Test:** `TestAutoPushPollerFiresOnProviderChange`

Set up a subscription with `serviceDefinition: "temp-sensor"`. Configure a mock SR that
returns `["ProviderA"]` on the first poll and `["ProviderA", "ProviderB"]` on the second.
Assert that `TriggerPush` (or the delivery mechanism) is called after the second poll but
not the first.

Use a short poll interval (1 ms) and a mock HTTP SR to avoid real network calls.

**Expected failure before implementation:** No trigger fired (poller not implemented).

---

### TDD cycle 54.2 — Poller does not fire when provider set is unchanged

**Test:** `TestAutoPushPollerNoFireWhenUnchanged`

Mock SR always returns the same provider list. After two poll cycles, assert no trigger fired.

---

### TDD cycle 54.3 — Poller skips subscriptions with empty ServiceDefinition

**Test:** `TestAutoPushPollerSkipsEmptyServiceDefinition`

Subscription with empty `ServiceDefinition`. After one poll cycle, assert no SR call is made
for that subscription (log check or mock HTTP call count).

---

### TDD cycle 54.4 — Poller continues when SR is unreachable (fail-open)

**Test:** `TestAutoPushPollerContinuesWhenSRUnreachable`

SR URL points to a non-listening address. After two poll cycles, assert no panic or goroutine
leak. Subscriptions remain intact.

---

### System test

Start DynamicOrchestration with `SR_POLL_URL=http://localhost:8080` and
`PUSH_POLL_INTERVAL_SECONDS=2`. Subscribe with `serviceDefinition: "light-sensor"` and a
notify URL pointing to a local HTTP server. Register a new provider for `light-sensor` in SR.
Wait 3 seconds. Assert the notify URL received a push notification.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/orchestration/dynamic/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on all modified packages.

### Documentation updates (after Step 54)

- `core/GAP_ANALYSIS.md` — mark G26 fully resolved; describe polling strategy, fail-open
  semantics, and the `ServiceDefinition` field addition to Subscription
- `core/SPEC.md` — add `SR_POLL_URL` and `PUSH_POLL_INTERVAL_SECONDS` to DynamicOrchestration
  configuration; add `serviceDefinition` to `CreateSubscriptionRequest` shape; document auto-push
  trigger semantics
- `README.md` — add `SR_POLL_URL` and `PUSH_POLL_INTERVAL_SECONDS`
- `CONFORMANCE.md` — move G26 from Partial → Resolved with Step 54;
  update DynamicOrchestration Behavior% rating

### Completion criteria

- [ ] `TestAutoPushPollerFiresOnProviderChange` passes
- [ ] `TestAutoPushPollerNoFireWhenUnchanged` passes
- [ ] `TestAutoPushPollerSkipsEmptyServiceDefinition` passes
- [ ] `TestAutoPushPollerContinuesWhenSRUnreachable` passes (fail-open)
- [ ] All existing push/subscription tests still pass
- [ ] `go test -race ./...` from `core/` passes
- [ ] `bash core/test-system.sh` passes
- [ ] Coverage ≥ 80% on all modified packages

---

## Step 55 — MQTTS: MQTT over TLS (G34 residual)

**Gap addressed:**
- **G34** (residual) — The MQTT adapter (`core/internal/mqttutil`) connects via plain TCP
  (`tcp://host:1883`). MQTTS (MQTT over TLS, `ssl://host:8883`) is not implemented.
  The `MQTT-SECURE-JSON` AH5 interface is not registered.

**Design:**
Extend `NewMQTTAdapter` with an optional `*tls.Config`. When non-nil, use `ssl://` scheme
and load the TLS config into `paho.mqtt.golang` `ClientOptions`. Add a new constructor:

```go
func NewMQTTAdapterWithTLS(brokerURL, clientID, systemTopic string,
    tlsCfg *tls.Config) (*MQTTAdapter, error)
```

When `MQTT_BROKER_TLS_CA_FILE` is set, load TLS config via `tlsutil.LoadClientTLSConfig`
and use this constructor. Register `MQTT-SECURE-JSON` interface in SR alongside
(or instead of) `MQTT-INSECURE-JSON` depending on whether TLS is configured.

**New environment variables:**

| Variable | Default | Description |
|---|---|---|
| `MQTT_BROKER_TLS_CA_FILE` | *(unset)* | CA certificate for MQTT broker TLS verification. When set, MQTT connects via `ssl://` and registers `MQTT-SECURE-JSON`. |
| `MQTT_BROKER_TLS_CERT_FILE` | *(unset)* | Client certificate for mutual MQTT TLS. Optional alongside `MQTT_BROKER_TLS_CA_FILE`. |
| `MQTT_BROKER_TLS_KEY_FILE` | *(unset)* | Client private key for mutual MQTT TLS. Required when `MQTT_BROKER_TLS_CERT_FILE` is set. |

**Prerequisites:** Pre-flight check passes.

**Files to modify (core/):**
- `core/internal/mqttutil/broker.go` — add `MQTTSecureInterfaceName = "MQTT-SECURE-JSON"` constant;
  add `NewMQTTAdapterWithTLS(brokerURL, clientID, topic string, tlsCfg *tls.Config) (*MQTTAdapter, error)`;
  update `NewMQTTAdapter` to accept optional TLS via a `WithTLS(cfg *tls.Config) Option` pattern
  or overload; set `ClientOptions.SetTLSConfig(tlsCfg)` when non-nil
- Each `core/cmd/*/main.go` — read `MQTT_BROKER_TLS_CA_FILE`; if set, call
  `tlsutil.LoadClientTLSConfig` and use `NewMQTTAdapterWithTLS`; register `MQTT-SECURE-JSON`
  interface in SR registration call
- `core/internal/mqttutil/broker_test.go` — add MQTTS constructor and interface name tests

---

### TDD cycle 55.1 — MQTTSecureInterfaceName constant is defined

**Test:** `TestMQTTSecureInterfaceNameDefined`

Assert `mqttutil.MQTTSecureInterfaceName == "MQTT-SECURE-JSON"`.

**Expected failure before implementation:** Constant does not exist (compile error).

---

### TDD cycle 55.2 — NewMQTTAdapterWithTLS accepts a TLS config

**Test:** `TestNewMQTTAdapterWithTLSAcceptsTLSConfig`

Call `NewMQTTAdapterWithTLS` with a non-nil `*tls.Config` and a mock `MQTTClient`. Assert
no error and the adapter is created. (Use `NewMQTTAdapterWithClient` pattern for mock injection.)

The TLS config is set on `ClientOptions` before `Connect()`; verify by inspecting the options
struct or using a mock client that captures the options.

---

### TDD cycle 55.3 — Nil TLS config falls back to insecure connection (regression guard)

**Test:** `TestNewMQTTAdapterNilTLSFallsBackToInsecure`

Call with `tlsCfg = nil`. Assert adapter created successfully and `MQTTInterfaceName` (not
`MQTTSecureInterfaceName`) would be registered. This validates backward compatibility.

---

### System test

Start a Mosquitto broker with TLS enabled (self-signed cert). Start ServiceRegistry with
`MQTT_BROKER_URL=ssl://localhost:8883`, `MQTT_BROKER_TLS_CA_FILE=...`. Assert the system
connects to the broker and subscribes to the request topic over TLS. Assert SR registration
includes `MQTT-SECURE-JSON` in `interfaces`.

### Coverage check

```bash
cd core
go test -coverprofile=coverage.out ./internal/mqttutil/...
go tool cover -func=coverage.out
```

Target: ≥ 80% on `internal/mqttutil/`.

### Documentation updates (after Step 55)

- `core/GAP_ANALYSIS.md` — update G34: mark MQTTS sub-gap resolved; describe the TLS
  constructor and the three new env vars
- `core/SPEC.md` — add `MQTT_BROKER_TLS_CA_FILE`, `MQTT_BROKER_TLS_CERT_FILE`,
  `MQTT_BROKER_TLS_KEY_FILE` to all system configuration tables (MQTT section);
  add `MQTT-SECURE-JSON` interface to the interface registration behaviour description
- `README.md` — add three new env vars to Configuration section
- `CONFORMANCE.md` — move G34 residual from Partial → Resolved with Step 55;
  update all system Endpoint% ratings (MQTT-SECURE-JSON interface now registered)

### Completion criteria

- [ ] `TestMQTTSecureInterfaceNameDefined` passes
- [ ] `TestNewMQTTAdapterWithTLSAcceptsTLSConfig` passes
- [ ] `TestNewMQTTAdapterNilTLSFallsBackToInsecure` passes (regression guard)
- [ ] All existing MQTT adapter tests still pass
- [ ] `go test -race ./...` from `core/` passes
- [ ] Coverage ≥ 80% on `internal/mqttutil/`

---

## Step 56 — Phase 5 documentation update

**Purpose:** Ensures every authoritative document reflects the final implemented state of
Phase 5. Apply after all Phase 5 implementation steps (50–55) are complete and passing.

**Prerequisites:** Steps 50–55 all complete and passing. Pre-flight check passes.

### `core/GAP_ANALYSIS.md`

For each resolved gap add (if not done per-step):
- A `**Status: Resolved in Step N**` line at the top of its section
- An implementation summary paragraph

Gaps to update: G4, G6, G23 (TRANSLATION_BRIDGE), G26 (residual), G34 (MQTTS), G47, G53.

### `CONFORMANCE.md`

1. Move all seven gaps (G4, G6, G23 residual, G26 residual, G34 residual, G47, G53) from
   **Open/Partial Gaps** table → **Resolved Gaps** table with Step numbers 50–55.
2. Update **Per-System Ratings** for all affected systems to match Phase 5 projected values.
3. Update Phase Plan: mark Phase 5 as **Complete**.
4. Update **last updated** timestamp.

### `CONFORMANCE_UPDATE_PLAN.md`

1. Tick all completion criteria checkboxes in Steps 50–55.
2. Add Phase 5 regression matrix (section 16).
3. Update status header: `Phases 1–5 complete`.

### `core/SPEC.md`

- Add `TOKEN_AUTH_URL` to ConsumerAuthorization configuration (Step 50)
- Add `JWT_PRIVATE_KEY_FILE` to ConsumerAuthorization configuration;
  document RSA_SHA256, RSA_SHA512, TRANSLATION_BRIDGE token variants (Step 51)
- Add `HTTPS_ONLY` to all system configuration tables (Step 52)
- Add `BandwidthBps`, `JitterMs`, `PacketLoss` to `QoSRecord`;
  add `MaxBandwidthBps`, `MaxJitterMs`, `MaxPacketLoss` to `QoSRequirement`;
  add `QOS_PROBE_TIMEOUT_SECONDS` (Step 53)
- Add `SR_POLL_URL` and `PUSH_POLL_INTERVAL_SECONDS` to DynamicOrchestration;
  add `serviceDefinition` to `CreateSubscriptionRequest` (Step 54)
- Add `MQTT_BROKER_TLS_CA_FILE`, `MQTT_BROKER_TLS_CERT_FILE`, `MQTT_BROKER_TLS_KEY_FILE`
  to all systems (Step 55)

### `core/EXAMPLES.md`

- Add example: ConsumerAuth token generation with `TOKEN_AUTH_URL` identity check (Step 50)
- Add example: RSA_SHA256 JWT token generation, public-key fetch, and manual verify (Step 51)
- Add example: HTTPS_ONLY deployment with curl mTLS flags (Step 52)
- Add example: orchestration with `maxBandwidthBps` QoS requirement (Step 53)
- Add example: subscription with `serviceDefinition` and auto-push trigger (Step 54)

### `README.md`

- Add `TOKEN_AUTH_URL` to ConsumerAuthorization
- Add `JWT_PRIVATE_KEY_FILE` to ConsumerAuthorization
- Add `HTTPS_ONLY` to all systems
- Add `QOS_PROBE_TIMEOUT_SECONDS` to DeviceQoSEvaluator
- Add `SR_POLL_URL` and `PUSH_POLL_INTERVAL_SECONDS` to DynamicOrchestration
- Add `MQTT_BROKER_TLS_CA_FILE`, `MQTT_BROKER_TLS_CERT_FILE`, `MQTT_BROKER_TLS_KEY_FILE` to all systems

### Completion criteria

- [ ] All seven gaps (G4, G6, G23 residual, G26 residual, G34 residual, G47, G53) appear in
  the Resolved Gaps table in `CONFORMANCE.md`
- [ ] None of the seven gaps remain in the Open/Partial Gaps tables in `CONFORMANCE.md`
- [ ] Phase Plan row for Phase 5 shows **Complete**
- [ ] All Steps 50–55 completion criteria checkboxes are `[x]`
- [ ] All new env vars documented in `core/SPEC.md` and `README.md`
- [ ] All new/changed endpoints documented in `core/SPEC.md`
- [ ] `core/EXAMPLES.md` updated with at least one example per Phase 5 step
- [ ] `core/GAP_ANALYSIS.md` shows resolved status for all seven gaps

---

## 16. Phase 5 — Regression matrix

Run after all Phase 5 steps are complete (Steps 50–55):

| Check | Steps |
|---|---|
| `cd core && go build ./...` | All Phase 5 |
| `cd core && go vet ./...` | All Phase 5 |
| `cd core && go test -race ./...` | All Phase 5 |
| `bash core/test-system.sh` | All Phase 5 |
| `go build ./...` (workspace root) | All Phase 5 |

