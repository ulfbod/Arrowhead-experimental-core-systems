# X.509 / mTLS Security Assessment

**Repository:** ArrowheadCore  
**Date:** 2026-05-08  
**Scope:** experiment-7 and core systems  
**Reference:** [Arrowhead Framework 5.2 documentation](https://aitia-iiot.github.io/ah5-docs-java-spring/home/release-versions/)

---

## 1. Implementation Conclusion

**Partially implemented**

---

## 2. Arrowhead 5.2 Conformity Conclusion

**Partially aligned**

---

## 3. Evidence

### 3.1 Certificate Authority — Active and Functional

`core/internal/ca/service/ca.go` generates a self-signed ECDSA P-256 root certificate at
startup (`NewCAService`) and issues leaf certificates on demand (`Issue`). Leaf certificates
carry both `ExtKeyUsageClientAuth` and `ExtKeyUsageServerAuth`, which is correct for mutual
TLS. Three CA endpoints are exposed over plain HTTP:

- `GET /ca/info` — returns CA certificate PEM
- `POST /ca/certificate/issue` — issues a signed leaf certificate
- `POST /ca/certificate/verify` — verifies a leaf certificate against the CA

The CA is not a stub. It signs real X.509 certificates with proper serial numbers, validity
windows, and key usage flags.

### 3.2 Dynamic Certificate Provisioning at Runtime

All experiment-7 Go services call the CA HTTP API at startup with retry loops (up to 10
attempts, 3 s delay):

1. `GET /ca/info` → parse PEM → build `*x509.CertPool`
2. `POST /ca/certificate/issue` → PEM cert+key → `tls.X509KeyPair`

Evidence: `cert-rest-authz/tlsconfig.go` (`fetchCACert`, `issueCert`),
`cert-consumer/main.go`, `data-provider-tls/main.go`, `robot-fleet-tls/main.go`,
`kafka-authz/main.go`.

The `cert-provisioner` init container performs the same flow for Kafka and RabbitMQ
infrastructure certs, writing PEM files to a shared Docker volume (`/certs`).

### 3.3 Mutual TLS — Implemented on One Path

The only path with full mutual TLS is **cert-consumer → cert-rest-authz**.

**Server side** (`cert-rest-authz/tlsconfig.go`, `buildServerTLSConfig`):

```go
&tls.Config{
    Certificates: []tls.Certificate{cert},
    ClientAuth:   tls.RequireAndVerifyClientCert,
    ClientCAs:    caPool,
    MinVersion:   tls.VersionTLS12,
}
```

`tls.RequireAndVerifyClientCert` means the TLS stack rejects the handshake entirely if no
valid client certificate is presented. The client CA pool is populated from the live CA.

**Client side** (`cert-consumer/main.go`):

```go
&tls.Config{
    Certificates: []tls.Certificate{ownCert},
    RootCAs:      caPool,
    MinVersion:   tls.VersionTLS12,
}
```

The consumer presents its issued certificate; the server certificate is verified against
the same CA pool.

**Consumer identity** (`cert-rest-authz/server.go`):

```go
consumer := r.TLS.PeerCertificates[0].Subject.CommonName
```

Identity is extracted after the completed TLS handshake — it is cryptographically bound.
No `X-Consumer-Name` header fallback exists. The handler returns 401 if `r.TLS == nil` or
if no peer certificate is present.

### 3.4 Server-Only TLS — Applied Broadly

Several paths use TLS for transport encryption without client authentication:

| Connection | Configuration |
|---|---|
| cert-rest-authz → data-provider-tls (upstream proxy) | `buildClientTLSConfig` — presents own cert, verifies server against CA pool |
| data-provider-tls HTTPS server | `tls.Listen` with own cert, `MinVersion: TLS12`, no `ClientAuth` |
| robot-fleet-tls → RabbitMQ | `amqp.DialTLS` with CA pool in `RootCAs` |
| robot-fleet-tls → Kafka | `kafka.Transport{TLS: &tls.Config{RootCAs: caPool}}` |
| data-provider-tls → Kafka | `kafka.Dialer{TLS: &tls.Config{RootCAs: caPool}}` |
| kafka-authz → Kafka | Same pattern, enabled by `CA_URL` env var |

`InsecureSkipVerify` does not appear in any production service code.

### 3.5 Infrastructure TLS — Kafka and RabbitMQ

**Kafka**: A custom Dockerfile (`kafka-tls.Dockerfile`) uses `kafka-tls-entrypoint.sh` to
convert PEM certificates to PKCS12 keystores via `openssl pkcs12 -export`, then exports
`KAFKA_SSL_KEYSTORE_*` and `KAFKA_SSL_TRUSTSTORE_*` environment variables before starting
the Confluent broker. `KAFKA_SSL_CLIENT_AUTH: none` — Kafka uses server-only TLS; clients
are not required to present certificates.

**RabbitMQ**: `rabbitmq/rabbitmq.conf` configures `listeners.ssl.default = 5671` with
`ssl_options.{cacertfile,certfile,keyfile}` pointing to provisioner-written volume paths.
`ssl_options.verify = verify_none` and `ssl_options.fail_if_no_peer_cert = false` — again,
server-only TLS.

### 3.6 Test Coverage for TLS Behaviour

| Test file | What is tested |
|---|---|
| `cert-rest-authz/server_test.go` | `TestMTLSProxy_noCert` → 401; `TestMTLSProxy_permit` / `_deny` inject `tls.ConnectionState{PeerCertificates: ...}` with specific CNs |
| `cert-rest-authz/tlsconfig_test.go` | `buildServerTLSConfig` and `buildClientTLSConfig` against live-generated test certs |
| `cert-consumer/poll_test.go` | Real `httptest.Server` started with TLS (self-signed cert with IP SAN `127.0.0.1`); confirms no `X-Consumer-Name` header is sent |
| `cert-provisioner/main_test.go` | HTTP-server-backed tests for `fetchCACert` and `issueCert` |

---

## 4. Gaps

### 4.1 mTLS Is Not Systemic — Core Systems Remain on Plain HTTP

The AH5.2 documentation states that every Local Cloud operates using mutual TLS
authentication across all inter-system communication. All four mandatory core systems
(ServiceRegistry :8080, Authentication :8081, ConsumerAuthorization :8082,
DynamicOrchestration :8083) still accept and produce plain HTTP. This is explicitly
acknowledged in `core/GAP_ANALYSIS.md G4`:

> "AH5 production deployments use certificate-based mutual authentication on all
> inter-system HTTP calls. All connections in this implementation are plain HTTP."

Service registration calls from experiment-7 services to the ServiceRegistry are
unauthenticated HTTP. The ServiceRegistry does not validate that the registering service
possesses a valid X.509 certificate, which AH5 requires as part of the onboarding flow.

### 4.2 Certificate Naming Does Not Follow AH5 Hierarchical Convention

AH5 specifies a hierarchical Common Name format:
`systemName.cloudName.operatorName.arrowhead.eu` (RFC 5280-compliant, tied to DNS naming).
The CA (`core/internal/ca/service/ca.go:99`) sets:

```go
Subject: pkix.Name{CommonName: req.SystemName}
```

Issued CNs are bare names such as `cert-consumer`, `kafka`, `rabbitmq`, `cert-rest-authz`.
These do not embed cloud identity or follow the AH5 X.509 profile naming hierarchy.

### 4.3 No Subject Alternative Names (SANs) in Issued Certificates

The leaf certificate template in `ca.go` contains no `DNSNames` or `IPAddresses` fields.
The AH5 X.509 profile (and RFC 5280 as enforced by Go's TLS stack since 1.15) requires SANs
for hostname verification. In practice this works within Docker (services use hostnames
matching the CN) but would fail strict hostname verification in environments where Go's TLS
stack checks SANs and the SAN list is absent.

The `cert-consumer` poll test works around this by using IP SANs in the test-only
certificate, underscoring that the production CA does not emit them.

### 4.4 No Certificate Revocation at the X.509 Level

The CA template sets `KeyUsage: x509.KeyUsageCRLSign` on the root certificate, but
`CAService` never generates CRLs. There is no CRL distribution point endpoint and no OCSP
support. A compromised certificate cannot be invalidated without restarting the entire stack.

The revocation test in `test-system.sh` tests *policy-level* revocation: removing a
ConsumerAuthorization grant propagates through `policy-sync` to AuthzForce, causing
`cert-rest-authz` to return 403. This is authorization revocation, not certificate
revocation. These are distinct mechanisms.

### 4.5 RabbitMQ and Kafka Use Server-Only TLS, Not mTLS

`ssl_options.fail_if_no_peer_cert = false` (RabbitMQ) and `KAFKA_SSL_CLIENT_AUTH: none`
(Kafka) mean that neither broker requires client certificates. Clients authenticate to
RabbitMQ via AMQP username/password credentials delegated to `topic-auth-xacml` via HTTP
auth backend, not via mutual TLS. This is a pragmatic design choice for these brokers, but
the "consistent mTLS" model does not extend to the messaging transport layer.

### 4.6 Authorization Model Diverges from AH5 JWT Token Model

AH5.2 specifies that the Authorization system issues JWT bearer tokens that consumers carry
to prove service access authorization, and providers verify these tokens using the
counterparty's public key. The implementation uses XACML/AuthzForce as the PDP with a
synchronizing `policy-sync` service pulling grants from ConsumerAuthorization. The core
`Authentication` system issues hex timestamp tokens (`core/GAP_ANALYSIS.md G3`), not JWTs.
The cert-consumer carries no bearer token — authorization is decided at request time by the
PEP (cert-rest-authz) calling AuthzForce. This is a functionally coherent but
architecturally different authorization model from what AH5 specifies.

### 4.7 CA Is Custom and Not Part of the AH5 Specification

`core/GAP_ANALYSIS.md G9` explicitly states: "This system has no counterpart in the AH5
specification." The CA HTTP API is a custom design. AH5 defines a Certificate Authority as
a supporting core system with a hierarchical PKI model (Master CA → Organization CA →
Local Cloud CA → end-entity). The flat single-CA design here does not match that hierarchy.

---

## 5. Summary Table

| Criterion | Status | Evidence |
|---|---|---|
| TLS used for service-to-service communication | Partial | experiment-7 services; plain HTTP to core systems |
| Client cert required at TLS level (mTLS) | Partial | cert-consumer→cert-rest-authz only; other paths: server-only TLS |
| Peer cert verification against CA pool | Yes | `ClientCAs`/`RootCAs` set on all TLS configs; `MinVersion: TLS12` everywhere |
| Consumer identity from cert CN | Yes | `r.TLS.PeerCertificates[0].Subject.CommonName` in `cert-rest-authz/server.go` |
| CA active and issuing runtime certs | Yes | `core/internal/ca/service/ca.go`; all experiment-7 services call CA at startup |
| SAN (DNS/IP) in issued certificates | No | Bare CN only; no `DNSNames` or `IPAddresses` in leaf template |
| AH5 hierarchical cert naming | No | Bare system names, not `sys.cloud.org.arrowhead.eu` |
| Certificate revocation (CRL/OCSP) | No | Policy-level (XACML) revocation only |
| `InsecureSkipVerify` in production code | No | Absent from all service code |
| mTLS on core system communication | No | `core/GAP_ANALYSIS.md G4` — documented gap |
| AH5 JWT bearer token authorization | No | XACML/AuthzForce used instead |
| Hierarchical PKI (Master→Org→Cloud CA) | No | Flat single self-signed CA |
