# Arrowhead Core Systems Specification

## Overview

This specification covers the eight Arrowhead core systems implemented in this repository.
The first six follow Arrowhead 5 (AH5); `Blacklist` is the seventh AH5 system; `CertificateAuthority` is an extension added for experiment-2.

| System | Port | Path prefix |
|---|---|---|
| ServiceRegistry | 8080 | `/serviceregistry` |
| Authentication | 8081 | `/authentication` |
| ConsumerAuthorization | 8082 | `/authorization` |
| DynamicOrchestration | 8083 | `/serviceorchestration/orchestration/pull` |
| SimpleStoreOrchestration | 8084 | `/serviceorchestration/orchestration/pull` |
| FlexibleStoreOrchestration | 8085 | `/serviceorchestration/orchestration/pull` |
| CertificateAuthority | 8086 | `/ca` |
| Blacklist | 8087 | `/blacklist` |

All systems expose a `GET /health` endpoint returning `{"status":"ok","system":"<name>"}`.

---

## Persistence

All seven systems select a storage backend from the `DB_PATH` environment variable:

| Value | Backend |
|---|---|
| unset (default) | In-memory Go maps — zero-setup, zero-cleanup, data lost on restart |
| `:memory:` | SQLite in-memory — same lifetime as process, useful for integration tests |
| `/path/to/file.db` | SQLite file-backed — data persists across restarts |

The ServiceRegistry creates **two** SQLite files when `DB_PATH` is set: the path given
(for legacy/AH4 registrations) and `<DB_PATH>.ah5` (for AH5 device/system/service
discovery records).

All other systems create a single file at `DB_PATH`.

**Clean full restart (Docker):**
```bash
docker compose down -v && docker compose up -d --build
```
The `-v` flag removes named volumes so the next start is from a clean state.

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

**Configuration (env vars):**
- `MGMT_AUTH_URL` — when set, all `/mgmt/*` endpoints require sysop Bearer token. When empty, management is open.
- `BLACKLIST_URL` — when set, `POST /serviceregistry/register` rejects registrations from blacklisted `providerSystem.systemName` (403 Forbidden). When empty, no blacklist check.
- `SR_AUTH_URL` — Authentication system URL for system-remove token verification. Default: `http://localhost:8081`.
- `REGISTER_AUTH_URL` — when set, `POST /serviceregistry/system-discovery/register` and `POST /serviceregistry/service-discovery/register` require `Authorization: Bearer <token>` whose verified `systemName` matches the `name`/`systemName` in the request body. Fail-closed: missing token → 401; network error → 401; name mismatch → 403. When empty, registration is open (development mode). Added in Step 33 (G10).

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

---

## 1a. ServiceRegistry — AH5 Discovery and Management Interfaces

The following endpoints implement the AH5 `serviceDiscovery`, `systemDiscovery`,
`deviceDiscovery`, and `serviceRegistryManagement` interfaces.
They are served on the same port (8080) as the legacy endpoints.

All errors use the shared format: `{"error": "message"}`.

---

### AH5 Shared Types

#### Address
```json
{ "type": "MAC|IP|...", "address": "string" }
```

#### Device
```json
{
  "name":      "string",
  "metadata":  { "key": "value" },
  "addresses": [ { "type": "...", "address": "..." } ],
  "createdAt": "RFC3339",
  "updatedAt": "RFC3339"
}
```

#### AH5System
```json
{
  "name":      "string",
  "metadata":  { "key": "value" },
  "version":   "string",
  "addresses": [ { "type": "...", "address": "..." } ],
  "device":    { /* Device, optional */ },
  "createdAt": "RFC3339",
  "updatedAt": "RFC3339"
}
```

#### ServiceDefinition
```json
{ "name": "string", "createdAt": "RFC3339", "updatedAt": "RFC3339" }
```

#### InterfaceTemplate
```json
{
  "name":                 "string",
  "protocol":             "string",
  "propertyRequirements": { "key": "value" },
  "createdAt":            "RFC3339",
  "updatedAt":            "RFC3339"
}
```

#### InterfaceInstance
```json
{
  "templateName": "string",
  "protocol":     "string",
  "policy":       "string",
  "properties":   { "key": "value" }
}
```

#### AH5ServiceInstance
```json
{
  "instanceId":            "string",
  "provider":              { /* AH5System */ },
  "serviceDefinitionName": "string",
  "version":               "string",
  "expiresAt":             "RFC3339",
  "metadata":              { "key": "value" },
  "interfaces":            [ { /* InterfaceInstance */ } ],
  "createdAt":             "RFC3339",
  "updatedAt":             "RFC3339"
}
```

---

### Device Discovery

#### POST /serviceregistry/device-discovery/register

Registers or updates a device (upsert by name).

**Request:**
```json
{ "name": "string", "metadata": {}, "addresses": [] }
```

**Response:**
- `201 Created` — newly registered device
- `200 OK` — existing device updated

**Validation errors (400):** `name` empty.

---

#### POST /serviceregistry/device-discovery/lookup

Returns devices matching optional filter criteria (all filters ANDed).

**Request (all fields optional):**
```json
{
  "deviceNames": ["string"],
  "addresses":   ["string"],
  "addressType": "string"
}
```

**Response: 200 OK:**
```json
{ "entries": [ /* Device[] */ ], "count": 0 }
```

---

#### DELETE /serviceregistry/device-discovery/revoke/{name}

Removes the named device.

**Response:**
- `200 OK` — device removed
- `204 No Content` — no matching device found

---

### System Discovery

#### POST /serviceregistry/system-discovery/register

Registers or updates a system (upsert by name).

**Request:**
```json
{
  "name":       "string",
  "metadata":   {},
  "version":    "string",
  "addresses":  [],
  "deviceName": "string"
}
```

**Note (G10):** The AH5 spec derives the system name from the caller's auth token.
This implementation requires it in the request body.

**Response:**
- `201 Created` — newly registered
- `200 OK` — existing system updated

**Validation errors (400):** `name` empty.

---

#### POST /serviceregistry/system-discovery/lookup

Returns systems matching optional filter criteria.

**Request (all fields optional):**
```json
{
  "systemNames": [],
  "addresses":   [],
  "addressType": "string",
  "versions":    [],
  "deviceNames": []
}
```

**Response: 200 OK:**
```json
{ "entries": [ /* AH5System[] */ ], "count": 0 }
```

---

#### DELETE /serviceregistry/system-discovery/revoke

Removes the system identified by the caller's Bearer token.

**Header:** `Authorization: Bearer <token>` (required)

