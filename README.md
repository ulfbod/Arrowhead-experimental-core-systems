# Arrowhead Core

A Go implementation of the six [Arrowhead 5](https://aitia-iiot.github.io/ah5-docs-java-spring/) core systems plus a Certificate Authority extension, with a built-in browser dashboard.  Experiments demonstrate the core systems in realistic scenarios.

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full structural overview.

---

## Core Systems

| System | Port | Binary |
|---|---|---|
| ServiceRegistry | 8080 | `cmd/serviceregistry` |
| Authentication | 8081 | `cmd/authentication` |
| ConsumerAuthorization | 8082 | `cmd/consumerauth` |
| DynamicOrchestration | 8083 | `cmd/dynamicorch` |
| SimpleStoreOrchestration | 8084 | `cmd/simplestoreorch` |
| FlexibleStoreOrchestration | 8085 | `cmd/flexiblestoreorch` |
| CertificateAuthority | 8086 | `cmd/ca` *(extension, not in AH5 spec)* |
| DeviceQoSEvaluator | 8088 | `cmd/deviceqoseval` *(Phase 3 — G35)* |
| TranslationManager | 8089 | `cmd/translationmgr` *(Phase 3 — G36)* |

All systems default to in-memory storage. Set `DB_PATH` to a file path for SQLite-backed persistence across restarts (see [Configuration](#configuration)).

---

## Quick Start

### Run all systems

Open six terminals (or use a process manager):

```bash
cd core
go run ./cmd/serviceregistry      # :8080
go run ./cmd/authentication       # :8081
go run ./cmd/consumerauth         # :8082
go run ./cmd/dynamicorch          # :8083
go run ./cmd/simplestoreorch      # :8084
go run ./cmd/flexiblestoreorch    # :8085
go run ./cmd/ca                   # :8086  (CA extension)
go run ./cmd/deviceqoseval        # :8088  (Device QoS Evaluator — Phase 3)
go run ./cmd/translationmgr       # :8089  (Translation Manager — Phase 3)
```

### Dashboard (development mode)

```bash
cd core/dashboard
npm install
npm run dev   # http://localhost:5173
```

The dashboard proxies all API calls through Vite to the running backends — no CORS required. It shows live health status for all six systems and provides panels for ServiceRegistry, ConsumerAuthorization, and Orchestration.

### Dashboard (production — served by ServiceRegistry binary)

```bash
cd core/dashboard && npm run build
cd core && go run ./cmd/serviceregistry
# Dashboard available at http://localhost:8080/
```

---

## Build & Test

```bash
cd core
go build ./...
go test ./...
```

All tests are self-contained — no database, no running servers, no environment variables needed. See [core/TESTING.md](core/TESTING.md) for the full test guide.

---

## Example Workflow

### 1. Register a service

```bash
curl -s -X POST http://localhost:8080/serviceregistry/register \
  -H 'Content-Type: application/json' \
  -d '{
    "serviceDefinition": "temperature-service",
    "providerSystem": { "systemName": "sensor-1", "address": "192.168.0.10", "port": 9001 },
    "serviceUri": "/temperature",
    "interfaces": ["HTTP-INSECURE-JSON"],
    "version": 1,
    "metadata": { "unit": "celsius" }
  }'
```

### 2. Grant authorization

```bash
curl -s -X POST http://localhost:8082/consumerauthorization/authorization/grant \
  -H 'Content-Type: application/json' \
  -d '{
    "consumerSystemName": "consumer-app",
    "providerSystemName": "sensor-1",
    "serviceDefinition":  "temperature-service"
  }'
```

### 3. Orchestrate dynamically (with auth check)

```bash
ENABLE_AUTH=true go run ./cmd/dynamicorch &

curl -s -X POST http://localhost:8083/serviceorchestration/orchestration/pull \
  -H 'Content-Type: application/json' \
  -d '{
    "requesterSystem":   { "systemName": "consumer-app", "address": "localhost", "port": 0 },
    "requestedService":  { "serviceDefinition": "temperature-service" },
    "orchestrationFlags": {}
  }'
```

### 4. Revoke authorization

```bash
curl -s -X DELETE http://localhost:8082/consumerauthorization/authorization/revoke/1
```

---

## Configuration

Each binary reads configuration from environment variables.

### Listen ports and persistence

| Variable | System | Default | Description |
|---|---|---|---|
| `PORT` | all | 8080–8086 (see table above) | HTTP listen port |
| `DB_PATH` | all | *(unset)* | Storage backend: unset = in-memory, `:memory:` = SQLite in-memory, file path = SQLite file-backed persistence |

The ServiceRegistry creates **two** SQLite files when `DB_PATH` is set: the given path (legacy registrations) and `<DB_PATH>.ah5` (AH5 device/system/service discovery records).

### Mutual TLS (optional)

All systems except the CA support an optional HTTPS listener alongside the plain HTTP one:

| Variable | Default | Description |
|---|---|---|
| `TLS_PORT` | *(unset)* | When set, starts an HTTPS listener on this port |
| `TLS_CERT_FILE` | *(required with TLS_PORT)* | PEM certificate file |
| `TLS_KEY_FILE` | *(required with TLS_PORT)* | PEM private key file |
| `TLS_CA_FILE` | *(optional)* | PEM CA certificate; when set, enforces mutual TLS (`RequireAndVerifyClientCert`) |

### Management access policy (all systems)

| Variable | Default | Description |
|---|---|---|
| `MGMT_AUTH_URL` | *(unset)* | When set, all `/mgmt/*` endpoints on every system require `Authorization: Bearer <token>` with `sysop: true`. Unset = open management (development mode). |

### ServiceRegistry — registration identity enforcement (Phase 3 / G10)

| Variable | Default | Description |
|---|---|---|
| `REGISTER_AUTH_URL` | *(unset)* | When set, system and service registration require `Authorization: Bearer <token>` whose verified `systemName` matches the `name`/`systemName` in the request body. Fail-closed: missing token → 401; network error → 401; name mismatch → 403. Unset = open registration (development mode). |

### ConsumerAuthorization — BASE64_SELF_CONTAINED tokens (Phase 3 / G23)

| Variable | Default | Description |
|---|---|---|
| `HMAC_SECRET` | `arrowhead-default-secret` | Secret used to sign `BASE64_SELF_CONTAINED` tokens (HMAC-SHA256). Set to a strong random value in production. |

### Blacklist integration (ServiceRegistry, ConsumerAuthorization, DynamicOrchestration, CertificateAuthority)

| Variable | Default | Description |
|---|---|---|
| `BLACKLIST_URL` | *(unset)* | When set, blacklisted systems are rejected at register/grant/orchestration/sign. Fail-closed: Blacklist unreachable is treated as blacklisted. |

### DynamicOrchestration

| Variable | Default | Description |
|---|---|---|
| `SERVICE_REGISTRY_URL` | `http://localhost:8080` | ServiceRegistry base URL |
| `CONSUMER_AUTH_URL` | `http://localhost:8082` | ConsumerAuthorization base URL |
| `AUTH_SYSTEM_URL` | `http://localhost:8081` | Authentication system base URL |
| `ENABLE_AUTH` | `false` | Filter providers via ConsumerAuthorization |
| `ENABLE_IDENTITY_CHECK` | `false` | Require a valid Bearer token; use verified identity for auth checks |
| `PUSH_DELIVERY_TIMEOUT_SECONDS` | `5` | HTTP timeout (seconds) for each push notification delivery attempt via `mgmt/push/trigger` |
| `QOS_EVALUATOR_URL` | *(unset)* | When set, DynamicOrchestration performs TCP RTT probes via the Device QoS Evaluator for candidates when `qualityRequirements[]` is present. Fail-open: evaluator unreachable → candidate included. |

### MQTT (Phase 3 / G34)

| Variable | Default | Description |
|---|---|---|
| `MQTT_BROKER_URL` | *(unset)* | When set (e.g. `tcp://localhost:1883`), the system subscribes to `ah5/<system>/request` and publishes replies to `ah5/<system>/reply/<correlationId>`. When empty, no MQTT listener is started. |

`ENABLE_IDENTITY_CHECK` connects Authentication and DynamicOrchestration: consumers must log in first and present their token when orchestrating. The verified `systemName` from the token replaces the self-reported value in the request body, preventing impersonation. See `core/GAP_ANALYSIS.md` (D8) for full design rationale.

### ServiceRegistry

| Variable | Default | Description |
|---|---|---|
| `SR_AUTH_URL` | `http://localhost:8081` | Authentication system base URL used to verify Bearer tokens on `DELETE /system-discovery/revoke` |

### Authentication

| Variable | Default | Description |
|---|---|---|
| `TOKEN_DURATION_SECONDS` | `3600` | Token lifetime |

### Blacklist

| Variable | Default | Description |
|---|---|---|
| `BLACKLIST_AUTH_URL` | *(unset)* | When set, `GET /blacklist/lookup` and `GET /blacklist/check/{name}` require `Authorization: Bearer <token>`. Unset = open access (development mode). |

### Deployment note — multi-host and DHCP environments

The defaults for `SERVICE_REGISTRY_URL`, `CONSUMER_AUTH_URL`, and `AUTH_SYSTEM_URL` all point to `localhost`. These work for a single-machine development setup where all binaries run on the same host. In any other deployment — multiple VMs, containers on separate hosts, or a DHCP network — **all three must be set explicitly** to the address where each system is reachable.

There are no hardcoded IP addresses in the source code. Every network address is read from an environment variable at startup.

---

## Experiments

Self-contained Docker Compose stacks that demonstrate the core systems in realistic scenarios. Experiments 1–5 are historical reference; **experiment-6 is the active baseline**; experiments 7–14 build on it progressively.

| Experiment | Description |
|---|---|
| [experiment-1](experiments/experiment-1/) | Interactive browser demo: register services, grant authorization, orchestrate |
| [experiment-2](experiments/experiment-2/) | Virtual local cloud with AMQP data plane: robot → RabbitMQ → edge-adapter → orchestrated consumer |
| [experiment-3](experiments/experiment-3/) | Direct AMQP subscriptions with broker-level topic authorization sourced from ConsumerAuth |
| [experiment-4](experiments/experiment-4/) | Geo-distributed consumers over AMQP: dual-layer authorization via `topic-auth-http` (live CA checks) + RabbitMQ user lifecycle management |
| [experiment-5](experiments/experiment-5/) | Unified XACML/ABAC policy projection across AMQP and Kafka: one AuthzForce PDP governs both transports; revocation propagates to all PEPs within one sync cycle |
| [experiment-6](experiments/experiment-6/) | Triple-transport policy projection (AMQP + Kafka + REST) with runtime-configurable `SYNC_INTERVAL`; active baseline for all later experiments |
| [experiment-7](experiments/experiment-7/) | X.509/TLS extension: REST consumers identified by cert CN; mTLS across all transport paths |
| [experiment-8](experiments/experiment-8/) | Arrowhead 5.2 profile-based PKI with enforced certificate hierarchy and compliance assessment |
| [experiment-9](experiments/experiment-9/) | UC3 "Lawn Mowing as a Service": multi-site robot fleets publish over Kafka + AMQP; Portal & Cloud ML aggregates streams; Service Partners consume via mTLS REST proxy PEP |
| [experiment-10](experiments/experiment-10/) | UC3 with classical PAP/PIP/PDP access-control architecture; eliminates sync delay by separating policy administration, information, and decision points |
| [experiment-11](experiments/experiment-11/) | Hybrid PAP/PIP/PDP (Strategy A): two policy sources merged into a single XACML PolicySet at push time |
| [experiment-12](experiments/experiment-12/) | DynamicOrchestration-XACML (Approach B): gRPC PDP interface replaces ConsumerAuthorization for orchestration decisions |
| [experiment-13](experiments/experiment-13/) | PKI identity unification: cert CN as XACML subject on all paths; cert-level ABAC attributes; CertificateLifecycle gRPC stream auto-populates PIP |
| [experiment-14](experiments/experiment-14/) | Connection-time certificate revocation: Kafka `ArrowheadPrincipalBuilder` plugin and RabbitMQ `topic-auth-xacml` pre-gate both reject revoked clients before the PDP is consulted |

### Experiment 6 quick start (active baseline)

```bash
cd experiments/experiment-6
docker compose up --build
```

Triple-transport authorization: a single ConsumerAuth grant propagates to AMQP (`topic-auth-xacml`), Kafka (`kafka-authz`), and REST (`rest-authz`) within one policy-sync cycle. Dashboard at **http://localhost:3006**. Full details in [experiments/experiment-6/README.md](experiments/experiment-6/README.md).

### Experiment 13 quick start

```bash
cd experiments/experiment-13
docker compose up --build -d
```

Three robot-fleet sites publish telemetry. PKI identity is unified: every client's cert CN becomes the XACML `subject-id` on Kafka, AMQP, and REST paths. Cert-level attributes (`certLevel`, `certValid`) are injected as XACML subject attributes. Dashboard at **http://localhost:3013**. Full details in [experiments/experiment-13/README.md](experiments/experiment-13/README.md).

### Experiment 14 quick start

```bash
cd experiments/experiment-14
docker compose up --build -d
```

Extends experiment-13 with fail-closed connection-time revocation: a Java `KafkaPrincipalBuilder` plugin rejects Kafka connections from revoked clients at the TLS handshake; `topic-auth-xacml` rejects AMQP connections at the `handleUser`/`handleVhost` stage, before AuthzForce is ever called. Dashboard at **http://localhost:3014**. Full details in [experiments/experiment-14/README.md](experiments/experiment-14/README.md).

---

## Reference

- [ARCHITECTURE.md](ARCHITECTURE.md) — structural overview and inter-system communication
- [core/DIAGRAMS.md](core/DIAGRAMS.md) — Mermaid architecture and sequence diagrams
- [core/SPEC.md](core/SPEC.md) — complete API specification for all six systems
- [core/TEST_PLAN.md](core/TEST_PLAN.md) — test scenarios and coverage per system
- [core/TESTING.md](core/TESTING.md) — how to run tests, key techniques, known limitations
- [core/GAP_ANALYSIS.md](core/GAP_ANALYSIS.md) — AH5 compliance notes and design decisions
- [Arrowhead 5 Documentation](https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/)
