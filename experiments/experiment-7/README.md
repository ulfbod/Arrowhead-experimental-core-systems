# Experiment 7 — X.509/TLS Extension: Certificate-Based Identity with mTLS

Experiment-7 extends the unified policy projection from experiment-6 with a consistent
X.509/TLS security model. The key advance is that REST consumers are now **identified by
the Common Name (CN) in their X.509 client certificate** instead of the self-reported
`X-Consumer-Name` HTTP header. All transport paths also add TLS encryption.

## What is new compared to experiment-6

| Aspect | Experiment-6 | Experiment-7 |
|---|---|---|
| REST consumer identity | `X-Consumer-Name` header (self-reported) | Client certificate CN (cryptographically verified) |
| REST encryption | Plaintext HTTP | HTTPS / mTLS |
| Kafka | Plaintext | SSL/TLS |
| RabbitMQ | Plaintext AMQP | AMQPS (TLS) |
| CA usage | Placeholder only | Active: issues certs for all services |
| Trust model | Docker network boundary | X.509 certificate chain |

## X.509/TLS Architecture

```
CoreCA :8086 (plain HTTP — it IS the trust anchor)
  │
  ├── cert-provisioner (one-shot init)
  │     └── writes /certs/kafka.{crt,key}, /certs/rabbitmq.{crt,key}, /certs/ca.crt
  │           │
  │           ├── Kafka SSL (9092)
  │           └── RabbitMQ AMQPS (5671)
  │
  └── All Go services: GET /ca/info + POST /ca/certificate/issue at startup
        │
        ├── cert-rest-authz  HTTPS mTLS server (9098) + HTTP health (9099)
        │     identity source: r.TLS.PeerCertificates[0].Subject.CommonName
        │     upstream: data-provider-tls (HTTPS)
        │
        ├── cert-consumer    mTLS client → cert-rest-authz:9098
        │     identity in cert: CN=cert-consumer
        │
        ├── data-provider-tls  HTTPS server (9094) + TLS Kafka consumer
        ├── robot-fleet-tls    AMQPS + TLS Kafka publisher
        ├── consumer-1/2/3     AMQPS consumers (TLS)
        └── kafka-authz        TLS Kafka consumer (SSE → analytics-consumer)
```

## What uses mTLS vs server-only TLS

| Connection | TLS Type | Identity Source |
|---|---|---|
| cert-consumer → cert-rest-authz | **Mutual TLS (mTLS)** | Client cert CN |
| cert-rest-authz → data-provider-tls | Server-only TLS | None (trusted upstream) |
| Go services → Kafka | Server-only TLS | AMQP credentials |
| Go services → RabbitMQ | Server-only TLS (AMQPS) | AMQP credentials |
| All → AuthzForce | Plain HTTP | Docker network |
| All → Core systems | Plain HTTP | Docker network |

**Documented plain-HTTP services (limitations):**
- Core systems: not modified (cross-experiment API stability)
- AuthzForce: Java service, complex TLS config beyond scope
- policy-sync → AuthzForce: AuthzForce is HTTP-only
- analytics-consumer → kafka-authz: SSE path is HTTP; TLS is on the Kafka layer

## Services