The `SR_AUTH_URL` configuration variable points to the Authentication system.
ServiceRegistry calls `GET <SR_AUTH_URL>/authentication/identity/verify/<token>` and
uses the returned `systemName` to identify which system to remove. If the header is
absent, the token is invalid, or the Authentication system is unreachable, the request
is rejected with `401 Unauthorized` (fail-closed).

**New env var:** `SR_AUTH_URL` — default `http://localhost:8081`.

**Response:**
- `204 No Content` — system removed (or no matching system found)
- `401 Unauthorized` — Bearer token absent, invalid, or Authentication unreachable

---

### Service Discovery

#### POST /serviceregistry/service-discovery/register

Registers or updates a service instance (upsert by `systemName + serviceDefinitionName + version`).

**Request:**
```json
{
  "systemName":            "string",
  "serviceDefinitionName": "string",
  "version":               "string",
  "expiresAt":             "RFC3339",
  "metadata":              {},
  "interfaces": [
    {
      "templateName": "http-json",
      "protocol":     "http",
      "policy":       "NONE",
      "properties":   {}
    }
  ]
}
```

The `interfaces` field accepts two forms per element:
- **Structured object** — `{"templateName":"http-json","protocol":"http","policy":"NONE","properties":{...}}`
- **Flat string (backward-compat)** — `"HTTP-INSECURE-JSON"` is wrapped as `{templateName, protocol:"http", policy:"NONE"}`

Valid `policy` values: `NONE`, `CERT_AUTH`, `TIME_LIMITED_TOKEN_AUTH`, `USAGE_LIMITED_TOKEN_AUTH`, `BASE64_SELF_CONTAINED_TOKEN_AUTH`, `RSA_SHA256_JSON_WEB_TOKEN_AUTH`, `RSA_SHA512_JSON_WEB_TOKEN_AUTH`. An unknown value returns 400.

If an interface template with the given `templateName` exists in the store, its `propertyRequirements` are validated against the provided `properties`. If no template is registered, the interface is accepted without validation.

**Note (G10):** The AH5 spec derives `systemName` from the auth token. This
implementation requires it in the request body.

**Response:**
- `201 Created` — newly registered
- `200 OK` — existing instance updated

**Validation errors (400):** `systemName` or `serviceDefinitionName` empty; unknown `policy` value.

---

#### POST /serviceregistry/service-discovery/lookup

Returns service instances matching the filter criteria. At least one primary filter must be provided.

**Request:**
```json
{
  "instanceIds":            [],
  "providerNames":          [],
  "serviceDefinitionNames": [],
  "versions":               [],
  "interfaceTemplateNames": []
}
```

At least one of `instanceIds`, `providerNames`, or `serviceDefinitionNames` must be non-empty; otherwise the request returns 400.

**Response: 200 OK:**
```json
{ "entries": [ /* AH5ServiceInstance[] */ ], "count": 0 }
```

---

#### DELETE /serviceregistry/service-discovery/revoke/{instanceId}

Removes the service instance with the given ID.

**Response:**
- `200 OK` — instance removed
- `204 No Content` — no matching instance found

---

### Service Registry Management

All management endpoints are under `/serviceregistry/mgmt/`.

#### Devices

| Method | Path | Description |
|---|---|---|
| `POST` | `/serviceregistry/mgmt/devices/query` | Query devices with optional filters |
| `POST` | `/serviceregistry/mgmt/devices` | Create new devices (400 if any exist) |
| `PUT`  | `/serviceregistry/mgmt/devices` | Update existing devices (400 if any not found) |
| `DELETE` | `/serviceregistry/mgmt/devices?names=X&names=Y` | Remove devices by name |

Create body: `{ "devices": [ { "name", "metadata", "addresses" } ] }`
Response: `{ "devices": [], "count": 0 }`

---

#### Systems

| Method | Path | Description |
|---|---|---|
| `POST` | `/serviceregistry/mgmt/systems/query` | Query systems with optional filters |
| `POST` | `/serviceregistry/mgmt/systems` | Create new systems (400 if any exist) |
| `PUT`  | `/serviceregistry/mgmt/systems` | Update existing systems (400 if any not found) |
| `DELETE` | `/serviceregistry/mgmt/systems?names=X&names=Y` | Remove systems by name |

Create body: `{ "systems": [ { "name", "metadata", "version", "addresses", "deviceName" } ] }`
Response: `{ "systems": [], "count": 0 }`

---

#### Service Definitions

| Method | Path | Description |
|---|---|---|
| `POST` | `/serviceregistry/mgmt/service-definitions/query` | List all service definitions |
| `POST` | `/serviceregistry/mgmt/service-definitions` | Create new definitions (400 if any exist) |
| `DELETE` | `/serviceregistry/mgmt/service-definitions?names=X&names=Y` | Remove by name |

Create body: `{ "serviceDefinitionNames": ["string"] }`
Response: `{ "serviceDefinitions": [], "count": 0 }`

---

#### Service Instances

| Method | Path | Description |
|---|---|---|
| `POST` | `/serviceregistry/mgmt/service-instances/query` | Query instances with optional filters |
| `POST` | `/serviceregistry/mgmt/service-instances` | Create new instances (400 if any exist) |
| `PUT`  | `/serviceregistry/mgmt/service-instances` | Update by instanceId (400 if not found) |
| `DELETE` | `/serviceregistry/mgmt/service-instances?serviceInstances=X` | Remove by instanceId |

Create body: `{ "instances": [ { "systemName", "serviceDefinitionName", "version", "expiresAt", "metadata", "interfaces" } ] }`
Update body: `{ "instances": [ { "instanceId", "expiresAt", "metadata", "interfaces" } ] }`
Response: `{ "instances": [], "count": 0 }`

---

#### Interface Templates

| Method | Path | Description |
|---|---|---|
| `POST` | `/serviceregistry/mgmt/interface-templates/query` | List all interface templates |
| `POST` | `/serviceregistry/mgmt/interface-templates` | Create new templates (400 if any exist) |
| `DELETE` | `/serviceregistry/mgmt/interface-templates?names=X&names=Y` | Remove by name |

Create body: `{ "interfaceTemplates": [ { "name", "protocol", "propertyRequirements" } ] }`
Response: `{ "interfaceTemplates": [], "count": 0 }`

---

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

**Configuration (env vars):**
- `MGMT_AUTH_URL` — when set, all `/mgmt/*` endpoints (identities, sessions) require sysop Bearer token. When empty, management is open.

### POST /authentication/identity/login

