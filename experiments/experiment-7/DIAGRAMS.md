# Experiment-7 Architecture Diagrams

## 1. TLS Trust Model

All certificates are issued by a single self-signed ECDSA P-256 root CA
(`core/cmd/ca`). The CA does not restart between service starts; its in-memory
state is ephemeral but consistent within a stack lifetime.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Root CA  (ca:8086)                                  │
│               self-signed ECDSA P-256, 10-year validity                      │
│               POST /ca/certificate/issue  (plain HTTP — bootstrap endpoint)  │
└──────────────────────────────┬──────────────────────────────────────────────┘
                               │  issues
        ┌──────────────────────┼──────────────────────────────────────────┐
        │                      │                                           │
   cert-provisioner    Go services (runtime)                     policy-sync
   (init container)    issue own cert at startup                 (file-based)
   writes to /certs    via POST /ca/certificate/issue            from /certs
        │
        ├── ca.crt               (CA root cert — trust anchor)
        ├── serviceregistry.crt/.key
        ├── authentication.crt/.key
        ├── consumerauth.crt/.key
        ├── dynamicorch.crt/.key
        ├── policy-sync.crt/.key
        ├── kafka.crt/.key + kafka-combined.pem
        └── rabbitmq.crt/.key + rabbitmq-combined.pem
```

---

## 2. Service Communication — TLS Coverage

```
                    ┌──────────── Docker network: exp7 ────────────────────────┐
                    │                                                            │
  ┌─────────────────┤  Core Systems (plain HTTP + mTLS)                         │
  │                 │                                                            │
  │  serviceregistry│  :8080 plain HTTP (healthchecks, setup seeding)           │
  │                 │  :8480 HTTPS/mTLS (RequireAndVerifyClientCert)            │
  │                 │         ▲ called by: robot-fleet-tls, dynamicorch          │
  │                 │                                                            │
  │  authentication │  :8081 plain HTTP (healthchecks)                          │
  │                 │  :8481 HTTPS/mTLS                                          │
  │                 │         ▲ called by: robot-fleet-tls, consumer-direct-tls, │
  │                 │           dynamicorch (identity check)                     │
  │                 │                                                            │
  │  consumerauth   │  :8082 plain HTTP (healthchecks, setup container)         │
  │                 │  :8482 HTTPS/mTLS                                          │
  │                 │         ▲ called by: policy-sync, dynamicorch              │
  │                 │                                                            │
  │  dynamicorch    │  :8083 plain HTTP (healthchecks)                          │
  │                 │  :8483 HTTPS/mTLS                                          │
  │                 │         ▲ called by: consumer-direct-tls                  │
  │                 │         ↓ outbound mTLS: serviceregistry:8480,            │
  │                 │           consumerauth:8482, authentication:8481           │
  │                 │                                                            │
  └─────────────────┤                                                            │
                    │  Support Services                                          │
                    │                                                            │
                    │  policy-sync    ──mTLS──▶  consumerauth:8482              │
                    │                 ──HTTP──▶  authzforce:8080                 │
                    │                                                            │
                    │  Experiment Services                                        │
                    │                                                            │
                    │  robot-fleet-tls                                           │
                    │    issues own cert at startup (CA at :8086 plain HTTP)    │
                    │    ──mTLS──▶  serviceregistry:8480                        │
                    │    ──mTLS──▶  authentication:8481                         │
                    │    ──AMQPS──▶ rabbitmq:5671  (CA pool, server-only TLS)  │
                    │    ──SSL───▶  kafka:9092      (CA pool, server-only TLS)  │
                    │                                                            │
                    │  consumer-direct-tls (×3)                                 │
                    │    issues own cert at startup                              │
                    │    ──mTLS──▶  authentication:8481                         │
                    │    ──mTLS──▶  dynamicorch:8483                            │
                    │    ──AMQPS──▶ rabbitmq:5671                               │
                    │                                                            │
                    │  cert-consumer                                             │
                    │    issues own cert at startup                              │
                    │    ──mTLS──▶  cert-rest-authz:9098   (identity from CN)  │
                    │                                                            │
                    │  cert-rest-authz                                           │
                    │    ──HTTPS──▶  data-provider-tls:9094  (server cert verify)│
                    │                                                            │
                    │  kafka-authz ──SSL──▶ kafka:9092                          │
                    │  data-provider-tls ──SSL──▶ kafka:9092                   │
                    │  analytics-consumer ──HTTP──▶ kafka-authz:9091            │
                    │                                                            │
                    └────────────────────────────────────────────────────────────┘
