# Support — Shared Services and Libraries

This folder contains reusable support modules for the ArrowheadCore experiments.
They are not part of the Arrowhead Core itself; instead they are enforcement
adapters, policy infrastructure, and shared Go libraries that experiment services
depend on.

---

## Module overview

| Module | Kind | Port | Used in |
|---|---|---|---|
| `authzforce` | Go library | — | topic-auth-xacml · kafka-authz · policy-sync · rest-authz |
| `authzforce-server` | Service | 8080 | experiment-5 · experiment-6 |
| `kafka-authz` | Service | 9091 | experiment-5 · experiment-6 |
| `message-broker` | Go library | — | experiment services (robot-fleet · consumers) |
| `policy-sync` | Service | 9095 | experiment-5 · experiment-6 |
| `rest-authz` | Service | 9093 | experiment-6 |
| `topic-auth-http` | Service | 9090 | experiment-4 |
| `topic-auth-sync` | Service | 9090 | — (alternative design, not wired) |
| `topic-auth-xacml` | Service | 9090 | experiment-5 · experiment-6 |

---

## Libraries

### `authzforce`

Go client package for the AuthzForce CE REST API (XACML REST Profile, OASIS).
All communication is XML. Exported API:

| Symbol | What it does |
|---|---|
| `New(baseURL)` | Returns a `*Client` pointing at an AuthzForce instance |
| `Client.EnsureDomain(externalID)` | Creates or looks up a policy domain; returns domain UUID |
| `Client.SetPolicy(domainID, xml, id, ver)` | Uploads a XACML 3.0 PolicySet and sets it as root |
| `Client.Decide(domainID, subject, resource, action)` | Evaluates a XACML request; returns `true` for Permit |
| `BuildPolicy(policySetID, version, grants)` | Compiles `[]Grant` into a XACML 3.0 PolicySet XML |

The client is imported by `topic-auth-xacml`, `kafka-authz`, and `policy-sync`.

---

### `message-broker`

Go package providing a publish/subscribe abstraction over a RabbitMQ topic
exchange. Experiment services import this package and supply a concrete AMQP URL
at startup. Core systems must not import it.

| Symbol | What it does |
|---|---|
| `New(Config)` | Dials RabbitMQ and declares a durable topic exchange |
| `Broker.Publish(routingKey, payload)` | Publishes a persistent message |
| `Broker.Subscribe(queue, bindingKey, handler)` | Declares a durable queue, binds it, delivers to handler goroutine |
| `Broker.Done()` | Channel closed when the AMQP connection drops |
| `Broker.Close()` | Releases channel and connection |

---

## RabbitMQ auth adapters

All three `topic-auth-*` services implement the RabbitMQ HTTP auth backend
protocol on the same four endpoints:

```
POST /auth/user      — credential check → "allow [tags]" or "deny"
POST /auth/vhost     — vhost access check → "allow" or "deny"
POST /auth/resource  — exchange/queue ops → always "allow"
POST /auth/topic     — routing-key enforcement → "allow" or "deny"
```

They differ in **where** authorization decisions come from.

---

### `topic-auth-http`

**Live ConsumerAuth check — used in experiment-4.**

RabbitMQ is configured with a single HTTP auth backend
(`auth_backends.1 = rabbit_auth_backend_http`) that points at this service.
Every broker operation (connect, vhost, bind, publish) triggers a real-time
query to ConsumerAuthorization (`GET /authorization/lookup`). A revoked grant
takes effect on the consumer's next broker operation — no polling delay.

Optional revocation loop: if `RABBITMQ_MGMT_URL` is set, a background goroutine
calls the RabbitMQ management API every `REVOCATION_INTERVAL` (default 15 s) and
force-closes AMQP connections belonging to consumers whose grant has been revoked.

Decision cache: `CACHE_TTL` (default 0 s = no caching) can reduce ConsumerAuth
traffic at the cost of delayed revocation within the TTL window.

Key environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `CONSUMERAUTH_URL` | `http://localhost:8082` | ConsumerAuth base URL |
| `AUTH_URL` | — | If set, logs in and carries Bearer token on CA calls |
| `RABBITMQ_MGMT_URL` | — | If set, enables the active revocation loop |
| `CACHE_TTL` | `0s` | Decision cache TTL |
| `REVOCATION_INTERVAL` | `15s` | How often the revocation loop runs |
| `PORT` | `9090` | HTTP port |