Issues a new token. Credentials are verified against the stored bcrypt hash.
Returns 401 if the `systemName` is not registered or the password does not match.
The `credentials` field must be a JSON object with a `password` key. A plain string,
`null`, or a missing `password` key returns `400 Bad Request`.

**Request:**
```json
{ "systemName": "string", "credentials": {"password": "string"} }
```

**Error responses:**
- `400 Bad Request` — `credentials` is not a `{"password":"..."}` object, or `password` key is absent
- `401 Unauthorized` — unknown `systemName` or wrong password

**Response: 201 Created:**
```json
{
  "token":          "string",
  "systemName":     "string",
  "expirationTime": "2006-01-02T15:04:05Z",
  "sysop":          false
}
```

**Response: 401 Unauthorized** — unknown systemName or wrong password.

### POST /authentication/identity/logout

Revokes the current token.

**Header:** `Authorization: Bearer <token>`

**Response: 200 OK**

### GET /authentication/identity/verify/{token}

Checks whether a token is still valid.

**Response: 200 OK:**
```json
{
  "verified":        true,
  "systemName":      "string",
  "loginTime":       "2006-01-02T15:04:05Z",
  "expirationTime":  "2006-01-02T15:04:05Z",
  "sysop":           false
}
```

`verified: false` is returned (not 401) when the token is expired or unknown. `sysop` reflects the identity record's flag.

---

### POST /authentication/identity/change

Rotates credentials for a system that has an active session.

**Request:**
```json
{
  "systemName":     "string",
  "credentials":    {"password": "string"},
  "newCredentials": {"password": "string"}
}
```

**Response: 200 OK** — no body.

**Response: 401 Unauthorized** — if no active session exists for `systemName`.

---

### POST /authentication/mgmt/identities/query

Returns all registered identities (no password hashes).

**Request:** `{}` (empty body or pagination fields)

**Response: 200 OK:**
```json
{ "identities": [{"systemName":"string","sysop":false,"createdBy":"string","createdAt":"DateTime","updatedAt":"DateTime"}], "totalCount": 1 }
```

---

### POST /authentication/mgmt/identities

Bulk-create identity records with bcrypt-hashed passwords.

**Request:**
```json
{
  "authenticationMethod": "PASSWORD",
  "identities": [{"systemName":"string","credentials":{"password":"string"},"sysop":false,"createdBy":"string"}]
}
```

**Response: 201 Created:**
```json
{ "identities": [{"systemName":"string","sysop":false,"createdBy":"string","createdAt":"DateTime","updatedAt":"DateTime"}] }
```

---

### PUT /authentication/mgmt/identities

Bulk-update credentials for existing identities. Returns 400 if any `systemName` is not found.

**Request:** same shape as POST.

**Response: 200 OK:** same shape as POST response.

---

### DELETE /authentication/mgmt/identities

Remove identities by name.

**Query:** `?names=sys1,sys2`

**Response: 200 OK** — no body.

---

### POST /authentication/mgmt/sessions

Returns all active (non-expired) sessions.

**Request:** `{}`

**Response: 200 OK:**
```json
{ "sessions": [{"token":"string","systemName":"string","loginTime":"DateTime","expirationTime":"DateTime"}], "totalCount": 1 }
```

---

### DELETE /authentication/mgmt/sessions

Revoke all sessions for the named systems.

**Query:** `?names=sys1,sys2`

**Response: 200 OK** — no body.

---

**Bootstrap:** When the identity store is empty at startup, a built-in `Sysop` identity is
auto-created (password from `SYSOP_PASSWORD` env var, default `arrowhead`).

---

## 3. ConsumerAuthorization — Port 8082

Manages provider-centric authorization policies. Each policy is identified by a composite
`instanceId` of the form `PR|LOCAL|<provider>|<targetType>|<target>`.

**Configuration (env vars):**
- `MGMT_AUTH_URL` — when set, all `/mgmt/*` endpoints require sysop Bearer token. When empty, management is open.
- `BLACKLIST_URL` — when set, `grant` rejects blacklisted providers (403), and `verify` returns `false` for blacklisted consumers or providers (without 4xx). When empty, no blacklist check.
- `HMAC_SECRET` — secret used to sign `BASE64_SELF_CONTAINED` tokens (HMAC-SHA256). Default: `arrowhead-default-secret`. Set to a strong random value in production. Added in Step 34 (G23).

### AuthPolicy

```json
{
  "instanceId":          "PR|LOCAL|TemperatureProvider|SERVICE_DEF|temperatureService",
  "authorizationLevel":  "PR",
  "cloud":               "LOCAL",
  "provider":            "TemperatureProvider",
  "targetType":          "SERVICE_DEF",
  "target":              "temperatureService",
  "description":         "optional",
  "defaultPolicy":       { "policyType": "WHITELIST", "policyList": ["ConsumerApp"] },
  "scopedPolicies":      {},
  "createdBy":           "string",
  "createdAt":           "2006-01-02T15:04:05Z"
}
```

**Policy types:** `ALL` (any consumer), `WHITELIST` (only listed consumers), `BLACKLIST` (any except listed).

**Target types:** `SERVICE_DEF`, `EVENT_TYPE`.

### POST /consumerauthorization/authorization/grant

Creates a new policy. Returns 409 if the `instanceId` already exists.

**Request:**
```json
{
  "provider":       "TemperatureProvider",
  "targetType":     "SERVICE_DEF",
  "target":         "temperatureService",
  "defaultPolicy":  { "policyType": "WHITELIST", "policyList": ["ConsumerApp"] },
  "scopedPolicies": {},
  "description":    "optional",
  "createdBy":      "optional"
}
```

**Response: 201 Created** — the stored `AuthPolicy`.

### DELETE /consumerauthorization/authorization/revoke/{instanceId}

Removes a policy by `instanceId`. Pipe characters in the path must be percent-encoded as `%7C`.

**Response: 200 OK** or **404 Not Found**.

### POST /consumerauthorization/authorization/lookup

Returns matching policies. At least one of `instanceIds`, `cloudIdentifiers`, or `targetNames` must be provided (returns 400 otherwise).

**Request:**
```json
{
  "instanceIds":      ["PR|LOCAL|Provider|SERVICE_DEF|svc"],
  "cloudIdentifiers": ["LOCAL"],
  "targetNames":      ["temperatureService"],
  "targetType":       "SERVICE_DEF"
}
```

**Response: 200 OK:**
```json
{ "policies": [ /* AuthPolicy[] */ ], "count": 1, "totalCount": 5 }
```

### POST /consumerauthorization/authorization/verify

