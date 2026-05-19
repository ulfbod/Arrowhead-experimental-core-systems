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

All systems are in-memory, stateless across restarts, and independently runnable.

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
curl -s -X POST http://localhost:8082/authorization/grant \
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

curl -s -X POST http://localhost:8083/orchestration/dynamic \
  -H 'Content-Type: application/json' \
  -d '{
    "requesterSystem":  { "systemName": "consumer-app", "address": "localhost", "port": 0 },
    "requestedService": { "serviceDefinition": "temperature-service" }
  }'
```

### 4. Revoke authorization

```bash
curl -s -X DELETE http://localhost:8082/authorization/revoke/1
```

---

## Configuration

Each binary reads configuration from environment variables:

| Variable | System | Default | Description |
|---|---|---|---|
| `PORT` | all | (see table above) | Listening port |
| `TOKEN_DURATION_SECONDS` | Authentication | `3600` | Token lifetime |
| `SERVICE_REGISTRY_URL` | DynamicOrchestration | `http://localhost:8080` | SR base URL |
| `CONSUMER_AUTH_URL` | DynamicOrchestration | `http://localhost:8082` | ConsumerAuth base URL |
| `AUTH_SYSTEM_URL` | DynamicOrchestration | `http://localhost:8081` | Authentication system base URL |
| `ENABLE_AUTH` | DynamicOrchestration | `false` | Filter providers via ConsumerAuthorization |
| `ENABLE_IDENTITY_CHECK` | DynamicOrchestration | `false` | Require a valid Bearer token; use verified identity for auth checks |

`ENABLE_IDENTITY_CHECK` connects the Authentication and DynamicOrchestration systems: consumers must log in first, then present their token when orchestrating. The verified `systemName` from the token replaces the self-reported value in the request body, preventing impersonation. See `core/GAP_ANALYSIS.md` (D8) for full design rationale.

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
