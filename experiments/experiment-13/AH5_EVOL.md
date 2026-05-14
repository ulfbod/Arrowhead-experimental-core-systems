# AH5_EVOL — PKI Identity Unification and CertificateLifecycle Extension

## What AH5 specifies

The AH5 specification defines a two-step PKI model:

1. **Onboarding** — a new system presents its public key to the Onboarding Controller,
   which issues a system certificate signed by the Local Cloud CA. The OU field encodes
   the certificate profile: `sy` for system, `on` for onboarding, `de` for device, `lo` for
   local cloud CA.

2. **mTLS everywhere** — AH5 mandates that all communications within a local cloud use
   mutual TLS. The system certificate issued in step 1 is the client credential.

The AH5 spec is silent on what happens *after* certificate issuance:

- There is no specified mechanism for a subscriber to learn which certificates are currently
  valid, which have been revoked, or what profile level a given certificate carries.
- The ConsumerAuthorization stores boolean `(consumer, provider, service)` grants; it does not
  store cert metadata.
- There is no standard event interface between the CA and downstream consumers of cert status.

## What Experiment-13 changes

Experiment-13 introduces two extensions on top of experiment-12:

### Extension 1: CertificateLifecycle gRPC interface

Profile-ca exposes a server-side streaming gRPC interface (`certlifecycle.proto`) that emits
lifecycle events for every certificate it manages:

```protobuf
service CertificateLifecycle {
  rpc Subscribe(SubscribeRequest) returns (stream CertEvent);
}
```

| Field | Meaning |
|---|---|
| `cn` | System name (cert Common Name) |
| `ou` | Certificate profile (`sy`, `on`, `de`, `lo`) |
| `type` | `ISSUED`, `REVOKED`, `EXPIRED`, `SNAPSHOT` |
| `issued_at` | RFC3339 timestamp |
| `expires_at` | RFC3339 timestamp |

PIP subscribes to this stream on startup, requests a snapshot (`include_snapshot=true`),
and stays connected. Every certificate in profile-ca's registry is reflected in PIP without
any manual registration.

This is architecturally novel — AH5 does not define a CA event interface. The
`certlifecycle.proto` contract is stored in `core-evol/proto/certlifecycle/` alongside
`authorize.proto` as a canonical interface registry.

### Extension 2: PKI identity on all data paths

AH5 mandates mTLS but does not specify what brokers must do with the client certificate
after the TLS handshake. In experiment-12:

- Kafka: `ssl.client.auth=none` — client certs accepted but not required; system name
  was self-reported in the Kafka message payload (unverified).
- RabbitMQ: `verify=verify_none` — no client cert required; AMQP username was
  self-reported (unverified).

Experiment-13 closes this gap:

| Path | Exp-12 identity source | Exp-13 identity source |
|---|---|---|
| Kafka | Self-reported name in payload (unverified) | Cert CN from TLS handshake (ssl.client.auth=required) |
| AMQP | Self-reported AMQP username (unverified) | Cert CN via rabbitmq_auth_mechanism_ssl (verified at TLS) |
| REST | Cert CN from mTLS (verified) | Cert CN from mTLS (unchanged) |

After this change all three PEPs extract the XACML `subject-id` from the same source:
the PKI certificate verified at the transport layer.

### Differences from the AH5 spec

| Dimension | AH5 | Experiment-13 |
|---|---|---|
| CA event model | Not specified | CertificateLifecycle gRPC (certlifecycle.proto) |
| PIP population | Not specified (no PIP concept in spec) | Auto-populated via gRPC stream; no manual registration |
| Broker client auth (Kafka) | mTLS required but use of CN not specified | ssl.client.auth=required; CN is XACML subject-id |
| Broker client auth (AMQP) | mTLS required but use of CN not specified | verify_peer + rabbitmq_auth_mechanism_ssl; CN is XACML subject-id |
| XACML subject attributes | Not specified | cert-level (sy/on/de/lo) and cert-valid (true/false) from PIP |
| Revocation propagation | Revocation endpoint on CA (not standardised) | DELETE /ca/certificates/{cn} → gRPC stream event → PIP update |
| Authorization of revoked certs | Not specified | cert-valid=false → AuthzForce DENY (all three paths) |