Checks whether a consumer is authorized for the target. Returns a **plain JSON Boolean** (`true` or `false`), not a wrapped object. If `provider` is given, only the policy for that specific provider is checked.

**Request:**
```json
{
  "consumer":   "ConsumerApp",
  "provider":   "TemperatureProvider",
  "target":     "temperatureService",
  "targetType": "SERVICE_DEF",
  "scope":      "read"
}
```

**Response: 200 OK:** `true` or `false`

### POST /consumerauthorization/authorization/mgmt/grant

Same as the main grant endpoint; alias for management access.

### DELETE /consumerauthorization/authorization/mgmt/revoke

Bulk revoke by instanceIds.

**Query:** `?instanceIds=id1,id2,...` (pipe chars must be percent-encoded)

**Response: 200 OK**

### POST /consumerauthorization/authorization/mgmt/query

Returns all policies (no filter required; optional filters accepted).

**Request:** `{}` or same shape as lookup.

**Response: 200 OK:**
```json
{ "policies": [ /* AuthPolicy[] */ ], "count": 1, "totalCount": 1 }
```

### POST /consumerauthorization/authorization/mgmt/check

Bulk verify — checks authorization for multiple requests.

**Request:** array of VerifyRequest objects.

**Response: 200 OK:** array of booleans.

---

### POST /consumerauthorization/authorization/mgmt/grant-policies

Bulk policy grant. Per-item errors do not abort the batch.

**Request:**
```json
{
  "policies": [
    { "provider": "SensorProvider", "targetType": "SERVICE_DEF", "target": "temperatureService", "defaultPolicy": { "policyType": "ALL" } }
  ]
}
```

**Response: 200 OK:**
```json
{
  "results": [
    { "instanceId": "PR|LOCAL|SensorProvider|SERVICE_DEF|temperatureService", "policy": { ... } }
  ],
  "count": 1
}
```

Per-item error: `{ "error": "authorization policy already exists" }`.

Requires `MGMT_AUTH_URL` (sysop Bearer token).

---

### DELETE /consumerauthorization/authorization/mgmt/revoke-policies

Bulk policy revocation by instanceIds (request body, not query param).

**Request:**
```json
{ "instanceIds": ["PR|LOCAL|SensorProvider|SERVICE_DEF|temperatureService"] }
```

**Response: 200 OK** (empty body). Missing instanceIds are ignored.

Requires `MGMT_AUTH_URL`.

---

### POST /consumerauthorization/authorization/mgmt/query-policies

Paginated policy query with optional filters.

**Request:**
```json
{
  "instanceIds": [],
  "targetNames": [],
  "pagination": { "pageNumber": 0, "pageSize": 20 }
}
```

**Response: 200 OK:**
```json
{ "policies": [...], "count": 20, "totalCount": 100 }
```

Requires `MGMT_AUTH_URL`.

---

### POST /consumerauthorization/authorization/mgmt/check-policies

Non-destructive bulk authorization check. Returns each tuple with an `authorized` boolean.

**Request:** array of VerifyRequest objects.

**Response: 200 OK:**
```json
{
  "results": [
    { "consumer": "ConsumerApp", "provider": "SensorProvider", "target": "temperatureService", "targetType": "SERVICE_DEF", "authorized": true }
  ],
  "count": 1
}
```

Requires `MGMT_AUTH_URL`.

---

## 3b. ConsumerAuthorization — authorization-token sub-service

The `authorization-token` sub-service issues short-lived tokens that a consumer presents to a provider as proof of authorization. Only `TIME_LIMITED_TOKEN` is fully implemented; all other variants return `501 Not Implemented`.

### POST /consumerauthorization/authorization-token/generate

Generates an authorization token for the specified target.

**Request:**
```json
{
  "tokenVariant": "TIME_LIMITED_TOKEN",
  "provider":     "SensorProvider",
  "targetType":   "SERVICE_DEF",
  "target":       "temperatureService",
  "scope":        "read",
  "consumer":     "ConsumerApp"
}
```

**Response: 201 Created** (`TokenDescriptor`):
```json
{
  "tokenType":  "TIME_LIMITED_TOKEN",
  "targetType": "SERVICE_DEF",
  "token":      "<uuid-hex>",
  "expiresAt":  "2025-06-01T12:00:00Z"
}
```

Field presence by variant:
- `TIME_LIMITED_TOKEN` — `expiresAt` is set; `usageLimit` omitted.
- `USAGE_LIMITED_TOKEN` — `usageLimit` is set (defaults to 1 when `maxUsageCount` is 0); `expiresAt` omitted.
- `BASE64_SELF_CONTAINED`, `RSA_SHA256_JSON_WEB_TOKEN`, `RSA_SHA512_JSON_WEB_TOKEN`, `TRANSLATION_BRIDGE_TOKEN` — both `expiresAt` and `usageLimit` omitted (expiry is embedded in the token payload or not applicable).

**Response: 501 Not Implemented** — unsupported `tokenVariant`.

Tokens expire after 1 hour.

---

### GET /consumerauthorization/authorization-token/verify/{accessToken}

Validates a previously generated token.

**Path param:** `accessToken` — the token string returned by `/generate`.

**Response: 200 OK** (`TokenVerifyResponse`):
```json
{
  "verified":      true,
  "consumerCloud": "LOCAL",
  "consumer":      "ConsumerApp",
  "targetType":    "SERVICE_DEF",
  "target":        "temperatureService",
  "scope":         null
}
```

**Response: 404 Not Found** — token unknown or expired.

---

### GET /consumerauthorization/authorization-token/public-key

Not implemented — always returns 404.

---

### POST /consumerauthorization/authorization-token/encryption-key

Registers an encryption key for a named system.

**Request:**
```json
{
  "systemName": "SensorProvider",
  "algorithm":  "RSA",
  "key":        "<base64-encoded-key>"
}
```

**Response: 201 Created** (empty body).

---

### DELETE /consumerauthorization/authorization-token/encryption-key

Removes the encryption key for a system.

**Query param:** `systemName` — required.

**Response: 200 OK** (empty body).

---

### POST /consumerauthorization/authorization-token/mgmt/generate-tokens

Bulk token generation. Per-item errors do not abort the batch.

**Request:**
```json
{
  "requests": [
    { "tokenVariant": "TIME_LIMITED_TOKEN", "provider": "SensorProvider", "targetType": "SERVICE_DEF", "target": "temperatureService" }
  ]
}
```

**Response: 200 OK:**
```json
{ "results": [{ "token": { "tokenType": "TIME_LIMITED_TOKEN", "token": "<hex>", "expiresAt": "..." } }], "count": 1 }
```

