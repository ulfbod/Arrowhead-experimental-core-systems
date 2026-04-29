# Gap Analysis — AH5 Compliance and Design Decisions

This document captures:
- Deviations from the official AH5 specification
- Design decisions made where the spec is silent or ambiguous
- Interpretations of the documentation
- Known ambiguities that may need revisiting

Documentation sources consulted:
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_registry/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/authorization/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/authentication/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_orchestration_dynamic/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_orchestration_simple_store/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_orchestration_flexible_store/

---

## Gaps

### G1 — FlexibleStore has no official specification

**Status:** Designed from first principles.

The AH5 documentation for FlexibleStore Orchestration is marked "Coming soon" and provides no API details, request/response shapes, or behavioral rules. The implementation extends SimpleStore with two additions:
- `priority` integer field on rules (lower value = returned first)
- `metadataFilter` map on rules (rule only matches when the request metadata is a superset of the filter)

These semantics are reasonable extensions of the SimpleStore design but are **not validated against any official specification**. The entire FlexibleStore system should be revisited once official documentation is published.

---

### G2 — No credential verification in Authentication

**Status:** Known gap, documented assumption.

`POST /authentication/identity/login` accepts any non-empty `systemName` and issues a token without verifying the `credentials` field. The AH5 spec does not define what credential verification entails (certificates, pre-shared keys, passwords), so validation has been deferred.

In production, this endpoint should perform actual credential checking before issuing tokens.

---

### G3 — Tokens are not cryptographically secure

**Status:** Known gap, sufficient for research.

Both the Authentication system and the ConsumerAuthorization token generator produce tokens derived from `time.Now().UnixNano()`, which is not cryptographically random. The Authentication token format is `hex(nanosecond timestamp)`; the ConsumerAuthorization token format is `hex(nanosecond timestamp)-consumerName-serviceDefinition`.

These are predictable and should be replaced with `crypto/rand`-based UUIDs before use in any security-sensitive scenario.

---

### G4 — No mutual TLS (mTLS)

**Status:** Out of scope for this implementation.

AH5 uses certificate-based mutual authentication between systems in production deployments. All HTTP connections here are plain HTTP. The `authenticationInfo` and `secure` fields on service instances are stored and returned but have no effect on transport security.

---

### G5 — No persistence

**Status:** By design for this use case.

All six systems store data in memory. All registrations, rules, and tokens are lost on restart. This is intentional for research and experimentation. A persistent backend (e.g. SQLite or PostgreSQL) would be required for production.

---

### G6 — Authentication and ConsumerAuthorization tokens are independent

**Status:** Architectural decision, possible gap.

The Authentication system issues identity tokens (for system login). The ConsumerAuthorization system issues authorization tokens (for service access). These two token types are not linked: the ConsumerAuthorization `/token/generate` endpoint does not require or validate an Authentication identity token from the requester. In AH5, these may be intended to be connected via the token-relay mechanism. The relationship is not documented clearly in the current AH5 docs.

---

### G7 — DynamicOrchestration calls the AH4-compatible query endpoint

**Status:** Works correctly, minor alignment issue.

The DynamicOrchestrator calls `POST /serviceregistry/query`, which is the AH4-style backward-compatible endpoint kept for this purpose. The AH5 spec defines `serviceDiscovery` as the canonical service name, but the HTTP path in this implementation is `/serviceregistry/query` for both AH4 and AH5 compatibility. This is intentional.

---

### G8 — Duplicate authorization rules are silently created

**Status:** Known gap.

`POST /authorization/grant` with identical `(consumer, provider, serviceDefinition)` values creates a second rule with a new ID rather than returning 409 Conflict. The spec does not explicitly define the expected behavior for duplicate grant requests. SPEC.md currently documents a 409, but the implementation does not enforce it. This should be reconciled.

---

### G9 — No expired-token cleanup background process

**Status:** Known minor gap.

The Authentication repository includes a `DeleteExpired()` method but there is no background goroutine calling it. Expired tokens remain in memory until they are accessed (at which point they are lazily deleted). Under high load or long uptime this could cause unbounded memory growth.

---

## Design Decisions

### D1 — Six independent binaries, one module

Each core system is its own binary (`cmd/<system>/`) but all live in the same Go module (`arrowhead/core`). This gives clean separation of concern while keeping the build simple: a single `go build ./...` produces all binaries. Systems communicate only via HTTP, never via Go package imports.

---

### D2 — FlexibleStore priority: 0 means lowest

When a FlexibleRule has `priority: 0` (the zero value, i.e. unset), it is treated as the lowest possible priority (`MaxInt32`). This allows rules to be created without specifying a priority and have them naturally fall to the bottom. Rules with explicit priority values (≥ 1) are returned first.

Alternative considered: treat 0 as highest priority. Rejected because it would make omitting the field dangerous (it would silently dominate all other rules).

---

### D3 — FlexibleStore metadataFilter direction

The `metadataFilter` on a rule is matched against the request's `requestedService.metadata`: the filter must be a **subset** of the request metadata. This means a rule with `metadataFilter: {region: "eu"}` matches a request that has `metadata: {region: "eu", unit: "celsius"}` but not a request with `metadata: {unit: "celsius"}`.

Alternative considered: match request metadata against service metadata (as SR does). Rejected because the filter is a rule-level concept, not a service-level concept.

---

