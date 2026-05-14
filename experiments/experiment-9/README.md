# Experiment 9 — UC3 "Lawn Mowing as a Service" with Arrowhead 5.2 PKI

Experiment-9 extends experiment-8 to the **AIMS5.0 UC3 scenario**: multiple robot-fleet
sites publish telemetry over Kafka (TLS) and AMQP (TLS); a **Portal & Cloud ML** service
aggregates all streams and exposes an HTTPS REST API; two **Service Partners** (SP1/SP2)
consume that API via the `pki-rest-authz` mTLS proxy PEP.

All services that access protected resources go through the full Arrowhead 5.2 PKI
lifecycle (`on → de → sy`) at startup and authenticate using their system certificate.
Authorization is enforced by AuthzForce (XACML domain `arrowhead-exp9`), kept in sync
by `policy-sync`.

---

## What is new compared to experiment-8

| Aspect | Experiment-8 | Experiment-9 |
|---|---|---|
| UC scenario | Single robot fleet → data-provider | UC3: 3 robot sites → Portal & Cloud ML → Service Partners |
| Robot fleet instances | 1 (`robot-fleet-tls`) | 3 (`robot-fleet-site-1/2/3`) |
| Portal service | `data-provider-tls` (HTTPS REST + Kafka) | `portal-cloud-ml` (Kafka SSE aggregator + HTTPS REST) |
| REST consumer | `pki-consumer` (polling mTLS client) | `service-partner-1/2` (polling mTLS clients, OU=sy) |
| SSE consumer | `analytics-consumer` (direct Kafka SSE) | `portal-cloud-ml` (Kafka SSE via kafka-authz) |
| Transport coverage | AMQP + Kafka SSE + mTLS REST | Same + AMQP from 3 sites |
| AuthzForce domain | `arrowhead-exp8` | `arrowhead-exp9` |
| Port set | 8087-9116 | 8187-9218 (all new, no overlap) |

---

## UC3 Scenario Overview

```
Robot Site 1 ┐
Robot Site 2 ├─ Kafka SSL / AMQP TLS ──► Portal & Cloud ML ──► pki-rest-authz mTLS ──► Service Partner 1
Robot Site 3 ┘                                                                        └► Service Partner 2
```

Each robot-fleet site publishes simulated IMU telemetry at 5 Hz to:
- **Kafka** (topic `arrowhead.telemetry`, SSL) — consumed by `portal-cloud-ml` via `kafka-authz` SSE
- **RabbitMQ** (AMQPS) — guarded by `topic-auth-xacml` XACML plugin

`portal-cloud-ml` aggregates all robot telemetry, stores the latest payload in memory, and
serves it over HTTPS REST (`/telemetry/latest`). Service Partners poll this endpoint every
5 seconds via `pki-rest-authz`, which enforces both `OU=sy` profile and an AuthzForce grant.

---

## The PKI Certificate Hierarchy

```
lo  — Local Cloud CA (self-signed root, generated at startup)
 └── on  — Onboarding cert  (POST /bootstrap/onboarding-cert, plain HTTP :8087, no auth)
      └── de  — Device cert   (mTLS POST /ca/device-cert, requires OU=on)
           └── sy  — System cert  (mTLS POST /ca/system-cert, requires OU=de)
                     └── Used as identity at pki-rest-authz :9208 (OU=sy enforced at TLS)
```

Services that complete the 4-step lifecycle:
- `portal-cloud-ml` — acquires `OU=sy` cert; uses it as the HTTPS server cert for `:9294`
- `pki-rest-authz` — acquires `OU=sy` cert; uses it as the mTLS server cert for `:9208`
- `service-partner-1/2` — each acquires `OU=sy` cert; present it as client cert to `pki-rest-authz`

---

## Authorization Grants (seeded by setup container)

| Consumer | Provider | Service |
|---|---|---|
| `portal-cloud-ml` | `robot-fleet-site-1` | `telemetry` |
| `service-partner-1` | `portal-cloud-ml` | `telemetry-rest` |
| `service-partner-2` | `portal-cloud-ml` | `telemetry-rest` |
| `test-probe` | `portal-cloud-ml` | `telemetry-rest` |
| `test-probe` | `robot-fleet-site-1` | `telemetry` |

Grants are pushed to AuthzForce by `policy-sync` every 10 seconds (XACML domain `arrowhead-exp9`).

---

## Services