Requires `MGMT_AUTH_URL`.

---

### DELETE /consumerauthorization/authorization-token/mgmt/revoke-tokens

Bulk token revocation. Missing tokens are silently ignored.

**Request:**
```json
{ "tokens": ["<hex-token-1>", "<hex-token-2>"] }
```

**Response: 200 OK** (empty body).

Requires `MGMT_AUTH_URL`.

---

### POST /consumerauthorization/authorization-token/mgmt/query-tokens

Returns all unexpired tokens with pagination.

**Request:**
```json
{ "pagination": { "pageNumber": 0, "pageSize": 20 } }
```

**Response: 200 OK:**
```json
{ "tokens": [...TokenRecord...], "count": 20, "totalCount": 100 }
```

Requires `MGMT_AUTH_URL`.

---

### POST /consumerauthorization/authorization-token/mgmt/add-encryption-keys

Bulk register encryption keys. Overwrites existing key for same system name.

**Request:**
```json
{ "keys": [{ "systemName": "SensorProvider", "algorithm": "RSA", "key": "<base64>" }] }
```

**Response: 201 Created** (empty body). Requires `MGMT_AUTH_URL`.

---

### DELETE /consumerauthorization/authorization-token/mgmt/remove-encryption-keys

Bulk remove encryption keys for named systems.

**Request:**
```json
{ "systemNames": ["SensorProvider"] }
```

**Response: 200 OK** (empty body). Requires `MGMT_AUTH_URL`.

---

## 4. DynamicOrchestration — Port 8083

Performs real-time discovery: queries the Service Registry and optionally filters by ConsumerAuthorization rules.

### POST /serviceorchestration/orchestration/pull

**Request:**

The `serviceRequirement` field is the AH5 spec name; `requestedService` is accepted as a backward-compatible alias. Both decode to the same field. Responses always use `serviceRequirement`.

```json
{
  "requesterSystem":    { "systemName": "", "address": "", "port": 0 },
  "serviceRequirement": { "serviceDefinition": "", "interfaces": [], "metadata": {}, "versionRequirement": "" },
  "orchestrationFlags": {
    "MATCHMAKING":       false,
    "ONLY_PREFERRED":    false,
    "ALLOW_INTERCLOUD":  false,
    "ONLY_INTERCLOUD":   false
  },
  "preferredProviders": [{ "systemName": "", "address": "", "port": 0 }]
}
```

**Response: 200 OK:**
```json
{
  "results": [
    {
      "providerName":        "string",
      "serviceDefinitition": "string",
      "cloudIdentitifer":    "LOCAL",
      "serviceInstanceId":   "string",
      "serviceUri":          "string",
      "interfaces":          ["string"],
      "exclusiveUntil":      "RFC3339 (omitted when provider is not locked)",
      "aliveUntil":          "string",
      "authorizationTokens": {
        "<interfaceName>": {
          "<scope>": {
            "tokenType":  "string",
            "targetType": "string",
            "token":      "string",
            "usageLimit": 0,
            "expiresAt":  "RFC3339"
          }
        }
      }
    }
  ]
}
```

Note: `serviceDefinitition` (double 't') and `cloudIdentitifer` (missing 'n') are intentional spec typos that match the AH5 wire format exactly.

- `cloudIdentitifer` is always `"LOCAL"` (intercloud not supported).
- `interfaces` is forwarded directly from the ServiceRegistry response.
- `exclusiveUntil` is set to the lock's expiry time (RFC3339) when the provider has an active orchestration lock; omitted otherwise. Only DynamicOrchestration checks locks.

**Behavior:**
1. If `ALLOW_INTERCLOUD` or `ONLY_INTERCLOUD` is `true` in `orchestrationFlags`: returns `501 Not Implemented` (intercloud orchestration is not supported).
2. If `ENABLE_IDENTITY_CHECK=true`: validates the `Authorization: Bearer <token>` header against the Authentication system. Returns `401 Unauthorized` if the token is absent, invalid, or expired. The verified `systemName` from the token replaces the self-reported `requesterSystem.systemName` for all subsequent checks.
3. Calls `POST /serviceregistry/service-discovery/lookup` with `{"serviceDefinitionNames": [<serviceDefinition>]}`.
4. If `ENABLE_AUTH=true`, calls `POST /consumerauthorization/authorization/verify` for each result and removes unauthorized providers.
5. If `orchestrationFlags.ONLY_PREFERRED=true` and `preferredProviders` is non-empty, filters results to only those matching a preferred provider's `systemName`.
6. If `orchestrationFlags.MATCHMAKING=true` and more than one result remains, truncates to the first result.
7. If `RELAY_TOKENS=true`, calls `POST /consumerauthorization/authorization-token/generate` for each remaining result and populates `authorizationTokens` on that result (outer key = interface name, inner key = `""`). Token relay errors are silently skipped (fail-open).
8. Returns the remaining results with `cloudIdentitifer="LOCAL"` and `interfaces` forwarded from the SR response.

**Configuration (env vars):**
- `SERVICE_REGISTRY_URL` — default `http://localhost:8080`
- `CONSUMER_AUTH_URL` — default `http://localhost:8082`
- `AUTH_SYSTEM_URL` — default `http://localhost:8081`
- `ENABLE_AUTH` — `true`/`false`, default `false`
- `ENABLE_IDENTITY_CHECK` — `true`/`false`, default `false`. When `true`, requires a valid Bearer token issued by the Authentication system. The verified identity overrides the self-reported `requesterSystem.systemName`, preventing impersonation.
- `MGMT_AUTH_URL` — when set, all `/mgmt/*` endpoints require an `Authorization: Bearer` token verified against this Authentication system URL. Only tokens with `sysop: true` are accepted. When empty, management endpoints are open (development mode).
- `BLACKLIST_URL` — when set, the Blacklist system is consulted to reject blacklisted requesters (step 2.5) and filter blacklisted providers (step 4). When empty, no blacklist check is performed.
- `PUSH_DELIVERY_TIMEOUT_SECONDS` — HTTP timeout per push notification delivery attempt. Default: `5`.
- `QOS_EVALUATOR_URL` — when set, DynamicOrchestration calls `POST <QOS_EVALUATOR_URL>/deviceqosevaluator/quality-evaluation/measure` for each candidate when `qualityRequirements[]` is present in the request. Fail-open: if the evaluator is unreachable, the candidate is included. When empty, a NopQoSClient (fail-open) is used. Added in Step 36 (G40).
- `RELAY_TOKENS` — when `true`, DynamicOrchestration calls `POST <CONSUMER_AUTH_URL>/consumerauthorization/authorization-token/generate` per orchestration result and embeds the returned token in `authorizationTokens` on each `OrchestrationResult`. Key structure: outer = interface name (defaults to `"HTTP-INSECURE-JSON"` when empty), inner = scope (`""` for unscoped/default). Added in Step 60 (G54).