### D4 — DynamicOrchestration treats ConsumerAuth failure as unauthorized

When `ENABLE_AUTH=true` and the ConsumerAuthorization system is unreachable (network error or non-200 response), the provider is excluded from results rather than included. This is a fail-closed security posture. A fail-open alternative (include on error) would be less secure but more resilient in partially degraded deployments.

---

### D5 — Port assignments

Ports were assigned to minimize conflict with common development tools:

| System | Port | Rationale |
|---|---|---|
| ServiceRegistry | 8080 | Matches AH4 default; expected by existing experiments |
| Authentication | 8081 | Sequential from SR |
| ConsumerAuthorization | 8082 | Sequential from Auth |
| DynamicOrchestration | 8083 | Sequential from CA |
| SimpleStoreOrchestration | 8084 | Sequential from Dynamic |
| FlexibleStoreOrchestration | 8085 | Sequential from SimpleStore |

All ports are overridable via the `PORT` environment variable.

---

### D6 — Path-prefixed health endpoints

Each binary registers `/health` (for direct access) and `/<system-prefix>/health` (for access through the Vite dev proxy). The prefixed routes are required because the dashboard communicates with all systems via relative paths proxied through port 5173 in development; the Vite proxy routes by path prefix, not by port. Without the prefixed route, the dashboard health check for non-SR systems would resolve to the wrong backend.

---

### D7 — Backward-compatible POST /serviceregistry/query

AH5 defines `serviceDiscovery` as the service name for the Service Registry, but this implementation keeps `POST /serviceregistry/query` as the primary discovery endpoint for backward compatibility with existing experiments. The `GET /serviceregistry/lookup` endpoint is the AH5-aligned alternative.

---

## Interpretations

### I1 — OrchestrationRequest shape is shared

The official AH5 docs define slightly different request shapes for dynamic vs. store-based orchestration. This implementation uses a single shared `OrchestrationRequest` type (`requesterSystem` + `requestedService`) for all three orchestration systems. The `requestedService.interfaces` and `requestedService.metadata` fields are used for filtering in dynamic mode and for MetadataFilter matching in FlexibleStore, but are ignored by SimpleStore.

---

### I2 — ConsumerAuthorization is separate from token-based auth

AH5 separates "ConsumerAuthorization" (can consumer X access provider Y's service Z?) from "Authentication" (is this system who it claims to be?). This implementation treats them as independent systems with independent APIs, which matches the AH5 separation of concerns. The `Authorization` path prefix is used for ConsumerAuthorization to match AH5 endpoint naming.

---

### I3 — ServiceInstance.secure field is not enforced

The `secure` field (`"NOT_SECURE"`, `"CERTIFICATE"`, `"TOKEN"`) is stored and returned as-is, but no validation of its allowed values is performed, and it has no effect on routing or access control. AH5 documents the field but does not specify server-side enforcement behavior.

---

### I4 — TokenResponse from ConsumerAuthorization omits expiry

The `POST /authorization/token/generate` response includes `token`, `consumerSystemName`, and `serviceDefinition` but not an `expiresAt` field. AH5 does not document this endpoint's response shape in detail. An expiry has been omitted since the token generation mechanism itself is a placeholder (see G3); when a proper implementation is added, expiry should be included.

---

## Ambiguities

### A1 — What should `credentials` contain in Authentication?

The `POST /authentication/identity/login` request includes a `credentials` string. The AH5 documentation does not specify the format: it could be a password, a PEM certificate, a pre-shared key, or something else. The field is accepted and ignored. The format needs to be defined before the Authentication system can perform real verification.

---

### A2 — Should duplicate grant requests return 409 or overwrite?

The AH5 documentation does not specify what happens when `POST /authorization/grant` is called twice with the same `(consumer, provider, serviceDefinition)` triple. Options are: return 409 Conflict, silently create a second rule (current behavior), or return the existing rule. See also G8.

---

### A3 — FlexibleStore priority range and meaning

No official documentation exists. It is unclear whether:
- Priority is a rank (1 = highest) or a weight
- 0 is a valid explicit value or reserved
- Negative values are allowed
- Ties in priority are broken deterministically

Current behavior: lower positive integer = higher priority; 0 = lowest priority (treated as MaxInt32); negative values are accepted and sort before priority 1.

---

### A4 — Is DynamicOrchestration expected to pass interface/version filters to SR?

`requestedService` includes optional `interfaces` and `metadata`. DynamicOrchestration forwards these to the SR query. Whether `versionRequirement` should also be forwarded (and where the consumer specifies it) is not documented. Currently, no `versionRequirement` field exists on `ServiceFilter`; version filtering is only available via the direct SR query interface.

---

### A5 — Relationship between Authentication tokens and orchestration

AH5 mentions token-relay as a mechanism where the orchestration response includes a token the consumer can use to authenticate to the provider. It is unclear whether:
- This token comes from the Authentication system
- This token comes from ConsumerAuthorization `/token/generate`
- They are the same token
- The orchestration response should include the token (currently it does not)

The `OrchestrationResult` in this implementation does not include a token field.

---

### A6 — Should unregister be DELETE or POST?

AH5 documentation for the Service Registry shows `serviceDiscovery` as a service name but does not prescribe HTTP methods for all operations. AH4 used POST for everything. This implementation uses DELETE for unregister, which is semantically correct REST. Experiments that used AH4-style POST for unregister will need updating.
