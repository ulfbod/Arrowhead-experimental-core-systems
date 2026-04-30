# Experiment 2 — Virtual Arrowhead Local Cloud with AMQP Data Plane

This experiment builds a complete virtual Arrowhead local cloud where three
services interact through both the Arrowhead core systems (for service
discovery and orchestration) and a RabbitMQ message broker (for the data
plane).

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Arrowhead Core (ports 8080–8086)                       │
│                                                         │
│  ServiceRegistry  Authentication  ConsumerAuthorization │
│       8080             8081              8082           │
│                                                         │
│  DynamicOrchestration          CertificateAuthority     │
│        8083                          8086               │
└────────────────────────┬────────────────────────────────┘
                         │ Arrowhead APIs (HTTP)
          ┌──────────────┼──────────────────────┐
          │              │                      │
          ▼              ▼                      ▼
  ┌───────────────┐ ┌──────────────┐  ┌─────────────────┐
  │ robot-        │ │ edge-adapter │  │    consumer     │
  │ simulator     │ │   :9001      │  │                 │
  │               │ │              │  │  orchestrates   │
  │ publishes     │ │ subscribes   │  │  via Orch →     │
  │ telemetry     │ │ to AMQP      │  │  polls          │
  │ every second  │ │ registers    │  │  /telemetry/    │
  └───────┬───────┘ │ with SR      │  │  latest         │
          │         └──────┬───────┘  └─────────────────┘
          │   AMQP          │ HTTP
          ▼                 ▼
  ┌──────────────────────────────┐
  │        RabbitMQ :5672        │
  │  exchange: arrowhead         │
  │  key:      telemetry.robot   │
  └──────────────────────────────┘
```

### Services

| Service | Role |
|---|---|
| **robot-simulator** | Publishes synthetic temperature/humidity telemetry to the AMQP exchange every second |
| **edge-adapter** | Bridges AMQP → HTTP: subscribes to `telemetry.#`, registers with ServiceRegistry, serves `GET /telemetry/latest` |
| **consumer** | Uses DynamicOrchestration for late binding to discover the telemetry service, then polls it every 5 seconds |

### Core systems used

| System | Used by |
|---|---|
| ServiceRegistry | edge-adapter registers; DynamicOrchestration queries |
| DynamicOrchestration | consumer orchestrates to find telemetry endpoint |
| CertificateAuthority | edge-adapter obtains a certificate at startup |

---

## Quick Start

### With Docker Compose (recommended)

```bash
cd experiments/experiment-2
docker compose up --build
```

The consumer will start logging orchestrated telemetry within ~15 seconds once
all services are healthy.  Watch with:

```bash
docker compose logs -f consumer
```

To stop:

```bash
docker compose down
```

### Running locally (without Docker)

Start RabbitMQ (e.g. via Docker):
```bash
docker run -d --name rabbitmq -p 5672:5672 rabbitmq:3.13-alpine
```

Start the Arrowhead core systems from `core/`:
```bash
go run ./cmd/serviceregistry &    # :8080
go run ./cmd/consumerauth &       # :8082
go run ./cmd/dynamicorch \
  ENABLE_AUTH=true &              # :8083
go run ./cmd/ca &                 # :8086
```

Add a ConsumerAuthorization rule so the consumer can access telemetry:
```bash
curl -X POST http://localhost:8082/authorization/grant \
  -H "Content-Type: application/json" \
  -d '{"consumerSystemName":"demo-consumer","providerSystemName":"edge-adapter","serviceDefinition":"telemetry"}'
```

Start the experiment services (each in a separate terminal from the repo root):
```bash
cd experiments/experiment-2/services/robot-simulator
AMQP_URL=amqp://guest:guest@localhost:5672/ go run .

cd experiments/experiment-2/services/edge-adapter
AMQP_URL=amqp://guest:guest@localhost:5672/ go run .

cd experiments/experiment-2/services/consumer
go run .
```

---

## Tests

### Unit tests (no external services required)

```bash
cd experiments/experiment-2/tests
go test ./...
```

These test:
- Broker rejects empty/unreachable URLs
- Telemetry JSON payload round-trip
- Orchestration request JSON shape

### Integration tests (require RabbitMQ)

```bash
cd experiments/experiment-2/tests
AMQP_URL=amqp://guest:guest@localhost:5672/ go test ./...
```

These additionally test:
- Full publish/subscribe round-trip
- Wildcard routing key `telemetry.#` matching

---

## Key Concepts Demonstrated

### Late binding via DynamicOrchestration

The consumer does not hard-code the telemetry endpoint. On every poll cycle it
calls `POST /orchestration/dynamic` to discover the current provider. If the
edge-adapter restarts on a different port, the consumer will automatically find
the new endpoint on the next cycle — no reconfiguration needed.

### AMQP data plane + Arrowhead control plane

Data (telemetry) flows through RabbitMQ independently of the Arrowhead
systems. Arrowhead is used only for the control plane: service registration and
discovery. This reflects a realistic IoT architecture where high-frequency
sensor data bypasses the orchestration layer.

### Certificate Authority

The edge-adapter obtains a certificate from the CA at startup, demonstrating
how the CA integrates into the onboarding flow. The certificate is currently
used for logging only; in a production deployment it would be presented in
mutual TLS connections to other systems.

### ConsumerAuthorization

DynamicOrchestration is run with `ENABLE_AUTH=true` in the Docker Compose
setup. The consumer must have an authorization grant in ConsumerAuthorization
before the orchestrator returns the edge-adapter's endpoint. The `init` step
in `docker-compose.yml` pre-populates this grant.

---

## Directory Structure

```
experiment-2/
├── docker-compose.yml
├── dockerfiles/
│   ├── core.Dockerfile          # shared Dockerfile for all core binaries
│   ├── robot-simulator.Dockerfile
│   ├── edge-adapter.Dockerfile
│   └── consumer.Dockerfile
├── services/
│   ├── robot-simulator/         # Go module: publishes AMQP telemetry
│   ├── edge-adapter/            # Go module: AMQP subscriber + HTTP server
│   └── consumer/                # Go module: orchestrated telemetry consumer
└── tests/                       # Go module: unit + integration tests
```

The shared message broker library lives at `support/message-broker/` (repo
root) and is referenced via `replace` directives in the service `go.mod` files.
