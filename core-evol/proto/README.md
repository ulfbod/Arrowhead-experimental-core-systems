# core-evol/proto — ADAPT gRPC Interface Registry

This directory contains the canonical gRPC interface specifications for the
ADAPT policy stack. Each subdirectory is one interface: a `.proto` file that is
the single source of truth, pre-generated Go bindings, and a human README.

All interfaces follow the same conventions:
- Inline XACML-aligned comments on every field and RPC
- gRPC server reflection enabled on every server implementation
- `make gen` (Docker-based) to regenerate Go bindings
- No external proto dependencies (no `google/protobuf/` imports)

---

## Interfaces

| Interface | Server | Clients | Port |
|---|---|---|---|
| [`authorize/`](authorize/) | `authz-pdp` | `dynamicorch-xacml` | :9550 |
| [`certlifecycle/`](certlifecycle/) | `profile-ca` | `pip` | :8089 |

---

## authorize.proto

PEP → PDP authorization decision interface (experiment-12+).

```
DynamicOrch-XACML  ──gRPC──▶  authz-pdp  ──HTTP/XML──▶  AuthzForce
```

## certlifecycle.proto

CA → subscriber certificate lifecycle event stream (experiment-13+).

```
profile-ca  ──gRPC stream──▶  PIP  ──HTTP──▶  kafka-authz
                                            ▶  topic-auth-xacml
                                            ▶  pki-rest-authz
```
