# Arrowhead Core – Service Registry

Minimal Arrowhead Core System – Service Registry implementation in Go.

## What Is Implemented

This repository contains a self-contained implementation of the **Arrowhead Core Service Registry** system, along with the Authorization and Orchestration core systems that the Service Registry depends on for a complete service-of-services pattern.

### Core systems

| System | Endpoints | Description |
|--------|-----------|-------------|
| **Service Registry** | `POST /registry/register`<br>`DELETE /registry/register?id=<id>`<br>`GET /registry`<br>`GET /registry/query` | Register, unregister, list, and query services by type or capability |
| **Authorization** | `POST /authorization/check`<br>`POST /authorization/policy`<br>`DELETE /authorization/policy?id=<id>`<br>`GET /authorization/policies` | Policy-based consumer/provider access control |
| **Orchestration** | `POST /orchestration`<br>`GET /orchestration/logs` | Discover an authorized provider for a requested service capability |
| **Health** | `GET /health` | Liveness probe |

### Key features

- Register services with ID, address, port, type, capabilities, and metadata
- Query the registry by service type or capability
- Authorization policies with wildcard support (`*`) for consumer, provider, and service name
- Orchestration finds the first authorized provider for a requested capability and returns an auth token and endpoint URL
- In-memory log of the last 200 orchestration decisions
- CORS headers on all responses (suitable for browser clients)
- Configurable port via `PORT` environment variable (default `8000`)

## What Was Removed

The following components were part of the original research prototype and have been removed:

- **Individual Digital Twins (iDT)**: robot, gas sensor, LHD vehicle, tele-remote operator services
- **Composite Digital Twins (cDT)**: mapping, gas monitoring, hazard detection, material handling, tele-remote intervention, and two upper-layer mission cDTs
- **Scenario orchestrator**: experiment runner that exercised the full digital-twin stack
- **React frontend**: web UI for monitoring the digital-twin system
- **Python experiment scripts**: QoS trade-off, degradation, failover, and uncertainty-simulation experiments
- **Experiment results and figures**: CSV data, aggregated plots, and publication PDFs
- **Runtime logs**: generated log files from experiment runs
- Unused common library code: HTTP client, provider-selection logic, QoS metrics, QoS logging, and all domain types unrelated to the core systems (machine states, gas levels, hazard reports, mission phases, etc.)

## Build and Run

### Prerequisites

- Go 1.21+ (for local build)
- Docker and Docker Compose (for containerised run)

### Local

```bash
cd backend
go build ./cmd/arrowhead
./arrowhead            # listens on :8000 by default
PORT=9000 ./arrowhead  # custom port
```

### Docker Compose

```bash
docker compose up --build
```

The service will be available at `http://localhost:8000`.

## API Quick Reference

### Register a service

```bash
curl -X POST http://localhost:8000/registry/register \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-sensor-1",
    "name": "Temperature Sensor 1",
    "address": "192.168.1.10",
    "port": 9001,
    "serviceType": "sensor",
    "capabilities": ["temperature-reading"],
    "metadata": {"unit": "celsius"}
  }'
```

### Query by capability

```bash
curl "http://localhost:8000/registry/query?capability=temperature-reading"
```

### Orchestrate (discover an authorized provider)

```bash
curl -X POST http://localhost:8000/orchestration \
  -H "Content-Type: application/json" \
  -d '{"consumerId": "my-consumer", "serviceName": "temperature-reading"}'
```

## Reference

Official Arrowhead Core documentation:
https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_registry/
