# CLAUDE.md — experiment-7

Experiment-7 is the **X.509/TLS extension** of experiment-6. Read this file fully before
starting any task here.

---

## What this experiment demonstrates

Certificate-based consumer identity using the core CertificateAuthority. Instead of the
self-reported `X-Consumer-Name` header (experiment-6), consumers now authenticate with
X.509 client certificates issued by the CA. The `cert-rest-authz` PEP reads the consumer
identity from the client certificate's Common Name (CN) and passes it to AuthzForce for
the same XACML authorization decision as experiment-6.

Full description: [`README.md`](README.md)

---

## Read before starting any task

1. **This file** — invariants and contracts specific to experiment-7.
2. **[`README.md`](README.md)** — service table, X.509 architecture, certificate provisioning flow.
3. **[`../CLAUDE_EXPERIMENTS.md`](../CLAUDE_EXPERIMENTS.md)** — canonical experiment rules.
4. **[`/EXPERIENCES.md`](../../EXPERIENCES.md)** — pre-flight checklist. Most relevant: EXP-001 (AuthzForce domain), EXP-007 (Kafka partition reader).

---

## Stack topology

```
ConsumerAuth :8082 (HTTP) / :8482 (HTTPS/mTLS)
    GET /authorization/lookup (every SYNC_INTERVAL) ← policy-sync (mTLS)
    ↓
policy-sync :9095   PUT PolicySet → AuthzForce (HTTP)
    ↓
AuthzForce :8186    ◄── single PDP for all three transports (HTTP)
    │
    ├── topic-auth-xacml :9090  →  RabbitMQ :5671 (AMQPS) → consumer-1/2/3
    ├── kafka-authz :9091       →  Kafka :9092 (SSL)       → analytics-consumer
    └── cert-rest-authz :9098   →  data-provider-tls :9094 (HTTPS) → cert-consumer

CoreCA :8086 (plain HTTP — trust anchor, bootstrap endpoint)
    │
    ├── cert-provisioner → /certs volume → Kafka, RabbitMQ, core systems, policy-sync
    └── (all Go services call CA at startup to get own cert)

Core systems (each has both plain HTTP and HTTPS/mTLS port):
    ServiceRegistry       :8080 (HTTP) / :8480 (HTTPS/mTLS)
    Authentication        :8081 (HTTP) / :8481 (HTTPS/mTLS)
    ConsumerAuthorization :8082 (HTTP) / :8482 (HTTPS/mTLS)
    DynamicOrchestration  :8083 (HTTP) / :8483 (HTTPS/mTLS, mTLS outbound)
```

---

## Critical invariants

### 1. AUTHZFORCE_DOMAIN must be `arrowhead-exp7` everywhere

`policy-sync`, `kafka-authz`, `cert-rest-authz`, and `topic-auth-xacml` must all use the
same `AUTHZFORCE_DOMAIN`. Any mismatch causes every auth check to return Deny silently
(EXP-001). The `test-system.sh` pre-flight verifies `domainExternalId` before auth tests.

### 2. cert-provisioner must complete before Kafka and RabbitMQ start

The `kafka` and `rabbitmq` services have `depends_on: cert-provisioner: condition:
service_completed_successfully`. Do not remove these dependencies. If cert-provisioner
fails, both brokers fail to start.

### 3. No InsecureSkipVerify in production paths

All TLS clients must verify server certificates against the CA pool. Tests using
self-signed test certs in `httptest` servers may use custom TLS configs, but never
set `InsecureSkipVerify: true` in service code.

### 4. Kafka uses partition reader (EXP-007)

`data-provider-tls` and `kafka-authz` use partition readers (not consumer groups). Do
not change them to consumer group readers.

### 5. Consumer identity from cert CN, not header

`cert-rest-authz` reads consumer identity from `r.TLS.PeerCertificates[0].Subject.CommonName`.
Do not add X-Consumer-Name header support to `cert-rest-authz` — that defeats the X.509
model. Use the plain `/auth/check` endpoint (on the HTTP health port 9099) for testing
the AuthzForce decision directly without a cert.

### 6. cert-provisioner writes combined PEM (cert+key) for RabbitMQ

RabbitMQ reads the cert from `ssl_options.certfile` and the key from `ssl_options.keyfile`.
cert-provisioner writes separate `.crt` and `.key` files. The `rabbitmq.conf` points to
these separate files. Kafka uses a PKCS12 keystore built by the TLS entrypoint script from
the same `.crt` and `.key` files.

---

## Key contracts

### cert-rest-authz mTLS proxy (port 9098)

- Requires a valid client certificate (TLS level).
- Consumer name = `cert.Subject.CommonName` of the first peer certificate.
- Makes AuthzForce decision for `(CN, DEFAULT_SERVICE, "invoke")`.
- Returns 403 if Deny; proxies to UPSTREAM_URL (data-provider-tls) if Permit.

### cert-rest-authz HTTP health/check (port 9099)

- No TLS. Used for Docker healthchecks, dashboard API, test scripts.
- `/health`, `/status`, `/auth/check` (explicit policy check without a cert).
- `/auth/check` takes `{"consumer":"...","service":"..."}` — same as experiment-6.

### cert-consumer