**Push trigger delivery semantics (`mgmt/push/trigger`):**
1. Records a `PUSH/PENDING` history entry.
2. Launches a goroutine that POSTs the subscription payload to the subscriber's `notifyInterface` URL. The URL is extracted from `notifyInterface.notifyUri`, `notifyInterface.uri`, or assembled from `notifyInterface.address` + `port` + `path`.
3. Updates the history entry to `PUSH/DELIVERED` (HTTP 2xx) or `PUSH/FAILED` (error or non-2xx) after the delivery attempt.
4. No retry. The handler returns 200 before delivery completes.

**Note:** `ENABLE_IDENTITY_CHECK` goes beyond the AH5 specification. See `GAP_ANALYSIS.md` for rationale and design decisions.

---

## 5. SimpleStoreOrchestration — Port 8084

Manages pre-configured peer-to-peer routing rules. Returns the single matching rule for a consumer+service pair.

### StoreRule

```json
{
  "id":                 "<UUID>",
  "consumerSystemName": "string",
  "serviceDefinition":  "string",
  "provider":           { "systemName": "", "address": "", "port": 0 },
  "serviceUri":         "string",
  "interfaces":         ["string"],
  "priority":           0,
  "metadata":           {}
}
```

Rule IDs are UUIDs (string). `priority` is optional (omitted when 0); lower value = higher priority.

### POST /serviceorchestration/orchestration/pull

**Request:** same shape as DynamicOrchestration.

**Response: 200 OK:** same shape as DynamicOrchestration.

Matches by `requesterSystem.systemName` + `requestedService.serviceDefinition`. Returns the first matching rule wrapped in an `OrchestrationResult` with `cloudIdentitifer="LOCAL"` and `interfaces` from the stored rule.

If `ALLOW_INTERCLOUD` or `ONLY_INTERCLOUD` is `true` in `orchestrationFlags`, returns `501 Not Implemented`.

### POST /serviceorchestration/orchestration/mgmt/simple-store/create

Creates a new rule. Validates all required fields.

**Response: 201 Created** — the stored `StoreRule` with generated UUID `id`.

### POST /serviceorchestration/orchestration/mgmt/simple-store/query

Returns all stored rules.

**Request:** `{}` (empty filter — currently all rules returned).

**Response: 200 OK:** `{ "rules": [ /* StoreRule[] */ ], "count": N, "totalCount": N }`

### POST /serviceorchestration/orchestration/mgmt/simple-store/modify-priorities

Updates the priority of one or more rules.

**Request:** `{ "priorities": { "<uuid>": <priority>, ... } }`

**Response: 200 OK:** updated `RulesResponse`.

### Legacy alias paths (kept during transition)

| Legacy path | Equivalent mgmt path |
|---|---|
| `GET /serviceorchestration/orchestration/simplestore/rules` | `POST mgmt/simple-store/query` |
| `POST /serviceorchestration/orchestration/simplestore/rules` | `POST mgmt/simple-store/create` |
| `DELETE /serviceorchestration/orchestration/simplestore/rules/{id}` | delete by UUID — 204 No Content or 404 Not Found |

---

## 5a. DynamicOrchestration Management — Port 8083

### Lock management

Lock records:

```json
{
  "id":                 1,
  "orchestrationJobId": "<UUID>",
  "serviceInstanceId":  "string",
  "owner":              "string",
  "expiresAt":          "2024-01-01T00:00:00Z",
  "temporary":          false
}
```

#### POST /serviceorchestration/orchestration/mgmt/lock/create

**Request:** `{ "owner": "string", "serviceInstanceId": "string", "orchestrationJobId": "string", "expiresAt": "RFC3339" (optional), "temporary": bool }`

**Response: 201 Created** — the stored `Lock`.

#### POST /serviceorchestration/orchestration/mgmt/lock/query

Returns all non-expired locks. Expired locks (where `expiresAt` is in the past) are silently excluded.

**Request:** `{}` (empty filter)

**Response: 200 OK:** `{ "locks": [ /* Lock[] */ ], "count": N }`

#### DELETE /serviceorchestration/orchestration/mgmt/lock/remove/{owner}

Removes all locks belonging to `owner`.

**Response: 204 No Content**

### Orchestration history

History entries are written by DynamicOrchestration on every successful pull call.

```json
{
  "id":                 "<UUID>",
  "status":             "DONE",
  "type":               "PULL",
  "requesterSystem":    "string",
  "serviceDefinition":  "string",
  "message":            "",
  "createdAt":          "2024-01-01T00:00:00Z",
  "finishedAt":         "2024-01-01T00:00:00Z"
}
```

#### POST /serviceorchestration/orchestration/mgmt/history/query

Returns all recorded history entries (both PULL and PUSH types).

**Request:** `{}` (empty filter)

**Response: 200 OK:** `{ "entries": [ /* HistoryEntry[] */ ], "count": N }`

History `status` values: `DONE` (pull completed), `PENDING` (push triggered, delivery stub), `ERROR`.
History `type` values: `PULL`, `PUSH`.

### Push orchestration (subscribe / unsubscribe)

Both DynamicOrchestration and SimpleStoreOrchestration expose these discovery endpoints.

**Subscription model:**

```json
{
  "id":                   "<UUID>",
  "ownerSystemName":      "ConsumerApp",
  "targetSystemName":     "ConsumerApp",
  "orchestrationRequest": { },
  "notifyInterface":      { "protocol": "http", "properties": {} },
  "expiredAt":            "2024-01-01T01:00:00Z",
  "createdAt":            "2024-01-01T00:00:00Z"
}
```

#### POST /serviceorchestration/orchestration/subscribe

Registers a push subscription. A duplicate subscribe (same `ownerSystemName` + `targetSystemName`) overwrites the existing entry.

**Response: 201 Created** (new) or **200 OK** (overwrite) — the `Subscription`.

#### DELETE /serviceorchestration/orchestration/unsubscribe/{subscriptionId}

Removes a subscription by ID.

**Response: 200 OK** (found) or **204 No Content** (not found).

### Push management (DynamicOrchestration only)

#### POST /serviceorchestration/orchestration/mgmt/push/subscribe

Operator subscribe on behalf of a system. Same body and response as the discovery subscribe endpoint.

