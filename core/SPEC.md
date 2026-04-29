# Arrowhead Core Systems Specification

## Overview

This specification covers the six Arrowhead 5 (AH5) core systems implemented in this repository:

| System | Port | Path prefix |
|---|---|---|
| ServiceRegistry | 8080 | `/serviceregistry` |
| Authentication | 8081 | `/authentication` |
| ConsumerAuthorization | 8082 | `/authorization` |
| DynamicOrchestration | 8083 | `/orchestration/dynamic` |
| SimpleStoreOrchestration | 8084 | `/orchestration/simplestore` |
| FlexibleStoreOrchestration | 8085 | `/orchestration/flexiblestore` |

All systems expose a `GET /health` endpoint returning `{"status":"ok","system":"<name>"}`.

---

## Shared Types

### System

```json
{
  "systemName": "string (required, non-empty)",
  "address":    "string (required, non-empty)",
  "port":       0
}
```

Optional on ServiceRegistry System:
- `authenticationInfo` (string)

---

## 1. ServiceRegistry — Port 8080

Manages service provider registrations and discovery.

### Service Instance

```json
{
  "id":                 1,
  "serviceDefinition": "string",
  "providerSystem":    { "systemName": "", "address": "", "port": 0 },
  "serviceUri":        "string",
  "interfaces":        ["string"],
  "version":           1,
  "metadata":          { "key": "value" },
  "secure":            "NOT_SECURE"
}
```

Uniqueness key: `(serviceDefinition, systemName, address, port, version)`.
Duplicate registration overwrites the existing entry (same `id`).

### POST /serviceregistry/register

Registers or updates a service instance.

**Request:**
```json
{
  "serviceDefinition": "string",
  "providerSystem": { "systemName": "", "address": "", "port": 0 },
  "serviceUri": "string",
  "interfaces": ["string"],
  "version": 1,
  "metadata": {},
  "secure": "NOT_SECURE"
}
```

**Response: 201 Created** — the stored `ServiceInstance`.

**Validation errors (400):**
- `serviceDefinition` empty
- `providerSystem` missing, `systemName`/`address` empty, `port` ≤ 0
- `serviceUri` empty
- `interfaces` empty

### POST /serviceregistry/query

Queries registered services. All filters are ANDed; a zero value for a filter means "no filter".

**Request (all fields optional):**
```json
{
  "serviceDefinition": "string",
  "interfaces": ["string"],
  "metadata": { "key": "value" },
  "versionRequirement": 0
}
```

**Response: 200 OK:**
```json
{
  "serviceQueryData": [ /* ServiceInstance[] */ ],
  "unfilteredHits": 0
}
```

Matching rules:
- `serviceDefinition` — exact string match
- `interfaces` — service must provide ALL requested (case-insensitive)
- `metadata` — service must contain ALL requested key-value pairs
- `versionRequirement` — exact match; 0 = no filter

### GET /serviceregistry/lookup

Same filtering as POST /query but via query parameters.

**Query params:** `serviceDefinition`, `version` (integer)

**Response: 200 OK** — same shape as POST /query.

### DELETE /serviceregistry/unregister

Removes a registered service instance.

**Request body:**
```json
{
  "serviceDefinition": "string",
  "providerSystem": { "systemName": "", "address": "", "port": 0 },
  "version": 1
}
```

**Response: 204 No Content** on success.
**Response: 404 Not Found** if the entry does not exist.

---

## 2. Authentication — Port 8081

Manages identity tokens for systems. Tokens are opaque strings with an expiry.

### POST /authentication/identity/login

Issues a new token.

**Request:**
```json
{ "systemName": "string", "credentials": "string" }
```

**Response: 201 Created:**
```json
{
  "token":      "string",
  "systemName": "string",
  "expiresAt":  "2006-01-02T15:04:05Z"
}
```

### DELETE /authentication/identity/logout

Revokes the current token.

**Header:** `Authorization: Bearer <token>`

**Response: 204 No Content**

### GET /authentication/identity/verify

Checks whether a token is still valid.

**Header:** `Authorization: Bearer <token>`

**Response: 200 OK:**
```json
{
  "valid":      true,
  "systemName": "string",
  "expiresAt":  "2006-01-02T15:04:05Z"
}
```

`valid: false` is returned (not 401) when the token is expired or unknown.

---

## 3. ConsumerAuthorization — Port 8082

Manages authorization rules that state which consumer may access which provider's service.

### AuthRule

```json
{
  "id":                   1,
  "consumerSystemName":   "string",
  "providerSystemName":   "string",
  "serviceDefinition":    "string"
}
```

### POST /authorization/grant

Creates a new rule. Returns 409 if an identical rule already exists.

**Request:**
```json
{
  "consumerSystemName": "string",
  "providerSystemName": "string",
  "serviceDefinition":  "string"
}
```

**Response: 201 Created** — the stored `AuthRule`.

### DELETE /authorization/revoke/{id}

Removes a rule by ID.

**Response: 204 No Content** or **404 Not Found**.

### GET /authorization/lookup

Returns matching rules.

