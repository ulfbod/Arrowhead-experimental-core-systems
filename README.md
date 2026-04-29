# Arrowhead Core

A Go implementation of the six [Arrowhead 5](https://aitia-iiot.github.io/ah5-docs-java-spring/) core systems, with a built-in browser dashboard.

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
| `ENABLE_AUTH` | DynamicOrchestration | `false` | Enable auth check |

---

## Reference

- [ARCHITECTURE.md](ARCHITECTURE.md) — structural overview and inter-system communication
- [DIAGRAMS.md](DIAGRAMS.md) — Mermaid architecture and sequence diagrams
- [core/SPEC.md](core/SPEC.md) — complete API specification for all six systems
- [core/GAP_ANALYSIS.md](core/GAP_ANALYSIS.md) — AH5 compliance notes and design decisions
- [Arrowhead 5 Documentation](https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/)
