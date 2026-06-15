# gRPC Interfaces vs Canonical PDP–PEP — Analysis

## Canonical PDP–PEP reference

The XACML reference architecture defines a four-component model:

```
PEP → PDP   "Should subject S access resource R with action A?"
PDP → PIP   (PDP synchronously fetches missing subject attributes)
PDP → PAP   (PDP reads applicable policies)
PDP → PEP   "PERMIT / DENY / INDETERMINATE"
PEP         enforces the decision at the moment of access
```

Key properties: unary request-response, stateless PEP, attribute fetching is the
PDP's responsibility (pull, per request), enforcement happens at the data plane.

---

## Interface 1 — AuthorizationPDP.Decide

This is the recognisably canonical interface. DynamicOrchestration is the PEP,
authz-pdp is the PDP, the interaction is a unary RPC per provider candidate, and
the Decision enum maps directly to XACML. Fail-closed semantics match the spec.
Three deviations are worth noting.

### Deviation 1 — Control-plane PEP, not data-plane

Standard PEPs sit at enforcement points: API gateways, message brokers, proxies.
They act at the moment a connection or request arrives. The DynamicOrchestration
PEP acts *before* any connection exists — it decides which provider endpoints to
*disclose* to a consumer during service discovery. The enforcement consequence is
"this consumer never learns the provider's address," not "this connection is
rejected." This is closer to capability-based security than reference monitor
style, and it provides a first authorization gate without any network traffic
reaching the provider at all.

### Deviation 2 — `provider` as a first-class XACML attribute

Standard XACML resource-id identifies a service type. Adding `provider` as a
separate attribute (`urn:arrowhead:attribute:provider-id`) allows policies of the
form: *"consumer X may access service telemetry, but only from
robot-fleet-site-1, not from robot-fleet-site-2."* In a distributed IoT network,
*which node* answers a request is a policy concern, not just which service type.
This is specific to the Arrowhead multi-provider topology and has no direct
equivalent in web-service XACML deployments.

### Deviation 3 — `action` as a namespace separator, not a verb

XACML `action-id` conventionally carries an HTTP method or operation (GET,
write, delete). Here `"orchestrate"` and `"consume"` are namespace tags that
direct each request to its applicable policy set without the orchestration and
enforcement policies colliding in the same PDP. A single authz-pdp instance
serves two semantically different PEP roles using the action field as a
discriminator. This is pragmatic but non-standard: the action is not describing
what the subject is doing to the resource, it is describing which authorization
layer is asking.

---

## Interface 2 — CertificateLifecycle.Subscribe

This interface is **not a PDP–PEP interaction**. No authorization decision is
made on this channel. profile-ca is a certificate authority, not a PDP. PIP is
not a PEP. The interface is a CA-to-PIP attribute provisioning channel: it
synchronises authentication state (who holds a valid certificate and at what
tier) into the attribute store that feeds downstream XACML decisions.

### Push-based attribute provisioning — an inversion of classic XACML

In standard XACML the PDP pulls attributes from the PIP synchronously at
decision time, per request. Here that is inverted:

| Dimension | Classic XACML | Experiment 13 |
|-----------|---------------|---------------|
| Attribute fetch model | PDP pulls from PIP at decision time | PIP subscribes to CA, pre-populates store |
| Timing | Synchronous, per-request | Asynchronous, event-driven |
| Attribute freshness | As fresh as the PDP's last query | As fresh as the last stream event |
| Who fetches attributes | PDP | PEP (queries PIP before calling PDP) |
| Revocation propagation | On next PDP query for that subject | CA pushes REVOKED event immediately |

The push model means revocation propagates faster — a DELETE on the CA triggers
a stream event that invalidates the SubjectStore entry before any subsequent
authorization request for that subject. The cost is a new class of failure mode:
stream disconnect, snapshot staleness, and the deliberate choice to retain last
known state on connectivity loss rather than purging the store (fail-safe-state,
not fail-closed, for the attribute channel).

### Authentication and authorization are causally coupled

In most architectures, authentication (is this identity valid?) and authorization
(is this action permitted?) are separate systems that share only an identity
token. Here they are causally wired:

```
Certificate issued  →  CertEvent{ISSUED}   →  cert-valid=true  →  XACML PERMIT possible
Certificate revoked →  CertEvent{REVOKED}  →  cert-valid=false →  XACML DENY certain
```

A revoked certificate does not merely fail the TLS handshake — it also causes
DENY at the policy layer because `cert-valid=false` makes the applicable XACML
rule evaluate to Deny. Authentication state and authorization state are unified
through the attribute pipeline. Neither can be bypassed independently: an
attacker who presents a revoked certificate is rejected by mTLS *and* by the
policy engine even if the TLS check somehow passed.

The `ou` field (cert level: `lo`, `on`, `de`, `sy`) adds a second authentication
attribute that authorization policy can condition on. A System-tier cert
(`ou=sy`) is required for data-plane access; an Onboarding-tier cert (`ou=on`)
grants only registration-scoped access regardless of what policies exist for the
service. Authentication tier becomes authorization input.

---

## The combined picture — a two-layer authorization stack

Standard PDP–PEP deployments have a single enforcement layer. Experiment 13 has
two, fed by a shared attribute pipeline:

```
Layer 1 — Control plane (service discovery)
  DynamicOrchestration (PEP) ──gRPC──▶ authz-pdp (PDP)
  action="orchestrate", provider=X
  Decision: which provider endpoints are disclosed to the consumer

Layer 2 — Data plane (connection enforcement)
  kafka-authz / pki-rest-authz (PEP) ──gRPC──▶ authz-pdp (PDP)
  action="consume", provider empty
  Decision: whether the data-plane connection is permitted

Shared attribute pipeline:
  profile-ca ──stream──▶ PIP SubjectStore
  cert-level and cert-valid injected as XACML subject attributes for both layers
```

Defence-in-depth follows: a consumer who bypasses the data-plane enforcement
still has no useful endpoint address because Layer 1 never disclosed it. A
consumer who somehow obtains an endpoint by other means is still blocked by
Layer 2. Both layers consume the same cert-level attributes from the same PIP,
so revocation propagates to both simultaneously.

The most non-standard aspect overall is that authentication (PKI) and
authorization (XACML) are not just co-located but causally coupled through the
CertificateLifecycle stream: a certificate lifecycle event *is* an authorization
attribute change, and the system treats it as such rather than keeping the two
worlds separate.

---

## Summary of deviations from canonical PDP–PEP

| Aspect | Canonical XACML | Experiment 13 |
|--------|----------------|---------------|
| PEP location | Data plane (enforcement at access time) | Layer 1: control plane (disclosure); Layer 2: data plane |
| Attribute fetch | PDP pulls from PIP synchronously | PIP subscribes to CA, pushes to store; PEP pre-fetches |
| Resource granularity | Service type | Service type + provider instance |
| Action semantics | Operation verb (GET, write) | Namespace tag (orchestrate / consume) |
| Revocation speed | Next PDP query | Immediate (stream event) |
| Auth–authz relationship | Separate systems, shared identity token | Causally coupled via attribute pipeline |
| Enforcement layers | Single | Two (disclosure + connection) |
| CertificateLifecycle role | No equivalent (PIP is internal to PDP) | Explicit CA-to-PIP gRPC stream; PIP is external |
