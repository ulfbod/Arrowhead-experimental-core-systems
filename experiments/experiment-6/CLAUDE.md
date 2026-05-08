# CLAUDE.md — experiment-6

Experiment-6 is the **active baseline** for future experiments. Read this file fully before
starting any task here.

---

## What this experiment demonstrates

Triple-transport unified policy projection using XACML/ABAC (AuthzForce). A single grant in
ConsumerAuthorization propagates to all three transports — AMQP, Kafka, and REST — within one
`SYNC_INTERVAL`. The experiment also exposes a runtime-configurable sync interval so the
enforcement lag can be observed interactively.

Full description, service table, and startup instructions: [`README.md`](README.md)  
Architecture and sequence diagrams: [`DIAGRAMS.md`](DIAGRAMS.md)

---

## Read before starting any task

1. **This file** — invariants and contracts specific to experiment-6.
2. **[`README.md`](README.md)** — service table with ports, startup order, architecture diagram.
3. **[`DIAGRAMS.md`](DIAGRAMS.md)** — component diagram and the three REST-path sequence diagrams.
4. **[`support/CLAUDE.md`](../../support/CLAUDE.md)** — stability status and spec file locations for all support modules used here.
5. **[`/EXPERIENCES.md`](../../EXPERIENCES.md)** — pre-flight checklist. The most relevant entries for this experiment: EXP-001 (hardcoded AuthzForce domain), EXP-007 (Kafka partition reader), EXP-010 (Docker build context for dashboard symlinks).

---

## Stack topology

```
ConsumerAuth :8082
    │  GET /authorization/lookup  (every SYNC_INTERVAL)
    ▼
policy-sync :9095
    │  PUT PolicySet (XACML 3.0, arrowhead-exp6 domain)
    ▼
AuthzForce :8186   ◄── single PDP/PAP for all three transports
    │
    ├── topic-auth-xacml :9090  →  RabbitMQ :5672   →  consumer-1/2/3 (AMQP)
    ├── kafka-authz :9091       →  Kafka :9092       →  analytics-consumer (SSE)
    └── rest-authz :9093        →  data-provider :9094  →  rest-consumer
                                       ▲
                               Kafka consumer (partition reader)
```

**robot-fleet** dual-publishes to both RabbitMQ and Kafka. `data-provider` consumes from
Kafka and serves a REST API that `rest-authz` proxies.

Dashboard nginx on port **3006** proxies `/api/*` to the backend services — all dashboard API
calls go through nginx, not directly to the services.

---

## Critical invariants

### 1. AUTHZFORCE_DOMAIN must be `arrowhead-exp6` everywhere

`policy-sync`, `kafka-authz`, `rest-authz`, and `topic-auth-xacml` all share an AuthzForce
domain. Every service must use the same `externalId`. Any mismatch causes **every auth check
to return Deny silently** — policy decisions appear correct but never Permit.

```yaml
# docker-compose.yml — must match in all four services:
AUTHZFORCE_DOMAIN: arrowhead-exp6
```

This is EXP-001. The `test-system.sh` pre-flight check asserts `domainExternalId` before
running any authorization tests.

### 2. `data-provider` must use a Kafka partition reader, not a consumer group

`data-provider` starts before `robot-fleet` creates the Kafka topic. A consumer group
reader commits offsets and misses messages produced before it joins. A partition reader
(offset `kafka.LastOffset`) replays correctly regardless of startup order.

The current implementation uses `kafka.NewReader` with `Partition: 0` — do not change it
to a consumer group. See EXP-007.

### 3. `data-provider` performs no authorization

`rest-authz` is the authorization boundary. `data-provider` is a plain data server — it
must not check `X-Consumer-Name` or query AuthzForce. Consumers that reach `data-provider`
directly (bypassing `rest-authz`) are implicitly authorized by the network topology; this
is intentional inside the Docker network.

---

## Key contracts

### X-Consumer-Name header