---

### `topic-auth-sync`

**Polling sync into RabbitMQ — alternative design, not used in any experiment.**

Instead of a live HTTP auth backend, this service periodically polls
ConsumerAuthorization (`GET /authorization/lookup`) and reconciles RabbitMQ
users and topic permissions via the RabbitMQ management API. Authorization
decisions are therefore evaluated by RabbitMQ's internal engine against the
pre-configured permissions.

Revocation latency equals `SYNC_INTERVAL` (default 10 s). Stale users
(consumers whose grant was removed) are deleted from RabbitMQ on the next sync.
Health endpoint returns 200 only after the first sync succeeds.

Key environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `CONSUMERAUTH_URL` | `http://localhost:8082` | ConsumerAuth base URL |
| `RABBITMQ_URL` | `http://localhost:15672` | RabbitMQ management URL |
| `RABBITMQ_EXCHANGE` | `arrowhead` | Topic exchange name |
| `SYNC_INTERVAL` | `10s` | Reconciliation period |
| `AUTH_URL` | — | If set, logs in and carries Bearer token on CA calls |
| `PORT` | `9090` | Health HTTP port |

---

### `topic-auth-xacml`

**AuthzForce XACML delegation — used in experiment-5.**

Same wire protocol as `topic-auth-http` but authorization decisions are
delegated to an AuthzForce XACML PDP instead of ConsumerAuthorization. This
makes the service a pure Policy Enforcement Point (PEP) with no direct CA
dependency.

On startup it resolves the AuthzForce domain (created by `policy-sync`) and
retries until the domain is available. An optional decision cache (`CACHE_TTL`)
reduces PDP traffic. A proactive revocation loop closes AMQP connections for
consumers whose XACML grant was removed, polled every `REVOCATION_INTERVAL`.

Key environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `AUTHZFORCE_URL` | `http://authzforce:8080/authzforce-ce` | AuthzForce base URL |
| `AUTHZFORCE_DOMAIN` | `arrowhead-exp5` | AuthzForce domain external ID |
| `RABBITMQ_MGMT_URL` | `http://rabbitmq:15672` | Management URL for revocation loop |
| `CACHE_TTL` | `0s` | XACML decision cache TTL |
| `REVOCATION_INTERVAL` | `15s` | Revocation loop period |
| `PORT` | `9090` | HTTP port |

---

## XACML policy infrastructure

### `authzforce-server`

Lightweight Go implementation of the AuthzForce CE REST API subset used by
the `authzforce` client. Intended as a drop-in replacement for the full Java
AuthzForce CE server in experiments where simplicity is preferred over
production fidelity.

Implemented endpoints:

| Endpoint | Method | Purpose |
|---|---|---|
| `/authzforce-ce/domains` | GET | Find domain by `?externalId=` |
| `/authzforce-ce/domains` | POST | Create domain |
| `/authzforce-ce/domains/{id}/pap/policies` | PUT | Upload XACML PolicySet |
| `/authzforce-ce/domains/{id}/pap/pdp.properties` | PUT | Set root policy ref (no-op) |
| `/authzforce-ce/domains/{id}/pdp` | POST | Evaluate XACML Request |
| `/health` | GET | Liveness probe |

The PDP evaluates grants by parsing `PolicyId` attributes with the pattern
`urn:arrowhead:grant:{consumer}:{service}` — the format produced by
`authzforce.BuildPolicy`. A request is Permitted if and only if the
(subject, resource) pair matches a stored grant.

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP port |

---

### `policy-sync`

Projects ConsumerAuthorization grants into AuthzForce XACML policies.

On startup it creates (or looks up) an AuthzForce domain, compiles the current
CA grant set into a XACML 3.0 PolicySet (via `authzforce.BuildPolicy`), and
pushes it to the AuthzForce PAP. A background loop re-syncs every
`SYNC_INTERVAL`. The version counter increments on every push so policy
versions are traceable.

Health endpoint (`/health`) returns 200 only after the first successful sync,
making it safe to use as a Docker healthcheck dependency for `topic-auth-xacml`,
`kafka-authz`, and `rest-authz`. A `/status` endpoint reports domain ID, version,
grant count, last sync timestamp, and current `syncInterval`.

