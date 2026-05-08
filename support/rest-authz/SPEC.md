# rest-authz — HTTP API Specification

`rest-authz` is a reverse-proxy PEP (Policy Enforcement Point) that sits in front of
an upstream REST service and enforces authorization by querying AuthzForce on every
request. Used in experiment-6.

---

## Environment Variables

| Variable | Default | Required | Description |
|---|---|:---:|---|
| `UPSTREAM_URL` | — | **Yes** | Upstream service base URL, no trailing slash |
| `AUTHZFORCE_URL` | `http://authzforce:8080/authzforce-ce` | No | AuthzForce base URL |
| `AUTHZFORCE_DOMAIN` | `arrowhead-exp6` | No | AuthzForce domain `externalId` |
| `DEFAULT_SERVICE` | `telemetry-rest` | No | Service name used when `X-Service-Name` is absent |
| `CACHE_TTL` | `0s` | No | Decision cache TTL; `0s` = no caching |
| `PORT` | `9093` | No | HTTP port |

---

## Authorization Model

Every proxied request is evaluated as the triple:

| Field | Source |
|---|---|
| `subject` (consumer) | `X-Consumer-Name` request header, or `?consumer=` query param |
| `resource` (service) | `X-Service-Name` request header, or `DEFAULT_SERVICE` env var |
| `action` | hardcoded `"invoke"` |

AuthzForce returns `Permit` or `Deny`. On `Permit` the request is forwarded to
`UPSTREAM_URL`. On `Deny` a `403` is returned.

**Sync-delay caveat:** enforcement lags ConsumerAuth by up to `SYNC_INTERVAL`
(policy-sync's reconciliation period). A revoked grant continues to produce `Permit`
until policy-sync uploads the next PolicySet version.

---

## Endpoints

### `GET /health`

Liveness probe. Always returns `200` once the process is running.

**Response `200 OK`**
```json
{"status":"ok"}
```

---

### `GET /status`

Cumulative request counters since startup.

**Response `200 OK`**
```json
{
  "requestsTotal": 42,
  "permitted":     38,
  "denied":         4
}
```

| Field | Type | Description |
|---|---|---|
| `requestsTotal` | integer | Total proxy requests evaluated |
| `permitted` | integer | Requests that received `Permit` |
| `denied` | integer | Requests that received `Deny` |

---

### `POST /auth/check`

Explicit authorization check against AuthzForce. Used by the dashboard and
test scripts. Does **not** proxy to the upstream.

**Request body**
```json
{"consumer": "demo-consumer-1", "service": "telemetry-rest"}
```

| Field | Type | Required | Description |
|---|---|:---:|---|
| `consumer` | string | **Yes** | Consumer system name |
| `service` | string | **Yes** | Service definition to check |

**Response `200 OK`** — decision returned
```json
{
  "consumer": "demo-consumer-1",
  "service":  "telemetry-rest",
  "permit":   true,
  "decision": "Permit"
}
```

| Field | Type | Description |
|---|---|---|
| `consumer` | string | Echo of request `consumer` |
| `service` | string | Echo of request `service` |
| `permit` | boolean | `true` = Permit, `false` = Deny |
| `decision` | string | `"Permit"` or `"Deny"` |

**Error responses**

| Status | Condition |
|---|---|
| `400 Bad Request` | Missing or invalid JSON, or `consumer`/`service` empty |
| `405 Method Not Allowed` | Non-POST request |
| `503 Service Unavailable` | AuthzForce unreachable |

---

### `* /*` — PEP Proxy (all other paths)

All requests to any other path are evaluated as authorization decisions and, if
permitted, reverse-proxied to `UPSTREAM_URL`.

**Required request header**
```
X-Consumer-Name: <consumer-system-name>
```
Alternatively, pass `?consumer=<name>` as a query parameter.

**Optional request header**
```
X-Service-Name: <service-definition>
```
Defaults to `DEFAULT_SERVICE` env var if absent. Both headers are stripped before
forwarding to the upstream.

**Response on Permit** — upstream response forwarded verbatim (status, headers, body)

**Response on Deny — `403 Forbidden`**
```json
{"error":"not authorized"}
```

**Error responses**

| Status | Body | Condition |
|---|---|---|
| `401 Unauthorized` | `{"error":"X-Consumer-Name header required"}` | No consumer identity |
| `403 Forbidden` | `{"error":"not authorized"}` | AuthzForce returns `Deny` |
| `502 Bad Gateway` | `{"error":"upstream unavailable"}` | Upstream unreachable |
| `503 Service Unavailable` | `{"error":"PDP unavailable"}` | AuthzForce unreachable |
