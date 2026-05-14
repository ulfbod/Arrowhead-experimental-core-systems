# AUTH_ALTERNATIVES — Cert-Level Enrichment and PKI Identity Approaches

## Context

Experiment-12 establishes XACML-based enforcement for all three data paths (Kafka, AMQP,
REST). PEPs call AuthzForce with `subject-id` and `resource-id`. The identity on Kafka
and AMQP is self-reported — the system name comes from the message payload or AMQP
username, neither of which is verified by the broker.

Two gaps remain after experiment-12:

| Gap | Description |
|---|---|
| **Identity spoofing** | On Kafka and AMQP paths, any client can claim any system name |
| **Cert status unknown** | AuthzForce knows nothing about whether a cert is current or revoked |

Experiment-13 addresses both gaps. The four alternative approaches below were considered
for each dimension.

---

## Dimension 1: Broker Identity Verification

### Approach Comparison

| # | Name | Description | Pros | Cons |
|---|---|---|---|---|
| **A** | Application-layer name claim | System name is passed in message header or AMQP username; no broker verification | — Zero broker config change | — Any client can claim any identity; XACML subject-id is unverified |
| **B** | Mutual TLS at broker; CN extracted by PEP | Broker enforces `ssl.client.auth=required` (Kafka) and `verify_peer` (RabbitMQ); PEP reads CN from broker-reported principal | — Identity is cryptographically bound to CA cert<br>— CN extraction is automatic (no app-layer change) | — Broker config changes required; clients must hold valid certs |
| **C** | mTLS + token forwarding | Client cert verified at TLS; additional JWT/token forwarded in message header for dual verification | — Defence-in-depth: two credentials | — Complexity: two auth paths to maintain; JWT issuance requires additional infrastructure |
| **D** | Per-topic ACLs without identity mapping | Kafka ACLs or RabbitMQ permissions control access by cert principal; no XACML involved | — Simple to operate | — Decouples enforcement from PAP/AuthzForce; loses unified policy store and audit log |

### Chosen Approach: B

**Rationale**: Approach B closes the identity spoofing gap at the TLS layer without
changing the PEP→AuthzForce protocol. The broker verifies the cert before any message
reaches the PEP; the PEP receives a CN it can trust. Approach C adds complexity without
architectural benefit given that mTLS already covers authentication. Approach D abandons
the unified AuthzForce enforcement model.

---

## Dimension 2: Cert Status in Authorization Decisions

### Approach Comparison

| # | Name | Description | Pros | Cons |
|---|---|---|---|---|
| **A** | PEP pre-gate (cert-valid check before AuthzForce) | PEP calls PIP; if cert-valid=false, reject immediately without calling AuthzForce | — Short-circuits AuthzForce call on revoked certs | — PEP is now a decision point, not just a translator. Audit log does not capture why the request was denied. |
| **B** | cert-valid as XACML subject attribute (chosen) | PEP enriches XACML request with cert-level and cert-valid; AuthzForce evaluates policy that requires cert-valid=true | — Single decision point (AuthzForce); full XACML audit trail<br>— cert-level can be used in conditions beyond revocation<br>— Policy expresses the requirement, not the PEP | — One extra PIP HTTP call per authorization request |
| **C** | OCSP/CRL in AuthzForce PIPs | AuthzForce fetches cert status via OCSP or CRL at decision time | — Standard PKI revocation mechanism | — Requires AuthzForce OCSP/CRL PIP configuration; adds external dependency; OCSP/CRL may not be available in closed IoT environments |
| **D** | No cert status in enforcement | Accept that revocation is reflected only through PAP policy deletion, not cert status | — Zero implementation cost | — Revoked cert holders continue to be authorized until PAP policy is also deleted; two-step revocation is an operational burden |

### Chosen Approach: B (decision D2)

**Rationale**: Approach B keeps AuthzForce as the single decision point. The audit log
captures not just PERMIT/DENY but also *why* — the cert-valid attribute value is part of
the logged XACML request. Approach A would split the decision between PEP and PDP,
complicating audit. Approach C requires infrastructure (OCSP responder) not available in
the experiment environment. Approach D leaves a dangerous revocation gap.

---

## Dimension 3: PEP-Side Caching of PIP Responses

### Approach Comparison

| # | Name | Description | Pros | Cons |
|---|---|---|---|---|
| **A** | No caching (chosen) | PEP calls PIP on every authorization request | — Revocation takes effect on the very next request<br>— Simple; no cache invalidation logic | — One PIP HTTP round-trip per authorization request |
| **B** | Short TTL cache (e.g., 5 s) | PEP caches PIP response for 5 seconds; re-fetches on expiry | — Reduces PIP load under high request rates | — Revocation may take up to TTL seconds to propagate to PEP decisions; breaks the end-to-end revocation guarantee |
| **C** | Push invalidation | PIP pushes invalidation events to PEPs when cert status changes | — Instant invalidation with local cache | — Requires PIP→PEP push infrastructure; PEP must maintain open connection to PIP; complex failure modes |
| **D** | Pre-shared cert registry at startup | PEP loads full cert registry from PIP on startup; no per-request calls | — Zero per-request latency after startup | — Revocation events never reach PEP after startup; essentially D in dimension 2 with extra steps |

