# Arrowhead Core – Service Registry

A Go implementation of the Arrowhead Core **Service Registry** system, with a clear separation between the stable core and experimental extensions.

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full structural overview.

---

## Quick Start

```bash
cd core
go build ./...
go test ./...
```

Or with Docker:

```bash
cd core
docker compose up --build
```

Service Registry available at `http://localhost:8080`.

---

## Reference

- [Arrowhead Service Registry – Official Documentation](https://aitia-iiot.github.io/ah5-docs-java-spring/core_systems/service_registry/)
