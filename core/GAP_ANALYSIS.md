# Gap Analysis — AH5 Compliance and Design Decisions

This document captures the current state of AH5 alignment: what is missing relative to the official specification, which design choices were made where the spec is silent, how ambiguous documentation was interpreted, and what remains genuinely unclear.

Documentation sources (reviewed May 2026 — full site traversal):
- https://aitia-iiot.github.io/ah5-docs-java-spring/home/welcome/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_registry/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/authorization/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/authentication/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_orchestration_dynamic/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_orchestration_simple_store/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_orchestration_flexible_store/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/blacklist/
- https://aitia-iiot.github.io/ah5-docs-java-spring/support_systems/device_qos_evaluator/
- https://aitia-iiot.github.io/ah5-docs-java-spring/support_systems/translation_manager/
- https://aitia-iiot.github.io/ah5-docs-java-spring/concepts/communication_profiles/
- https://aitia-iiot.github.io/ah5-docs-java-spring/concepts/general_management/

---

## Gaps

### G1 — FlexibleStore has no official specification

The AH5 documentation page for FlexibleStore Orchestration is marked "Coming soon" and provides no API contract, request/response shapes, or behavioral rules. The implementation is designed from first principles as an extension of SimpleStore, adding:

- `priority` integer field on rules — lower value means higher priority; 0 is treated as lowest priority (see D2)
- `metadataFilter` map on rules — a rule only matches when the request's metadata is a superset of the filter (see D3)

The entire FlexibleStore system should be reviewed against official documentation once it is published.

---

### G2 — Credentials are not verified in Authentication

**Status: Resolved in Step 13**

`POST /authentication/identity/login` now verifies the submitted password against a
bcrypt-hashed credential stored in the identity repository. Unknown system names and wrong
passwords return 401. A built-in `Sysop` identity is bootstrapped at startup (password
from `SYSOP_PASSWORD` env var, default `arrowhead`). Management endpoints (G21) allow
pre-registering system identities before any login attempts.

---

### G3 — Tokens are not cryptographically secure

**Status: Resolved in Step 1**

Authentication tokens are generated as `hex(time.Now().UnixNano())`. ConsumerAuthorization tokens are generated as `hex(time.Now().UnixNano())-consumerName-serviceDefinition`. Both are predictable. Replacing these with `crypto/rand`-based UUIDs is required before any security-sensitive use.

Both token generators have been replaced with `crypto/rand` UUID v4 (see `internal/authentication/service/auth.go` and `internal/consumerauth/service/auth.go`).

---

### G4 — No mutual TLS

AH5 production deployments use certificate-based mutual authentication on all inter-system HTTP calls. All connections in this implementation are plain HTTP by default. The `authenticationInfo` and `secure` fields on service instances are stored and returned but have no effect on transport security.

**Partial closure in experiment-7:** The four mandatory core systems (ServiceRegistry :8080/:8480,
Authentication :8081/:8481, ConsumerAuthorization :8082/:8482, DynamicOrchestration :8083/:8483)
now support optional mutual TLS on a configurable `TLS_PORT`. When `TLS_CERT_FILE`, `TLS_KEY_FILE`,
and `TLS_CA_FILE` environment variables are set, the service starts an HTTPS listener with
`tls.RequireAndVerifyClientCert` alongside the existing plain HTTP listener (retained for
healthchecks and bootstrap). In experiment-7, all inter-system service calls use the TLS ports;
only the Docker healthchecks and the one-shot seed container use plain HTTP. The CA remains on
plain HTTP (bootstrapping constraint: services must reach it to get their own certificates).
See `experiments/experiment-7/DIAGRAMS.md` for full coverage details.

---

### G5 — No persistence

**Status: Resolved in Step 9**

All six systems (plus the CA) now support SQLite-backed persistence selected by the `DB_PATH` environment variable:
- Unset: in-memory Go maps (original behavior, unchanged)
- `:memory:`: SQLite in-memory (useful for test isolation)
- File path: SQLite file-backed (data survives restarts)

Each system has a `sqlite.go` implementing its repository interface via `modernc.org/sqlite` (pure Go, no CGO). The `DB_PATH` convention is documented in `SPEC.md`. Docker Compose stacks in experiments 9, 13, and 14 are configured with named volumes.

---

### G6 — Authentication and ConsumerAuthorization tokens are partially decoupled

The Authentication system issues identity tokens (who are you?). The ConsumerAuthorization system issues authorization tokens (are you allowed to call this service?). `POST /consumerauthorization/authorization/token/generate` does not require or validate a prior identity token from the Authentication system — these remain independent.

However, DynamicOrchestration now connects the two when `ENABLE_IDENTITY_CHECK=true`: the identity token from the Authentication system is verified before the ConsumerAuthorization check, and the verified systemName from the token is used as the consumer identity. This partial coupling closes the impersonation gap (see D8). A5 covers remaining ambiguities about the full AH5 token-relay mechanism.

---

### G7 — DynamicOrchestration uses the AH4-compatible query endpoint

**Status: Resolved in Step 17**

DynamicOrchestrator now calls `POST /serviceregistry/service-discovery/lookup` (the canonical AH5 endpoint) instead of the legacy `POST /serviceregistry/query`. The shared orchestration model was updated in the same step:
- `OrchestrationRequest` accepts `serviceRequirement` (AH5 spec) or `requestedService` (backward-compat alias); encodes as `serviceRequirement`
- `OrchestrationResponse` uses `results` field (was `response`); `OrchestrationResult` uses flat `providerName string` (was nested provider object)
- AH5 spec typos `serviceDefinitition` (double 't') and `cloudIdentitifer` (missing 'n') are preserved as intentional wire-format field names

