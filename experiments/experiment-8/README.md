# Experiment 8 — Arrowhead 5.2 Profile-Based PKI with Enforced Certificate Hierarchy

Experiment-8 extends experiment-7 with an **Arrowhead 5.2 Local Cloud CA** that enforces
a strict certificate-profile hierarchy. Instead of issuing all certificates through a
single flat endpoint, the CA now tracks the *profile* of each certificate (encoded in the
Subject `OrganizationalUnit` field) and requires callers to prove they hold the correct
prior certificate before issuing the next one.

## What is new compared to experiment-7

| Aspect | Experiment-7 | Experiment-8 |
|---|---|---|
| CA implementation | Core CA (`POST /ca/certificate/issue`) | `profile-ca` service: full Arrowhead 5.2 Local Cloud CA |
| Certificate profiles | None — all certs are equivalent | `lo` → `on` → `de` → `sy` hierarchy |
| Profile enforcement | None | Device cert requires `OU=on`; system cert requires `OU=de` |
| Bootstrap endpoint | Single plain-HTTP issue endpoint | `POST /bootstrap/onboarding-cert` (no auth, plain HTTP) |
| Lifecycle | 2-step: GET /ca/info + POST /ca/certificate/issue | 4-step: info → onboarding → device → system |
| mTLS port | No separate CA mTLS port | CA exposes `:8088` for profile-enforced cert issuance |
| Consumer name | `cert-consumer` | `pki-consumer` |
| AuthzForce domain | `arrowhead-exp7` | `arrowhead-exp8` |

## The PKI Certificate Hierarchy

The Arrowhead 5.2 Local Cloud CA issues four certificate profiles:

```
lo  — Local Cloud CA (self-signed root, generated at startup)
 └── on  — Onboarding certificate (no auth required; issued over plain HTTP)
      └── de  — Device certificate  (requires presenting OU=on cert over mTLS)
           └── sy  — System certificate (requires presenting OU=de cert over mTLS)
```

Each profile is encoded in the Subject `OrganizationalUnit` (OU) field of the issued
certificate. The enforcement rules are:

- `POST /bootstrap/onboarding-cert` (plain HTTP `:8087`): no client cert required; issues `OU=on`
- `POST /ca/device-cert` (mTLS `:8088`): client cert must have `OU=on`; issues `OU=de`
- `POST /ca/system-cert` (mTLS `:8088`): client cert must have `OU=de`; issues `OU=sy`
- `POST /ca/certificate/issue` (plain HTTP `:8087`): backward-compat; issues infra certs for Kafka, RabbitMQ, core systems (no profile)

## Application Service Lifecycle

Application services (`pki-consumer`, `pki-rest-authz`, `data-provider-tls`) obtain their
system certificate at startup through the full 4-step lifecycle:

```
1. GET  http://profile-ca:8087/ca/info
        → CA certificate PEM → build in-memory CA pool

2. POST http://profile-ca:8087/bootstrap/onboarding-cert  {"systemName":"<name>"}
        → OU=on onboarding certificate (plain HTTP, no auth)

3. POST https://profile-ca:8088/ca/device-cert  {"systemName":"<name>"}
        (mTLS: client presents OU=on cert)
        → OU=de device certificate

4. POST https://profile-ca:8088/ca/system-cert  {"systemName":"<name>"}
        (mTLS: client presents OU=de cert)
        → OU=sy system certificate ← used for all subsequent mTLS communication
```

Infrastructure services (`cert-provisioner`, core systems, policy-sync) still use the
backward-compatible `POST /ca/certificate/issue` endpoint (plain HTTP, no profile).

## Architecture

```
profile-ca :8087 (plain HTTP — bootstrap + infra issue)
profile-ca :8088 (mTLS HTTPS — profile-enforced device/system issuance)
  │
  ├── cert-provisioner (one-shot init)
  │     └── writes /certs/kafka.{crt,key}, /certs/rabbitmq.{crt,key}, /certs/ca.crt
  │           │
  │           ├── Kafka SSL (9092)
  │           └── RabbitMQ AMQPS (5671)
  │
  └── All app services: 4-step lifecycle → OU=sy system cert at startup
        │
        ├── pki-rest-authz  HTTPS mTLS server (9108) + HTTP health (9109)
        │     identity source: r.TLS.PeerCertificates[0].Subject.CommonName
        │     upstream: data-provider-tls (HTTPS)
        │
        ├── pki-consumer    mTLS client → pki-rest-authz:9108
        │     identity in cert: CN=pki-consumer, OU=sy
        │
        └── data-provider-tls  HTTPS server (9094) + TLS Kafka consumer
```

## Services

