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

**Implementation status (as of May 2026):** Phases 1, 2, and 3 complete (Steps 1–39, E1–E5).
Resolved: G2, G3, G5, G7, G8, G10, G11, G12, G13, G14, G15, G16, G17, G18, G19, G20, G21, G22, G23, G24, G25, G26, G27, G28, G29, G30, G34, G35, G36, G37, G38, G39, G40, G41, G42, G43.
Partial: G4 (mTLS experiment-7 only), G6 (optional coupling), G34 (MQTT adapter only — MQTTS unimplemented).
Open: None.
Design decisions (not conformance gaps): G1 (FlexibleStore — no spec), G9 (CA — intentional extension).

---

## Gaps

### G1 — FlexibleStore has no official specification

The AH5 documentation page for FlexibleStore Orchestration is marked "Coming soon" and provides no API contract, request/response shapes, or behavioral rules. The implementation is designed from first principles as an extension of SimpleStore, adding:

- `priority` integer field on rules — lower value means higher priority; 0 is treated as lowest priority (see D2)
- `metadataFilter` map on rules — a rule only matches when the request's metadata is a superset of the filter (see D3)

The entire FlexibleStore system should be reviewed against official documentation once it is published.

**Status:** Design decision — no official spec exists; not a conformance gap. Review when AH5 publishes the FlexibleStore specification.

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

**Status:** Partial — mTLS available in `core/` systems via `TLS_PORT`/`TLS_CERT_FILE`/`TLS_KEY_FILE`/`TLS_CA_FILE` env vars (experiment-7). CA remains plain HTTP (bootstrapping constraint). Full AH5 production mTLS across all systems is Phase 3 work.

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

**Status:** Partial — optional identity-to-authorization coupling via `ENABLE_IDENTITY_CHECK` (DynamicOrchestration only). Full AH5 token relay (ConsumerAuth requires prior Authentication token) is not enforced.

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

**Status:** Resolved in Step 33 — `VerifyTokenIdentity` helper added to `httputil`; `handleSystemRegister` and `handleServiceRegister` enforce identity when `REGISTER_AUTH_URL` is set. Fail-closed: missing token → 401; name mismatch → 403; auth unreachable → 401. Open registration retained when `REGISTER_AUTH_URL` is unset (development/PoC mode).

---

### G11 — System revoke derives identity from verified token

**Status: Resolved in Step E1**

`DELETE /serviceregistry/mgmt/systems/revoke` now extracts the `Authorization: Bearer <token>`
header and calls `GET <authURL>/authentication/identity/verify/<token>` to resolve the
system name. If the token is absent or the authentication system is unreachable or returns
a non-200 response, the request is rejected with 401 (fail-closed). When `authURL` is not
configured, the endpoint falls back to the `?name=` query parameter (backward-compatible
for unauthenticated deployments).

The legacy `?name=`-based `DELETE /serviceregistry/system-discovery/revoke` endpoint is
retained but still uses query-parameter identity.

---

### G9 — Certificate Authority is not part of AH5

The `cmd/ca` binary and `internal/ca/` packages implement a Certificate Authority that issues and verifies X.509 ECDSA leaf certificates. This system has no counterpart in the AH5 specification; it was added for experiment-2 to support certificate-based system identity. All crypto uses Go stdlib only (`crypto/ecdsa`, `crypto/x509`, `encoding/pem`). All state is in-memory (G5 applies here too).

The CA intentionally does not enforce mutual TLS — it provides certificates that *could* be used for mTLS, but the current HTTP transport remains plain (see G4). Connecting the CA to the Authentication system's credential verification is a possible future extension.

**Status:** Design decision — intentional extension beyond AH5; not a conformance gap. CA is useful for experiments requiring X.509 identity but has no AH5 spec counterpart.

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

**Status: Resolved in Steps 7 and 12** (Step 7: logout method, verify path param, login response fields; Step 12: verify response `expirationTime` and `sysop`; intentional deviation in ConsumerAuth verify response documented below)

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

**Status: Resolved in Step 29**

AH5 all query and list operations accept a `pagination` object (`page`, `size`, `direction`, `sortField`) and return bounded result sets.

**Implementation:** A generic `model.Paginate[T]` helper (`core/internal/model/paginate.go`) sorts by a string key field and applies `PageNumber`/`PageSize` offsets. Zero `PageSize` returns all results. The helper is wired into every query and list handler across all six core systems:
- ServiceRegistry: `mgmt/systems/query`, `service-discovery/lookup`
- Authentication: `mgmt/identities/query`, `mgmt/sessions`
- ConsumerAuthorization: `authorization/lookup`, `authorization/mgmt/query`, and the new bulk `mgmt/query-policies`
- DynamicOrchestration: `mgmt/push/query`, `mgmt/lock/query`, `mgmt/history/query`
- SimpleStoreOrchestration: `mgmt/simple-store/query`
- FlexibleStoreOrchestration: `mgmt/flexiblestore/rules`

