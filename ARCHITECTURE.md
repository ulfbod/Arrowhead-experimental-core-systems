# Architecture

This repository is divided into two clearly separated areas.

---

## /core — Arrowhead Core Service Registry

A strict, spec-compliant implementation of the Arrowhead Core Service Registry system.

- Governed by `core/SPEC.md`, `core/TEST_PLAN.md`, `core/EXAMPLES.md`
- Minimal, stable, and independently buildable
- No dependency on `experiments/`

See [`core/CLAUDE.md`](core/CLAUDE.md) for implementation rules.

### Running

```bash
cd core
go build ./...
go test ./...

# or with Docker:
docker compose up --build
```

Service Registry available at `http://localhost:8080`.

---

## /experiments — Experimental Extensions

Exploratory code built on top of the core. May include frontends, simulation drivers, client libraries, or analysis tools.

- Communicates with core via HTTP only — no internal package imports
- Self-contained per experiment; each subdirectory manages its own dependencies
- Not held to the strict correctness standard of `core/`

See [`experiments/CLAUDE_EXPERIMENTS.md`](experiments/CLAUDE_EXPERIMENTS.md) for rules.

---

## Boundary Rule

```
experiments/ ──HTTP──▶ core/
```

The only permitted interface between the two areas is the core's HTTP API:

| Endpoint | Description |
|---|---|
| `POST /serviceregistry/register` | Register a service instance |
| `POST /serviceregistry/query` | Query registered services |
| `GET /health` | Liveness check |

No code in `experiments/` may import packages from `core/internal/`.

---

## Repository Structure

```
/
├── ARCHITECTURE.md
├── README.md
├── LICENSE
├── .gitignore
│
├── core/
│   ├── CLAUDE.md
│   ├── SPEC.md
│   ├── TEST_PLAN.md
│   ├── EXAMPLES.md
│   ├── docker-compose.yml
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/serviceregistry/
│   └── internal/
│       ├── api/
│       ├── config/
│       ├── model/
│       ├── repository/
│       └── service/
│
└── experiments/
    └── CLAUDE_EXPERIMENTS.md
```