| Service | Host port(s) | Role |
|---|---|---|
| `profile-ca` | **8187** (HTTP), **8188** (mTLS) | Arrowhead 5.2 Local Cloud CA |
| `cert-provisioner` | — | Init: writes infra certs to `/certs` volume |
| `serviceregistry` | **8590** (mTLS) | Service registration |
| `authentication` | **8591** (mTLS) | Identity tokens |
| `consumerauth` | **8592** (mTLS) | Authorization grants |
| `dynamicorch` | **8593** (mTLS) | Orchestration |
| `authzforce` | **8296** (HTTP) | XACML PDP/PAP |
| `policy-sync` | **9205** (HTTP) | ConsumerAuth → XACML compiler |
| `topic-auth-xacml` | — (internal) | AMQP authorization plugin |
| `kafka-authz` | **9201** (HTTP) | Kafka SSE PEP |
| `pki-rest-authz` | **9208** (mTLS), **9209** (HTTP health) | REST mTLS PEP |
| `portal-cloud-ml` | **9207** (HTTP health/stats) | Kafka SSE aggregator + HTTPS REST :9294 |
| `service-partner-1` | **9211** (HTTP health) | mTLS polling client |
| `service-partner-2` | **9212** (HTTP health) | mTLS polling client |
| `robot-fleet-site-1` | **9216** (HTTP control) | Telemetry publisher (2 robots) |
| `robot-fleet-site-2` | **9217** (HTTP control) | Telemetry publisher (2 robots) |
| `robot-fleet-site-3` | **9218** (HTTP control) | Telemetry publisher (2 robots) |
| `rabbitmq` | **15678** (mgmt HTTP) | AMQP broker with TLS |
| `dashboard` | **3009** (HTTP) | Browser UI (nginx proxy) |

All ports are offset from experiments 1–8 to allow multiple stacks to coexist.

---

## Quick Start

```bash
cd experiments/experiment-9
docker compose up -d --build
# Wait ~90s for all services to be healthy
bash test-system.sh
```

Open the dashboard at http://localhost:3009

---

## Verifying Service Partner Access

Check the PKI lifecycle completed and telemetry is flowing:

```bash
# portal-cloud-ml health and stats
curl http://localhost:9207/health
curl http://localhost:9207/stats

# service-partner-1 stats (msgCount should be growing)
curl http://localhost:9211/health
curl http://localhost:9211/stats   # msgCount increments every 5s on Permit

# service-partner-2 stats
curl http://localhost:9212/stats
```

Verify pki-rest-authz is enforcing access:

```bash
# Auth check via plain HTTP health port (no mTLS needed)
curl "http://localhost:9209/auth-check?consumer=service-partner-1&service=telemetry-rest"
# → {"decision":"Permit"}

curl "http://localhost:9209/auth-check?consumer=unknown-system&service=telemetry-rest"
# → {"decision":"Deny"}
```

Revoke and restore a service partner's access:

```bash
# Find the grant ID
GRANT_ID=$(curl -s http://localhost:8592/authorization/lookup \
  | python3 -c "import sys,json; gs=json.load(sys.stdin); \
    print(next(g['id'] for g in gs if g['consumerSystemName']=='service-partner-1'))")

# Revoke — service-partner-1 will see 403 after next policy-sync (up to 10s)
curl -X DELETE http://localhost:8592/authorization/revoke/$GRANT_ID

# Restore
curl -s -X POST http://localhost:8592/authorization/grant \
  -H 'Content-Type: application/json' \
  -d '{"consumerSystemName":"service-partner-1","providerSystemName":"portal-cloud-ml","serviceDefinition":"telemetry-rest"}'
```

---

## Verifying the PKI Profile Hierarchy

```bash
# Get CA cert
curl -s http://localhost:8187/ca/info \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["certificate"])' > /tmp/ca9.crt

# Obtain an onboarding cert (no auth)
curl -s -X POST http://localhost:8187/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' \
  -d '{"systemName":"test-probe"}'
# → cert {OU=on, CN=test-probe}

# Attempt to get system cert directly from onboarding cert (must fail)
# (requires OU=de client cert — TLS rejection or 403)
```

---

## Running Unit Tests (no Docker required)

```bash
go test arrowhead/experiment9/portal-cloud-ml    # ≥80% coverage
go test arrowhead/experiment9/service-partner    # ≥80% coverage
```

Or from the repo root workspace:

```bash
go test ./experiments/experiment-9/services/...
```

---

## Key Concepts Demonstrated

- **UC3 multi-site IoT**: Three geographically separated robot sites publish to a shared
  Kafka topic; a cloud-aggregation service normalises all streams behind a single REST API.

- **PKI lifecycle as admission control**: `portal-cloud-ml` and `service-partner-1/2` each
  execute the full `on → de → sy` lifecycle at startup. Only after holding a valid `OU=sy`
  system certificate can a service present itself to `pki-rest-authz`.

- **Two-layer access control**: XACML authorization (AuthzForce) AND cryptographic profile
  enforcement (OU=sy at TLS) must both succeed for a service partner to receive data.

- **Decoupled aggregation**: `portal-cloud-ml` subscribes to Kafka via `kafka-authz` SSE
  and caches the latest telemetry. Service partners poll a stable REST endpoint rather than
  connecting directly to Kafka.

- **Policy propagation latency**: Revocation takes effect within one `SYNC_INTERVAL` (10 s)
  — the window between `DELETE /authorization/revoke` and the next AuthzForce PolicySet push.

- **Ephemeral certificates**: All system certs are generated at container startup from the
  CA's current keypair. Restarting any service re-runs the full 4-step lifecycle.

---

## WSL2 Note

If running under WSL2 without `networkingMode=mirrored`, localhost ports may not forward.
Add to `%UserProfile%\.wslconfig`:

```
[wsl2]
networkingMode=mirrored
```