All paginated responses include `count` (page size) and `totalCount` (full collection size) fields.

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

All five endpoints are now implemented. `TIME_LIMITED_TOKEN` is fully functional: generates a UUID token stored in memory with a 1-hour expiry, verifiable via the verify endpoint. `USAGE_LIMITED_TOKEN` is implemented: token expires after `maxUsageCount` verifications. `BASE64_SELF_CONTAINED` is implemented: HMAC-SHA256-signed JSON payload verifiable without server state using `HMAC_SECRET` env var. JWT variants and `TRANSLATION_BRIDGE_TOKEN` return `501 Not Implemented`. The `public-key` endpoint returns 404 (JWT signing not implemented). Encryption keys are stored in memory only — there is no JWT signing integration.

**Resolved in Step 34 (G23):** `USAGE_LIMITED_TOKEN` and `BASE64_SELF_CONTAINED` variants are now fully implemented.

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

**Intercloud flags (resolved in Step E4):** `ALLOW_INTERCLOUD` and `ONLY_INTERCLOUD` now
return `501 Not Implemented` in both DynamicOrchestration and SimpleStoreOrchestration.
The error is detected before any SR query so no work is wasted. FlexibleStore does not
expose `OrchestrationFlags` in its request model and is unaffected.

---

### G26 — Subscription and push orchestration model — **Resolved (Step 31)**

**Status: Resolved in Step 31** (partially resolved in Step 19)

AH5 defines a push-style orchestration alongside pull:

- `POST /serviceorchestration/orchestration/subscribe` — consumer subscribes with a callback interface; receives push notifications when matching providers change
- `DELETE /serviceorchestration/orchestration/unsubscribe/{id}` — cancel a subscription
- Push management endpoints for operators: subscribe on behalf of systems, trigger push, query subscriptions

**Implemented in Step 19:** `subscribe` and `unsubscribe` endpoints on both DynamicOrchestration and SimpleStoreOrchestration. Push management on DynamicOrchestration: `mgmt/push/subscribe`, `mgmt/push/unsubscribe`, `mgmt/push/trigger`, `mgmt/push/query`. Same endpoints in `core-evol/internal/orchestration` (`dynamicorch-xacml`).

**Implemented in Step 31 (delivery):**
- `mgmt/push/trigger` now performs actual HTTP POST delivery to the subscriber's `notifyInterface` URL.
- Delivery is fire-and-forget: launched in a goroutine, returns 200 to the caller immediately.
- History entry is created as `PUSH/PENDING` and updated to `PUSH/DELIVERED` (HTTP 2xx) or `PUSH/FAILED` (error or non-2xx) after delivery.
- No retry. A failed delivery is recorded and the next trigger is a clean attempt.
- `PUSH_DELIVERY_TIMEOUT_SECONDS` env var (default: `5`) controls the HTTP timeout per delivery.
- URL extracted from `notifyInterface` map: tries `"notifyUri"`, then `"uri"`, then assembles from `"address"` + `"port"` + `"path"`.
- Same delivery logic implemented in `core-evol/internal/orchestration/handler.go`.

**Remaining sub-gap:** Background provider-change polling (automatic push when registry changes) is not implemented — triggers must be fired manually via `mgmt/push/trigger`.

---

### G27 — Lock management and orchestration history absent — **RESOLVED (Step 18)**

**Status: Resolved in Step 18**

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

**Status:** Resolved in Step 38 — `core/internal/mqttutil` package provides `MQTTAdapter` with subscribe/publish support using paho.mqtt.golang. The `MQTTInterfaceName = "MQTT-INSECURE-JSON"` constant is defined. Full cross-cutting integration across all systems requires a running MQTT broker (e.g. Mosquitto); the adapter architecture is in place. MQTTS (TLS) remains unimplemented.

---

### G35 — Device QoS Evaluator support system not implemented

AH5 defines a Device QoS Evaluator as a support system (not a core system) that provides quality-of-service measurements for devices and services. It exposes two services:

- `qualityEvaluation` — `POST /deviceqosevaluator/quality-evaluation/measure`: trigger a latency/RTT measurement to a target host/port
- `deviceQualityDataManagement` — management endpoints for stored quality records (`/deviceqosevaluator/quality-evaluation/mgmt/`)

The Device QoS Evaluator is consumed by the Orchestration systems when evaluating `qosRequirements` (see G40). Without it, QoS-filtered orchestration is non-functional.