| Service | Port(s) exposed to host | Role |
|---|---|---|
| profile-ca | **8087** (HTTP bootstrap/infra), **8088** (HTTPS mTLS profiled) | Arrowhead 5.2 Local Cloud CA |
| cert-provisioner | — | Init: issues Kafka/RabbitMQ/core certs to /certs volume |
| serviceregistry | **8490** (HTTPS/mTLS) | Service registration — plain HTTP :8080 is Docker-internal only |
| authentication | **8491** (HTTPS/mTLS) | Identity tokens — plain HTTP :8081 is Docker-internal only |
| consumerauth | **8492** (HTTPS/mTLS) | Authorization grants — plain HTTP :8082 is Docker-internal only |
| dynamicorch | **8493** (HTTPS/mTLS) | Orchestration — plain HTTP :8083 is Docker-internal only |
| authzforce | **8196** (HTTP) | XACML PDP/PAP |
| policy-sync | **9105** (HTTP) | CA→XACML compiler |
| topic-auth-xacml | 9090 (internal) | AMQP PEP (manages RabbitMQ users) |
| kafka-authz | **9101** (HTTP) | Kafka SSE PEP; TLS Kafka connection |
| pki-rest-authz | **9108** (HTTPS/mTLS), **9109** (HTTP health) | REST mTLS PEP |
| data-provider-tls | 9094 (HTTPS, internal) | HTTPS REST + TLS Kafka consumer |
| pki-consumer | **9107** (HTTP health) | mTLS polling client |
| robot-fleet-tls | **9116**→9003 (HTTP) | AMQPS + TLS Kafka publisher |
| consumer-1/2/3 | — | AMQPS consumers |
| analytics-consumer | **9014** (HTTP health) | Kafka SSE consumer |
| rabbitmq | 5671 (AMQPS, internal), **15677** (mgmt HTTP) | AMQP broker with TLS |
| kafka | 9092 (SSL, internal) | Kafka broker with SSL |

All ports are offset from experiment-7 to allow both stacks to run simultaneously.

## Certificate Provisioning

**cert-provisioner (infrastructure certs, backward-compat path):**
1. Calls `GET http://profile-ca:8087/ca/info` → writes `/certs/ca.crt`
2. Calls `POST http://profile-ca:8087/ca/certificate/issue {"systemName":"kafka"}` → writes `/certs/kafka.{crt,key}`
3. Calls `POST http://profile-ca:8087/ca/certificate/issue {"systemName":"rabbitmq"}` → writes `/certs/rabbitmq.{crt,key}`

**App services (4-step lifecycle at startup):**
1. GET /ca/info → in-memory CA pool
2. POST /bootstrap/onboarding-cert → in-memory `OU=on` cert
3. POST /ca/device-cert (mTLS + OU=on) → in-memory `OU=de` cert
4. POST /ca/system-cert (mTLS + OU=de) → final `OU=sy` system cert used for all mTLS

Certs are ephemeral: each stack restart generates fresh certs from a new CA keypair.

## Quick Start

```bash
cd experiments/experiment-8
docker compose up -d --build
# Wait ~90s for all services to start
bash test-system.sh
```

Open http://localhost:9109/status to see pki-rest-authz statistics.

## Verifying the Profile Hierarchy

Get the CA cert:
```bash
curl -s http://localhost:8087/ca/info | python3 -c \
  'import sys,json; print(json.load(sys.stdin)["certificate"])' > /tmp/ca8.crt
```

Obtain an onboarding cert (no auth required):
```bash
curl -s -X POST http://localhost:8087/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' \
  -d '{"systemName":"test-probe"}' \
  | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"]); print(d["privateKey"])' \
  > /tmp/on.pem
# Split cert and key manually; OU in Subject should be "on"
```

Attempt to get a device cert presenting the onboarding cert (succeeds):
```bash
curl --cert /tmp/on.crt --key /tmp/on.key --cacert /tmp/ca8.crt \
  -X POST https://localhost:8088/ca/device-cert \
  -H 'Content-Type: application/json' \
  -d '{"systemName":"test-probe"}'
# → 201 Created, OU=de
```

Attempt to get a system cert directly (wrong profile — fails):
```bash
curl --cert /tmp/on.crt --key /tmp/on.key --cacert /tmp/ca8.crt \
  -X POST https://localhost:8088/ca/system-cert \
  -H 'Content-Type: application/json' \
  -d '{"systemName":"test-probe"}'
# → 403 Forbidden: wrong profile: got on, want de
```

## Running Unit Tests (no Docker required)

```bash
go test arrowhead/experiment8/profile-ca
go test arrowhead/experiment8/pki-consumer
go test arrowhead/experiment8/pki-rest-authz
go test arrowhead/experiment8/cert-provisioner
go test arrowhead/experiment8/data-provider-tls
```

## Key Concepts Demonstrated

- **Arrowhead 5.2 profile hierarchy**: The `lo → on → de → sy` chain enforces that a
  system must progress through onboarding before it can obtain a system identity. No
  single step can be skipped.
- **Profile enforcement at the CA**: The CA, not the application, verifies that the caller
  holds the correct prior profile certificate. This is cryptographic admission control.
- **Two-port CA design**: The plain HTTP port handles bootstrap and backward-compatible
  infra cert issuance. The mTLS port enforces profile hierarchy for all application certs.
- **Ephemeral identity lifecycle**: System certs are obtained at runtime, not pre-provisioned.
  Restarting a service causes it to re-execute the full 4-step lifecycle.
- **Unified policy**: The same AuthzForce XACML policy (domain `arrowhead-exp8`) still
  governs access across AMQP, Kafka, and REST. Profile-based PKI adds a layer below
  authorization — authentication is now strictly hierarchical.

## WSL2 + Docker Engine Note

If running under WSL2 without `networkingMode=mirrored`, localhost ports may not forward
correctly. Add to `%UserProfile%\.wslconfig`:
```
[wsl2]
networkingMode=mirrored
```
