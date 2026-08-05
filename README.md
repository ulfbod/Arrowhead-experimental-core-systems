# Arrowhead 5.2 Core Systems + ADAPI Extensions

A Go reference implementation of the [Arrowhead Framework 5.2](https://aitia-iiot.github.io/ah5-docs-java-spring/) core systems, extended with **ADAPI** (Arrowhead Data-plane Authorization Policy Interfaces) — two typed gRPC interfaces that project control-plane authorization grants and PKI certificate state onto live data flows across AMQP, Kafka, and REST transports.

The repository serves as both a spec-compliant standalone implementation and a research platform for exploring policy-based IoT data-plane authorization.

**Conformance:** ~95-97% across all spec-defined systems (see [CONFORMANCE.md](CONFORMANCE.md)) | **Go 1.25+** | **License:** [MIT](LICENSE)

---

## Core Systems

Nine systems, each running as its own binary:

| System | Port | Description |
|---|---|---|
| ServiceRegistry | 8080 | Service registration and discovery |
| Authentication | 8081 | Identity tokens and credential verification |
| ConsumerAuthorization | 8082 | Authorization grants, tokens, and policy lookup |
| DynamicOrchestration | 8083 | Real-time service lookup with optional auth filtering |
| SimpleStoreOrchestration | 8084 | Rule-based orchestration |
| FlexibleStoreOrchestration | 8085 | Priority-based orchestration *(extension)* |
| CertificateAuthority | 8086 | X.509 certificate signing and revocation *(extension)* |
| DeviceQoSEvaluator | 8088 | TCP RTT/jitter/bandwidth probing |
| TranslationManager | 8089 | JSON field-remapping bridges |

All systems default to in-memory storage. Set `DB_PATH` for SQLite-backed persistence. See [CONFIGURATION.md](CONFIGURATION.md) for the full environment variable reference.

## ADAPI Extensions (core-evol/)

The [`core-evol/`](core-evol/) directory contains evolved core system variants that implement the ADAPI gRPC interfaces:

- **`authorize.proto`** — PEP-to-PDP interface for authorization decisions
- **`certlifecycle.proto`** — certificate lifecycle event stream from CA to all PEPs

These interfaces close the gap between Arrowhead's control-plane authorization and real-time data-plane enforcement. See [core-evol/README.md](core-evol/README.md).

---

## Prerequisites

- **Go 1.25+** (required by the `modernc.org/sqlite` dependency)
- **Docker** and **Docker Compose** (for running experiment stacks)
- **Node.js 18+** (only for dashboard development)

## Quick Start

### Run all core systems

```bash
cd core
go run ./cmd/serviceregistry      # :8080
go run ./cmd/authentication       # :8081
go run ./cmd/consumerauth         # :8082
go run ./cmd/dynamicorch          # :8083
go run ./cmd/simplestoreorch      # :8084
go run ./cmd/flexiblestoreorch    # :8085
go run ./cmd/ca                   # :8086
```

### Dashboard

```bash
# Development (hot-reload via Vite)
cd core/dashboard && npm install && npm run dev   # http://localhost:5173

# Production (served by ServiceRegistry)
cd core/dashboard && npm run build
cd core && go run ./cmd/serviceregistry           # http://localhost:8080/
```

## Build and Test

```bash
cd core
go build ./...
go test ./...
```

All tests are self-contained — no database, no running servers, no environment variables needed. See [core/TESTING.md](core/TESTING.md) for details.

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

## Experiments

Self-contained Docker Compose stacks that demonstrate the core systems in realistic IoT scenarios. Experiments 1-5 are historical reference; **experiment-6 is the active baseline**; experiments 7-14 build on it progressively.

| # | Focus | Description |
|---|---|---|
| [1](experiments/experiment-1/) | Browser demo | Interactive register/grant/orchestrate workflow |
| [2](experiments/experiment-2/) | AMQP data plane | Robot fleet publishes via RabbitMQ; edge-adapter bridges to orchestrated consumer |
| [3](experiments/experiment-3/) | Broker-level authz | Direct AMQP subscriptions with topic authorization sourced from ConsumerAuth |
| [4](experiments/experiment-4/) | Geo-distributed | Dual-layer authorization via `topic-auth-http` (live CA checks) + RabbitMQ user lifecycle |
| [5](experiments/experiment-5/) | XACML/ABAC | Unified policy projection across AMQP and Kafka via one AuthzForce PDP |
| **[6](experiments/experiment-6/)** | **Triple-transport** | **AMQP + Kafka + REST policy projection with configurable `SYNC_INTERVAL` (active baseline)** |
| [7](experiments/experiment-7/) | X.509/TLS | REST consumers identified by cert CN; mTLS across all transport paths |
| [8](experiments/experiment-8/) | AH5.2 PKI | Profile-based certificate hierarchy with compliance assessment |
| [9](experiments/experiment-9/) | UC3 scenario | Multi-site robot fleets; Portal and Cloud ML aggregation; mTLS REST proxy PEP |
| [10](experiments/experiment-10/) | PAP/PIP/PDP | Classical access-control architecture; eliminates sync delay |
| [11](experiments/experiment-11/) | Hybrid policy | Two policy sources merged into a single XACML PolicySet at push time |
| [12](experiments/experiment-12/) | gRPC PDP | DynamicOrchestration delegates to external XACML PDP via `authorize.proto` |
| [13](experiments/experiment-13/) | PKI identity | Cert CN as XACML subject on all paths; CertificateLifecycle gRPC stream populates PIP |
| [14](experiments/experiment-14/) | Connection-time revocation | Kafka and RabbitMQ reject revoked certs before the PDP is consulted |

### Run an experiment

```bash
cd experiments/experiment-6
docker compose up --build        # Dashboard at http://localhost:3006
```

---

## Project Structure

```
core/           Arrowhead 5.2 core systems (strict, spec-compliant)
core-evol/      ADAPI extensions — gRPC PDP and cert lifecycle interfaces
support/        Shared modules: PEPs, policy-sync, AuthzForce client, AMQP wrapper
experiments/    Self-contained Docker Compose experiment stacks
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full structural overview, API surface, and directory tree.

---

## Publications

**[AIMS 5.0 Poster — UC03 ADAPI](AIMS5.0_Poster_UC03%20ADAPI%20v10.pdf)** — ADAPI extends Arrowhead authorization to the data plane via two typed gRPC interfaces that project control-plane grants and PKI certificate state onto live AMQP, Kafka, and REST flows.

**Contact:** Ulf Bodin and Olov Schelen, LTU — {ulf.bodin, olov.schelen}@ltu.se

---

## Reference

| Document | Description |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | System diagram, API surface, directory tree |
| [CONFIGURATION.md](CONFIGURATION.md) | Environment variable reference for all systems |
| [CONFORMANCE.md](CONFORMANCE.md) | AH5 conformance assessment with per-system ratings |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to report issues and submit changes |
| [core/SPEC.md](core/SPEC.md) | Complete API specification |
| [core/GAP_ANALYSIS.md](core/GAP_ANALYSIS.md) | AH5 compliance notes and design decisions |
| [core/DIAGRAMS.md](core/DIAGRAMS.md) | Mermaid architecture and sequence diagrams |
| [core/TESTING.md](core/TESTING.md) | Test guide and techniques |
| [support/README.md](support/README.md) | Support module overview and deployment reference |
| [AH5 Documentation](https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/) | Official Arrowhead 5 specification |