`rest-authz` reads consumer identity from the `X-Consumer-Name` HTTP request header
(fallback: `?consumer=` query parameter). The service name comes from `X-Service-Name`
(fallback: `DEFAULT_SERVICE` env var, default `telemetry-rest`).

This is **self-reported identity** — the consumer declares who it is. Trust is enforced
at the Docker network boundary, not by the Authentication service. Do not add
Authentication-service verification to this flow without reconsidering the architecture.

For every new REST consumer in experiment-7:
- Set `X-Consumer-Name` to the consumer's system name as registered in ConsumerAuthorization.
- Ensure a grant exists in ConsumerAuthorization before the consumer starts polling.

### Sync-delay caveat

REST enforcement lags ConsumerAuthorization by up to `SYNC_INTERVAL` (default 10 s).
After a grant is revoked in CA:
- AMQP: denied on the next RabbitMQ broker operation (immediate after sync).
- Kafka: `event: revoked` SSE message + re-check on next 100-message boundary (after sync).
- REST: continues to return `200 OK` until `policy-sync` uploads the next PolicySet version.

**Practical implication:** do not write tests that assert REST denial immediately after
revocation — add a `sleep` of at least `SYNC_INTERVAL` (the `test-system.sh` waits 30 s).

The sync interval is runtime-configurable via:
```
POST http://localhost:9095/config
{"syncInterval": "10s"}
```
Or through the dashboard Config tab, which calls the same endpoint via nginx.

### policy-sync /config endpoint

`policy-sync` exposes a `/config` endpoint that accepts `{"syncInterval": "Ns"}` (min 1 s).
Changes take effect on the next sleep iteration without restart. Use this in tests to reduce
the sync window when verifying revocation propagation.

---

## Service reference

| Service | Internal port | Exposed port | What it does |
|---|---|---|---|
| serviceregistry | 8080 | 8080 | Service registration |
| consumerauth | 8082 | 8082 | Grant source of truth |
| authzforce | 8080 (internal) | 8186 | XACML PDP/PAP |
| policy-sync | 9095 | — | CA → AuthzForce compiler; `/config` for SYNC_INTERVAL |
| topic-auth-xacml | 9090 | — | AMQP PEP |
| kafka-authz | 9091 | 9091 | Kafka SSE PEP; `/auth/check`, `/stream/{consumer}` |
| rest-authz | 9093 | 9093 | REST reverse-proxy PEP; `/auth/check`, `/telemetry/latest` |
| data-provider | 9094 | — | Kafka consumer + REST API; `/telemetry/latest`, `/stats` |
| rest-consumer | 9097 | — | REST polling client; `/health`, `/stats` |
| robot-fleet | 9003 | 9106→9003 | Dual AMQP+Kafka publisher |
| analytics-consumer | — | — | Kafka SSE subscriber |
| dashboard (nginx) | 80 | 3006 | React SPA + `/api/*` proxy |
| rabbitmq | 5672 / 15672 | 15676 | AMQP broker; management UI on 15676 |

**Key spec files for support services:**
- `rest-authz`: [`support/rest-authz/SPEC.md`](../../support/rest-authz/SPEC.md)
- `kafka-authz`: [`support/kafka-authz/SPEC.md`](../../support/kafka-authz/SPEC.md)
- `policy-sync`: [`support/policy-sync/SPEC.md`](../../support/policy-sync/SPEC.md)

Always read the relevant `SPEC.md` before writing TypeScript types, test assertions, or
dashboard API calls against a support service — never infer field names from code (EXP-009).

---

## Environment variables for experiment-6 Go services

### data-provider

| Variable | Default | Description |
|---|---|---|
| `KAFKA_BROKERS` | `kafka:9092` | Comma-separated Kafka addresses |
| `KAFKA_TOPIC` | `arrowhead.telemetry` | Topic to consume |
| `PORT` | `9094` | HTTP listen port |

### rest-consumer

