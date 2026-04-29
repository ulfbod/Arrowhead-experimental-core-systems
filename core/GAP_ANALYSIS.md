# Gap Analysis — AH5 Compliance and Design Decisions

This document captures the current state of AH5 alignment: what is missing relative to the official specification, which design choices were made where the spec is silent, how ambiguous documentation was interpreted, and what remains genuinely unclear.

Documentation sources:
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_registry/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/authorization/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/authentication/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_orchestration_dynamic/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_orchestration_simple_store/
- https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_orchestration_flexible_store/

---

## Gaps

### G1 — FlexibleStore has no official specification

The AH5 documentation page for FlexibleStore Orchestration is marked "Coming soon" and provides no API contract, request/response shapes, or behavioral rules. The implementation is designed from first principles as an extension of SimpleStore, adding:

- `priority` integer field on rules — lower value means higher priority; 0 is treated as lowest priority (see D2)
- `metadataFilter` map on rules — a rule only matches when the request's metadata is a superset of the filter (see D3)

The entire FlexibleStore system should be reviewed against official documentation once it is published.

---

### G2 — Credentials are not verified in Authentication

`POST /authentication/identity/login` accepts any non-empty `systemName` and issues a token. The `credentials` field is accepted and ignored. The AH5 specification does not define the credential format or the verification mechanism (certificate, pre-shared key, password), so the check is absent. See also A1.

---

### G3 — Tokens are not cryptographically secure

Authentication tokens are generated as `hex(time.Now().UnixNano())`. ConsumerAuthorization tokens are generated as `hex(time.Now().UnixNano())-consumerName-serviceDefinition`. Both are predictable. Replacing these with `crypto/rand`-based UUIDs is required before any security-sensitive use.

---

### G4 — No mutual TLS

AH5 production deployments use certificate-based mutual authentication on all inter-system HTTP calls. All connections in this implementation are plain HTTP. The `authenticationInfo` and `secure` fields on service instances are stored and returned but have no effect on transport security.

---

### G5 — No persistence

All six systems store state in memory. All data is lost on restart. This is intentional for research use; a persistent backend would be required for production.

---

### G6 — Authentication and ConsumerAuthorization tokens are decoupled

The Authentication system issues identity tokens (who are you?). The ConsumerAuthorization system issues authorization tokens (are you allowed to call this service?). The two are not linked: `POST /authorization/token/generate` does not require or validate a prior identity token from the Authentication system. AH5 describes a token-relay mechanism connecting these, but the relationship is not clearly specified in the current documentation. See also A5.

---

### G7 — DynamicOrchestration uses the AH4-compatible query endpoint

The DynamicOrchestrator calls `POST /serviceregistry/query`, which is the AH4-style endpoint kept for backward compatibility. AH5 defines `serviceDiscovery` as the canonical service name, but does not change the path structure in a way that would break this. The behavior is correct; only the endpoint naming is a minor alignment issue.

---

### G8 — No expired-token background cleanup

The Authentication repository has a `DeleteExpired()` method but no background goroutine invokes it. Expired tokens are removed lazily on access. Under sustained load or long uptime, stale expired tokens accumulate in memory until they happen to be looked up.

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

Every binary registers both `/health` (for direct access by port) and `/<prefix>/health` (e.g. `/authorization/health`, `/orchestration/simplestore/health`). The prefixed route is needed because the dashboard in development communicates via relative paths through the Vite proxy on port 5173, which routes by path prefix. Without it, health checks for non-SR systems cannot be proxied correctly.

---

### D7 — POST /serviceregistry/query is retained alongside AH5-aligned endpoints

`POST /serviceregistry/query` is kept as the primary discovery endpoint for backward compatibility with existing experiments. `GET /serviceregistry/lookup` is available as the AH5-aligned alternative. DynamicOrchestration uses the POST form internally.

---

## Interpretations

### I1 — A single OrchestrationRequest shape is used across all three orchestrators

The AH5 documentation suggests slightly different request shapes for dynamic vs. store-based orchestration. This implementation uses one shared type (`requesterSystem` + `requestedService`) for all three. The `requestedService.interfaces` and `requestedService.metadata` fields are forwarded to the Service Registry in dynamic mode and used for MetadataFilter matching in FlexibleStore; they are ignored by SimpleStore.

---

### I2 — ConsumerAuthorization and Authentication are fully independent systems

AH5 treats these as separate concerns: Authentication establishes identity; ConsumerAuthorization governs access. This implementation has two independent systems with no runtime coupling between them. The `Authorization` path prefix on ConsumerAuthorization endpoints matches AH5 naming conventions.

---

### I3 — The `secure` field is stored but not enforced

`ServiceInstance.secure` accepts values such as `"NOT_SECURE"`, `"CERTIFICATE"`, `"TOKEN"`. The field is stored and returned verbatim. No validation of allowed values is performed, and the value has no effect on routing, filtering, or transport behavior. AH5 documents the field without specifying server-side enforcement.

---

### I4 — ConsumerAuthorization token response omits expiry

`POST /authorization/token/generate` returns `token`, `consumerSystemName`, and `serviceDefinition` but no `expiresAt`. AH5 does not specify the response shape for this endpoint. Expiry is omitted because the token format itself is a placeholder (see G3) and adding an expiry would imply a token lifecycle that is not yet implemented.

---

## Ambiguities

### A1 — Format and content of Authentication `credentials`

The `POST /authentication/identity/login` body includes a `credentials` string. The AH5 documentation does not define the format. It could be a password, a PEM-encoded certificate, a pre-shared key, or a signed assertion. The verification mechanism is equally unspecified.

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