### Why it moves beyond the spec

1. **No CA event standard exists in AH5.** The CertificateLifecycle gRPC interface is an
   architectural addition. Any AH5-compliant implementation that does not expose this
   interface is incompatible with the PIP auto-population mechanism.

2. **XACML attributes are not AH5-defined.** The attributes
   `urn:arrowhead:attribute:cert-level` and `urn:arrowhead:attribute:cert-valid` are
   experiment-defined extensions. An AH5-compliant PAP/CA does not know about them.

3. **Broker configuration beyond the spec.** Requiring `ssl.client.auth=required` on
   Kafka and `rabbitmq_auth_mechanism_ssl` on RabbitMQ are deployment choices not mandated
   by the AH5 spec. They strengthen the security model but constrain broker operability.

## Identity unification outcome

After experiment-13, the identity chain for all three data paths is:

```
Arrowhead CA (profile-ca)
  └─ issues system cert (CN=sp1, OU=sy)
       └─ client presents cert at TLS handshake
            └─ broker verifies against CA trust store
                 └─ broker extracts CN → PEP receives verified identity
                      └─ PEP: XACML subject-id = CN (cryptographically bound)
```

There is no longer a gap between the PKI identity (cert CN) and the authorization
identity (XACML subject-id). Spoofing a system name requires compromising the CA.

## CertificateLifecycle event flow (new in exp-13)

```
profile-ca.IssueSystemCert("sp1", OU=sy)
    ↓ persists to cert registry
    ↓ emits CertEvent{cn=sp1, ou=sy, type=ISSUED}
PIP subscriber (persistent gRPC stream)
    ↓ store.Register("sp1", "sy", valid=true)
PEP query: GET /pip/attributes/sp1
    → {certLevel:"sy", certValid:true}
PEP adds to XACML: cert-level=sy, cert-valid=true
AuthzForce evaluates policy including cert-level condition
    → PERMIT or DENY depending on policy

--- On revocation ---

DELETE /ca/certificates/sp1
    ↓ emits CertEvent{cn=sp1, type=REVOKED}
PIP: store.Register("sp1", "sy", valid=false)
Next PEP request for sp1:
    GET /pip/attributes/sp1 → {certValid:false}
    XACML: cert-valid=false → AuthzForce DENY
```

## Proto registry location

```
core-evol/proto/
├── authorize/
│   ├── authorize.proto         — PEP↔PDP gRPC (exp-12+)
│   └── ...
└── certlifecycle/
    ├── certlifecycle.proto     — CA→PIP event stream (exp-13+)
    └── ...
```

Both interfaces follow the same conventions: inline XACML-aligned comments,
gRPC server reflection enabled, Makefile for Docker-based regeneration.

## Pros of this evolution

- **Cryptographic identity on all paths**: self-reported names are no longer accepted
  on any data path. Identity is tied to a CA-signed certificate.
- **Instant revocation propagation**: DELETE /ca/certificates/{cn} → gRPC event → PIP
  update → next PEP request uses cert-valid=false. No polling, no TTL lag.
- **Zero-configuration PIP**: new system certs appear in PIP automatically. No setup
  service registration step required.
- **Unified cert-level conditioning**: policies can express `certLevel=sy` requirements
  without changes to the AuthzForce engine.
- **Fail-closed on PIP failures**: unreachable PIP or missing CN → certValid=false → DENY.

## Cons of this evolution

- **New CA dependency**: all three PEPs and PIP now depend on profile-ca being reachable.
  A CA outage does not affect existing authorized connections (PIP keeps last known state)
  but new certificate issuances will not propagate until the stream reconnects.
- **No PEP-side caching**: every PEP authorization requires one HTTP round-trip to PIP.
  Under high request rates this adds latency proportional to PIP response time.
- **Non-standard CA event interface**: certlifecycle.proto is not an AH5-defined interface.
  An AH5-compliant CA that does not expose it cannot feed PIP.
- **Broker hardening is deployment-specific**: ssl.client.auth=required and
  rabbitmq_auth_mechanism_ssl require specific broker configurations that must be
  managed as infrastructure.
