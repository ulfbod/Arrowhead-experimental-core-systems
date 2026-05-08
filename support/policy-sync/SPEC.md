# policy-sync — HTTP API Specification

`policy-sync` projects ConsumerAuthorization grants into AuthzForce XACML policies.
On startup it creates (or looks up) an AuthzForce domain, compiles the current grant
set from ConsumerAuth into a XACML 3.0 PolicySet, and uploads it to the AuthzForce
PAP. A background loop re-syncs every `SYNC_INTERVAL`. Used in experiments 5 and 6.

`/health` returns `503` until the first sync succeeds, making it safe to use as a
Docker `healthcheck` dependency for `topic-auth-xacml`, `kafka-authz`, and `rest-authz`.

---

## Environment Variables

| Variable | Default | Required | Description |
|---|---|:---:|---|
| `AUTHZFORCE_URL` | `http://authzforce:8080/authzforce-ce` | No | AuthzForce base URL |
| `AUTHZFORCE_DOMAIN` | `arrowhead-exp5` | No | AuthzForce domain `externalId`; must match the domain used by all PEP services |
| `CONSUMERAUTH_URL` | `http://consumerauth:8082` | No | ConsumerAuthorization base URL |
| `SYNC_INTERVAL` | `30s` | No | Reconciliation period; also configurable at runtime via `POST /config` |
| `PORT` | `9095` | No | HTTP port |
| `AUTH_URL` | — | No | If set, policy-sync logs in to the Authentication service and carries a Bearer token on ConsumerAuth calls |
| `SYSTEM_NAME` | `policy-sync` | No | System name for Authentication login |
| `SYSTEM_CREDENTIALS` | `sync-secret` | No | Credentials for Authentication login |

**Critical:** `AUTHZFORCE_DOMAIN` must be identical across policy-sync and all PEP
services (`topic-auth-xacml`, `kafka-authz`, `rest-authz`) in the same experiment
stack. A mismatch causes all PEP decisions to return `Deny` (see `EXPERIENCES.md`
EXP-001).

---

## Endpoints

### `GET /health`

Readiness/liveness probe.

**Response `200 OK`** — first sync has completed; PEP services may start accepting requests
```json
{"status":"ok"}
```

**Response `503 Service Unavailable`** — still initialising or syncing
```json
{"status":"syncing"}
```

---

### `GET /status`

Current sync state. Use this to verify that the running process is using the correct
domain before investigating authorization failures.

**Response `200 OK`**
```json
{
  "synced":           true,
  "version":          3,
  "domainId":         "abc123-internal-id",
  "domainExternalId": "arrowhead-exp6",
  "grants":           7,
  "lastSyncedAt":     "2026-05-07T12:34:56Z",
  "syncInterval":     "30s"
}
```

| Field | Type | Description |
|---|---|---|
| `synced` | boolean | `true` after the first successful sync |
| `version` | integer | Policy version counter; increments on every sync call |
| `domainId` | string | AuthzForce internal domain UUID |
| `domainExternalId` | string | AuthzForce domain `externalId` (from `AUTHZFORCE_DOMAIN`). **Always verify this matches the experiment before investigating PEP failures.** |
| `grants` | integer | Number of grants compiled into the current PolicySet |
| `lastSyncedAt` | string (RFC3339) | UTC timestamp of the last successful sync; empty string before first sync |
| `syncInterval` | string | Current effective sync interval (may differ from `SYNC_INTERVAL` if updated via `POST /config`) |

---

### `GET /config`

Returns the current runtime sync interval.

**Response `200 OK`**
```json
{"syncInterval":"30s"}
```

| Field | Type | Description |
|---|---|---|
| `syncInterval` | string | Current effective sync interval as a Go duration string (e.g. `"30s"`, `"1m"`, `"5s"`) |

---

### `POST /config`

Updates the sync interval at runtime without restarting the process. The change
takes effect at the start of the next sync sleep (the current sleep is not
interrupted).

**Request body**
```json
{"syncInterval": "5s"}
```

| Field | Type | Required | Constraints | Description |
|---|---|:---:|---|---|
| `syncInterval` | string | **Yes** | Valid Go duration, ≥ `1s` | New reconciliation period |

**Valid duration formats:** `"5s"`, `"30s"`, `"1m"`, `"2m30s"`, etc.

**Response `200 OK`** — updated interval echoed back
```json
{"syncInterval":"5s"}
```

**Error responses**

| Status | Condition |
|---|---|
| `400 Bad Request` | Invalid JSON, invalid duration format, or duration < 1s |
| `405 Method Not Allowed` | Non-GET/POST method |