---

### G8 — No expired-token background cleanup

**Status: Resolved in Step 12**

`NewAuthServiceWithCleanup(repo, tokenDuration, cleanupInterval)` starts a background goroutine that calls `repo.DeleteExpired()` on the configured interval (default 5 minutes via `NewAuthService`). Lazy deletion on access is retained as a secondary path.

---

### G10 — System and provider identity derived from request body, not auth token

AH5 `systemDiscovery/register` and `serviceDiscovery/register` are designed for
self-registration: the system name is derived from the caller's identity (mTLS
certificate or verified auth token), not from the request body. This ensures a
system cannot register under an arbitrary name.

This implementation requires the name in the request body:

- `POST /serviceregistry/system-discovery/register` — `name` field (required)
- `POST /serviceregistry/service-discovery/register` — `systemName` field (required)
- `DELETE /serviceregistry/system-discovery/revoke` — `?name=` query parameter

Similarly, `serviceDiscovery/revoke` in AH5 identifies the caller from its token;
here the instance is identified by `instanceId` in the path, which is AH5-aligned
for revoke but ownership is not enforced.

**Root cause:** Credential verification is a stub (G2). Until real auth is in place
the server cannot derive the caller's name from a token.

**Impact:** Any client can register or revoke under any name. This is acceptable for
in-memory research use but must be resolved before any security-sensitive deployment.

---

### G11 — System revoke uses query parameter instead of auth-token identity

AH5 `DELETE /serviceregistry/system-discovery/revoke` has no path or body parameter
— the system to revoke is inferred from the Authorization header. Because credential
verification is stubbed (G2, G10), this implementation uses `?name=<systemName>` as
a query parameter. This deviates from the AH5 wire protocol for this endpoint.

---

### G9 — Certificate Authority is not part of AH5

The `cmd/ca` binary and `internal/ca/` packages implement a Certificate Authority that issues and verifies X.509 ECDSA leaf certificates. This system has no counterpart in the AH5 specification; it was added for experiment-2 to support certificate-based system identity. All crypto uses Go stdlib only (`crypto/ecdsa`, `crypto/x509`, `encoding/pem`). All state is in-memory (G5 applies here too).

The CA intentionally does not enforce mutual TLS — it provides certificates that *could* be used for mTLS, but the current HTTP transport remains plain (see G4). Connecting the CA to the Authentication system's credential verification is a possible future extension.

---

### G12 — ConsumerAuthorization base path is `/authorization`, not `/consumerauthorization`

**Status: Resolved in Step 6**

AH5 names the system `ConsumerAuthorization` and rooted its API at `/consumerauthorization`. This implementation previously used `/authorization` as the base path, causing AH5-compliant clients to receive 404 errors.

All routes now use the spec-compliant prefix `/consumerauthorization/authorization/`:
- `POST /consumerauthorization/authorization/grant`
- `DELETE /consumerauthorization/authorization/revoke/{id}`
- `GET /consumerauthorization/authorization/lookup`
- `POST /consumerauthorization/authorization/verify`
- `POST /consumerauthorization/authorization/token/generate`
- `GET /consumerauthorization/authorization/health`

The orchestrator's `checkAuthorized` call was updated from `/authorization/verify` to `/consumerauthorization/authorization/verify`. All experiment Go services, docker-compose files, test-system.sh scripts, and dashboard TypeScript files were updated to match. `core/SPEC.md` was updated accordingly.

---

### G13 — ServiceInstanceID is an integer, not the composite string `SystemName|ServiceName|version`

**Status: Resolved in Step 2**

AH5 defines `ServiceInstanceID = <SystemName>|<ServiceName>|<version>` (e.g., `AlertProvider1|alertService1|1.0.0`). This composite ID is used as the path parameter in `DELETE /service-discovery/revoke/{serviceInstanceId}` and as the value in management delete requests.

This implementation auto-assigns an integer `id` to each `AH5ServiceInstance` and uses that integer as the path and query parameter.

**Impact:** Clients that construct IDs from the spec-defined format cannot address instances in this implementation.