This support system has no implementation in this repository. It was first documented in the AH5 docs; it is not referenced in any experiment.

**Fix:** Implement as a new binary under `core/cmd/deviceqoseval`. At minimum: accept a measurement request (target host:port), perform a TCP RTT probe, store the result, and expose the management query endpoint.

**Status:** Resolved in Step 35 — `core/internal/deviceqoseval/` package provides model, repository, service, and API layers. Binary at `core/cmd/deviceqoseval` (port 8088). Measure endpoint performs TCP RTT probe; management query endpoint supports host/port filtering.

---

### G36 — Translation Manager support system not implemented

AH5 defines a Translation Manager as a support system providing protocol and data model translation bridges. It exposes three services:

- `translationBridge` — `POST /translationmanager/translation/translate`: translate a payload from a source format/protocol to a target format/protocol
- `translationReport` — `GET /translationmanager/translation/status/{bridgeId}`: inspect a bridge status
- `translationBridgeManagement` — CRUD for bridge configurations (`/translationmanager/translation/mgmt/`)

The Translation Manager is invoked by DynamicOrchestration when the `ALLOW_TRANSLATION` flag is set and no directly-compatible provider exists. This implementation accepts `ALLOW_TRANSLATION` but treats it as a no-op (G25 note).

**Fix:** Implement as a new binary. For research use, a minimal translation bridge (e.g., JSON field remapping) is sufficient. Full protocol-level translation (HTTP ↔ MQTT) is a major effort.

**Status:** Resolved in Step 37 — `core/internal/translationmgr/` package provides model, service (field-remapping JSON bridges), and API layers. Binary at `core/cmd/translationmgr` (port 8089). Bridge CRUD and field-key remapping translate endpoint are implemented.

---

### G37 — Management access policy not implemented

**Status: Resolved in Step 27**

AH5 defines three access control modes for all core systems' management endpoints (`/mgmt/*`):

- `sysop-only` — only the Sysop identity may call management endpoints
- `whitelist` — a pre-configured list of system names may call management endpoints
- `authorization` — ConsumerAuthorization governs access, treating management as a service

**Implementation:** `httputil.RequireManagementAuth(w, r, mgmtAuthURL, origin)` checks the `Authorization: Bearer <token>` header by calling the Authentication system's `/authentication/identity/verify` endpoint and asserting `sysop: true`. Returns 401 (missing token), 403 (non-sysop), or 503 (auth system unreachable). The `sysop` property is set automatically on the bootstrapped Sysop identity.

All eight core systems accept `MGMT_AUTH_URL` env var. When set to the Authentication system base URL, all management endpoints are guarded. When empty (default), management access is open (development mode). `core-evol/internal/orchestration` uses an inline equivalent `requireMgmtAuth` method (cannot import `core/internal/httputil`).

---

### G38 — authorizationTokenManagement bulk endpoints not fully implemented

**Status: Resolved in Step 30**

AH5 ConsumerAuthorization exposes `authorizationTokenManagement` as a set of sysop bulk endpoints under `/consumerauthorization/authorization-token/mgmt/`:

- `POST mgmt/generate-tokens` — batch token generation for multiple consumers/providers
- `POST mgmt/query-tokens` — paginated query of issued tokens
- `DELETE mgmt/revoke-tokens` — batch revocation
- `POST mgmt/add-encryption-keys` — register bulk encryption keys for JWT providers
- `DELETE mgmt/remove-encryption-keys` — bulk key removal

**Implementation:** All five bulk endpoints are now implemented in `core/internal/consumerauth/api/handler.go` and `service/auth.go`. Each bulk call iterates the input list and delegates to the existing single-item service logic; per-item errors are captured in the result without aborting the batch. `mgmt/query-tokens` uses the shared `model.Paginate[T]` helper. All endpoints require management auth (`MGMT_AUTH_URL`).

---

### G39 — authorizationManagement bulk endpoints absent

**Status: Resolved in Step 30**

AH5 ConsumerAuthorization exposes `authorizationManagement` bulk endpoints under `/consumerauthorization/authorization/mgmt/`:

- `POST mgmt/grant-policies` — bulk policy grant (multiple consumer/provider/service tuples)
- `DELETE mgmt/revoke-policies` — bulk policy revocation
- `POST mgmt/query-policies` — paginated query with filters (consumer, provider, targetType, policyType)
- `POST mgmt/check-policies` — non-destructive bulk check (returns which tuples are authorized)