| Variable | Default | Description |
|---|---|---|
| `CONSUMER_NAME` | `rest-consumer` | Value of `X-Consumer-Name` header sent to rest-authz |
| `REST_AUTHZ_URL` | `http://rest-authz:9093` | Base URL of the rest-authz proxy |
| `SERVICE` | `telemetry-rest` | Value of `X-Service-Name` header |
| `POLL_INTERVAL` | `2s` | How often to call rest-authz (min 1 s) |
| `HEALTH_PORT` | `9097` | Port for `/health` and `/stats` |

---

## Dashboard

### Shared files (symlinked from support/dashboard-shared/)

Ten source files in `dashboard/src/` are **symlinks** to `support/dashboard-shared/`:

```
main.tsx  hooks/usePolling.ts
config/context.tsx  config/defaults.ts  config/types.ts
components/StatusDot.tsx
views/HealthView.tsx  views/GrantsView.tsx  views/PolicyView.tsx  views/LiveDataView.tsx
```

**Always edit the canonical file in `support/dashboard-shared/`** — never edit the symlinks.
Run `bash support/dashboard-shared/check-dashboard-shared.sh` after any shared-file change to
verify all 20 symlinks are intact (10 for experiment-5, 10 for experiment-6).

The shared layer has a full test suite: `cd support/dashboard-shared && npm test` (41 tests).

### Experiment-6-specific files

These live only in `dashboard/src/` and are not shared:

| File | What it does |
|---|---|
| `App.tsx` | Root component; tab routing (Health, Grants, LiveData, Policy, Kafka, Rest, Config) |
| `api.ts` | All `fetch` calls to `/api/*`; defines response types and polling functions |
| `types.ts` | TypeScript types mirroring backend JSON shapes |
| `config/storage.ts` | localStorage load/save for `DashboardConfig` |
| `views/KafkaView.tsx` | Kafka transport tab (kafka-authz status + SSE stream) |
| `views/RestView.tsx` | REST transport tab (rest-authz status + rest-consumer stats) |
| `views/ConfigView.tsx` | Config tab (SYNC_INTERVAL control) |
| `components/ConsumerStatsPanel.tsx` | REST consumer stats display |
| `components/GrantsPanel.tsx` | Grant list from ConsumerAuth |
| `components/SystemHealthGrid.tsx` | Service health grid |

When adding a new view or component for experiment-7, follow the pattern in `KafkaView.tsx`
and `RestView.tsx` — they demonstrate how to use `usePolling` from the shared layer.

### Type-checking and tests

```bash
cd dashboard
npm run typecheck   # tsc --noEmit (fast, no Docker required)
npm test            # vitest
```

Type-check is also integrated into `test-system.sh` and runs before Docker pre-flight.

---

## Running the full test suite

```bash
cd experiments/experiment-6
docker compose up -d --build
bash test-system.sh
```

The script:
1. Type-checks the dashboard (`npm run typecheck`) — no Docker required.
2. Waits for nginx, AuthzForce, and all PEPs to be healthy.
3. Verifies `policy-sync` is synced to `arrowhead-exp6` before any auth tests (EXP-001).
4. Runs 11 sections covering AuthzForce endpoints, policy-sync /status and /config,
   kafka-authz, rest-authz, data access, consumer stats, and revocation propagation.

---

## Prohibitions

- Do NOT change `data-provider`'s Kafka reader to a consumer group — it must use a
  partition reader (EXP-007).
- Do NOT add Authorization-service verification to the REST path — identity is
  self-reported via `X-Consumer-Name`; the trust boundary is the Docker network.
- Do NOT hardcode `arrowhead-exp6` in new services — always read from `AUTHZFORCE_DOMAIN`
  env var. New experiments must use a unique domain name (e.g. `arrowhead-exp7`).
- Do NOT edit symlinked dashboard files directly — edit the canonical file in
  `support/dashboard-shared/`.
- Do NOT write tests that assert REST denial immediately after CA revocation — REST
  enforcement is delayed by up to `SYNC_INTERVAL` (sync-delay caveat).
- Do NOT modify any file under `core/` — add new endpoints to the core HTTP API instead.