```

---

## 3. Certificate Provisioning Sequence

```
                  CA              cert-provisioner        services
                  |                      |                    |
  stack start     |                      |                    |
  ────────────────▶  GET /ca/info        |                    |
                  |◀──────────────────── |                    |
                  |  POST /issue kafka   |                    |
                  |◀──────────────────── |                    |
                  |  POST /issue rabbitmq|                    |
                  |◀──────────────────── |                    |
                  |  POST /issue serviceregistry              |
                  |◀──────────────────── |                    |
                  |  POST /issue authentication               |
                  |◀──────────────────── |                    |
                  |  POST /issue consumerauth                 |
                  |◀──────────────────── |                    |
                  |  POST /issue dynamicorch                  |
                  |◀──────────────────── |                    |
                  |  POST /issue policy-sync                  |
                  |◀──────────────────── |                    |
                  |  exit 0 (all files written to /certs)     |
                  |                      |                    |
                  |                      |    Kafka, RabbitMQ, core services,
                  |                      |    policy-sync start and mount /certs
                  |                      |    ─────────────────────────────────▶
                  |                      |                    |
                  |   Go services (robot-fleet-tls, cert-consumer, etc.)
                  |   POST /ca/certificate/issue (runtime, own cert)
                  |◀──────────────────────────────────────────|
```

---

## 4. mTLS Handshake — Core Service Path

Example: `consumer-direct-tls → dynamicorch:8483`

```
consumer-direct-tls                    dynamicorch:8483
       │                                      │
       │─── TLS ClientHello ────────────────▶│
       │◀── TLS ServerHello + Certificate ───│
       │    (cert CN=dynamicorch, SAN=dynamicorch,
       │     verified against /certs/ca.crt)  │
       │─── Certificate (own cert, CN=demo-consumer-1) ──▶│
       │    (dynamicorch verifies against /certs/ca.crt)  │
       │─── TLS Finished ──────────────────▶│
       │◀── TLS Finished ───────────────────│
       │  HTTPS POST /orchestration/dynamic  │
       │─────────────────────────────────────▶│
       │◀─────────────────────────────────────│
```

---

## 5. Gap Status Summary (G4)

| Path | Before G4 fix | After G4 fix |
|---|---|---|
| robot-fleet-tls → serviceregistry | plain HTTP | mTLS (runtime-issued cert) |
| robot-fleet-tls → authentication | plain HTTP | mTLS |
| consumer-direct-tls → authentication | plain HTTP | mTLS |
| consumer-direct-tls → dynamicorch | plain HTTP | mTLS |
| dynamicorch → serviceregistry | plain HTTP | mTLS (file cert) |
| dynamicorch → consumerauth | plain HTTP | mTLS |
| dynamicorch → authentication | plain HTTP | mTLS |
| policy-sync → consumerauth | plain HTTP | mTLS (file cert) |
| cert-consumer → cert-rest-authz | mTLS (unchanged) | mTLS (unchanged) |
| cert-rest-authz → data-provider-tls | HTTPS (unchanged) | HTTPS (unchanged) |

**Out of scope (documented):**
- CA itself: plain HTTP — bootstrap trust anchor, cannot self-authenticate
- AuthzForce: Java service on plain HTTP — external component
- Kafka/RabbitMQ: server-only TLS — no client cert required by brokers (KAFKA_SSL_CLIENT_AUTH: none)
