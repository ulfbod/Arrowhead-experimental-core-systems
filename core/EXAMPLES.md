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