**Query params (all optional):** `consumer`, `provider`, `service`

**Response: 200 OK:**
```json
{ "rules": [ /* AuthRule[] */ ], "count": 0 }
```

### POST /authorization/verify

Checks whether a consumer→provider→service combination is authorized.

**Request:**
```json
{
  "consumerSystemName": "string",
  "providerSystemName": "string",
  "serviceDefinition":  "string"
}
```

**Response: 200 OK:**
```json
{ "authorized": true, "ruleId": 1 }
```

### POST /authorization/token/generate

Generates an authorization token for an authorized pair (used by orchestration).

**Request:**
```json
{
  "consumerSystemName": "string",
  "providerSystemName": "string",
  "serviceDefinition":  "string"
}
```

**Response: 200 OK:**
```json
{ "token": "string", "expiresAt": "2006-01-02T15:04:05Z" }
```

Returns 403 if the pair is not authorized.

---

## 4. DynamicOrchestration — Port 8083

Performs real-time discovery: queries the Service Registry and optionally filters by ConsumerAuthorization rules.

### POST /orchestration/dynamic

**Request:**
```json
{
  "requesterSystem":  { "systemName": "", "address": "", "port": 0 },
  "requestedService": { "serviceDefinition": "", "interfaces": [], "metadata": {} }
}
```

**Response: 200 OK:**
```json
{
  "response": [
    {
      "provider": { "systemName": "", "address": "", "port": 0 },
      "service":  { "serviceDefinition": "", "serviceUri": "", "interfaces": [] }
    }
  ]
}
```

**Behavior:**
1. Calls `POST /serviceregistry/query` with `requestedService` as filters.
2. If `ENABLE_AUTH=true`, calls `POST /authorization/verify` for each result and removes unauthorized providers.
3. Returns the remaining results.

**Configuration (env vars):**
- `SERVICE_REGISTRY_URL` — default `http://localhost:8080`
- `CONSUMER_AUTH_URL` — default `http://localhost:8082`
- `ENABLE_AUTH` — `true`/`false`, default `false`

---

## 5. SimpleStoreOrchestration — Port 8084

Manages pre-configured peer-to-peer routing rules. Returns the single matching rule for a consumer+service pair.

### StoreRule

```json
{
  "id":                 1,
  "consumerSystemName": "string",
  "serviceDefinition":  "string",
  "provider":           { "systemName": "", "address": "", "port": 0 },
  "serviceUri":         "string",
  "interfaces":         ["string"],
  "metadata":           {}
}
```

### POST /orchestration/simplestore

**Request:** same shape as DynamicOrchestration.

**Response: 200 OK:** same shape as DynamicOrchestration.

Matches by `requesterSystem.systemName` + `requestedService.serviceDefinition`. Returns the first matching rule wrapped in an `OrchestrationResult`.

### GET /orchestration/simplestore/rules

Returns all stored rules.

**Response: 200 OK:** `{ "rules": [ /* StoreRule[] */ ], "count": N }`

### POST /orchestration/simplestore/rules

Creates a new rule. Validates all required fields.

**Response: 201 Created** — the stored `StoreRule`.

### DELETE /orchestration/simplestore/rules/{id}

Removes a rule by ID.

**Response: 204 No Content** or **404 Not Found**.

---

## 6. FlexibleStoreOrchestration — Port 8085

Extends SimpleStore with priority ordering and metadata-filter matching per rule.

### FlexibleRule

```json
{
  "id":                 1,
  "consumerSystemName": "string",
  "serviceDefinition":  "string",
  "priority":           1,
  "metadataFilter":     { "key": "value" },
  "provider":           { "systemName": "", "address": "", "port": 0 },
  "serviceUri":         "string",
  "interfaces":         ["string"],
  "metadata":           {}
}
```

**Priority:** lower integer = higher priority. `0` is treated as lowest priority (equivalent to `MaxInt`).

**MetadataFilter:** the request's `requestedService.metadata` must contain all key-value pairs defined in the rule's `metadataFilter` for the rule to match.

### POST /orchestration/flexiblestore

**Request:** same shape as DynamicOrchestration.

**Response: 200 OK:** same shape as DynamicOrchestration, but results include `"priority"` field, sorted ascending (highest priority first).

Matching rules:
1. `consumerSystemName` must equal `requesterSystem.systemName`
2. `serviceDefinition` must equal `requestedService.serviceDefinition`
3. Rule's `metadataFilter` must be a subset of request's `requestedService.metadata`

### GET /orchestration/flexiblestore/rules

Returns all stored rules.

### POST /orchestration/flexiblestore/rules

Creates a new rule.

**Response: 201 Created** — the stored `FlexibleRule`.

### DELETE /orchestration/flexiblestore/rules/{id}

Removes a rule by ID.

---

## Error Format

All error responses use:
```json
{ "error": "human-readable message" }
```

Standard HTTP status codes:
- `400` Bad Request — validation failure
- `403` Forbidden — not authorized
- `404` Not Found — resource does not exist
- `405` Method Not Allowed
- `409` Conflict — duplicate resource
- `500` Internal Server Error
