# Experiment 10 — UC3 with Clean PAP/PIP/PDP Access-Control Architecture

Experiment-10 extends experiment-9 to implement a **classical PAP/PIP/PDP access-control
architecture**. The central problem addressed: in experiment-9, `ConsumerAuthorization` was
the single source of truth for XACML policies, with `policy-sync` polling it on a fixed interval
(10 s default) and pushing to AuthzForce. This introduced a sync delay and an architectural
ambiguity (ConsumerAuth was not designed to be a policy store).

Experiment-10 eliminates this ambiguity:

| Component | Role |
|---|---|
| **PAP** (Policy Administration Point) | Single source of truth for XACML policies; pushes to AuthzForce on every Create/Delete (zero delay) |
| **PIP** (Policy Information Point)    | Subject attribute registry: maps system names to cert level (lo/on/de/sy) and validity |
| **PDP**                               | AuthzForce (unchanged) — evaluates XACML decisions |
| **PEP**                               | `pki-rest-authz` (mTLS) and `kafka-authz` (Kafka) — unchanged |

`policy-sync` is removed. ConsumerAuth is still present for Arrowhead 5 orchestration
(DynamicOrchestration uses it) but is no longer the source of truth for XACML policies.

---

## What is new compared to experiment-9

| Aspect | Experiment-9 | Experiment-10 |
|---|---|---|
| Policy source of truth | ConsumerAuthorization | PAP (new service) |
| Policy distribution | policy-sync (polls every 10 s) | PAP pushes immediately on change |
| Revocation propagation delay | ≤10 s (sync interval) | 0 s (synchronous push) |
| Subject attribute resolution | Embedded in PEP | PIP (new service) |
| Admin UI | None | `admin.html` (PAP + PIP CRUD) |
| AuthzForce domain | `arrowhead-exp9` | `arrowhead-exp10` |
| Port set | 8187-9218 | 8287-9318 (all new, no overlap) |

---

## UC3 Scenario Overview

```
Robot Site 1 ┐
Robot Site 2 ├─ Kafka SSL / AMQP TLS ──► Portal & Cloud ML ──► pki-rest-authz mTLS ──► Service Partner 1
Robot Site 3 ┘                                                                        └► Service Partner 2

PAP ──────────────────────────────────────────────────────────────────────► AuthzForce PDP
PIP ─────────────────────────────────── (attribute lookup by PEPs / admin dashboard)
```

Each robot-fleet site publishes simulated IMU telemetry at 5 Hz to:
- **Kafka** (topic `arrowhead.telemetry`, SSL) — consumed by `portal-cloud-ml` via `kafka-authz` SSE
- **RabbitMQ** (AMQPS) — guarded by `topic-auth-xacml` XACML plugin

`portal-cloud-ml` aggregates all robot telemetry and serves it over HTTPS REST.
Service Partners poll via `pki-rest-authz` (mTLS + XACML PEP).
Policies are managed via the PAP REST API; changes take effect immediately.

---

## The PKI Certificate Hierarchy

```
lo  — Local Cloud CA (self-signed root, generated at startup)
 └── on  — Onboarding cert  (POST /bootstrap/onboarding-cert, plain HTTP :8087, no auth)
      └── de  — Device cert   (mTLS POST /ca/device-cert, requires OU=on)
           └── sy  — System cert  (mTLS POST /ca/system-cert, requires OU=de)
                     └── Used as identity at pki-rest-authz :9208 (OU=sy enforced at TLS)
```

---

## Architecture: PAP/PIP/PDP

```
Admin / Test ──POST /policies──► PAP :9305 ──BuildPolicy()+SetPolicy()──► AuthzForce :8080
                                  │
                                  └── GET /policies, DELETE /policies/{id}
                                  
Admin / Test ──POST /subjects──► PIP :9306
                                  │
                                  └── GET /attributes/{name}

pki-rest-authz ──/auth/check──► AuthzForce (XACML Decide) → Permit/Deny
kafka-authz    ──/auth/check──► AuthzForce (XACML Decide) → Permit/Deny
```

The PAP uses the `support/authzforce` library's `BuildPolicy()` + `SetPolicy()` to compile
and upload a XACML PolicySet on every mutation. The PDP (AuthzForce) is always consistent
with the PAP — there is no polling window.

---

## Services and Ports

