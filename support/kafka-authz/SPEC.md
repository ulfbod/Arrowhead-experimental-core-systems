# kafka-authz — HTTP API Specification

`kafka-authz` is a Kafka SSE (Server-Sent Events) proxy PEP. Consumers connect to
it rather than Kafka directly. On connection it queries AuthzForce; if the decision
is `Permit` it subscribes to the corresponding Kafka topic and streams messages as
SSE events. Used in experiments 5 and 6.

---

## Environment Variables

| Variable | Default | Required | Description |
|---|---|:---:|---|
| `KAFKA_BROKERS` | `kafka:9092` | No | Comma-separated broker list |
| `AUTHZFORCE_URL` | `http://authzforce:8080/authzforce-ce` | No | AuthzForce base URL |
| `AUTHZFORCE_DOMAIN` | `arrowhead-exp5` | No | AuthzForce domain `externalId` |
| `PORT` | `9091` | No | HTTP port |

---

## Authorization Model

Each `/stream/{consumerName}` connection is evaluated as:

| Field | Source |
|---|---|
| `subject` (consumer) | `{consumerName}` path segment |
| `resource` (service) | `?service=` query param, default `"telemetry"` |
| `action` | hardcoded `"subscribe"` |

If the initial decision is `Permit`, authorization is re-checked every 100 messages.
If the re-check returns `Deny`, a `event: revoked` SSE event is sent and the stream
is closed.

**Topic mapping:** `service` → Kafka topic `arrowhead.<service>`
(e.g. `service=telemetry` → topic `arrowhead.telemetry`).

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

Current stream counts.

**Response `200 OK`**
```json
{
  "activeStreams": 2,
  "totalServed":  1480
}
```

| Field | Type | Description |
|---|---|---|
| `activeStreams` | integer | Number of currently open SSE connections across all consumers |
| `totalServed` | integer | Total Kafka messages forwarded since startup |

---

### `GET /stream/{consumerName}`

SSE stream of Kafka messages for an authorized consumer.

**Path parameter**

| Parameter | Description |
|---|---|
| `consumerName` | Consumer system name; used as the XACML subject |

**Query parameter**

| Parameter | Default | Description |
|---|---|---|
| `service` | `telemetry` | Service definition; used as the XACML resource and to derive the Kafka topic (`arrowhead.<service>`) |

**Response `200 OK`** — SSE stream (`Content-Type: text/event-stream`)

Messages are emitted as SSE `data` events. The connection is kept alive with SSE
comment keepalives (`: keepalive`) every 5 seconds.

```
data: {"robotId":"robot-1","temperature":22.4,"timestamp":"..."}

data: {"robotId":"robot-2","temperature":19.1,"timestamp":"..."}

: keepalive

```

**Mid-stream revocation event** — emitted if AuthzForce returns `Deny` on a
periodic re-check (every 100 messages), then the stream closes:
```
event: revoked
data: {"reason":"grant revoked"}

```

**Kafka read error event** — emitted on a Kafka read failure, then the stream closes:
```
event: error
data: {"error":"<error message>"}

```

**Error responses**

| Status | Body | Condition |
|---|---|---|
| `400 Bad Request` | plain text | `consumerName` path segment is empty |
| `403 Forbidden` | `{"error":"not authorized"}` | AuthzForce returns `Deny` |
| `405 Method Not Allowed` | plain text | Non-GET request |
| `503 Service Unavailable` | plain text | AuthzForce unreachable |

---

### `POST /auth/check`

Explicit authorization check against AuthzForce. Used by the dashboard and test
scripts. Does **not** open a Kafka stream.

**Request body**
```json
{"consumer": "analytics-consumer", "service": "telemetry"}
```

| Field | Type | Required | Description |
|---|---|:---:|---|
| `consumer` | string | **Yes** | Consumer system name |
| `service` | string | **Yes** | Service definition to check |

**Response `200 OK`**
```json
{
  "consumer": "analytics-consumer",
  "service":  "telemetry",
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