A `/config` endpoint allows runtime-configurable `SYNC_INTERVAL` without restart
(used by the experiment-6 dashboard Config tab):

```
POST /config  {"syncInterval":"15s"}   → {"syncInterval":"15s"}
GET  /config                           → {"syncInterval":"15s"}
```

Minimum interval is 1 s. The new interval takes effect at the start of the next
sleep; the current sleep is not interrupted. Uses `atomic.Int64` for thread safety.

Key environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `AUTHZFORCE_URL` | `http://authzforce:8080/authzforce-ce` | AuthzForce base URL |
| `AUTHZFORCE_DOMAIN` | `arrowhead-exp5` | AuthzForce domain externalId — **must match the domain used by all PEPs** |
| `CONSUMERAUTH_URL` | `http://consumerauth:8082` | ConsumerAuth base URL |
| `SYNC_INTERVAL` | `30s` | Initial reconciliation period (runtime-configurable via /config) |
| `AUTH_URL` | — | If set, logs in and carries Bearer token on CA calls |
| `PORT` | `9095` | Health/status HTTP port |

---

## REST enforcement

### `rest-authz`

HTTP reverse proxy Policy Enforcement Point backed by AuthzForce XACML — used in experiment-6.

Acts as a transparent reverse proxy. Every proxied request must carry an `X-Consumer-Name`
header (or `?consumer=` query parameter). `rest-authz` queries AuthzForce for the triple
`(consumer, service, "invoke")`; if the decision is Permit it forwards the request to
`UPSTREAM_URL`, otherwise it returns 403 Forbidden.

This is the third PEP in the unified policy projection model of experiment-6: it shares
the same AuthzForce domain with `topic-auth-xacml` (AMQP) and `kafka-authz` (Kafka), so
a single grant in ConsumerAuthorization simultaneously authorises a consumer on all three
transports.

**Sync-delay caveat:** REST enforcement lags ConsumerAuthorization by up to `SYNC_INTERVAL`
(the policy-sync period). A revoked grant continues to produce Permit decisions until
policy-sync uploads the next PolicySet version to AuthzForce.

Endpoints:

| Endpoint | Method | Purpose |
|---|---|---|
| `/health` | GET | Liveness probe |
| `/status` | GET | Request counters (total, permitted, denied) |
| `/auth/check` | POST | Explicit AuthzForce decision for `{"consumer":"…","service":"…"}` |
| `/*` | ALL | PEP: check AuthzForce, proxy to upstream if Permit |

Key environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `AUTHZFORCE_URL` | `http://authzforce:8080/authzforce-ce` | AuthzForce base URL |
| `AUTHZFORCE_DOMAIN` | `arrowhead-exp6` | AuthzForce domain external ID |
| `UPSTREAM_URL` | (required) | Upstream service base URL, no trailing slash |
| `DEFAULT_SERVICE` | `telemetry-rest` | Service name when `X-Service-Name` is absent |
| `CACHE_TTL` | `0s` | Decision cache TTL; 0 = no caching |
| `PORT` | `9093` | HTTP port |

---

## Kafka enforcement

### `kafka-authz`

Kafka SSE proxy with AuthzForce enforcement — used in experiment-5.

Consumers connect to `GET /stream/{consumerName}?service=<service>`. The
handler queries AuthzForce; if the decision is Permit it subscribes to the
corresponding Kafka topic (`arrowhead.<service>`) and forwards messages as
Server-Sent Events. If Deny it returns 403 immediately.

Authorization is re-checked every 100 messages to detect mid-stream
revocation. On revocation an SSE `revoked` event is sent and the stream is
closed.

Endpoints:

| Endpoint | Method | Purpose |
|---|---|---|
| `/health` | GET | Liveness probe |
| `/status` | GET | Active stream count and total served |
| `/stream/{consumerName}` | GET | SSE stream (authorized) |
| `/auth/check` | POST | Explicit AuthzForce decision (for dashboard) |

Key environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `KAFKA_BROKERS` | `kafka:9092` | Comma-separated broker list |
| `AUTHZFORCE_URL` | `http://authzforce:8080/authzforce-ce` | AuthzForce base URL |
| `AUTHZFORCE_DOMAIN` | `arrowhead-exp5` | AuthzForce domain external ID |
| `PORT` | `9091` | HTTP port |
