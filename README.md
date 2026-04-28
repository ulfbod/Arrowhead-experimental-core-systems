# Arrowhead Core – Service Registry

A Go implementation of the Arrowhead Core **Service Registry** system.

## Overview

The Service Registry enables service providers to register themselves and allows consumers to discover services via structured queries. It is the central lookup component in an Arrowhead Core deployment.

Implemented according to the official specification:
https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_registry/

---

## API

### POST `/serviceregistry/register`

Register a service instance. Registering the same `(serviceDefinition, systemName, address, port, version)` tuple again overwrites the existing entry.

**Request**
```json
{
  "serviceDefinition": "temperature-service",
  "providerSystem": {
    "systemName": "sensor-1",
    "address": "192.168.0.10",
    "port": 8080,
    "authenticationInfo": "optional"
  },
  "serviceUri": "/temperature",
  "interfaces": ["HTTP-SECURE-JSON"],
  "version": 1,
  "metadata": {
    "region": "eu",
    "unit": "celsius"
  },
  "secure": "NOT_SECURE"
}
```

**Response** — `201 Created` with the stored service instance including its assigned `id`.

Required fields: `serviceDefinition`, `providerSystem` (with `systemName`, `address`, `port` > 0), `serviceUri`, `interfaces` (non-empty).
Optional fields: `authenticationInfo`, `version` (defaults to 1), `metadata`, `secure`.

---

### POST `/serviceregistry/query`

Query registered services. All provided fields are applied as AND filters.

**Request**
```json
{
  "serviceDefinition": "temperature-service",
  "interfaces": ["HTTP-SECURE-JSON"],
  "metadata": { "region": "eu" },
  "versionRequirement": 1
}
```

All fields are optional. An empty request returns all registered services.

**Matching rules:**
- `serviceDefinition` — exact match
- `interfaces` — service must provide **all** requested interfaces (case-insensitive)
- `metadata` — service must contain **all** requested key-value pairs
- `versionRequirement` — service version must equal the requested value

**Response** — `200 OK`
```json
{
  "serviceQueryData": [ ... ],
  "unfilteredHits": 4
}
```

---

### GET `/health`

Returns `200 OK` with `{"status": "ok"}`.

---

## Build and Run

### Prerequisites

- Go 1.25+
- Docker and Docker Compose (optional)

### Local

```bash
cd backend
go build ./cmd/serviceregistry
./serviceregistry            # listens on :8080
PORT=9000 ./serviceregistry  # custom port
```

### Docker Compose

```bash
docker compose up --build
```

The service will be available at `http://localhost:8080`.

---

## Project Structure

```
backend/
├── cmd/serviceregistry/   # entry point
└── internal/
    ├── api/               # HTTP handlers
    ├── service/           # business logic and validation
    ├── repository/        # in-memory storage
    └── model/             # data types
```

---

## Tests

```bash
cd backend
go test ./...
```

---

## Reference

- [Arrowhead Service Registry – Official Documentation](https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_registry/)
