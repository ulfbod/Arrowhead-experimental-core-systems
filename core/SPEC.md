# Arrowhead Core Systems Specification

## Overview

This specification covers the seven Arrowhead core systems implemented in this repository.
The first six follow Arrowhead 5 (AH5); the seventh (CA) is an extension added for experiment-2.

| System | Port | Path prefix |
|---|---|---|
| ServiceRegistry | 8080 | `/serviceregistry` |
| Authentication | 8081 | `/authentication` |
| ConsumerAuthorization | 8082 | `/authorization` |
| DynamicOrchestration | 8083 | `/orchestration/dynamic` |
| SimpleStoreOrchestration | 8084 | `/orchestration/simplestore` |
| FlexibleStoreOrchestration | 8085 | `/orchestration/flexiblestore` |
| CertificateAuthority | 8086 | `/ca` |

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

#### DELETE /serviceregistry/system-discovery/revoke?name={name}

Removes the named system.

**Note (G10):** AH5 identifies the system from the auth token. This implementation
uses a `?name=` query parameter.

**Response:**
- `200 OK` — system removed
- `204 No Content` — no matching system found
- `400 Bad Request` — `name` parameter missing

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
  "interfaces":            []
}
```

**Note (G10):** The AH5 spec derives `systemName` from the auth token. This
implementation requires it in the request body.

**Response:**
- `201 Created` — newly registered
- `200 OK` — existing instance updated

**Validation errors (400):** `systemName` or `serviceDefinitionName` empty.

---

#### POST /serviceregistry/service-discovery/lookup

Returns service instances matching optional filter criteria.

**Request (all fields optional):**
```json
{
  "instanceIds":            [],
  "providerNames":          [],
  "serviceDefinitionNames": [],
  "versions":               [],
  "interfaceTemplateNames": []
}
```

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
1. If `ENABLE_IDENTITY_CHECK=true`: validates the `Authorization: Bearer <token>` header against the Authentication system. Returns `401 Unauthorized` if the token is absent, invalid, or expired. The verified `systemName` from the token replaces the self-reported `requesterSystem.systemName` for all subsequent checks.
2. Calls `POST /serviceregistry/query` with `requestedService` as filters.
3. If `ENABLE_AUTH=true`, calls `POST /authorization/verify` for each result and removes unauthorized providers.
4. Returns the remaining results.

**Configuration (env vars):**
- `SERVICE_REGISTRY_URL` — default `http://localhost:8080`
- `CONSUMER_AUTH_URL` — default `http://localhost:8082`
- `AUTH_SYSTEM_URL` — default `http://localhost:8081`
- `ENABLE_AUTH` — `true`/`false`, default `false`
- `ENABLE_IDENTITY_CHECK` — `true`/`false`, default `false`. When `true`, requires a valid Bearer token issued by the Authentication system. The verified identity overrides the self-reported `requesterSystem.systemName`, preventing impersonation.

**Note:** `ENABLE_IDENTITY_CHECK` goes beyond the AH5 specification. See `GAP_ANALYSIS.md` for rationale and design decisions.

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

