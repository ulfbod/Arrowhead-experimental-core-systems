# Examples

---

## Example 1: Full Registration

```json
{
  "serviceDefinition": "temperature-service",
  "providerSystem": {
    "systemName": "sensor-1",
    "address": "192.168.0.10",
    "port": 8080
  },
  "serviceUri": "/temperature",
  "interfaces": ["HTTP-SECURE-JSON"],
  "version": 1,
  "metadata": {
    "region": "eu",
    "unit": "celsius"
  },
  "secure": "NOT_SECURE"
}
```

---

## Example 2: Authentication — Login with structured credentials

Request to `POST /authentication/identity/login`:

```json
{
  "systemName": "sensor-1",
  "credentials": { "password": "s3cr3t" }
}
```

`credentials` must be a JSON object with a `password` key.
A plain string (`"credentials":"s3cr3t"`), `null`, or an object missing `password`
returns `400 Bad Request`.

Response `201 Created`:

```json
{
  "token":          "3a8f2c1d-...",
  "systemName":     "sensor-1",
  "expirationTime": "2026-05-30T10:00:00Z",
  "sysop":          false
}
```

---

## Example 3: Orchestration response — all Phase 1 fields

Response to `POST /serviceorchestration/orchestration/pull` (success, provider not locked):

```json
{
  "results": [
    {
      "providerName":        "sensor-1",
      "serviceDefinitition": "temperature-service",
      "cloudIdentitifer":    "LOCAL",
      "serviceInstanceId":   "sensor-1::temperature-service::1.0.0",
      "serviceUri":          "/temperature",
      "interfaces":          ["HTTP-SECURE-JSON"],
      "aliveUntil":          "2026-05-30T12:00:00Z"
    }
  ]
}
```

When the provider holds an active orchestration lock, `exclusiveUntil` is included:

```json
{
  "results": [
    {
      "providerName":        "sensor-1",
      "serviceDefinitition": "temperature-service",
      "cloudIdentitifer":    "LOCAL",
      "serviceInstanceId":   "sensor-1::temperature-service::1.0.0",
      "serviceUri":          "/temperature",
      "interfaces":          ["HTTP-SECURE-JSON"],
      "exclusiveUntil":      "2026-05-30T11:30:00Z",
      "aliveUntil":          "2026-05-30T12:00:00Z"
    }
  ]
}
```

Notes:
- `cloudIdentitifer` is always `"LOCAL"` (spec typo — missing 'n' — intentional).
- `serviceDefinitition` has a double 't' — also intentional spec typo.
- `interfaces` is forwarded from the ServiceRegistry response.
- `exclusiveUntil` is omitted when the provider has no active lock.

---

## Example 4: Intercloud flags — 501 response

Request with `ALLOW_INTERCLOUD: true`:

```json
{
  "requesterSystem":    { "systemName": "consumer", "address": "localhost", "port": 0 },
  "serviceRequirement": { "serviceDefinition": "temperature-service" },
  "orchestrationFlags": { "ALLOW_INTERCLOUD": true }
}
```

Response `501 Not Implemented`:

```json
{
  "errorMessage":  "intercloud orchestration is not supported",
  "errorCode":     501,
  "exceptionType": "NOT_IMPLEMENTED",
  "origin":        "dynamicorch"
}
```

Same applies to `ONLY_INTERCLOUD: true`. Both DynamicOrchestration and
SimpleStoreOrchestration return 501 for these flags.

---

## Example 5: Blacklist — discovery with Bearer, mgmt/query with mode

`GET /blacklist/lookup` when `BLACKLIST_AUTH_URL` is configured — no token:

Response `401 Unauthorized`:

```json
{
  "errorMessage":  "Authorization: Bearer token required",
  "errorCode":     401,
  "exceptionType": "AUTH_EXCEPTION",
  "origin":        "blacklist"
}
```

`POST /blacklist/mgmt/query` with mode enum:

```json
{ "mode": "ACTIVES" }
```

Returns only entries where `active: true`. Valid values: `"ALL"`, `"ACTIVES"`, `"INACTIVES"`.

`POST /blacklist/mgmt/query` with invalid mode:

```json
{ "mode": "BOGUS" }
```