**Implementation:** All four endpoints are now implemented in `core/internal/consumerauth/api/handler.go` and `service/auth.go`. `mgmt/grant-policies` iterates the input policies list, calls `Grant` per item, and returns a per-item result including the created instanceId or error string. `mgmt/revoke-policies` accepts `{"instanceIds": [...]}` in the request body. `mgmt/query-policies` supports optional filter (instanceIds, targetNames, cloudIdentifiers) and pagination. `mgmt/check-policies` accepts a list of `VerifyRequest` tuples and returns each with an `authorized: bool` field. All endpoints require management auth.

---

### G40 — QoS requirements not supported in orchestration requests

**Status:** Resolved in Step 36 — `QualityRequirements []QoSRequirement` added to `OrchestrationRequest`. `DynamicOrchestrator` accepts a `QoSEvaluatorClient` interface; when `qualityRequirements` are present the orchestrator calls `Measure(host, port)` for each candidate and excludes unreachable providers or those exceeding `maxLatencyMs`. Fail-open: QoS evaluator unreachable → candidate included. `QOS_EVALUATOR_URL` env var wires the real Device QoS Evaluator (G35) into `cmd/dynamicorch`.

AH5 `ServiceOrchestrationRequest` includes a `qualityRequirements[]` array. Each entry specifies quality dimensions that candidate providers must satisfy. The orchestrator is expected to call the Device QoS Evaluator (G35) to retrieve RTT/bandwidth metrics for each candidate and filter out providers that do not meet the requirements.

**Fix:** Completed. `QoSRequirements []QoSRequirement` added to the request model; full QoS-based filtering via Device QoS Evaluator implemented.

**Result-fields resolved in Step E3:** `OrchestrationResult` now includes:
- `cloudIdentifier` — always `"LOCAL"` (this implementation is a single-cloud deployment)
- `interfaces[]` — forwarded from the SR response for DynamicOrchestration; forwarded from stored rule for SimpleStore/FlexibleStore
- `exclusiveUntil` — populated from the lock store when a lock exists (DynamicOrchestration); empty for store-based orchestrators

---

### G41 — Blacklist Bearer token enforcement and `mode` filter

**Status: Resolved in Step E2 (Step H)**

Both behavioral gaps are now closed:

1. **Bearer enforcement:** When `authURL` is configured, `GET /blacklist/lookup` and `GET /blacklist/check/{name}` require `Authorization: Bearer <token>`. A missing token returns 401. Without `authURL` (unauthenticated deployments), the endpoints remain open.

2. **Mode filter enum:** `POST /blacklist/mgmt/query` accepts `mode` field with values `ALL`, `ACTIVES`, or `INACTIVES`. An invalid mode value returns 400. The mode is mapped to the internal active filter before querying.

---

### G42 — Blacklist enforcement not integrated with other core systems

**Status: Resolved in Step 28**

The Blacklist system exists as a standalone service (G28/Step 20) but was not consulted by any other core system.

**Implementation:** `core/internal/blacklist/client/client.go` defines a `BlacklistClient` interface with `IsBlacklisted(ctx, name) (bool, error)` and two implementations:
- `HTTPClient` — calls the Blacklist system's `/blacklist/check/{name}` endpoint; **fail-closed**: returns `(true, nil)` on any network error
- `NopClient` — always returns `(false, nil)` (used in tests and when `BLACKLIST_URL` is unset)

Wired into:
- **ServiceRegistry** (`register` handler): rejects requests from blacklisted `providerSystem.systemName` with 403
- **DynamicOrchestration** (`Orchestrate`): rejects blacklisted requesters (Step 2.5); filters blacklisted providers from results (Step 4)
- **ConsumerAuthorization** (`grant` handler): rejects blacklisted provider with 403; (`verify` handler): returns `false` for blacklisted consumer or provider (without 4xx — consistent with verify semantics)
- `CertificateAuthority` (`/ca/sign` handler): rejects blacklisted system names with 403

All systems accept `BLACKLIST_URL` env var. When empty (default), `NopClient{}` is used and no blacklist check is performed.

---

### G43 — Authentication `credentials` validated as a structured object

**Status: Resolved in Step E5**

When an identity repository is configured, `POST /authentication/identity/login` now strictly
validates the `credentials` field:

- `null`, missing, or non-object values → **400 Bad Request**
- Object without a `"password"` key → **400 Bad Request**
- Valid `{"password": "..."}` object → proceeds to bcrypt verification

Without an identity repository (unauthenticated mode), the handler still accepts plain strings
and objects for backward compatibility.

Seven experiment service files have been updated to send `{"password": "..."}` objects instead
of plain strings: experiment-4, 5, 7 (consumer-direct-tls, robot-fleet-tls), and
experiment-13 (robot-fleet-tls).

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