#### DELETE /serviceorchestration/orchestration/mgmt/push/unsubscribe?ids=uuid1,uuid2

Cancels subscriptions by comma-separated IDs.

**Response: 204 No Content**

#### POST /serviceorchestration/orchestration/mgmt/push/trigger

Manually triggers a push notification for a subscription. Records a `PUSH/PENDING` history entry.
Actual notification delivery is a stub — no HTTP call is made to the subscriber.

**Request:** `{ "subscriptionId": "<UUID>" }`

**Response: 200 OK** — `{ "status": "triggered", "subscriptionId": "<UUID>" }`

**Response: 404** if subscription not found.

#### POST /serviceorchestration/orchestration/mgmt/push/query

Returns all active subscriptions.

**Request:** `{}` (empty filter)

**Response: 200 OK:** `{ "subscriptions": [ /* Subscription[] */ ], "count": N }`

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

### POST /serviceorchestration/orchestration/pull

**Request:** same shape as DynamicOrchestration.

**Response: 200 OK:** same shape as DynamicOrchestration, but results include `"priority"` field, sorted ascending (highest priority first).

Matching rules:
1. `consumerSystemName` must equal `requesterSystem.systemName`
2. `serviceDefinition` must equal `requestedService.serviceDefinition`
3. Rule's `metadataFilter` must be a subset of request's `requestedService.metadata`

### GET /serviceorchestration/orchestration/flexiblestore/rules

Returns all stored rules.

### POST /serviceorchestration/orchestration/flexiblestore/rules

Creates a new rule.

**Response: 201 Created** — the stored `FlexibleRule`.

### DELETE /serviceorchestration/orchestration/flexiblestore/rules/{id}

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

---

## 7. CertificateAuthority — Port 8086

Issues and verifies X.509 leaf certificates for Arrowhead systems.  The CA
generates a self-signed ECDSA P-256 root at startup; all state is in-memory.

### POST /ca/certificate/issue

Issues a new leaf certificate.

**Request body:**
```json
{
  "systemName":   "string (required, non-empty)",
  "validDays":    0,
  "cloudName":    "string (optional)",
  "operatorName": "string (optional)"
}
```

`validDays` overrides the default certificate lifetime when > 0.

When `cloudName` and `operatorName` are both provided, the certificate Subject CN is
set to the AH5 hierarchical name `systemName.cloudName.operatorName.arrowhead.eu`
and both the bare `systemName` and the hierarchical name are included as DNS SANs.
This enables AH5-conformant cert naming while preserving Docker hostname verification
(Go 1.15+ ignores CN for TLS hostname verification and requires a matching SAN).

When `cloudName`/`operatorName` are omitted, the CN and DNS SAN are set to the bare
`systemName` as before.

**Response: 201 Created**
```json
{
  "systemName":  "string",
  "certificate": "PEM string",
  "privateKey":  "PEM string",
  "issuedAt":    "RFC3339",
  "expiresAt":   "RFC3339"
}
```

**Errors:** `400` for missing systemName or bad JSON; `405` for non-POST.

### POST /ca/certificate/verify

Verifies a PEM-encoded certificate against this CA.

**Request body:**
```json
{ "certificate": "PEM string" }
```

**Response: 200 OK** (always 200; `valid` field carries the verdict)
```json
{
  "valid":      true,
  "systemName": "string",
  "reason":     "string (non-empty when valid=false)"
}
```

### GET /ca/info

Returns the CA's own certificate.

**Response: 200 OK**
```json
{
  "commonName":  "string",
  "certificate": "PEM string"
}
```

### POST /ca/certificate/revoke

Revokes a previously issued certificate. The certificate must have been issued by
this CA. Revoking an already-revoked certificate is idempotent.

**Request body:**
```json
{ "certificate": "PEM string" }
```

**Response: 200 OK**
```json
{
  "systemName": "string",
  "revokedAt":  "RFC3339"
}
```

**Errors:** `400` if `certificate` is empty, not valid PEM, or not issued by this CA; `405` for non-POST.

After revocation, `POST /ca/certificate/verify` for the same certificate returns
`"valid": false` with `"reason": "certificate has been revoked"`.

### GET /ca/crl

Returns the current Certificate Revocation List (CRL) signed by this CA. The CRL
is generated fresh on each call and is valid for 24 hours.

**Response: 200 OK** — PEM-encoded CRL (`Content-Type: application/x-pem-file`)

```
-----BEGIN X509 CRL-----
...
-----END X509 CRL-----
```

**Errors:** `405` for non-GET; `500` if CRL generation fails (rare, indicates internal crypto error).

### GET /ca/health  ·  GET /health

Returns `{"status":"ok","system":"ca"}`.

---

## 8. Blacklist — Port 8087

Maintains a list of blacklisted systems. Other core systems do not enforce blacklist checks automatically; enforcement integration is a future step.

### Entry model

```json
{
  "systemName": "string",
  "reason":     "string",
  "expiresAt":  "RFC3339 DateTime (optional — omitted means never expires)",
  "active":     true,
  "createdBy":  "string",
  "createdAt":  "RFC3339 DateTime",
  "updatedAt":  "RFC3339 DateTime"
}
```

**New env var:** `BLACKLIST_AUTH_URL` — when set, discovery endpoints require a Bearer token.
When unset (default), Bearer checks are skipped (development/open mode).

### GET /blacklist/lookup

Returns all active, non-expired blacklist entries applicable to the caller.

**Header:** `Authorization: Bearer <token>` — required when `BLACKLIST_AUTH_URL` is set.
Returns `401 Unauthorized` if the token is absent and `BLACKLIST_AUTH_URL` is configured.

**Response: 200 OK**
```json
{ "entries": [...], "count": N }
```

### GET /blacklist/check/{systemName}

Returns `true` if the named system has at least one active, non-expired blacklist entry; `false` otherwise.

**Header:** `Authorization: Bearer <token>` — required when `BLACKLIST_AUTH_URL` is set.

**Response: 200 OK** — plain JSON boolean (`true` or `false`).

### POST /blacklist/mgmt/query

Returns all entries, with optional filtering.

**Request body (all fields optional):**
```json
{
  "systemNames": ["string"],
  "mode":        "ALL"
}
```

`mode` controls which entries are returned:
- `"ALL"` (or omitted) — all entries regardless of active status
- `"ACTIVES"` — only entries where `active: true`
- `"INACTIVES"` — only entries where `active: false`

Any other `mode` value returns `400 Bad Request`.

**Response: 200 OK**
```json
{ "entries": [...], "count": N }
```

### POST /blacklist/mgmt/create