Response `400 Bad Request`:

```json
{
  "errorMessage":  "invalid mode: must be ACTIVES, INACTIVES, or ALL",
  "errorCode":     400,
  "exceptionType": "INVALID_PARAMETER",
  "origin":        "blacklist"
}
```

---

## Example 6: Paginated service list

`POST /serviceregistry/mgmt/systems/query` with pagination:

```json
{
  "pagination": { "pageNumber": 0, "pageSize": 2 }
}
```

Response `200 OK`:

```json
{
  "systems": [
    { "systemName": "sensor-1", "address": "10.0.0.1", "port": 8090 },
    { "systemName": "sensor-2", "address": "10.0.0.2", "port": 8090 }
  ],
  "count": 2,
  "totalCount": 5
}
```

`count` = page size; `totalCount` = full collection. Zero `pageSize` returns all results.

---

## Example 7: Bulk grant-policies request and response

`POST /consumerauthorization/authorization/mgmt/grant-policies` (requires sysop Bearer):

```json
{
  "policies": [
    {
      "provider": "SensorProvider",
      "targetType": "SERVICE_DEF",
      "target": "temperatureService",
      "defaultPolicy": { "policyType": "ALL" }
    },
    {
      "provider": "SensorProvider",
      "targetType": "SERVICE_DEF",
      "target": "temperatureService",
      "defaultPolicy": { "policyType": "ALL" }
    }
  ]
}
```

Response `200 OK` (second entry is a duplicate):

```json
{
  "results": [
    {
      "instanceId": "PR|LOCAL|SensorProvider|SERVICE_DEF|temperatureService",
      "policy": { "instanceId": "PR|LOCAL|...", "provider": "SensorProvider", "... ": "..." }
    },
    { "error": "authorization policy already exists" }
  ],
  "count": 2
}
```

---

## Example 8: Bulk check-policies mixed result

`POST /consumerauthorization/authorization/mgmt/check-policies` (requires sysop Bearer):

```json
[
  { "consumer": "ConsumerApp", "provider": "SensorProvider", "target": "temperatureService", "targetType": "SERVICE_DEF" },
  { "consumer": "ConsumerApp", "provider": "OtherProvider", "target": "unknownService", "targetType": "SERVICE_DEF" }
]
```

Response `200 OK`:

```json
{
  "results": [
    { "consumer": "ConsumerApp", "provider": "SensorProvider", "target": "temperatureService", "targetType": "SERVICE_DEF", "authorized": true },
    { "consumer": "ConsumerApp", "provider": "OtherProvider",  "target": "unknownService",     "targetType": "SERVICE_DEF", "authorized": false }
  ],
  "count": 2
}
```

---

## Example 9: Push trigger — delivery confirmed in history

**Step 1: Subscribe with a notifyUri**

`POST /serviceorchestration/orchestration/mgmt/push/subscribe` (requires sysop Bearer):

```json
{
  "ownerSystemName": "ConsumerApp",
  "targetSystemName": "temperatureService",
  "notifyInterface": { "notifyUri": "http://consumer-app:9090/push-callback" }
}
```

Response `201 Created`:
```json
{ "id": "sub-uuid-...", "ownerSystemName": "ConsumerApp", ... }
```

**Step 2: Trigger push**

`POST /serviceorchestration/orchestration/mgmt/push/trigger`:

```json
{ "subscriptionId": "sub-uuid-..." }
```

Response `200 OK` (immediate, before delivery completes):
```json
{ "status": "triggered", "subscriptionId": "sub-uuid-..." }
```

The orchestrator launches a goroutine that POSTs to `http://consumer-app:9090/push-callback`. History is updated to `PUSH/DELIVERED` or `PUSH/FAILED`.

**Step 3: Verify delivery in history**

`POST /serviceorchestration/orchestration/mgmt/history/query`:

```json
{}
```

Response includes:
```json
{
  "entries": [
    {
      "id": "...",
      "status": "DELIVERED",
      "type": "PUSH",
      "requesterSystem": "ConsumerApp",
      "message": "triggered for subscription sub-uuid-..."
    }
  ],
  "count": 1
}
```

---

## Example 10: USAGE_LIMITED token generation and exhaustion

Generate a token with a fixed usage count:

`POST /consumerauthorization/authorization-token/generate` (requires sysop Bearer):

```json
{
  "tokenVariant": "USAGE_LIMITED_TOKEN",
  "provider": "SensorProvider",
  "targetType": "SERVICE_DEF",
  "target": "temperatureService",
  "consumer": "ConsumerApp",
  "maxUsageCount": 2
}
```

Response `201 Created`:

```json
{
  "tokenType": "USAGE_LIMITED_TOKEN",
  "targetType": "SERVICE_DEF",
  "token": "9f4e3b2a1d0c...",
  "expiresAt": ""
}
```

Verify the token (first call): `GET /consumerauthorization/authorization-token/verify/9f4e3b2a1d0c...` → `200 OK`

Verify the token (second call): → `200 OK`

Verify the token (third call — exhausted): → `403 Forbidden`

---

## Example 11: BASE64_SELF_CONTAINED token — self-verifiable without stored state

Generate a self-contained token (no server state required for verification):

`POST /consumerauthorization/authorization-token/generate` (requires sysop Bearer):

```json
{
  "tokenVariant": "BASE64_SELF_CONTAINED",
  "provider": "SensorProvider",
  "targetType": "SERVICE_DEF",
  "target": "temperatureService",
  "consumer": "ConsumerApp"
}
```

Response `201 Created`:

```json
{
  "tokenType": "BASE64_SELF_CONTAINED",
  "targetType": "SERVICE_DEF",
  "token": "eyJwcm92aWRlciI6Ii4uLiJ9.a3f9c2b1...",
  "expiresAt": ""
}
```

The token is a `<base64-payload>.<hex-hmac>` string. Verification decodes and checks the HMAC using `HMAC_SECRET`; no database lookup is needed. The token can be verified on any instance of ConsumerAuthorization sharing the same `HMAC_SECRET`.

---

## Example 12: Orchestration request with qualityRequirements

Filter orchestration candidates by maximum acceptable latency:

`POST /serviceorchestration/orchestration/pull` (or the AH5 path):

```json
{
  "requesterSystem": { "systemName": "ConsumerApp", "address": "consumer-host", "port": 9090 },
  "serviceRequirement": { "serviceDefinition": "temperature-service" },
  "qualityRequirements": [
    { "maxLatencyMs": 50 }
  ]
}
```

DynamicOrchestration calls `POST /deviceqosevaluator/quality-evaluation/measure` for each
candidate. Candidates with `latencyMs > 50` or `reachable: false` are excluded.
Fail-open: if the QoS Evaluator is unreachable, all candidates are included.

Response `200 OK` (only fast providers):

```json
{
  "results": [
    {
      "providerName": "fast-sensor",
      "serviceDefinitition": "temperature-service",
      "cloudIdentitifer": "LOCAL"
    }
  ]
}
```

`QOS_EVALUATOR_URL` must be set to enable QoS filtering (e.g., `http://localhost:8088`).

---

## Example 13: JSON field-remapping bridge in Translation Manager

**Step 1: Create a bridge**

`POST /translationmanager/translation/mgmt/bridges` (requires sysop Bearer):

```json
{
  "sourceFormat": "sensor-v1",
  "targetFormat": "sensor-v2",
  "fieldMappings": {
    "temperature": "temp_celsius",
    "humidity":    "rel_humidity"
  }
}
```

Response `201 Created`:

```json
{
  "id": "bridge-uuid-...",
  "sourceFormat": "sensor-v1",
  "targetFormat": "sensor-v2",
  "fieldMappings": { "temperature": "temp_celsius", "humidity": "rel_humidity" },
  "active": true,
  "createdAt": "2026-05-31T10:00:00Z"
}
```

**Step 2: Translate a payload**

`POST /translationmanager/translation/translate`:

```json
{
  "bridgeId": "bridge-uuid-...",
  "payload": { "temperature": 22.5, "humidity": 61.0 }
}
```

Response `200 OK`:

```json
{
  "bridgeId": "bridge-uuid-...",
  "originalPayload":   { "temperature": 22.5, "humidity": 61.0 },
  "translatedPayload": { "temp_celsius": 22.5, "rel_humidity": 61.0 }
}
```
