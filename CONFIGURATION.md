# Configuration Reference

Each core system binary reads configuration from environment variables. All variables are optional — defaults are suitable for single-machine development.

## Listen Ports and Persistence

| Variable | System | Default | Description |
|---|---|---|---|
| `PORT` | all | 8080-8086 (see [README](README.md)) | HTTP listen port |
| `DB_PATH` | all | *(unset)* | Storage backend: unset = in-memory, `:memory:` = SQLite in-memory, file path = SQLite file-backed persistence |

The ServiceRegistry creates **two** SQLite files when `DB_PATH` is set: the given path (legacy registrations) and `<DB_PATH>.ah5` (AH5 device/system/service discovery records).

## Mutual TLS

All systems except the CA support an optional HTTPS listener alongside the plain HTTP one:

| Variable | Default | Description |
|---|---|---|
| `TLS_PORT` | *(unset)* | When set, starts an HTTPS listener on this port |
| `TLS_CERT_FILE` | *(required with TLS_PORT)* | PEM certificate file |
| `TLS_KEY_FILE` | *(required with TLS_PORT)* | PEM private key file |
| `TLS_CA_FILE` | *(optional)* | PEM CA certificate; when set, enforces mutual TLS (`RequireAndVerifyClientCert`) |

## Management Access Policy

| Variable | Default | Description |
|---|---|---|
| `MGMT_AUTH_URL` | *(unset)* | When set, all `/mgmt/*` endpoints require `Authorization: Bearer <token>` with `sysop: true`. Unset = open management (development mode). |

## ServiceRegistry

| Variable | Default | Description |
|---|---|---|
| `SR_AUTH_URL` | `http://localhost:8081` | Authentication system URL for verifying Bearer tokens on `DELETE /system-discovery/revoke` |
| `REGISTER_AUTH_URL` | *(unset)* | When set, registration requires Bearer token whose verified `systemName` matches the request body. Fail-closed. |

## Authentication

| Variable | Default | Description |
|---|---|---|
| `TOKEN_DURATION_SECONDS` | `3600` | Token lifetime |

## ConsumerAuthorization

| Variable | Default | Description |
|---|---|---|
| `HMAC_SECRET` | `arrowhead-default-secret` | Secret for `BASE64_SELF_CONTAINED` tokens (HMAC-SHA256). Set to a strong random value in production. |

## DynamicOrchestration

| Variable | Default | Description |
|---|---|---|
| `SERVICE_REGISTRY_URL` | `http://localhost:8080` | ServiceRegistry base URL |
| `CONSUMER_AUTH_URL` | `http://localhost:8082` | ConsumerAuthorization base URL |
| `AUTH_SYSTEM_URL` | `http://localhost:8081` | Authentication system base URL |
| `ENABLE_AUTH` | `false` | Filter providers via ConsumerAuthorization |
| `ENABLE_IDENTITY_CHECK` | `false` | Require a valid Bearer token; use verified identity for auth checks |
| `PUSH_DELIVERY_TIMEOUT_SECONDS` | `5` | HTTP timeout (seconds) for push notification delivery via `mgmt/push/trigger` |
| `QOS_EVALUATOR_URL` | *(unset)* | When set, performs TCP RTT probes via the Device QoS Evaluator when `qualityRequirements[]` is present. Fail-open. |
| `RELAY_TOKENS` | `false` | When `true`, embeds authorization tokens in `OrchestrationResult` after each successful orchestration |

`ENABLE_IDENTITY_CHECK` connects Authentication and DynamicOrchestration: consumers must log in first and present their token when orchestrating. The verified `systemName` from the token replaces the self-reported value in the request body, preventing impersonation. See [core/GAP_ANALYSIS.md](core/GAP_ANALYSIS.md) (D8) for the full design rationale.

## Blacklist

| Variable | Default | Description |
|---|---|---|
| `BLACKLIST_URL` | *(unset)* | When set, blacklisted systems are rejected at register/grant/orchestration/sign. Fail-closed: unreachable = blacklisted. |
| `BLACKLIST_AUTH_URL` | *(unset)* | When set, blacklist lookup endpoints require Bearer token authentication. |

## MQTT (Phase 3)

| Variable | Default | Description |
|---|---|---|
| `MQTT_BROKER_URL` | *(unset)* | When set (e.g. `tcp://localhost:1883`), subscribes to `ah5/<system>/request` and publishes replies to `ah5/<system>/reply/<correlationId>`. |

## Deployment Notes

The defaults for `SERVICE_REGISTRY_URL`, `CONSUMER_AUTH_URL`, and `AUTH_SYSTEM_URL` all point to `localhost`. In any multi-host deployment (multiple VMs, containers on separate hosts, DHCP), **all three must be set explicitly** to the address where each system is reachable.

There are no hardcoded IP addresses in the source code. Every network address is read from an environment variable at startup.
