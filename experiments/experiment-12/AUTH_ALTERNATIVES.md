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
| **B** | DynamicOrch queries AuthzForce | DynamicOrch calls AuthzForce PDP before returning results; replaces CA.verify with a single XACML decision | — One authority (AuthzForce) for both planes<br>— Single XACML call per orchestration (vs N calls for N providers)<br>— Fail-closed by default: AF unavailable → deny orchestration<br>— Revocation propagates instantly and uniformly to both planes<br>— Policy semantics are XACML-expressive (attributes, conditions) vs boolean CA grants | — DynamicOrch must be extended beyond AH5 spec (needs core-evol module)<br>— DynamicOrch becomes AF-aware; adds dependency on AuthzForce being available<br>— ConsumerAuth becomes orchestration-only (may confuse operators maintaining legacy grants) |
| **C** | Reverse sync service | A background service polls AuthzForce/PAP and reconciles ConsumerAuth grants accordingly | — Core systems unchanged (ConsumerAuth and DynamicOrch untouched)<br>— Conceptually clean separation | — Eventual consistency: sync lag means orchestration may serve stale grants<br>— A sync failure is silent — no immediate feedback<br>— Extra service to operate, monitor, and keep in sync with both APIs<br>— Effectively moves the split problem to a new component |
| **D** | Accept decoupling | Document that orchestration (CA) and enforcement (AuthzForce) are independent; operators maintain both | — Zero implementation cost<br>— AH5 systems fully intact | — Operators must double-manage grants in two systems<br>— Divergence between orchestration routing and enforcement becomes a permanent operational burden<br>— Consumers may receive endpoints they cannot use, or fail to receive endpoints they can use |

---

## Chosen Approach: B

Experiment-12 implements **Approach B**: DynamicOrchestration-XACML.

The key design decisions:

1. **Single XACML decision per orchestration request** — `AuthzForce.Decide(domainID, consumer, service, "consume")` replaces N per-provider `CA.verify` calls. The decision is made on the `(consumer, service)` pair, not `(consumer, provider, service)`, because XACML policies are managed at service-definition granularity.

2. **All-or-nothing result set** — if the consumer is Permitted, all providers of that service are returned. If Denied, the list is empty. This reflects that XACML policies in this architecture do not distinguish between individual providers.

3. **Fail-closed** — if AuthzForce is unavailable, orchestration returns an empty list. This is the same behaviour as the existing CA.verify approach when CA is unreachable.

4. **`ENABLE_AUTH=false` passthrough** — the orchestrator can be run without XACML for development or testing, matching the existing ENABLE_AUTH pattern.

5. **AH5 evolution in `core-evol/`** — the DynamicOrch-XACML service lives outside `core/` to keep the spec-compliant core intact. See `AH5_EVOL.md`.
