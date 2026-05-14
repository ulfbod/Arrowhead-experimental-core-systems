# CertificateLifecycle — gRPC Interface Spec

The `CertificateLifecycle` service is the canonical interface between the
Arrowhead profile-ca (Certificate Authority) and any subscriber that needs
real-time notification of certificate lifecycle events.

It is the second gRPC interface in the ADAPT stack, following `authorize.proto`.

---

## Purpose

profile-ca issues certificates with Arrowhead profile tiers encoded in the
Subject OU (`lo`, `on`, `de`, `sy`). Any component that needs to know a
system's cert level — in particular, the PIP (Policy Information Point) —
subscribes to this stream instead of polling a REST endpoint.

The PIP subscribes on startup, receiving a snapshot of all current certificates
followed by a live stream of issuance, revocation, and expiry events. It
auto-populates its SubjectStore, making cert-level attributes available to PEPs
for XACML enrichment without manual operator intervention.

---

## Service definition

```protobuf
service CertificateLifecycle {
  rpc Subscribe(SubscribeRequest) returns (stream CertEvent);
}

message SubscribeRequest {
  bool include_snapshot = 1;
}

message CertEvent {
  string    cn        = 1;  // system name (CN)
  string    ou        = 2;  // cert level: lo | on | de | sy
  EventType type      = 3;  // ISSUED | REVOKED | EXPIRED | SNAPSHOT
  string    issued_at  = 4; // RFC3339
  string    expires_at = 5; // RFC3339
}
```

---

## Decision semantics

| EventType | Subscriber action                    |
|-----------|--------------------------------------|
| ISSUED    | `PIP.Register(cn, ou, valid=true)`   |
| SNAPSHOT  | `PIP.Register(cn, ou, valid=true)`   |
| REVOKED   | `PIP.Register(cn, ou, valid=false)`  |
| EXPIRED   | `PIP.Register(cn, ou, valid=false)`  |

---

## grpcurl examples

```bash
# List services (reflection must be enabled on profile-ca)
grpcurl -plaintext localhost:8089 list

# Subscribe with snapshot (first connect / reconnect)
grpcurl -plaintext \
  -d '{"include_snapshot": true}' \
  localhost:8089 arrowhead.ca.v1.CertificateLifecycle/Subscribe

# Subscribe live-only (subsequent connects with warm cache)
grpcurl -plaintext \
  -d '{"include_snapshot": false}' \
  localhost:8089 arrowhead.ca.v1.CertificateLifecycle/Subscribe
```

---

## Reconnect / failure behaviour

- If the stream is interrupted the subscriber SHOULD reconnect with
  `include_snapshot=true` to re-baseline.
- While disconnected the subscriber MUST NOT purge its SubjectStore.
  Stale-but-non-empty attributes are safer than denying all cert-level checks.
- Reconnect with exponential backoff (initial 1 s, max 30 s).

---

## Regenerating Go bindings

```bash
make gen   # requires Docker
```
