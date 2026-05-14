# Experiment 13 — PKI Identity Unification + CertificateLifecycle gRPC Interface

## Summary

Extends experiment-12 with three architectural changes:

1. **CertificateLifecycle gRPC interface** — profile-ca exposes a typed event stream
   (`certlifecycle.proto`) that PIP subscribes to. PIP is auto-populated on startup;
   no manual subject registration required.

2. **PKI identity on all data paths** — Kafka and RabbitMQ now require client
   certificates (`ssl.client.auth=required`, `rabbitmq_auth_mechanism_ssl`). The
   consumer's cert CN becomes the XACML `subject-id` on every path, replacing
   self-reported system names.

3. **Cert-level enrichment in XACML** — all three PEPs (kafka-authz,
   topic-auth-xacml, pki-rest-authz) query PIP before calling AuthzForce and inject
   `urn:arrowhead:attribute:cert-level` and `urn:arrowhead:attribute:cert-valid` as
   XACML subject attributes. Policies can now condition on cert level.

## Design decisions

| Decision | Choice | Rationale |
|---|---|---|
| D1 | No PEP-side caching of PIP responses | Revocation propagates instantly; caching would break the end-to-end guarantee |
| D2 | cert-valid as XACML attribute, not a PEP pre-gate | PDP is the single decision point; audit log captures why a request was denied |
| D3 | Hard mTLS at broker level | No fallback path — uncertified clients are rejected at the TLS handshake |

## New interfaces

### `core-evol/proto/certlifecycle/certlifecycle.proto`

Canonical event stream interface between profile-ca (server) and PIP (subscriber).

```
grpcurl -plaintext localhost:8589 list
grpcurl -plaintext -d '{"include_snapshot":true}' \
  localhost:8589 arrowhead.ca.v1.CertificateLifecycle/Subscribe
```

### `core-evol/proto/authorize/authorize.proto` (unchanged from exp-12)

```
grpcurl -plaintext localhost:9650 list
```

## New local service copies

| Service | Path | Change |
|---|---|---|
| profile-ca | `experiments/experiment-13/services/profile-ca/` | + cert registry, gRPC server, `DELETE /ca/certificates/{cn}` |
| pip | `experiments/experiment-13/services/pip/` | + gRPC subscriber to profile-ca |
| kafka-authz | `experiments/experiment-13/services/kafka-authz/` | + PIP query, cert-level XACML enrichment |
| topic-auth-xacml | `experiments/experiment-13/services/topic-auth-xacml/` | + PIP query, cert-level XACML enrichment |
| pki-rest-authz | `experiments/experiment-13/services/pki-rest-authz/` | + PIP query, cert-level XACML enrichment |

## Ports (experiment-12 + 100)

| Service | Host port |
|---|---|
| profile-ca HTTP | 8587 |
| profile-ca mTLS | 8588 |
| profile-ca gRPC | **8589** (new) |
| AuthzForce | 8696 |
| ServiceRegistry TLS | 8990 |
| DynamicOrch-XACML | 8993 |
| authz-pdp gRPC | 9650 |
| PAP | 9605 |
| PIP | 9606 |
| kafka-authz | 9601 |
| pki-rest-authz mTLS | 9608 |
| pki-rest-authz HTTP | 9609 |
| Dashboard | 3013 |

## Running

```bash
cd experiments/experiment-13
docker compose up --build
```

System tests:

```bash
bash test-system.sh
```

## What changes from experiment-12

The `setup` service no longer seeds PIP subjects — they arrive automatically via
the CertificateLifecycle gRPC stream when profile-ca issues certs. Revocation via
`DELETE /ca/certificates/{cn}` propagates to PIP within one stream event.

RabbitMQ `rabbitmq.conf` now has `ssl_options.verify=verify_peer` and
`fail_if_no_peer_cert=true`. Kafka has `KAFKA_SSL_CLIENT_AUTH=required`. Both
changes mean consumers must present a cert signed by the Arrowhead CA.