- Issues own cert at startup: `POST http://ca:8086/ca/certificate/issue {"systemName":"cert-consumer"}`.
- Uses cert CN as identity when polling (no X-Consumer-Name header).
- Stats transport field: `"rest-mtls"` (distinguishes from experiment-6's `"rest"`).

### Sync-delay caveat (inherited from experiment-6)

REST enforcement lags ConsumerAuth by up to `SYNC_INTERVAL` (default 10 s). See `CLAUDE.md`
for experiment-6 for details. The `test-system.sh` waits 30 s after revocation before
asserting Deny.

---

## Environment variables for experiment-7 Go services

### cert-rest-authz

| Variable | Default | Description |
|---|---|---|
| `CA_URL` | `http://ca:8086` | Core CA base URL |
| `AUTHZFORCE_URL` | `http://authzforce:8080/authzforce-ce` | AuthzForce base URL |
| `AUTHZFORCE_DOMAIN` | `arrowhead-exp7` | AuthzForce domain externalId |
| `UPSTREAM_URL` | — | HTTPS URL of data-provider-tls (required) |
| `DEFAULT_SERVICE` | `telemetry-rest` | Service name for AuthzForce check |
| `CACHE_TTL` | `0s` | Decision cache TTL |
| `PORT` | `9099` | HTTP port for health/status/auth-check |
| `TLS_PORT` | `9098` | HTTPS port for mTLS proxy |

### cert-consumer

| Variable | Default | Description |
|---|---|---|
| `CONSUMER_NAME` | `cert-consumer` | System name and cert CN |
| `CA_URL` | `http://ca:8086` | Core CA base URL |
| `CERT_REST_AUTHZ_URL` | `https://cert-rest-authz:9098` | mTLS proxy URL |
| `SERVICE` | `telemetry-rest` | X-Service-Name equivalent (X-Service-Name header) |
| `POLL_INTERVAL` | `2s` | How often to poll |
| `HEALTH_PORT` | `9096` | HTTP port for health/stats |

### data-provider-tls

| Variable | Default | Description |
|---|---|---|
| `KAFKA_BROKERS` | `kafka:9092` | Kafka SSL broker addresses |
| `KAFKA_TOPIC` | `arrowhead.telemetry` | Topic to consume |
| `PORT` | `9094` | HTTPS listen port |
| `CA_URL` | `http://ca:8086` | Core CA base URL |

### robot-fleet-tls

Same as experiment-5's robot-fleet plus:

| Variable | Default | Description |
|---|---|---|
| `CA_URL` | `http://ca:8086` | Core CA base URL |
| `AMQP_URL` | — | Must use `amqps://` scheme for TLS |
| `KAFKA_BROKERS` | `kafka:9092` | Kafka SSL broker addresses |
| `SR_URL` | — | Must use `https://` when mTLS is enabled (port 8480) |
| `AUTH_URL` | — | Must use `https://` when mTLS is enabled (port 8481) |

### Core systems (TLS)

| Variable | Default | Description |
|---|---|---|
| `PORT` | system-specific | Plain HTTP port (healthchecks, bootstrap) |
| `TLS_PORT` | — | HTTPS+mTLS port (when set, starts second listener) |
| `TLS_CERT_FILE` | — | PEM certificate file (required with TLS_PORT) |
| `TLS_KEY_FILE` | — | PEM private key file (required with TLS_PORT) |
| `TLS_CA_FILE` | — | PEM CA file; when set, RequireAndVerifyClientCert |

### policy-sync (TLS)

| Variable | Default | Description |
|---|---|---|
| `TLS_CERT_FILE` | — | PEM certificate for mTLS to ConsumerAuthorization |
| `TLS_KEY_FILE` | — | PEM private key |
| `TLS_CA_FILE` | — | PEM CA file for server cert verification |

---

## Prohibitions

- Do NOT use `InsecureSkipVerify: true` in any service code.
- Do NOT add `X-Consumer-Name` header support to `cert-rest-authz` — cert CN is the identity.
- Do NOT change Kafka readers to consumer groups (EXP-007).
- Do NOT hardcode `arrowhead-exp7` — always read from `AUTHZFORCE_DOMAIN` env var.
- Do NOT modify files under `core/` — new CA API endpoints would go there if needed.
- Do NOT remove cert-provisioner's dependency on `ca` being healthy.

---

## Running the full test suite

```bash
cd experiments/experiment-7
docker compose up -d --build
bash test-system.sh
```

The script covers:
1. Core and PEP service health + policy-sync domain check
2. CA issue/verify endpoints
3. AuthzForce server endpoints
4. policy-sync /status
5. kafka-authz authorization (Kafka path)
6. cert-rest-authz /auth/check (HTTP, no cert needed)
7. mTLS flow: curl with client cert → 200; without cert → rejected
8. cert-consumer msgCount > 0 (mTLS polling works)
9. analytics-consumer msgCount > 0 (Kafka TLS path works)
10. cert-rest-authz /status counters
11. Revocation propagation (cert-consumer Deny after grant removed)
12. kafka-authz SSE stream

Unit tests (no Docker required):
```bash
go test arrowhead/experiment7/cert-provisioner
go test arrowhead/experiment7/cert-rest-authz
go test arrowhead/experiment7/cert-consumer
go test arrowhead/experiment7/data-provider-tls
go test arrowhead/kafka-authz        # TLS Kafka support
go test arrowhead/message-broker     # TLS AMQP support
```