| Service | Internal port | Host port | Description |
|---|---|---|---|
| `profile-ca` (HTTP) | 8087 | 8287 | Local Cloud CA — onboarding endpoint |
| `profile-ca` (mTLS) | 8088 | 8288 | Local Cloud CA — device/system cert endpoint |
| `authzforce` | 8080 | 8396 | AuthzForce CE (XACML PDP) |
| `serviceregistry` (TLS) | 8490 | 8690 | Arrowhead ServiceRegistry |
| `authentication` (TLS) | 8491 | 8691 | Arrowhead Authentication |
| `consumerauth` (TLS) | 8492 | 8692 | Arrowhead ConsumerAuthorization (orchestration only) |
| `dynamicorch` (TLS) | 8493 | 8693 | Arrowhead DynamicOrchestration |
| `pap` | 9305 | 9305 | PAP — Policy Administration Point |
| `pip` | 9306 | 9306 | PIP — Policy Information Point |
| `kafka-authz` | 9101 | 9301 | Kafka XACML PEP |
| `pki-rest-authz` (mTLS) | 9208 | 9308 | REST mTLS PEP (OU=sy + XACML) |
| `pki-rest-authz` (HTTP) | 9209 | 9309 | REST PEP auth/check endpoint |
| `portal-cloud-ml` | 9207 | 9307 | Portal & Cloud ML (aggregator) |
| `service-partner-1` | 9201 | 9311 | Service Partner 1 |
| `service-partner-2` | 9202 | 9312 | Service Partner 2 |
| `robot-fleet-site-1` | 9003 | 9316 | Robot Fleet Site 1 |
| `robot-fleet-site-2` | 9003 | 9317 | Robot Fleet Site 2 |
| `robot-fleet-site-3` | 9003 | 9318 | Robot Fleet Site 3 |
| `rabbitmq` (management) | 15672 | 15779 | RabbitMQ management UI |
| `dashboard` | 80 | 3010 | Monitoring + Admin UI |

---

## Running the Experiment

```bash
cd experiments/experiment-10
docker compose up -d --build

# Open monitoring dashboard
xdg-open http://localhost:3010

# Open policy admin dashboard
xdg-open http://localhost:3010/admin.html

# Run system tests (requires stack to be up and healthy)
bash test-system.sh
```

---

## PAP API

```
GET  /health          → {"status":"ok"}
GET  /status          → {"policies":N,"version":N,"domainExternalId":"arrowhead-exp10"}
GET  /policies        → {"policies":[...],"count":N}
POST /policies        → {"subject":"...","resource":"...","action":"...","effect":"Permit"}
GET  /policies/{id}   → Policy
DELETE /policies/{id} → 204 No Content
```

## PIP API

```
GET  /health               → {"status":"ok"}
GET  /status               → {"subjects":N}
GET  /subjects             → {"subjects":[...],"count":N}
POST /subjects             → {"name":"...","certLevel":"sy","valid":true}
GET  /subjects/{name}      → Subject
DELETE /subjects/{name}    → 204 No Content
GET  /attributes/{name}    → {"systemName":"...","certLevel":"sy","valid":true}
```

---

## Key Design Decisions

1. **PAP pushes synchronously**: `store.Add()` / `store.Delete()` hold the response until
   `pusher.Push()` completes. The HTTP handler returns 201/204 only after AuthzForce has
   acknowledged the new policy. If AuthzForce is down, the policy is still stored (for
   resilience) but the push failure is logged.

2. **Pusher interface**: The `Pusher` interface in `pap/server.go` allows test isolation
   via `noopPusher` without a real AuthzForce. The production implementation in `main.go`
   uses `authzforcePusher` which calls `authzforce.BuildPolicy()` + `SetPolicy()`.

3. **PIP is additive**: PIP is a subject attribute registry; it does not replace mTLS cert
   verification in `pki-rest-authz`. The PEP still verifies `OU=sy` at the TLS layer;
   PIP provides application-layer attribute resolution for the admin dashboard and for
   future XACML attribute-mapping use cases.

4. **ConsumerAuth retained**: ConsumerAuth is still used by DynamicOrchestration for
   service discovery. The setup container registers grants in both PAP (XACML policy)
   and ConsumerAuth (orchestration) at startup.

5. **No policy-sync**: The 10 s sync interval is eliminated. Revocation takes effect
   the moment the PAP `DELETE /policies/{id}` response returns.