| Service | Port(s) | Role |
|---|---|---|
| ca | 8086 | Core CA: issues X.509 certs (plain HTTP) |
| cert-provisioner | — | Init: issues Kafka/RabbitMQ certs to /certs volume |
| serviceregistry | 8080 | Service registration |
| authentication | 8081 | Identity tokens |
| consumerauth | 8082 | Authorization grants (source of truth) |
| dynamicorch | 8083 | Orchestration |
| authzforce | 8186 (host) | XACML PDP/PAP (HTTP) |
| policy-sync | 9095 | CA→XACML compiler; `/config` for SYNC_INTERVAL |
| topic-auth-xacml | 9090 | AMQP PEP (manages RabbitMQ users) |
| kafka-authz | 9091 | Kafka SSE PEP; TLS Kafka connection |
| cert-rest-authz | **9098** (HTTPS/mTLS), **9099** (HTTP health) | REST mTLS PEP |
| data-provider-tls | 9094 (HTTPS, internal) | HTTPS REST + TLS Kafka consumer |
| cert-consumer | 9096 (HTTP health, internal) | mTLS polling client |
| robot-fleet-tls | 9106→9003 | AMQPS + TLS Kafka publisher |
| consumer-1/2/3 | — | AMQPS consumers |
| analytics-consumer | 9004 (health) | Kafka SSE consumer |
| rabbitmq | 5671 (AMQPS), 15676 (mgmt) | AMQP broker with TLS |
| kafka | 9092 (SSL) | Kafka broker with SSL |

## Certificate Provisioning

Certificates are issued by the core CertificateAuthority service at runtime:

**cert-provisioner (infrastructure certs):**
1. Calls `GET http://ca:8086/ca/info` → writes `/certs/ca.crt`
2. Calls `POST http://ca:8086/ca/certificate/issue {"systemName":"kafka"}` → writes `/certs/kafka.{crt,key}`
3. Calls `POST http://ca:8086/ca/certificate/issue {"systemName":"rabbitmq"}` → writes `/certs/rabbitmq.{crt,key}`

**Go services (own certs, loaded at startup):**
1. `GET http://ca:8086/ca/info` → in-memory CA cert pool (for verifying peers)
2. `POST http://ca:8086/ca/certificate/issue {"systemName":"<name>"}` → in-memory cert+key

Certs are ephemeral: each stack restart generates fresh certs. This is correct because
all certs share the same CA root, so old certs automatically become invalid.

## Quick Start

```bash
cd experiments/experiment-7
docker compose up -d --build
# Wait ~60s for all services to start
bash test-system.sh
```

Open http://localhost:9099/status to see cert-rest-authz statistics.

## Verifying mTLS

Obtain the CA cert and a test certificate:
```bash
# Get CA cert
curl -s http://localhost:8086/ca/info | python3 -c \
  'import sys,json; print(json.load(sys.stdin)["certificate"])' > /tmp/ca.crt

# Issue a test-probe certificate
curl -s -X POST http://localhost:8086/ca/certificate/issue \
  -H 'Content-Type: application/json' \
  -d '{"systemName":"test-probe"}' | python3 -c \
  'import sys,json; d=json.load(sys.stdin); print(d["certificate"]); print(d["privateKey"])' \
  | awk '/BEGIN CERT/{found=1} found{print}' > /tmp/probe.crt
# (Separate key manually or use jq if available)
```

Access with client cert (authorized):
```bash
curl --cert /tmp/probe.crt --key /tmp/probe.key --cacert /tmp/ca.crt \
  https://localhost:9098/telemetry/latest
# → 200 OK with telemetry JSON
```

Access without client cert (rejected):
```bash
curl --cacert /tmp/ca.crt https://localhost:9098/telemetry/latest
# → TLS handshake error (server requires client cert)
```

## Key Concepts Demonstrated

- **Certificate-as-identity**: The consumer CN in the client certificate replaces the
  `X-Consumer-Name` header. The identity is now cryptographically bound.
- **CA-rooted trust**: All services trust certs issued by the same CA. No hard-coded
  cert files — the CA's private key never leaves the CA service.
- **Unified policy still works**: The same AuthzForce XACML policy enforces access on
  AMQP (topic-auth-xacml), Kafka (kafka-authz), and REST (cert-rest-authz).
- **Infrastructure TLS**: Kafka and RabbitMQ use TLS for transport encryption.

## WSL2 + Docker Engine Note

If running under WSL2 without `networkingMode=mirrored`, localhost ports may not
forward correctly. Add to `%UserProfile%\.wslconfig`:
```
[wsl2]
networkingMode=mirrored
```
