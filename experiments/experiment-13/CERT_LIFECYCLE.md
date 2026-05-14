# CERT_LIFECYCLE — CertificateLifecycle gRPC Interface

## Overview

`certlifecycle.proto` is the canonical interface between the Arrowhead profile-ca
(Certificate Authority) and any subscriber that needs real-time notification of
certificate lifecycle events.

It is the second gRPC interface in the ADAPT stack, following `authorize.proto`.
Both follow the same conventions: inline XACML-aligned comments, gRPC server
reflection, and a Makefile for code regeneration.

## Event model

```
profile-ca.IssueSystemCert("sp1", OU=sy)
    ↓ emits CertEvent{cn=sp1, ou=sy, type=ISSUED}
PIP (subscriber)
    ↓ store.Register("sp1", "sy", valid=true)
PIP HTTP  GET /attributes/sp1
    → {certLevel:"sy", valid:true}
PEP (kafka-authz / topic-auth-xacml / pki-rest-authz)
    ↓ adds cert-level attrs to XACML request
AuthzForce → evaluates policy including cert-level condition
```

## Revocation propagation

```
profile-ca  DELETE /ca/certificates/sp1
    ↓ marks revoked, emits CertEvent{type=REVOKED}
PIP  store.Register("sp1", "sy", valid=false)
    ↓ next PEP query
PEP  GET /pip/attributes/sp1 → {certLevel:"sy", valid:false}
    ↓ XACML request includes cert-valid=false
AuthzForce → DENY (policy requires cert-valid=true)
```

Propagation latency: one gRPC stream event + one PIP HTTP round-trip.
No polling, no TTL expiry, no eventual-consistency lag.

## Subscriber reconnect behaviour

PIP reconnects with exponential backoff (1 s → 30 s max) if the stream is interrupted.
While disconnected, PIP retains its last known state. It does NOT purge the store.
On reconnect it calls `Subscribe({include_snapshot: true})` to re-baseline.

## grpcurl inspection

```bash
# profile-ca gRPC reflection
grpcurl -plaintext localhost:8589 list
grpcurl -plaintext localhost:8589 describe arrowhead.ca.v1.CertificateLifecycle

# Subscribe and watch events (CTRL-C to stop)
grpcurl -plaintext \
  -d '{"include_snapshot": true}' \
  localhost:8589 arrowhead.ca.v1.CertificateLifecycle/Subscribe

# Revoke a cert and observe the REVOKED event on a connected subscriber
curl -X DELETE http://localhost:8587/ca/certificates/service-partner-1
```

## XACML attributes added by PEPs

```xml
<!-- Added to subject category by all three PEPs (exp-13) -->
<Attribute AttributeId="urn:arrowhead:attribute:cert-level">
  <AttributeValue>sy</AttributeValue>    <!-- from PIP -->
</Attribute>
<Attribute AttributeId="urn:arrowhead:attribute:cert-valid">
  <AttributeValue>true</AttributeValue>  <!-- from PIP; false after revocation -->
</Attribute>
```

## Fail-closed behaviour

| Condition | PEP behaviour |
|---|---|
| PIP unreachable | certLevel="", certValid=false → AuthzForce likely DENY |
| PIP returns 404 | certLevel="", certValid=false → AuthzForce likely DENY |
| profile-ca gRPC stream down | PIP keeps last known state; PEP decisions use stale-but-non-empty values |
| cert CN not in PIP (stream lag) | certValid=false → DENY until event arrives |

## Proto regeneration

```bash
make -C core-evol/proto/certlifecycle gen
```
