# Experiment 1 — Service Registry Browser Demo

This experiment is the simplest entry point into the repository. It demonstrates
how an external client interacts with the Arrowhead Core Service Registry
exclusively via its HTTP API, and provides an interactive browser UI for
registering and querying services. There is no Docker stack — all components run
directly on the developer machine against the core started separately.

---

## What is demonstrated

- Registering a service instance via `POST /serviceregistry/register`
- Querying the registry with filters (serviceDefinition, interfaces, metadata,
  version) via `POST /serviceregistry/query`
- The correct pattern for a browser frontend that cannot issue cross-origin
  requests to a core that serves no CORS headers: the Vite dev proxy (or a
  thin Go CORS-proxy service) forwards requests transparently
- The correct pattern for a Go service that self-registers with the core on
  startup without importing any `core/internal` package

---

## Components

| Component | Directory | Port | Description |
|---|---|---|---|
| `example-client` | `example-client/` | — | Go CLI: registers two sample services and runs four queries; exits after printing results |
| `example-service` | `service/` | 9090 | Go HTTP server: self-registers with the core, exposes `GET /hello`, and proxies `POST /query` to the core with CORS headers |
| `frontend` | `frontend/` | 5173 (dev) | React + Vite browser UI: register and query services interactively; Vite proxies `/serviceregistry/*` to the core |

No AMQP broker, Kafka, or AuthzForce — only the Arrowhead Core Service Registry
is required.

---

## Prerequisites

- Arrowhead Core running (ServiceRegistry at `http://localhost:8080`)
- Go 1.22+
- Node.js 18+ (for the frontend)

Start the core from the repo root:

```bash
cd core
go run ./cmd/serviceregistry
```

Or with Docker:

```bash
cd core
docker compose up -d
```

---

## Quick Start

### Option A — CLI demo only

Run `example-client` directly; no other components needed beyond the core:

```bash
cd experiments/experiment-1/example-client
go run main.go
```

Override the core URL if needed:

```bash
CORE_URL=http://localhost:9000 go run main.go
```

The client registers two `temperature-service` instances and runs four queries
(all services, EU region filter, interface filter, unknown service), then exits.

### Option B — Browser UI (dev mode, Vite proxy)

The frontend proxies `/serviceregistry/*` directly to the core via Vite. No
`example-service` needed:

```bash
cd experiments/experiment-1/frontend
npm install
npm run dev
```

Open **http://localhost:5173** in a browser.

### Option C — example-service + browser UI

Start `example-service` first (it self-registers with the core), then start the
frontend. The frontend still uses the Vite proxy in dev mode, but `example-service`
appears in the registry so you can observe it in the query results.

```bash
# Terminal 1
cd experiments/experiment-1/service
go run main.go
# Optionally: PORT=9090 CORE_URL=http://localhost:8080 go run main.go

# Terminal 2
cd experiments/experiment-1/frontend
npm install && npm run dev
```

---

## Environment Variables

| Variable | Component | Default | Description |
|---|---|---|---|
| `CORE_URL` | `example-client`, `example-service` | `http://localhost:8080` | Base URL of the Arrowhead Core Service Registry |
| `PORT` | `example-service` | `9090` | Port `example-service` listens on and registers itself under |

---

## Unit Tests

`example-service` has handler-level tests covering `GET /hello` and the CORS
proxy (`POST /query`, `OPTIONS` preflight, upstream error, CORS headers):

```bash
cd experiments/experiment-1/service
go test ./...
```

---

## Architecture Note

```
browser / curl
     │
     ▼
 frontend :5173         example-client (CLI)
     │  (Vite proxy)           │
     └──────────────┬──────────┘
                    │ HTTP
                    ▼
          ServiceRegistry :8080
          (Arrowhead Core)

example-service :9090 also registers with the core independently.
Its /query endpoint is an alternative CORS proxy for the built
frontend (outside of Vite dev mode).
```

The core is never modified. All communication is via its public HTTP API.
