# Core-Evol — ADAPI Extensions

Evolved core system variants that implement the **ADAPI** (Arrowhead Data-plane Authorization Policy Interfaces) gRPC interfaces. These extend — but do not replace — the [core systems](../core/).

## Binaries

| Binary | Description |
|---|---|
| `cmd/authz-pdp` | gRPC authorization PDP server (`authorize.proto` on port 9550) |
| `cmd/dynamicorch-xacml` | DynamicOrchestration variant that delegates authorization decisions to an external XACML PDP via gRPC instead of ConsumerAuthorization |

## Protocol Definitions

| Proto | Purpose |
|---|---|
| [`proto/authorize/`](proto/authorize/) | PEP-to-PDP gRPC interface for authorization decisions |
| [`proto/certlifecycle/`](proto/certlifecycle/) | Certificate lifecycle event stream — CA pushes cert state changes to all PEPs |

These two interfaces are the core of ADAPI: they connect Arrowhead's control-plane authorization and PKI to transport-level enforcement points.

## Used By

Experiments 12-14 use core-evol binaries and proto definitions. See:
- [experiment-12](../experiments/experiment-12/) — gRPC PDP replaces ConsumerAuthorization
- [experiment-13](../experiments/experiment-13/) — PKI identity unification via CertificateLifecycle stream
- [experiment-14](../experiments/experiment-14/) — connection-time certificate revocation

## Build

```bash
cd core-evol
go build ./...
```

Requires the `support/authzforce` module (resolved via `go.work` or the `replace` directive in `go.mod`).