### Chosen Approach: A (decision D1)

**Rationale**: The end-to-end revocation guarantee (`DELETE cert → AuthzForce DENY` within
one stream event + one HTTP round-trip) is a stated design goal of experiment-13. Any form
of caching at the PEP breaks this guarantee. The PIP HTTP call is a local docker-internal
call in the experiment stack; latency is negligible. Approach B introduces a hidden
revocation window. Approach C adds significant complexity for a marginal latency benefit.

---

## Dimension 4: CA → PIP Population Mechanism

### Approach Comparison

| # | Name | Description | Pros | Cons |
|---|---|---|---|---|
| **A** | Manual registration by setup service | Setup service calls `POST /pip/subjects` for each system cert after issuance | — Simple; no new interfaces | — Must be manually maintained; out of sync if cert issued outside setup script; setup failures leave PIP empty |
| **B** | Polling / reconciliation loop | PIP periodically queries profile-ca cert registry and reconciles its local store | — No gRPC interface required; works with any CA that exposes a cert list API | — Polling interval = minimum revocation propagation delay; eventual consistency |
| **C** | gRPC event stream (chosen) | Profile-ca exposes `CertificateLifecycle.Subscribe`; PIP subscribes and receives typed events (ISSUED, REVOKED, EXPIRED) | — Push-based: propagation delay = one gRPC stream event<br>— Snapshot on reconnect: PIP can re-baseline if stream is interrupted<br>— Typed interface: ou field carries cert profile (sy/on/de/lo) | — New gRPC interface to maintain; adds profile-ca as a direct dependency of PIP |
| **D** | Shared database | Profile-ca and PIP share a cert registry via a common DB | — No inter-service call on read path | — Tight operational coupling; shared DB is an antipattern for independent services |

### Chosen Approach: C (new interface CertificateLifecycle.proto)

**Rationale**: Approach A (manual registration) was the experiment-12 baseline; its main
failure mode is divergence between the setup script and the actual cert state. Approach C
eliminates this by making PIP a passive subscriber to CA events — it is *always* consistent
with profile-ca's state, even if a cert was issued outside the normal setup path. Approach B
(polling) still has a revocation window equal to the polling interval. Approach D introduces
shared-state coupling that is inappropriate for independent services.

The gRPC interface is defined in `core-evol/proto/certlifecycle/certlifecycle.proto` as a
canonical contract, following the same conventions as `authorize.proto`.

---

## Summary of chosen approaches

| Decision | Choice | Key reason |
|---|---|---|
| D1: No PEP-side caching | No cache | Revocation propagates instantly; caching would break the guarantee |
| D2: cert-valid as XACML attribute | AuthzForce decides | Single decision point; audit log captures the cert-valid value |
| D3: Hard mTLS at broker | Approach B (mTLS + CN extraction) | Cryptographically binds broker identity to CA cert |
| D4: CA → PIP channel | Approach C (gRPC stream) | Push-based, typed, snapshot-on-reconnect; eliminates manual sync |

---

## Chosen approach: combined architecture

```
profile-ca ──gRPC stream──▶ PIP
                               │
                        (cert-level, cert-valid)
                               │
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
        kafka-authz    topic-auth-xacml   pki-rest-authz
              │                │                │
              └────────────────┴────────────────┘
                               │
                          AuthzForce
                     (single decision point)
```

PEPs are thin translators: they verify broker identity (Kafka principal / AMQP CN / TLS CN),
enrich the XACML request with cert attributes from PIP, and delegate the decision entirely
to AuthzForce. The PEP never makes a binary allow/deny — it only translates.

### Key interface

```protobuf
// core-evol/proto/certlifecycle/certlifecycle.proto
service CertificateLifecycle {
  rpc Subscribe(SubscribeRequest) returns (stream CertEvent);
}
message SubscribeRequest {
  bool include_snapshot = 1;
}
message CertEvent {
  string cn = 1;        // system name
  string ou = 2;        // cert profile (sy/on/de/lo)
  EventType type = 3;   // ISSUED | REVOKED | EXPIRED | SNAPSHOT
  string issued_at = 4;
  string expires_at = 5;
}
```

### XACML enrichment added by PEPs (exp-13)

```xml
<!-- All three PEPs add these two attributes before calling AuthzForce -->
<Attribute AttributeId="urn:arrowhead:attribute:cert-level">
  <AttributeValue>sy</AttributeValue>    <!-- cert OU from PIP -->
</Attribute>
<Attribute AttributeId="urn:arrowhead:attribute:cert-valid">
  <AttributeValue>true</AttributeValue>  <!-- false after revocation -->
</Attribute>
```

The `subject-id` attribute is now cryptographically verified in all three paths.
`cert-level` and `cert-valid` allow policies to express conditions such as
"only system-level certs may consume high-sensitivity telemetry" or
"deny any request with cert-valid=false" — independent of the PAP policy for that subject.
