# ARROWHEAD_COMPLIANCE_AND_ADDED_VALUES.md — Experiment 8

Assessment of Arrowhead 5.2 compliance, compliance gaps, and added value.

The central theme of this assessment is the architectural decision to use a **separate,
transport-agnostic Policy Decision Point (PDP)** — AuthzForce with XACML — as the
authorization arbiter across multiple heterogeneous communication models. This is the
primary design choice that differentiates the experiment series from the AH5.2 specification,
and it is also the most significant architectural contribution. The PKI profile hierarchy
(experiment-8's other main addition) feeds directly into the identity model that this PDP
relies on.

---

## 1. The Central Design Choice: Transport-Agnostic PDP

### 1.1 The Problem

IoT systems are inherently multi-protocol. A robot fleet publishes telemetry simultaneously
to an AMQP broker (RabbitMQ) and a streaming platform (Kafka). Analytics pipelines consume
via SSE or Kafka. REST APIs serve on-demand queries. Future devices may use MQTT for
constrained-network publishing. Management tools use HTTP.

The AH5.2 authorization specification (§9) defines authorization using **JWT bearer tokens**
issued per service interaction. This approach has well-known properties:

- A token is bearer-specific: whoever holds it can use it (no identity binding at transport layer)
- Token revocation requires either short expiry (operational overhead) or a revocation list per service
- JWT is fundamentally a REST/HTTP construct; applying it to AMQP, Kafka, or MQTT requires embedding tokens in application payloads, which is not natively supported by those protocols
- Separate token issuers per transport create policy drift — a grant may be encoded differently in Kafka vs AMQP vs REST, and the two can fall out of sync

### 1.2 The Solution: Separate PDP with Per-Transport PEPs

This experiment series uses a **centralized XACML Policy Decision Point** (AuthzForce, one
per experiment domain) combined with a **per-transport Policy Enforcement Point** (PEP) that
delegates all authorization decisions to the PDP:

```
ConsumerAuth (authority)
      │
      ▼  policy-sync polls every SYNC_INTERVAL, compiles to XACML, uploads
AuthzForce PDP (domain: arrowhead-exp8)
      │
      ├─── topic-auth-xacml    ← AMQP broker plugin, intercepts broker operations
      ├─── kafka-authz         ← Kafka SSE gateway, enforces on connect + per 100 msgs
      ├─── pki-rest-authz      ← mTLS reverse proxy, enforces per REST request
      └─── (mqtt-authz)        ← MQTT broker plugin (not demonstrated; same pattern)
```

Each PEP knows how to speak XACML to AuthzForce. None of them carry policy state — they
are stateless enforcement proxies. The PDP is the single source of truth.

### 1.3 Why This Matters

**Grant once, enforced everywhere.** Adding a ConsumerAuth grant for
`(pki-consumer, data-provider-tls, telemetry-rest)` means that after one SYNC_INTERVAL,
pki-consumer is authorized to access `telemetry-rest` via REST (mTLS). If a second
PEP for the same service existed (MQTT, AMQP), the same grant covers it — no additional
configuration is needed per transport.

**Revoke once, propagated everywhere.** Removing a ConsumerAuth grant propagates to ALL
PEPs within one SYNC_INTERVAL. With JWT, you would need to wait for token expiry (or push
revocations to each service's revocation list separately).

**Transport-neutral identity.** The XACML subject is a string (the consumer system name),
not a token format. In the REST path it comes from the X.509 certificate CN. In the AMQP
path it is the username. In a hypothetical MQTT path it would be the client ID. The PDP
sees the same request shape regardless of which PEP produced it.

**Uniform policy expression.** XACML policy rules are transport-agnostic predicates:
`subject.id = "pki-consumer" AND resource.id = "telemetry-rest" AND action.id = "invoke"`.
The same rule applies whether the request arrives from a Kafka consumer, an AMQP client,
or a REST caller.

---

## 2. Multi-Transport Enforcement: Implementation Details

### 2.1 Transport Coverage in Experiment 8

| Transport | Protocol | PEP | Enforcement Point | AuthzForce Call |
|---|---|---|---|---|
| AMQP | AMQPS/RabbitMQ (port 5671) | `topic-auth-xacml` | Per broker operation (connect, publish, consume) | Synchronous; zero cache TTL |
| Kafka/SSE | Kafka SSL (port 9092) via SSE | `kafka-authz` | On SSE connect + every 100 messages | Re-authorized mid-stream |
| REST/mTLS | HTTPS (port 9108) | `pki-rest-authz` | Per request | Synchronous; zero cache TTL |
| MQTT | (not demonstrated) | (not implemented) | Would intercept CONNECT and SUBSCRIBE | Same pattern as topic-auth-xacml |

### 2.2 How Each PEP Extracts Consumer Identity

The most security-sensitive question in a multi-transport system is: **how does the PEP
know who the consumer is, and can that identity be forged?**

| PEP | Identity Source | Forgeable? | Notes |
|---|---|---|---|
| `topic-auth-xacml` | AMQP username in CONNECT frame | Requires valid password | Broker-level authentication; username passed to AuthzForce |
| `kafka-authz` | URL path parameter (`/stream/{consumerName}`) | Yes (HTTP header only) | Self-reported name; adequate for closed experiments, weaker security |
| `pki-rest-authz` (exp-7) | X.509 cert CN — any CA-issued cert | No — TLS-verified | Cryptographic identity; cert issued by CA |
| `pki-rest-authz` (exp-8) | X.509 cert CN — OU=sy cert required | No — TLS + profile | Strongest: cryptographic identity AND profile enforcement |

This progression — from self-reported identity (AMQP username, Kafka URL parameter) to
cryptographically verified identity with profile enforcement — is the experiment series'
main narrative arc. In all cases the same PDP is consulted; only the quality of the identity
input improves.

### 2.3 Enforcement Timing Model

Different transports enforce authorization at different points in the interaction lifecycle:

| Transport | When enforced | Latency effect | Revocation behavior |
|---|---|---|---|
| AMQP (topic-auth-xacml) | Per broker operation | Negligible (AuthzForce co-located) | Effective immediately on next operation |
| Kafka/SSE (kafka-authz) | On SSE connect + every 100 msgs | SSE stream may carry up to 99 msgs after revocation | "revoked" event sent; stream torn down within 100 messages |
| REST/mTLS (pki-rest-authz) | Per HTTP request | Negligible | Effective immediately on next request |
| Policy propagation (all) | policy-sync uploads every SYNC_INTERVAL | Up to SYNC_INTERVAL lag | AuthzForce holds stale policy until next sync |

The **SYNC_INTERVAL lag** is the governing constraint for all PEPs. A revoked ConsumerAuth
grant does not affect any PEP until policy-sync runs the next cycle. The dashboard allows
SYNC_INTERVAL to be changed at runtime to demonstrate this caveat interactively.

---

## 3. AH5.2 Compliance Assessment

### 3.1 PKI and Identity

| AH5.2 Requirement | Section | Status | Notes |
|---|---|---|---|
| Local Cloud CA with profile hierarchy (lo/on/de/sy) | §6 | ✓ **Implemented** | `profile-ca` implements full hierarchy with enforcement |
| Onboarding cert via HTTP bootstrap | §6.2 | ✓ **Implemented** | `POST /bootstrap/onboarding-cert` on HTTP :8087 |
| Device cert via mTLS + OU=on | §6.3 | ✓ **Implemented** | `POST /profile/device-cert` on mTLS :8088 |
| System cert via mTLS + OU=de | §6.3 | ✓ **Implemented** | `POST /profile/system-cert` on mTLS :8088 |
| System cert identity for service access | §6.4 | ✓ **Implemented** | `pki-rest-authz` enforces OU=sy at TLS layer |
| AH5.2 hierarchical CN format | §6.1 | ~ **Partial** | Supported when cloudName+operatorName provided |
| Certificate revocation (CRL) | §6.4 | ~ **Partial** | In-memory only; no persistent CRL |
| Master CA → Org CA → Cloud CA hierarchy | §6 | ✗ **Scoped** | Only Local Cloud CA; full org hierarchy out of scope |

### 3.2 Service Interaction and Authorization

| AH5.2 Requirement | Section | Status | Notes |
|---|---|---|---|
| Service discovery via ServiceRegistry | §5 | ✓ **Demonstrated** | SR query demonstrated in PKI Added Value tab |
| Authorization via JWT bearer tokens | §9 | ✗ **Deviated** | XACML/AuthzForce used instead — see Section 4 |
| Per-service authorization check | §9 | ✓ **Implemented** | Every PEP checks AuthzForce per interaction |
| Authorization revocation | §9 | ✓ **Implemented** | Propagates to all PEPs within SYNC_INTERVAL |
| All inter-system communication via mTLS | §8 | ~ **Partial** | Core systems have optional TLS_PORT; plain HTTP retained for Docker |

---

## 4. The Authorization Deviation: XACML vs JWT

The AH5.2 specification defines a JWT-based authorization model (§9). This experiment series
intentionally deviates to XACML/AuthzForce. This section makes the tradeoffs explicit.

### 4.1 Why JWT Does Not Fit Multi-Transport Well

AH5.2 JWT authorization tokens are issued per service interaction by the Authorization
system and carried as Bearer tokens in HTTP headers. This model has inherent limitations
for multi-transport deployments:

- **HTTP-centric**: JWT Bearer tokens are designed for HTTP Authorization headers. AMQP, Kafka, and MQTT have no standard header mechanism for JWT in the base protocol. Tokens would need to be embedded in application-level message envelopes, requiring changes to all producers and consumers.
- **Decentralized enforcement**: Each service must validate JWT signatures locally. This distributes key material and validation logic, increasing the attack surface. In the PEP pattern, validation is centralized in the PDP.
- **Revocation latency**: JWT revocation requires either very short token lifetimes (creating high token-issuance overhead) or a per-service revocation endpoint. The AH5.2 spec does not define a revocation mechanism.
- **Policy as tokens**: JWT encodes authorization decisions as token claims at issuance time. The policy state is frozen in the token. XACML evaluates policy at decision time — the PDP always sees the current policy, not the policy as it was when a token was issued.

### 4.2 XACML/AuthzForce Tradeoffs

| Property | JWT (AH5.2 spec) | XACML/AuthzForce (this experiment) |
|---|---|---|
| Transport compatibility | HTTP native; AMQP/Kafka require embedding | Transport-neutral (PEP translates per transport) |
| Revocation model | Token expiry or per-service revocation list | Single policy update propagates to all PEPs |
| Policy evaluation time | At token issuance (offline) | Per request (online, current policy) |
| Spec compliance | Fully compliant | Intentional deviation (documented in GAP_ANALYSIS G3) |
| Policy complexity | Limited to token claims | Full XACML predicates (subject/resource/action/environment) |
| Latency | Near-zero (JWT validation is local) | Round-trip to AuthzForce per request |
| Single point of failure | No (JWT validated locally) | Yes (AuthzForce must be reachable) |
| Operational complexity | Token issuance infrastructure | AuthzForce instance; policy-sync service |

The XACML/AuthzForce approach trades spec compliance for operational uniformity across
transports and richer policy semantics. For a research experiment demonstrating multi-transport
authorization, it is the better architectural fit. For a production AH5.2 deployment
requiring interoperability, JWT would be necessary.

---

## 5. Certificate Hierarchy Comparison

| Level | AH5.2 Spec | Experiment-7 | Experiment-8 |
|---|---|---|---|
| Root | Master CA (per organization) | Single flat CA (core `ca`) | profile-ca root (lo, self-signed) |
| Intermediate | Org CA | N/A | N/A (scoped to Local Cloud) |
| Local Cloud | Local Cloud CA | N/A | profile-ca (:8087/:8088) |
| Onboarding | OU=on, via HTTP bootstrap | N/A (any cert accepted) | ✓ `POST /bootstrap/onboarding-cert` |
| Device | OU=de, via mTLS + OU=on | N/A | ✓ `POST /profile/device-cert` |
| System | OU=sy, via mTLS + OU=de | Any cert from flat CA | ✓ `POST /profile/system-cert` |

The profile hierarchy feeds the PDP: in experiment-8 the XACML subject identity is derived
from a certificate that has been cryptographically bound to a specific lifecycle stage. An
attacker cannot claim a system-level identity without possessing a valid OU=sy certificate,
which in turn requires having previously obtained OU=on and OU=de certificates in order.

---

## 6. mTLS Authorization Identity Progression

| Version | Consumer Identity Source | Forgeable? | PDP Input Quality |
|---|---|---|---|
| Experiment-6 | `X-Consumer-Name` HTTP header | Yes — header is self-reported | Low |
| Experiment-7 | X.509 cert CN (any CA-issued cert) | No — TLS-verified | Medium |
| Experiment-8 | X.509 cert CN (OU=sy only) | No — TLS-verified + profile enforced | High |

Each experiment improves the quality of the identity input to the same PDP. The PDP's
XACML evaluation logic and the ConsumerAuth grant table do not change — only the
trustworthiness of the subject attribute fed into the XACML request improves.

---

## 7. Cross-Transport Policy Lifecycle

The table below shows the effect of a single ConsumerAuth grant/revoke action across all
active transports in experiment-8.

| Action | Policy-Sync behavior | AMQP (topic-auth-xacml) | Kafka/SSE (kafka-authz) | REST/mTLS (pki-rest-authz) |
|---|---|---|---|---|
| Add grant | Next sync uploads new PolicySet | Permit on next broker op | Permit on next SSE connect | Permit on next request |
| Revoke grant | Next sync uploads PolicySet without grant | Deny on next broker op | "revoked" event sent; stream torn down within 100 msgs | Deny on next request |
| Restore grant | Next sync uploads PolicySet with grant | Permit restored | Permit on next reconnect | Permit restored |
| Sync lag | Up to SYNC_INTERVAL | ← all three transports share this lag | ← | ← |

**Key observation**: The grant/revoke operation is performed once in ConsumerAuth. The
propagation to all three (or four) transports is automatic and mediated entirely by
policy-sync and AuthzForce. No per-transport configuration changes are required.

---

## 8. MQTT as a Fourth Transport (Architectural Extension)

MQTT is not demonstrated in experiment-8, but the architecture is explicitly designed to
accommodate it. The addition would require:

1. **An MQTT broker** (e.g., Eclipse Mosquitto or EMQX) with a plugin/hook interface for
   external authorization calls.

2. **`mqtt-authz` service** — a PEP analogous to `topic-auth-xacml`. On MQTT CONNECT or
   SUBSCRIBE, the broker calls `mqtt-authz`, which extracts the client ID (consumer identity)
   and topic (service definition), then calls AuthzForce at
   `POST /domains/arrowhead-exp8/pdp/evaluate` with:
   ```json
   {
     "subject": {"id": "<MQTT client ID>"},
     "resource": {"id": "<MQTT topic>"},
     "action": {"id": "subscribe"}
   }
   ```

3. **No changes to ConsumerAuth, policy-sync, or AuthzForce.** The existing XACML PolicySet
   already covers any `(consumer, service)` pair. If a grant exists for
   `(sensor-node, telemetry)`, MQTT subscription to the `arrowhead.telemetry` topic by
   a client with ID `sensor-node` would automatically receive Permit.

4. **Identity caveat**: MQTT client IDs are self-reported (same limitation as the Kafka URL
   parameter in experiment-8). Strong identity would require MQTT over TLS with client
   certificates — the same solution applied in the REST path.

This extensibility is the central added value of the PEP/PDP architecture: each new
transport protocol adds one PEP service; the authorization policy remains unchanged.

---

## 9. Extensions Beyond AH5.2 Docs

| Extension | Description | Rationale |
|---|---|---|
| XACML/AuthzForce PDP | Authorization via XACML PolicySet instead of JWT bearer tokens | Transport-agnostic; single policy covers AMQP, Kafka, REST, and future MQTT |
| PEP-per-transport pattern | Separate PEP service for each communication model | Decouples transport semantics from authorization logic |
| policy-sync | Translates ConsumerAuth grants to XACML; uploads to PDP | Single translation layer; grants in ConsumerAuth propagate to all transports |
| Per-100-message re-authorization | kafka-authz re-queries AuthzForce every 100 Kafka messages | Revocation takes effect mid-stream without closing the SSE connection at every message |
| SSE-based Kafka gateway | `kafka-authz` exposes SSE over HTTP instead of native Kafka API | Browser-observable stream; allows dashboard monitoring without a Kafka client |
| AH5.2 profile-ca | profile-ca with lo→on→de→sy enforcement | Stronger identity input for the PDP; OU=sy cert tied to lifecycle position |
| Profile enforcement at PEP | `pki-rest-authz` rejects non-OU=sy certs before the PDP call | Defense in depth: transport-level gate + authorization policy gate |
| policy-sync runtime reconfiguration | `POST /config` changes SYNC_INTERVAL without restart | Interactive demonstration of the sync-delay caveat across all transports |
| AuthzForce domain isolation | Each experiment uses a separate AUTHZFORCE_DOMAIN | Prevents cross-experiment policy pollution in shared AuthzForce instance |

---

## 10. Compliance Gap Summary

### Resolved in Experiment 8

| Gap | Description |
|---|---|
| G-PKI-1 | Flat CA without profile hierarchy → replaced by profile-ca with lo/on/de/sy |
| G-PKI-2 | No profile enforcement at PEP → pki-rest-authz enforces OU=sy at TLS layer |
| G-PKI-3 | No sequential lifecycle enforcement → profile-ca rejects out-of-order requests |

### Partially Resolved (inherited from experiment-7)

| Gap | Description | Remaining |
|---|---|---|
| G4 | Core system mTLS | Optional TLS_PORT; plain HTTP retained for Docker healthchecks |
| G2 | AH5.2 hierarchical CN naming | Only works when cloudName+operatorName provided at issuance |

### Intentional Deviations

| Gap | Description | Rationale |
|---|---|---|
| G3 | XACML/AuthzForce instead of AH5.2 JWT authorization model | Transport-neutral PDP enables uniform enforcement across AMQP, Kafka, REST |
| G5 | In-memory revocation state (resets on CA restart) | Ephemeral by design for experiments; production deployment would use persistent storage |
| G9 | No Master/Org CA hierarchy; only Local Cloud CA | Scope decision; Master/Org CA hierarchy is out of scope for single-cloud experiments |

---

## 11. Added Value Summary

### 11.1 Transport-Agnostic Authorization (primary contribution)

The use of a separate PDP (AuthzForce/XACML) as the single authorization arbiter for all
communication models is the primary architectural contribution of this experiment series:

- **One grant covers all transports.** A ConsumerAuth entry for `(consumer, service)` is
  enforced by AMQP, Kafka/SSE, and REST PEPs equally, after one SYNC_INTERVAL. No
  per-transport policy management is needed.

- **One revocation covers all transports.** Removing a grant from ConsumerAuth propagates
  to all active PEPs within SYNC_INTERVAL, regardless of how many transports are in use.

- **The pattern scales to new protocols.** MQTT, CoAP, WebSocket, or any other protocol
  with a hook/plugin interface can add a PEP that calls the same AuthzForce domain. The
  authorization policy does not change.

- **Identity quality is separable from authorization logic.** The XACML evaluation is
  identical regardless of whether the consumer identity came from an HTTP header
  (experiment-6), a flat CA cert (experiment-7), or a profile-CA system cert (experiment-8).
  Improving identity quality does not require rewriting the PDP or the grant table.

### 11.2 AH5.2 Profile PKI (secondary contribution, experiment-8 specific)

- **profile-ca** implements the AH5.2 Local Cloud CA profile hierarchy, the first in this
  series to do so.
- **pki-rest-authz** adds profile enforcement as a gate before the PDP — only system-level
  identity (OU=sy) is admitted to the authorization decision.
- **pki-consumer** demonstrates the full `on → de → sy` lifecycle end-to-end in Docker.

### 11.3 Observability and Documentation

- The **dashboard** provides live visibility into all three transport paths simultaneously,
  making the cross-transport authorization story directly observable.
- The **PKI Added Value tab** provides an interactive lifecycle demo, identity-to-authorization
  trace, and extensions comparison table.
- This document and the Mermaid diagrams provide a structured compliance and design-decision
  record for evaluators and future experiments.
