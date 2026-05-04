# Experiment 5 — Key Values

This document explains what experiment-5 adds beyond experiment-4's dual-layer
authorization model, and why a unified XACML policy projection matters for
multi-transport Arrowhead local clouds.

---

## Baseline: per-transport authorization in experiment-4

Experiment-4 addresses the two core geo-distribution problems (endpoint
unreachability and policy fragmentation) by:

1. Routing AMQP traffic through a shared broker (RabbitMQ) as intermediary.
2. Delegating RabbitMQ authorization to `topic-auth-http`, which queries
   ConsumerAuthorization live on every broker operation.

This works well for a single transport, but introduces a new problem when a
second transport (Kafka, MQTT, REST SSE) is added:

- Each transport requires its own authorization adapter.
- Each adapter independently queries ConsumerAuthorization.
- There is no shared policy representation: two adapters may reach different
  decisions if one caches responses while the other does not.
- Revocation propagates to each transport at a different time, creating a
  window where a grant has been revoked in CA but enforcement has not yet caught
  up on all transports.

---

## Problem context: multi-transport local clouds

Modern IIoT architectures frequently use multiple messaging transports:

- **AMQP** for real-time device telemetry (low latency, rich routing).
- **Kafka** for analytics pipelines (high throughput, replay, partitioning).
- **REST SSE** for browser-side consumers (firewall-friendly long-poll).

Each transport has its own native access-control model. Keeping those models
consistent with ConsumerAuthorization grants requires a separate integration
effort per transport. The result is policy sprawl: the "true" access control
state is the intersection of CA grants, broker ACLs, Kafka ACLs, and any
per-transport cache state — none of which are guaranteed to agree.

---

## Values delivered by experiment-5

### 1. Single policy representation governs all transports

`policy-sync` compiles CA grants into a XACML 3.0 PolicySet and uploads it to
AuthzForce. Both transport-specific PEPs (topic-auth-xacml for AMQP,
kafka-authz for Kafka) query the same AuthzForce domain. There is one policy
object, one version number, and one audit trail — regardless of how many
transports are active.

### 2. Revocation is transport-agnostic

Revoking a grant in CA causes `policy-sync` to upload a new PolicySet version
within one sync interval (default 10 s). Both PEPs begin denying access on the
next check — without any per-transport configuration change and without
understanding the internals of the other transport.

This is structurally different from experiment-4, where each adapter queried CA
independently and could diverge if one cached a stale positive decision.

### 3. XACML as a formal policy language

XACML 3.0 separates the policy authoring concern (CA → policy-sync) from the
policy evaluation concern (AuthzForce PDP). The PolicySet is a machine-readable
artefact that can be versioned, diffed, audited, and evaluated by any
XACML-conformant PDP — not a bespoke in-memory data structure.

The `deny-unless-permit` combining algorithm means the default is deny:
adding a grant is an explicit allow, and the absence of a grant is an implicit
deny with no ambiguity.

### 4. Kafka path with revocation mid-stream

`kafka-authz` demonstrates active revocation over a long-lived SSE stream.
After a grant is revoked, `kafka-authz` sends `event: revoked` and closes the
stream the next time it re-checks AuthzForce (every 100 messages). The consumer
receives a machine-readable signal that access was policy-terminated rather than
experiencing a silent connection drop.

---

## What experiment-5 does not deliver

- **Per-message attribute-based access control.** XACML attributes are currently
  limited to subject (consumer name) and resource (service definition). Message
  content, time-of-day, or sensor type conditions are not expressed in the policy.
- **Cross-cloud policy federation.** AuthzForce runs within the local cloud.
  Multi-cloud scenarios require federation between AuthzForce instances, which is
  not implemented here.
- **Kafka-native authorization.** The Kafka broker itself uses no ACLs; kafka-authz
  acts as an application-layer proxy. A future experiment could replace this with
  Kafka's native ACL integration backed by the same PDP.
- **mTLS.** All inter-service communication remains plain HTTP; the CA service is
  declared but not wired.

---

## Comparison table

| Property | Experiment-4 | Experiment-5 |
|---|---|---|
| **Transport(s)** | AMQP | AMQP + Kafka |
| **Policy authority** | ConsumerAuth (live HTTP per check) | AuthzForce XACML (compiled PolicySet) |
| **Policy language** | None — implicit in code | XACML 3.0 PolicySet |
| **PEP count** | 1 (topic-auth-http) | 2 (topic-auth-xacml + kafka-authz) |
| **PDP count** | N/A (each PEP calls CA) | 1 (AuthzForce, shared by all PEPs) |
| **Revocation propagation** | Immediate (live CA check) | Within sync interval (~10 s) |
| **Policy consistency** | Guaranteed (live) | Guaranteed within sync window |
| **Policy auditability** | None (implicit) | XACML PolicySet versioned in AuthzForce |
| **Kafka revocation** | N/A | `event: revoked` SSE message + re-check |
