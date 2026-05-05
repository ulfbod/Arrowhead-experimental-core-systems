# Experiment 6 — Key Values

This document explains what experiment-6 adds beyond experiment-5's dual-transport
unified policy projection, and what the REST transport demonstrates about sync-delay
and policy enforcement across heterogeneous communication patterns.

---

## Baseline: dual-transport unified policy in experiment-5

Experiment-5 proves that a single XACML PolicySet in AuthzForce can simultaneously
govern two transports (AMQP and Kafka).  Revocation propagates to both within one
sync cycle.  The limitation is that REST-based consumers — which are ubiquitous in
IIoT dashboards, analytics pipelines, and ad-hoc tooling — are not covered.

---

## Values delivered by experiment-6

### 1. Third transport covered by the same policy

`rest-authz` is a reverse proxy PEP that stands in front of `data-provider`.  Every
HTTP request carries an `X-Consumer-Name` header; rest-authz queries AuthzForce for
the `(consumer, service, "invoke")` triple and either proxies the request or returns
403.  The consumer identity model is intentionally simple: there is no mTLS or token
verification at the PEP — trust is in the network layer (internal Docker network).
A production deployment would add certificate-based identity.

The key result: the same AuthzForce domain and the same XACML PolicySet that governs
AMQP consumers and Kafka SSE consumers now also governs REST consumers.  Adding a
grant in CA authorises a system on all three transports simultaneously.

### 2. Sync-delay made explicit and observable

In experiments 4 and 5 the sync delay is a documented property but it is not easy to
observe: AMQP enforcement is per-broker-operation (effectively instant after the next
sync), and Kafka enforcement is per-100-messages (bounded but not visible in the UI).

REST enforcement surfaces the sync delay more directly:
- `rest-consumer` polls every 2 s and reports `deniedCount` / `lastDeniedAt`.
- The dashboard Config tab controls `SYNC_INTERVAL` at runtime via `POST /config`.
- Setting a long interval (e.g. 60 s) creates a wide window: the user can revoke a
  grant in the Grants tab, observe REST requests continuing to return 200 OK, and then
  see the transition to 403 exactly when the sync cycle fires.

This makes the sync-delay caveat tangible rather than theoretical.

### 3. SYNC_INTERVAL is runtime-configurable without restart

`policy-sync` stores the sleep duration in an `atomic.Int64`.  The `POST /config`
endpoint updates it in-place; the next sleep iteration picks up the new value.  This
is additive relative to experiment-5 (which has a fixed `SYNC_INTERVAL` env var) and
backward-compatible: experiment-5 runs identically since it never calls `/config`.

### 4. REST data service separates policy enforcement from data storage

`data-provider` knows nothing about authorization.  It consumes from Kafka and serves
raw telemetry over HTTP.  Authorization lives entirely in `rest-authz`, which is
deployed in front of it.  This separation means:
- data-provider can be replaced or scaled independently.
- A future experiment could add row-level filtering in rest-authz without changing
  data-provider.
- The PEP → upstream split mirrors real deployments where an existing microservice
  is retrofitted with a policy enforcement layer.

---

## What experiment-6 does not deliver

- **mTLS consumer identity.** Consumer identity is taken from the `X-Consumer-Name`
  header, which is untrusted without network-layer controls.  A production system would
  verify identity via a client certificate or Bearer token.
- **Per-request attribute-based control.** XACML attributes are still limited to
  subject (consumer name) and resource (service definition).  Time-of-day, payload
  type, or geographic constraints are not expressed in the policy.
- **Kafka-native REST API.** data-provider serves the latest message only.  A
  full-featured data service would support pagination, time-range queries, and
  per-robot filtering — all subject to the same AuthzForce policy.
- **Cross-cloud federation.** AuthzForce runs within the local cloud; multi-cloud
  scenarios require inter-domain policy federation.

---

## Comparison table

| Property | Experiment-5 | Experiment-6 |
|---|---|---|
| **Transports** | AMQP + Kafka | AMQP + Kafka + REST |
| **PEP count** | 2 | 3 |
| **REST enforcement** | None | rest-authz → AuthzForce |
| **REST data service** | None | data-provider (Kafka consumer + REST API) |
| **SYNC_INTERVAL** | Fixed (env var) | Runtime-configurable (POST /config) |
| **Sync-delay observability** | Implicit | Explicit via rest-consumer deniedCount |
| **Consumer identity (REST)** | N/A | X-Consumer-Name header (network trust) |
| **Policy auditability** | XACML PolicySet versioned in AuthzForce | Same |
| **Backward compatibility** | N/A | experiment-5 unaffected (no breaking changes) |
