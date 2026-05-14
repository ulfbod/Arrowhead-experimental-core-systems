# AUTH_ALTERNATIVES — Policy Synchronisation Approaches

## Context

Experiment-10 establishes PAP/AuthzForce as the enforcement authority for data-plane
access (Kafka, AMQP, REST). The PAP holds the canonical XACML policy set and pushes
it to AuthzForce on every Create/Delete.

DynamicOrchestration still calls ConsumerAuthorization.verify (CA) to decide whether
to return a provider endpoint to a consumer. This creates a split between two
authorities for the same access-control question:

| Plane | Authority |
|---|---|
| Orchestration (which endpoints to return) | ConsumerAuthorization |
| Data plane (whether to accept traffic) | AuthzForce (via PAP) |

A consumer denied at data plane may still receive a provider endpoint from orchestration
— and vice versa, a revoked CA grant may leave a consumer stranded with no way to
re-discover its endpoint even though PAP still allows the traffic.

The four approaches below address this split.

---

## Approach Comparison

| # | Name | Description | Pros | Cons |
|---|---|---|---|---|
| **A** | PAP writes to ConsumerAuth | When a policy is added/deleted in PAP, PAP calls ConsumerAuth to add/remove the corresponding grant | — Single edit point (PAP) keeps both systems in sync automatically<br>— Consumers get correct orchestration results | — PAP grows awareness of ConsumerAuth API (tight coupling)<br>— Two writes per policy change (atomicity risk: PAP saves, CA write fails → divergence)<br>— Core ConsumerAuth is unmodified but relies on PAP knowing its internal model<br>— Circular dependency: CA → DynamicOrch → SR; now also PAP → CA |
| **B** | DynamicOrch queries AuthzForce | DynamicOrch calls AuthzForce PDP before returning results; replaces CA.verify with per-provider XACML decisions | — One authority (AuthzForce) for both planes<br>— Fail-closed: AF unavailable → deny (skip provider)<br>— Revocation propagates instantly and uniformly to both planes<br>— Policy semantics are XACML-expressive (attributes, conditions)<br>— Per-provider granularity via resource encoding `service@provider` | — DynamicOrch must be extended beyond AH5 spec (needs core-evol module)<br>— DynamicOrch becomes AF-aware; adds dependency on AuthzForce being available<br>— Two policy namespaces in PAP: `service@provider` (orchestration) and `service` (enforcement)<br>— N XACML calls per orchestration request (one per SR candidate) |
| **C** | Reverse sync service | A background service polls AuthzForce/PAP and reconciles ConsumerAuth grants accordingly | — Core systems unchanged (ConsumerAuth and DynamicOrch untouched)<br>— Conceptually clean separation | — Eventual consistency: sync lag means orchestration may serve stale grants<br>— A sync failure is silent — no immediate feedback<br>— Extra service to operate, monitor, and keep in sync with both APIs<br>— Effectively moves the split problem to a new component |
| **D** | Accept decoupling | Document that orchestration (CA) and enforcement (AuthzForce) are independent; operators maintain both | — Zero implementation cost<br>— AH5 systems fully intact | — Operators must double-manage grants in two systems<br>— Divergence between orchestration routing and enforcement becomes a permanent operational burden<br>— Consumers may receive endpoints they cannot use, or fail to receive endpoints they can use |

---

## Chosen Approach: B (with gRPC interface and separate XACML attributes)

Experiment-12 implements **Approach B**: DynamicOrchestration-XACML with per-provider
decisions through a gRPC PDP interface defined in `core-evol/proto/authorize/authorize.proto`.

### gRPC interface

The PEP (DynamicOrch-XACML) does not call AuthzForce directly. It calls `authz-pdp`, a
gRPC service that implements `authorize.proto`. This makes the contract between
DynamicOrchestration and any XACML PDP explicit and language-independent.

```protobuf
service AuthorizationPDP {
  rpc Decide(DecisionRequest) returns (DecisionResponse);
}
message DecisionRequest {
  string subject;   // consumer system name
  string service;   // service definition (XACML resource-id)
  string provider;  // provider system name (optional; XACML provider-id)
  string action;    // "orchestrate" or "consume"
}
```

### Policy namespace convention

PAP manages two namespaces distinguished by `action`, not by `resource` encoding:

| action        | provider field | Used by                              | Meaning                               |
|---|---|---|---|
| `orchestrate` | set            | authz-pdp (DynamicOrch-XACML)        | Consumer may use this specific provider |
| `consume`     | empty          | kafka-authz, topic-auth-xacml, etc.  | Consumer may use this service (any provider) |

Example: to grant `service-partner-1` access to `telemetry-rest` served by `portal-cloud-ml`:
```json
// Orchestration — per-provider (action=orchestrate + provider field)
{"subject":"service-partner-1","resource":"telemetry-rest","provider":"portal-cloud-ml","action":"orchestrate","effect":"Permit"}
// Enforcement — service-level (action=consume, no provider)
{"subject":"service-partner-1","resource":"telemetry-rest","action":"consume","effect":"Permit"}
```

The `action` value separates the two namespaces naturally. An orchestration policy
(`action=orchestrate`) never matches an enforcement request (`action=consume`) and vice versa.
No `@`-concatenation required.

### Key design decisions

1. **gRPC PDP interface** — `authorize.proto` is the canonical PEP↔PDP contract. Any
   XACML-compatible PDP that implements this interface can replace AuthzForce. gRPC
   reflection is enabled for interactive inspection with `grpcurl`.

2. **Separate XACML attributes** — `service` maps to `resource-id`; `provider` maps to
   `urn:arrowhead:attribute:provider-id`. These are distinct XACML attributes, not a
   concatenated string. Policies can match either field independently.

3. **Action-based namespacing** — `action=orchestrate` vs `action=consume` cleanly separates
   orchestration policies from enforcement policies without string encoding.

4. **Per-provider XACML loop** — for each SR candidate provider, call `authz-pdp.Decide`
   (over gRPC) with separate `service` and `provider` fields. Permits are included; Denys
   and gRPC errors are excluded (fail-closed per provider).

5. **Pluggable backend** — `AUTH_BACKEND=consumerauth` switches DynamicOrch-XACML to AH5
   ConsumerAuthorization without changing the orchestration loop. This enables direct
   comparison of the two authorization approaches.

6. **Enforcement remains service-level** — data-plane PEPs (kafka-authz, pki-rest-authz)
   call AuthzForce directly with `action=consume` and no provider attribute.

7. **`ENABLE_AUTH=false` passthrough** — disables authorization entirely, returning all SR results.