Bulk-creates blacklist entries. All entries must have a non-empty `reason`; returns `400` if any entry is missing it.

**Request body:**
```json
{
  "entries": [
    {
      "systemName": "string (required)",
      "reason":     "string (required, max 1024 chars)",
      "expiresAt":  "RFC3339 DateTime (optional)",
      "createdBy":  "string (optional)"
    }
  ]
}
```

**Response: 201 Created**
```json
{ "entries": [...], "count": N }
```

**Errors:** `400` if `reason` is absent on any entry or JSON is invalid.

### DELETE /blacklist/mgmt/remove?names=name1,name2

Inactivates (sets `active: false`) all entries for the named systems. Does **not** delete records.

**Response: 200 OK**
```json
{ "count": N }
```
where `N` is the number of entries that were inactivated.

### GET /blacklist/health  ·  GET /health

Returns `{"status":"ok","system":"blacklist"}`.

---

## 9. GeneralManagement (cross-cutting — all systems)

Every core system exposes two shared management endpoints under its path prefix.

### POST /<prefix>/general/mgmt/logs

Queries the in-memory ring-buffer log of the system. The buffer holds the most recent 1000 entries (configurable via `LOG_BUFFER_SIZE` env var, not yet implemented).

**Request body (all fields optional):**
```json
{
  "pagination": { "pageNumber": 0, "pageSize": 20 },
  "from":       "RFC3339 DateTime",
  "to":         "RFC3339 DateTime",
  "severity":   "INFO | WARN | ERROR | DEBUG",
  "loggerStr":  "partial-logger-name"
}
```

Filters: `severity` is exact match; `loggerStr` is substring match; `from`/`to` are inclusive time bounds.

**Response: 200 OK**
```json
{
  "entries": [
    {
      "logId":     "string",
      "entryDate": "RFC3339 DateTime",
      "logger":    "string",
      "severity":  "INFO | WARN | ERROR | DEBUG",
      "message":   "string",
      "exception": "string (optional)"
    }
  ],
  "count": N
}
```

**Errors:** `400` if `from` is after `to`.

### GET /<prefix>/general/mgmt/get-config?keys=KEY1,KEY2

Returns the values of the requested configuration keys for this system. Unknown keys are omitted from the response.

**Response: 200 OK** — flat `{"KEY": "value"}` map (only requested keys that exist).

### System path prefixes

| System | Prefix |
|---|---|
| ServiceRegistry | `serviceregistry` |
| Authentication | `authentication` |
| ConsumerAuthorization | `authorization` |
| DynamicOrchestration | `serviceorchestration/orchestration` |
| SimpleStoreOrchestration | `serviceorchestration/orchestration` |
| FlexibleStoreOrchestration | `serviceorchestration/orchestration` |
| CertificateAuthority | `ca` |
| Blacklist | `blacklist` |
| DeviceQoSEvaluator | `deviceqosevaluator` |
| TranslationManager | `translationmanager` |

---

## 9. DeviceQoSEvaluator — Port 8088

New system added in Step 35 (G35). Performs TCP RTT measurements and stores QoS records.

**Configuration (env vars):**
- `PORT` — listen port. Default: `8088`.

### POST /deviceqosevaluator/quality-evaluation/measure

Performs a TCP dial to `host:port`, measures latency, stores the result, and returns the QoSRecord.

**Request:**
```json
{ "host": "string", "port": "string" }
```

**Response: 200 OK:**
```json
{
  "id": "uuid", "host": "string", "port": "string",
  "latencyMs": 12, "measuredAt": "RFC3339", "reachable": true
}
```

### POST /deviceqosevaluator/quality-evaluation/mgmt/query

Returns stored QoS records, optionally filtered by host/port.

**Request:**
```json
{ "host": "string (optional)", "port": "string (optional)" }
```

**Response: 200 OK:**
```json
{ "records": [...], "count": N, "totalCount": N }
```

### GET /deviceqosevaluator/health

**Response: 200 OK:** `{"status": "UP"}`

---

## 10. TranslationManager — Port 8089

New system added in Step 37 (G36). Manages JSON field-remapping translation bridges.

**Configuration (env vars):**
- `PORT` — listen port. Default: `8089`.

### POST /translationmanager/translation/translate

Translates a JSON payload using the field mappings of the specified bridge.

**Request:**
```json
{ "bridgeId": "string", "payload": { ... } }
```

**Response: 200 OK:**
```json
{
  "bridgeId": "string",
  "originalPayload": { ... },
  "translatedPayload": { ... }
}
```

**Errors:** `404` if bridge not found.

### GET /translationmanager/translation/status/{bridgeId}

Returns the bridge configuration.

**Response: 200 OK:** Bridge object. `404` if not found.

### POST /translationmanager/translation/mgmt/bridges

Creates a new translation bridge.

**Request:**
```json
{
  "sourceFormat": "sensor-v1", "targetFormat": "sensor-v2",
  "fieldMappings": { "temperature": "temp" }
}
```

**Response: 201 Created:** Bridge object with generated `id`.

### GET /translationmanager/translation/mgmt/bridges

Returns all bridges.

**Response: 200 OK:** Array of bridge objects.

### DELETE /translationmanager/translation/mgmt/bridges/{id}

Deletes a bridge. **Response: 200 OK.** `404` if not found.

### GET /translationmanager/health

**Response: 200 OK:** `{"status": "UP"}`

---

## 11. MQTT Communication Profiles (G34)

Added in Step 38. The `core/internal/mqttutil` package provides `MQTTAdapter` which enables
any core system to subscribe to MQTT request topics and publish replies.

**Topic scheme:**
- Request: `ah5/<system>/request`
- Reply: `ah5/<system>/reply/<correlationId>`

**Request payload:**
```json
{ "path": "/health", "method": "GET", "correlationId": "abc", "body": "..." }
```

**Configuration (env vars):**
- `MQTT_BROKER_URL` — when set (e.g. `tcp://localhost:1883`), the system creates an `MQTTAdapter` and subscribes to its request topic. When empty, no MQTT listener is started.

**Interface name:** `MQTT-INSECURE-JSON` (defined as `mqttutil.MQTTInterfaceName`).

**qualityRequirements[] in OrchestrationRequest (G40):**

When `qualityRequirements[]` is present in a DynamicOrchestration pull request, the orchestrator
calls the Device QoS Evaluator for each candidate. Candidates are excluded if:
- The provider is unreachable (TCP probe fails), OR
- `latencyMs > maxLatencyMs` in any requirement

Fail-open: if the QoS evaluator is unreachable, the candidate is included.