**Fix:** `compositeServiceID(systemName, serviceDefName, version)` in `internal/repository/ah5_memory.go` constructs the key; `SaveServiceInstance` and `CreateServiceInstance` use it instead of the counter. Path parameter is correctly decoded via `r.URL.Path` (Go's HTTP server auto-decodes percent-encoding). Note: pipe characters must be percent-encoded by clients as `%7C`.

---

### G14 — Version not normalised from empty string to `1.0.0`

**Status: Resolved in Step 2**

AH5 specifies that an empty `version` field in registration requests normalises to `1.0.0` (semantic versioning). The reference implementation confirms this: `"version": ""` is stored and returned as `"1.0.0"`.

This implementation stores the version field as-is; empty string is preserved.

**Fix:** `normaliseVersion(v string)` in `internal/service/ah5_registry.go` is called in `RegisterService` and `RegisterSystem`. Management endpoints (`CreateServiceInstances`, `UpdateServiceInstances`) do NOT normalise — callers must supply the intended version explicitly.

---

### G15 — HTTP method and response-field mismatches on Authentication and ConsumerAuthorization

**Status: Partially resolved in Step 7 (Authentication endpoints)**

Several endpoints deviate from the AH5 wire protocol in HTTP method or response shape:

| Point | AH5 Spec | Implementation | Status |
|-------|----------|----------------|--------|
| Logout | `POST /authentication/identity/logout` | ~~`DELETE`~~ → `POST` | Resolved in Step 7 |
| Verify token location | Path param: `GET /authentication/identity/verify/{token}` | ~~Authorization header~~ → path param | Resolved in Step 7 |
| Login response | `{"token": "...", "expirationTime": "..."}` | ~~`expiresAt`~~ → `expirationTime`; added `sysop: false` | Resolved in Step 7 |
| Verify response | `{"verified": bool, "systemName", "loginTime", "expirationTime", "sysop"}` | Added `expirationTime` (RFC3339) and `sysop: false` | Resolved in Step 12 |
| Authorization verify response | Plain Boolean | `{"authorized": bool, "ruleId": int64}` | Remaining deviation |

All Authentication-specific deviations are fully resolved as of Step 12. The remaining deviation (ConsumerAuthorization verify returning a structured object instead of a plain boolean) is documented and intentional — the extra fields (`ruleId`) are useful for debugging.

---

### G16 — Metadata lookup uses exact match only; AH5 defines rich comparison operators

**Status: Resolved in Step 5**

The `metadataRequirementsList` / `metadataRequirementList` field in AH5 lookup requests supports structured operators: `LESS_THAN_OR_EQUALS_TO`, `GREATER_THAN_OR_EQUALS_TO`, `EQUALS_TO`, `NOT_EQUALS_TO`, `CONTAINS`, `NOT_CONTAINS`. Boolean values may be expressed as `{"key": true}` directly.

All six operators are implemented in `matchesMetadata` (`ah5_registry.go`). The `MetadataRequirement` struct and `MetadataOp` constants live in `ah5_types.go`. Both wire forms are supported:
- Structured: `{"op":"CONTAINS","value":"world"}`
- Shorthand: `"prod"` or `true` (implicit `EQUALS_TO`)

`MetadataRequirements map[string]MetadataRequirement` is wired into `LookupDevices`, `LookupSystems`, and `LookupServices`. Numeric operators (`LESS_THAN_OR_EQUALS_TO`, `GREATER_THAN_OR_EQUALS_TO`) parse both values as `float64`; non-parseable values fail the match.

---

### G17 — `alivesAt` expiry filtering not implemented

**Status: Resolved in Step 3**

AH5 `ServiceLookupRequest` includes an `alivesAt` DateTime field that filters out service instances whose `expiresAt` is before the given timestamp. This is the primary mechanism for excluding stale registrations from discovery results.

`expiresAt` is stored on `AH5ServiceInstance` but is not checked during lookup regardless of whether `alivesAt` is set.

**Fix:** `AlivesAt string` added to `model.ServiceLookupRequest`; filter applied in `LookupServices` in `ah5_registry.go`. Services with no `expiresAt` are treated as immortal and always included.

---

### G18 — `423 Locked` not returned when deleting a device with dependent systems

**Status: Resolved in Step 3**

AH5 specifies that `DELETE /serviceregistry/device-discovery/revoke/{name}` returns `423 Locked` if the device has registered systems that depend on it.

This implementation returns `200 OK` or `204 No Content` unconditionally, without checking for dependent systems.

**Fix:** `AH5Store.HasDependentSystems` in `ah5_memory.go` checks for referencing systems. `RevokeDevice` in `ah5_registry.go` returns `(bool, error)` — returns `(false, ErrLocked)` when dependents exist. `handleDeviceRevoke` in `ah5_handler.go` maps `ErrLocked` to `http.StatusLocked` (423).

---

### G19 — Naming conventions not validated at input

**Status: Resolved in Step 4**

AH5 specifies strict naming rules:

| Entity | Convention | Max | Regex |
|--------|-----------|-----|-------|
| SystemName | PascalCase, letters/digits only, starts with letter | 63 chars | `^[A-Z][A-Za-z0-9]{0,62}$` |
| DeviceName | UPPER_SNAKE_CASE, letters/digits/underscore, no trailing `_` | 63 chars | `^[A-Z][A-Z0-9_]{0,61}[A-Z0-9]$` |
| ServiceDefinitionName | camelCase, starts with letter | 63 chars | `^[a-z][A-Za-z0-9]{0,62}$` |
| InterfaceTemplateName | snake_case, starts with letter | 63 chars | `^[a-z][a-z0-9_]{0,62}$` |

Validation is enforced at the discovery register endpoints (`handleDeviceRegister`, `handleSystemRegister`, `handleServiceRegister`) and the management interface templates create endpoint. Returns 400 Bad Request for non-conformant names.

**Edge case:** `reDeviceName` requires ≥ 2 characters (start anchor `[A-Z]` + mandatory end `[A-Z0-9]`). Single-character device names are rejected. This matches the AH5 spec intent for UPPER_SNAKE_CASE identifiers.

**Implemented in:** `core/internal/api/validate.go`

---

### G20 — No pagination on query endpoints

AH5 all query and list operations accept a `pagination` object (`page`, `size`, `direction`, `sortField`) and return bounded result sets. This implementation returns the full in-memory collection on every query.

**Fix:** Add a generic `Paginate[T]` helper and apply it in each query handler across all six systems.

---

### G21 — Authentication management endpoints absent

**Status: Resolved in Step 13**

All six management endpoints are now implemented:

- `POST /authentication/mgmt/identities/query` — list registered identities
- `POST /authentication/mgmt/identities` — create identities with bcrypt-hashed credentials
- `PUT /authentication/mgmt/identities` — update credentials (bulk)
- `DELETE /authentication/mgmt/identities?names=X` — remove identities
- `POST /authentication/mgmt/sessions` — query active sessions
- `DELETE /authentication/mgmt/sessions?names=X` — revoke sessions

Identity records are stored in `MemoryIdentityRepository` (default) or `SQLiteIdentityRepository`
(when `DB_PATH` is set). This also resolves G2.

---

### G22 — Authorization policy model is simpler than AH5 specifies

**Status: Resolved in Step 14**

ConsumerAuthorization now uses the AH5 provider-centric policy model:

- `AuthorizationPolicyType`: `ALL`, `WHITELIST`, `BLACKLIST` (SYS_METADATA remains a stub)
- `instanceId`: composite string `PR|LOCAL|<provider>|<targetType>|<target>`
- `targetType`: `SERVICE_DEF` (EVENT_TYPE accepted but functionally equivalent)
- `scopedPolicies`: per-scope policy map (stored and evaluated)

The old integer-ID tuple model has been replaced. All six management endpoints are implemented.
Verify returns a plain JSON Boolean as specified by AH5 (not a wrapped object).

---

### G23 — Token management system incomplete

**Status: Partially resolved in Step 15**

AH5 ConsumerAuthorization provides a typed token lifecycle under `authorizationToken`:

- Six token variants: `TIME_LIMITED`, `USAGE_LIMITED`, `BASE64_SELF_CONTAINED`, `RSA_SHA256_JSON_WEB_TOKEN`, `RSA_SHA512_JSON_WEB_TOKEN`, `TRANSLATION_BRIDGE_TOKEN`
- `POST /consumerauthorization/authorization-token/generate` — generate a token for a specific consumer, provider, target, and scope
- `GET /consumerauthorization/authorization-token/verify/{accessToken}` — verify and decode a token
- `GET /consumerauthorization/authorization-token/public-key` — retrieve the public key for JWT verification by providers
- `POST /consumerauthorization/authorization-token/encryption-key` — register a per-provider encryption key
- `DELETE /consumerauthorization/authorization-token/encryption-key` — remove an encryption key

All five endpoints are now implemented. `TIME_LIMITED_TOKEN` is fully functional: generates a UUID token stored in memory with a 1-hour expiry, verifiable via the verify endpoint. Unsupported variants (`USAGE_LIMITED`, `BASE64_SELF_CONTAINED`, JWT variants, `TRANSLATION_BRIDGE_TOKEN`) return `501 Not Implemented`. The `public-key` endpoint returns 404 (JWT signing not implemented). Encryption keys are stored in memory only — there is no JWT signing integration.

---

### G24 — Orchestration path prefix and deployment model diverge from AH5

**Status: Resolved in Step 8**

AH5 deploys Dynamic and SimpleStore orchestration as separate systems that both expose the path `/serviceorchestration`. They are distinguished by the `orchestrationStrategy` metadata field on their registered service (`dynamic` or `simpleStore`). The pull endpoint is `POST /serviceorchestration/orchestration/pull` for both.

This implementation runs three orchestrators as separate binaries on different ports. Previously they used non-spec paths (`/orchestration/dynamic`, etc.); all three now expose `/serviceorchestration/orchestration/pull`.

| Binary | Port | Path |
|--------|------|------|
| DynamicOrch | 8083 | `/serviceorchestration/orchestration/pull` |
| SimpleStoreOrch | 8084 | `/serviceorchestration/orchestration/pull` |
| FlexibleStoreOrch | 8085 | `/serviceorchestration/orchestration/pull` |

System-specific rule management paths (`/serviceorchestration/orchestration/simplestore/rules`, `/serviceorchestration/orchestration/flexiblestore/rules`) are extension endpoints not defined by the AH5 spec (see G1). FlexibleStore itself has no spec counterpart (G1).

---

### G25 — `orchestrationFlags` absent from orchestration request

**Status: Resolved in Step 8**

AH5 `ServiceOrchestrationRequest` requires an `orchestrationFlags` object with Boolean keys: `MATCHMAKING`, `ALLOW_TRANSLATION`, `ONLY_PREFERRED`, `ONLY_EXCLUSIVE`, `ALLOW_INTERCLOUD`, `ONLY_INTERCLOUD`.

`orchestrationFlags` is now present in `OrchestrationRequest`. `MATCHMAKING` (truncate to one result) and `ONLY_PREFERRED` (filter by `preferredProviders`) are fully implemented in DynamicOrchestration. `ALLOW_TRANSLATION` and `ONLY_EXCLUSIVE` are accepted but are no-op stubs.

**Remaining gap — intercloud flags (Step 25):** `ALLOW_INTERCLOUD` and `ONLY_INTERCLOUD` are
currently accepted and silently ignored, giving callers a misleading successful response.
They should return `501 Not Implemented` to signal that intercloud orchestration is not
supported. This is addressed in Step 25.

---

### G26 — Subscription and push orchestration model — **Partially resolved (Step 19)**

AH5 defines a push-style orchestration alongside pull:

- `POST /serviceorchestration/orchestration/subscribe` — consumer subscribes with a callback interface; receives push notifications when matching providers change
- `DELETE /serviceorchestration/orchestration/unsubscribe/{id}` — cancel a subscription
- Push management endpoints for operators: subscribe on behalf of systems, trigger push, query subscriptions

**Implemented in Step 19:**
- `subscribe` and `unsubscribe` endpoints on both DynamicOrchestration and SimpleStoreOrchestration. In-memory `SubscriptionStore`; duplicate subscribe (same owner+target) overwrites and returns 200.
- Push management on DynamicOrchestration: `mgmt/push/subscribe`, `mgmt/push/unsubscribe`, `mgmt/push/trigger`, `mgmt/push/query`.
- `trigger` records a `PUSH/PENDING` history entry. Actual notification delivery to the subscriber's `notifyInterface` address is a **stub** — no HTTP call is made. This limitation is intentional for the research context.
- `core-evol/internal/orchestration` (`dynamicorch-xacml`) also implements the same subscribe/unsubscribe and push management endpoints (same in-memory stores; same semantics).

**Remaining gap:** Background provider-change polling and real notification delivery to arbitrary HTTP endpoints are not implemented.

---

### G27 — Lock management and orchestration history absent — **RESOLVED (Step 18)**

~~AH5 DynamicOrchestration exposes two management services with no implementation here.~~

Implemented in Step 18:

**Lock management** — all three endpoints now implemented with in-memory store; expired locks excluded from query.

**Orchestration history** — DynamicOrchestration records a `DONE` history entry on every successful `pull` call; `mgmt/history/query` returns all entries.

**SimpleStore mgmt paths** — also migrated to AH5-aligned paths (`mgmt/simple-store/create`, `query`, `modify-priorities`) with UUID record IDs. Old `/simplestore/rules` paths remain as aliases.

`core-evol/internal/orchestration` (`dynamicorch-xacml`) also implements lock management and history (same three lock endpoints, same `mgmt/history/query`; in-memory stores). UUID generation uses `crypto/rand` (no external dependency).

---

### G28 — Blacklist system not implemented

**Status: Resolved in Step 20**

All five AH5 Blacklist endpoints are implemented at base path `/blacklist` (port 8087):

**Discovery** (`blacklistDiscovery`):
- `GET /blacklist/lookup` — returns active, non-expired entries
- `GET /blacklist/check/{systemName}` — returns Boolean

**Management** (`blacklistManagement`):
- `POST /blacklist/mgmt/query` — list entries with optional filters (systemNames, active)
- `POST /blacklist/mgmt/create` — bulk create; `reason` mandatory; 400 if absent
- `DELETE /blacklist/mgmt/remove?names=X` — inactivates entries (sets `active: false`; does not delete records)

`DB_PATH` env var selects in-memory or SQLite backend. Integration with other core systems (enforcing blacklist checks on each request) is deferred to a future step.

---

### G30 — Service interface model used flat strings; ServiceLookupRequest accepted empty filter

**Status: Resolved in Step 16**

The `interfaces` field in `ServiceRegistrationRequest` previously allowed only structured `InterfaceInstance` objects but had no validation on the `policy` field, and the `service-discovery/lookup` endpoint accepted requests with no filter (returning all instances).

Resolved:
- `InterfaceInstance.UnmarshalJSON` now accepts both structured objects and bare strings (backward-compat: `"HTTP-INSECURE-JSON"` is wrapped as `{templateName, protocol:"http", policy:"NONE"}`).
- `SecurityPolicy` enum and `IsValidSecurityPolicy` added; `handleServiceRegister` returns 400 for unknown `policy` values.
- `handleServiceLookup` now requires at least one of `instanceIds`, `providerNames`, or `serviceDefinitionNames`; returns 400 if none are present. The management `/mgmt/service-instances/query` endpoint is unrestricted.

---

### G29 — GeneralManagement endpoints absent on all systems

**Status: Resolved in Step 21**

Both endpoints are implemented in the shared `core/internal/generalmgmt` package and registered on all eight `core/` systems (ServiceRegistry, Authentication, ConsumerAuthorization, DynamicOrchestration, SimpleStoreOrchestration, FlexibleStoreOrchestration, CertificateAuthority, Blacklist). `core-evol/internal/generalmgmt` provides the same implementation for `dynamicorch-xacml` (port 8083, prefix `serviceorchestration/orchestration`):

- `POST /<prefix>/general/mgmt/logs` — queries an in-memory ring buffer (1000 entries); supports severity (exact), loggerStr (substring), from/to (RFC3339 time range) and pagination filters; returns 400 if from > to
- `GET /<prefix>/general/mgmt/get-config?keys=X` — returns values for named configuration keys; unknown keys are omitted

All systems now use `log/slog` backed by the ring buffer. The startup `log.Printf` calls have been replaced with `slog.Info`.

---

### G34 — MQTT/MQTTS communication profiles not supported

AH5 specifies four communication profiles that every core service MUST support:

- `generic_http` — plain HTTP (implemented)
- `generic_https` — HTTP over TLS (partial — experiment-7 mTLS only)
- `generic_mqtt` — MQTT
- `generic_mqtts` — MQTT over TLS

Clients discover which profiles a service offers via the `interfaces` field in its ServiceRegistry registration. Services must handle requests arriving over any supported profile equivalently.

This implementation registers all systems with `interfaces: ["HTTP-INSECURE-JSON"]` only. No MQTT broker integration exists. Constrained IoT devices that communicate exclusively via MQTT cannot interact with any core system.

**Scope:** Cross-cutting — affects ServiceRegistry, Authentication, ConsumerAuthorization, and all Orchestration systems.

**Fix:** Requires MQTT broker (e.g., Mosquitto), per-service topic routing, and request/response mapping. High effort. For research and teaching purposes, document the gap explicitly rather than leaving it implicit.

---

### G35 — Device QoS Evaluator support system not implemented

AH5 defines a Device QoS Evaluator as a support system (not a core system) that provides quality-of-service measurements for devices and services. It exposes two services:

- `qualityEvaluation` — `POST /deviceqosevaluator/quality-evaluation/measure`: trigger a latency/RTT measurement to a target host/port
- `deviceQualityDataManagement` — management endpoints for stored quality records (`/deviceqosevaluator/quality-evaluation/mgmt/`)

The Device QoS Evaluator is consumed by the Orchestration systems when evaluating `qosRequirements` (see G40). Without it, QoS-filtered orchestration is non-functional.

This support system has no implementation in this repository. It was first documented in the AH5 docs; it is not referenced in any experiment.

**Fix:** Implement as a new binary under `core/cmd/deviceqoseval`. At minimum: accept a measurement request (target host:port), perform a TCP RTT probe, store the result, and expose the management query endpoint.

---

### G36 — Translation Manager support system not implemented

AH5 defines a Translation Manager as a support system providing protocol and data model translation bridges. It exposes three services:

- `translationBridge` — `POST /translationmanager/translation/translate`: translate a payload from a source format/protocol to a target format/protocol
- `translationReport` — `GET /translationmanager/translation/status/{bridgeId}`: inspect a bridge status
- `translationBridgeManagement` — CRUD for bridge configurations (`/translationmanager/translation/mgmt/`)

The Translation Manager is invoked by DynamicOrchestration when the `ALLOW_TRANSLATION` flag is set and no directly-compatible provider exists. This implementation accepts `ALLOW_TRANSLATION` but treats it as a no-op (G25 note).

**Fix:** Implement as a new binary. For research use, a minimal translation bridge (e.g., JSON field remapping) is sufficient. Full protocol-level translation (HTTP ↔ MQTT) is a major effort.

---

### G37 — Management access policy not implemented

AH5 defines three access control modes for all core systems' management endpoints (`/mgmt/*`):

- `sysop-only` — only the Sysop identity may call management endpoints
- `whitelist` — a pre-configured list of system names may call management endpoints
- `authorization` — ConsumerAuthorization governs access, treating management as a service

This implementation exposes all management endpoints without authentication. Any caller on the network can query, create, or delete records via management endpoints.

**Scope:** Cross-cutting — affects all eight core systems (ServiceRegistry, Authentication, ConsumerAuthorization, DynamicOrchestration, SimpleStoreOrchestration, FlexibleStoreOrchestration, CertificateAuthority, Blacklist).

**Fix:** Add a configurable `MGMT_ACCESS_POLICY` environment variable (default `sysop-only`). For `sysop-only`, validate the `Authorization: Bearer` header via the Authentication system's verify endpoint before any management operation. This is the same pattern as D8 for orchestration identity check.

---

### G38 — authorizationTokenManagement bulk endpoints not fully implemented

**Status: Partially resolved in Step 15**

AH5 ConsumerAuthorization exposes `authorizationTokenManagement` as a set of sysop bulk endpoints under `/consumerauthorization/authorization-token/mgmt/`:

- `POST mgmt/generate-tokens` — batch token generation for multiple consumers/providers
- `POST mgmt/query-tokens` — paginated query of issued tokens
- `DELETE mgmt/revoke-tokens` — batch revocation
- `POST mgmt/add-encryption-keys` — register bulk encryption keys for JWT providers
- `DELETE mgmt/remove-encryption-keys` — bulk key removal

Step 15 implemented the individual endpoints (`/generate`, `/verify`, `/public-key`, `/encryption-key`). The bulk management endpoints listed above are absent.

**Fix:** Implement each bulk endpoint as a wrapper that invokes the individual logic in a loop, returning a batch result list. Revocation is the highest-priority fix (needed for incident response).

---

### G39 — authorizationManagement bulk endpoints absent

AH5 ConsumerAuthorization exposes `authorizationManagement` bulk endpoints under `/consumerauthorization/authorization/mgmt/`:

- `POST mgmt/grant-policies` — bulk policy grant (multiple consumer/provider/service tuples)
- `DELETE mgmt/revoke-policies` — bulk policy revocation
- `POST mgmt/query-policies` — paginated query with filters (consumer, provider, targetType, policyType)
- `POST mgmt/check-policies` — non-destructive bulk check (returns which tuples are authorized)

This implementation has individual grant (`/grant`), revoke (`/revoke/{id}`), lookup (`/lookup`), and verify (`/verify`) endpoints but not the bulk management paths above.

**Fix:** Implement each endpoint. `mgmt/grant-policies` and `mgmt/revoke-policies` loop over the input list and call existing store logic. `mgmt/query-policies` and `mgmt/check-policies` are new query paths.

---

### G40 — QoS requirements not supported in orchestration requests

AH5 `ServiceOrchestrationRequest` includes a `qualityRequirements[]` array. Each entry specifies quality dimensions that candidate providers must satisfy. The orchestrator is expected to call the Device QoS Evaluator (G35) to retrieve RTT/bandwidth metrics for each candidate and filter out providers that do not meet the requirements.

This implementation's `OrchestrationRequest` has no `qualityRequirements` field. The field is silently ignored if a client sends it.

**Fix:** Add `QoSRequirements []QoSRequirement` to the request model. Initially implement as an accepted-but-ignored stub. Full evaluation requires G35 (Device QoS Evaluator). Document the stub explicitly.

**Note:** `ServiceOrchestrationResult` is also missing three spec-defined fields: `cloudIdentifier` (which cloud the provider belongs to), `exclusiveUntil` (populated from lock store when a lock exists), and `interfaces[]` (interface definitions forwarded from SR). These should be added in the same change.

---

### G41 — Blacklist Bearer token enforcement and `mode` filter absent

**Status: Resolved in Step 20 for endpoints; behavioral gaps remain**

While the Blacklist system endpoints are implemented (G28/Step 20), two behavioral gaps remain:

1. **Bearer enforcement:** `GET /blacklist/lookup` and `GET /blacklist/check/{name}` are specified to require `Authorization: Bearer <token>` (validated against the Authentication system). The implementation returns results without any authentication check.

2. **Mode filter enum:** `POST /blacklist/mgmt/query` should accept a `mode` field with values `ALL`, `ACTIVES`, or `INACTIVES`. The implementation uses `active: *bool` (three-valued boolean) which is functionally equivalent but wire-incompatible with AH5 clients that send the string enum.

**Fix:** Add an auth middleware to Blacklist discovery handlers. Change `active *bool` to `mode string` in the query request model and map `ALL`→nil, `ACTIVES`→true, `INACTIVES`→false before filtering.

---

### G42 — Blacklist enforcement not integrated with other core systems

The Blacklist system exists as a standalone service (G28/Step 20) but is not consulted by any other core system. The AH5 spec intends that:

- **ServiceRegistry** rejects `register` requests from blacklisted systems
- **Orchestration** excludes blacklisted providers from orchestration results
- **ConsumerAuthorization** rejects `grant` and `verify` for blacklisted consumers or providers

None of these cross-checks exist. A blacklisted system can freely register, be returned as an orchestration result, and obtain authorization grants.

**Fix:** Add a `BlacklistClient` interface with a `IsBlacklisted(ctx, name) (bool, error)` method. Wire it into ServiceRegistry's register handler, Orchestration's result filter, and ConsumerAuthorization's grant/verify handlers. Use `fail-closed` semantics (treat Blacklist unreachability as blacklisted) consistent with D4.

---

### G43 — Authentication `credentials` not validated as a structured object

AH5 specifies `POST /authentication/identity/login` accepts `credentials` as a JSON object
`{"password": "..."}`. The `AuthenticationMethod` enum currently only documents `PASSWORD`.

This implementation accepts `credentials` as any JSON value. If `credentials` is a bare
string, an array, or `null`, the handler currently passes whatever it receives to the bcrypt
comparison (which then fails with "wrong password") and returns 401 rather than the correct
400 for a malformed request. The `password` sub-field is not validated to be present or
non-empty before the bcrypt call.

**Root cause:** A1 (now resolved as a gap rather than an ambiguity — the spec clearly uses
`{"password": "..."}` in its login examples even if the field format is not formally specified
in a schema section).

**Impact:** Malformed login requests that omit the password key receive a misleading 401 instead
of 400. AH5 clients that construct credentials incorrectly get no actionable feedback.

**Fix:** Define a `Credentials` struct `{Password string}` with custom `UnmarshalJSON` that
rejects non-object JSON values. Return 400 if the password key is absent or the value is
not a JSON object.

---

## Design Decisions

### D1 — Six independent binaries, one Go module

Each core system is its own binary under `cmd/`. All share the Go module `arrowhead/core`. Systems communicate exclusively via HTTP; no system imports another's Go packages. A single `go build ./...` from `core/` produces all binaries.

---

### D2 — FlexibleStore: priority 0 means lowest

A `FlexibleRule` with `priority: 0` (the Go zero value, i.e. field omitted) is treated as `MaxInt32` during sorting. Rules with a positive priority are returned before rules with priority 0. This makes omitting the field safe — an unprioritized rule naturally falls to the end rather than dominating all others.

---

### D3 — FlexibleStore: metadataFilter is a rule-level subset check

A rule's `metadataFilter` must be a **subset** of the request's `requestedService.metadata` for the rule to match. A rule with `metadataFilter: {region: "eu"}` matches a request with `metadata: {region: "eu", unit: "celsius"}` but not one with only `metadata: {unit: "celsius"}`. An empty or absent filter matches every request.

This direction (filter ⊆ request) is the inverse of how the Service Registry applies metadata matching (request ⊆ stored service), because the filter here is a rule-selection predicate, not a service-capability filter.

---

### D4 — DynamicOrchestration: ConsumerAuth failure is fail-closed

When `ENABLE_AUTH=true` and the ConsumerAuthorization system returns an error or is unreachable, the candidate provider is excluded from results. The alternative — include the provider on error — would be more resilient in degraded deployments but weaker in security posture.

---

### D5 — Port assignments

| System | Port |
|---|---|
| ServiceRegistry | 8080 |
| Authentication | 8081 |
| ConsumerAuthorization | 8082 |
| DynamicOrchestration | 8083 |
| SimpleStoreOrchestration | 8084 |
| FlexibleStoreOrchestration | 8085 |

Port 8080 matches the AH4 default and is expected by existing experiments. Ports 8081–8085 are sequential. All are overridable via the `PORT` environment variable.

---

### D6 — Each binary exposes a path-prefixed health endpoint

Every binary registers both `/health` (for direct access by port) and `/<prefix>/health` (e.g. `/consumerauthorization/authorization/health`, `/orchestration/simplestore/health`). The prefixed route is needed because the dashboard in development communicates via relative paths through the Vite proxy on port 5173, which routes by path prefix. Without it, health checks for non-SR systems cannot be proxied correctly.

---

### D7 — POST /serviceregistry/query is retained alongside AH5-aligned endpoints

`POST /serviceregistry/query` is kept as the primary discovery endpoint for backward compatibility with existing experiments. `GET /serviceregistry/lookup` is available as the AH5-aligned alternative. DynamicOrchestration uses the POST form internally.

---

### D8 — DynamicOrchestration: identity verification goes beyond AH5 spec

**This is an explicit extension beyond the AH5 specification.**

AH5 does not specify that `POST /serviceorchestration/orchestration/pull` requires authentication. The spec is silent on which endpoints of which systems must be token-gated and in what order. The implementation adds an optional `ENABLE_IDENTITY_CHECK` mode that:

1. Requires an `Authorization: Bearer <token>` header on `POST /serviceorchestration/orchestration/pull`.
2. Calls `GET /authentication/identity/verify/{token}` on the Authentication system to validate the token.
3. Extracts the verified `systemName` from the token response.
4. Replaces the self-reported `requesterSystem.systemName` in the request body with the verified name for all ConsumerAuthorization checks.
5. Returns `401 Unauthorized` if the token is absent, expired, invalid, or if the Authentication system is unreachable (fail-closed).

**Motivation:** Without this check, any client can claim to be an authorized consumer by setting `requesterSystem.systemName` to an arbitrary value. ConsumerAuthorization rules check the name string only — they cannot verify identity. This extension binds the orchestration request to a verified system identity, closing the impersonation gap.

**Assumptions beyond spec:**
- The Authentication system is assumed to be available when `ENABLE_IDENTITY_CHECK=true`. An unreachable auth system blocks all orchestration (fail-closed by design, consistent with D4).
- Credential verification in login remains a stub (see G2). Until G2 is resolved, `ENABLE_IDENTITY_CHECK` prevents name spoofing but not token theft (since any system can log in as any name).
- The verification protocol is the existing `GET /authentication/identity/verify` with `Authorization: Bearer <token>`, which is within the AH5 spec for the Authentication system.
- `ENABLE_IDENTITY_CHECK` is independent of `ENABLE_AUTH`: identity verification and authorization grant checking can be combined or used separately.

---

### D10 — AH5 discovery and management run alongside the legacy endpoints

The ServiceRegistry now exposes two parallel API surfaces on port 8080:

1. **Legacy (AH4-compatible):** `POST /serviceregistry/register`, `POST /serviceregistry/query`, `GET /serviceregistry/lookup`, `DELETE /serviceregistry/unregister`. These operate on the existing `ServiceInstance` model (integer IDs, address+port system identity). Experiments 1–7 and DynamicOrchestration use these exclusively.

2. **AH5:** `/serviceregistry/device-discovery/*`, `/serviceregistry/system-discovery/*`, `/serviceregistry/service-discovery/*`, `/serviceregistry/mgmt/*`. These operate on separate in-memory stores with the AH5 `Device`, `AH5System`, `ServiceDefinition`, `InterfaceTemplate`, and `AH5ServiceInstance` models (string IDs, structured address lists, timestamps).

The two surfaces are independent at both the data and API layers — they do not share a store and operate on different model types. This avoids any risk of breaking existing experiments while adding full AH5 alignment. The stores are wired separately in `cmd/serviceregistry/main.go`.

---

### D9 — CA uses stdlib-only X.509, no external PKI library

The CA uses only Go standard library packages (`crypto/ecdsa`, `crypto/elliptic`, `crypto/rand`, `crypto/x509`, `encoding/pem`) to generate and sign certificates.  No external PKI libraries are used, consistent with CLAUDE.md's minimal-dependency rule.

The CA root certificate is self-signed ECDSA P-256, valid for 10 years from startup.  Leaf certificates use ECDSA P-256 as well, with both client and server extended key usages so they can represent any Arrowhead system in a future mTLS deployment.

Serial numbers are allocated with an `atomic.Int64`, starting at 2 (1 is reserved for the CA root), ensuring uniqueness within a process lifetime without a database.

---

## Interpretations

### I1 — A single OrchestrationRequest shape is used across all three orchestrators

The AH5 documentation suggests slightly different request shapes for dynamic vs. store-based orchestration. This implementation uses one shared type (`requesterSystem` + `requestedService`) for all three. The `requestedService.interfaces` and `requestedService.metadata` fields are forwarded to the Service Registry in dynamic mode and used for MetadataFilter matching in FlexibleStore; they are ignored by SimpleStore.

---

### I2 — ConsumerAuthorization and Authentication are fully independent systems

AH5 treats these as separate concerns: Authentication establishes identity; ConsumerAuthorization governs access. This implementation has two independent systems with no runtime coupling between them. The `/consumerauthorization/authorization/` path prefix on ConsumerAuthorization endpoints matches AH5 naming conventions.

---

### I3 — The `secure` field is stored but not enforced

`ServiceInstance.secure` accepts values such as `"NOT_SECURE"`, `"CERTIFICATE"`, `"TOKEN"`. The field is stored and returned verbatim. No validation of allowed values is performed, and the value has no effect on routing, filtering, or transport behavior. AH5 documents the field without specifying server-side enforcement.

---

### I4 — ConsumerAuthorization token response omits expiry

`POST /authorization/token/generate` returns `token`, `consumerSystemName`, and `serviceDefinition` but no `expiresAt`. AH5 does not specify the response shape for this endpoint. Expiry is omitted because the token format itself is a placeholder (see G3) and adding an expiry would imply a token lifecycle that is not yet implemented.

---

## Ambiguities

### A1 — Format and content of Authentication `credentials`

**Reclassified as G43.** The AH5 login examples consistently use `{"password": "..."}` as the
`credentials` value, making this a gap (missing validation) rather than a genuine ambiguity.
The `AuthenticationMethod` enum (`PASSWORD`) further confirms the expected format.
See G43 for the fix.

---

### A2 — FlexibleStore priority semantics

No official documentation exists. Open questions:
- Is priority a strict rank or a weight?
- Are negative values valid?
- How are ties broken? (Currently: insertion order, which is non-deterministic under concurrent access.)

---

### A3 — Version filtering in orchestration requests

`ServiceFilter` in the orchestration request has no `versionRequirement` field. Version filtering is available when querying the Service Registry directly but is not exposed through any orchestration system. Whether orchestration consumers are expected to be able to express version preferences is not specified.

---

### A4 — Token relay between orchestration and providers

AH5 mentions a token-relay mechanism where the orchestration response carries a token the consumer presents to the provider to prove authorization. It is not specified:
- Whether the token originates from Authentication or ConsumerAuthorization
- Whether the orchestration response body should include a `token` field
- How the provider validates the token

The current `OrchestrationResult` type has no token field.

---

### A5 — HTTP method for unregister

AH5 documentation refers to `serviceDiscovery` as a service name but does not prescribe HTTP methods exhaustively. AH4 used POST for all operations. This implementation uses DELETE for unregistration, which is semantically correct REST. Existing experiments that call unregister via POST will need updating.
