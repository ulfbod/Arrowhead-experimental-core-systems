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

Self-contained scenarios that demonstrate the core systems in realistic settings.

| Experiment | Description |
|---|---|
| [experiment-1](experiments/experiment-1/) | Interactive browser demo: register services, grant authorization, orchestrate |
| [experiment-2](experiments/experiment-2/) | Virtual local cloud with AMQP data plane: robot → RabbitMQ → edge-adapter → orchestrated consumer |
| [experiment-3](experiments/experiment-3/) | Direct AMQP subscriptions with broker-level topic authorization sourced from ConsumerAuth |
| [experiment-4](experiments/experiment-4/) | Geo-distributed consumers over AMQP: dual-layer authorization via topic-auth-http (live CA checks) + RabbitMQ user lifecycle management |
| [experiment-5](experiments/experiment-5/) | Unified XACML/ABAC policy projection across AMQP and Kafka: one AuthzForce PDP governs both transports; revocation propagates to all PEPs within one sync cycle |

### Experiment 4 quick start

```bash
cd experiments/experiment-4
docker compose up --build
```

Three AMQP consumers connect via full AHC orchestration flow. `topic-auth-http` enforces RabbitMQ topic authorization using live ConsumerAuth checks and manages the RabbitMQ user lifecycle. Full details in [experiments/experiment-4/README.md](experiments/experiment-4/README.md).

### Experiment 5 quick start

```bash
cd experiments/experiment-5
docker compose up --build
```

`robot-fleet` publishes telemetry to both RabbitMQ (AMQP) and Kafka simultaneously. `policy-sync` compiles ConsumerAuth grants into a XACML 3.0 PolicySet and uploads it to AuthzForce. Both `topic-auth-xacml` (AMQP PEP) and `kafka-authz` (Kafka SSE PEP) evaluate the same policy — revoking a grant propagates to all transports within one sync cycle. Open the dashboard at **http://localhost:3005**. Full details in [experiments/experiment-5/README.md](experiments/experiment-5/README.md).

---

## Reference

- [ARCHITECTURE.md](ARCHITECTURE.md) — structural overview and inter-system communication
- [core/DIAGRAMS.md](core/DIAGRAMS.md) — Mermaid architecture and sequence diagrams
- [core/SPEC.md](core/SPEC.md) — complete API specification for all six systems
- [core/TEST_PLAN.md](core/TEST_PLAN.md) — test scenarios and coverage per system
- [core/TESTING.md](core/TESTING.md) — how to run tests, key techniques, known limitations
- [core/GAP_ANALYSIS.md](core/GAP_ANALYSIS.md) — AH5 compliance notes and design decisions
- [Arrowhead 5 Documentation](https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/)
