# Experiment 12 — DynamicOrchestration-XACML (Approach B) with gRPC PDP Interface

## What this experiment demonstrates

Experiment-12 implements **Approach B** from `AUTH_ALTERNATIVES.md`:
DynamicOrchestration uses a XACML PDP instead of ConsumerAuthorization
to decide which service endpoints to return.

The key addition over the original design: the interface between the orchestrator
and the PDP is defined as a gRPC service (`authorize.proto`). This makes the
contract explicit, typed, and discoverable — and it is the same interface any
XACML-compatible PDP would implement.

### Three architectural layers

```
DynamicOrch-XACML  (PEP — gRPC client)
      ↓  gRPC  (authorize.proto)
authz-pdp          (PDP adapter — gRPC server)
      ↓  HTTP/XML
AuthzForce CE      (XACML engine)
```

### Two authorization backends (pluggable)

`AUTH_BACKEND` selects the decider used by DynamicOrch-XACML:

| Backend        | Env value       | Implements              | Notes                             |
|---|---|---|---|
| gRPC (default) | `grpc`          | `authorize.proto`       | calls authz-pdp → AuthzForce      |
| ConsumerAuth   | `consumerauth`  | AH5 CA.verify over HTTP | spec-compliant; no XACML          |

The same orchestration loop runs in both modes. Only the authorization call changes.

## What changed from the original experiment-12 design

| Dimension | Before | After |
|---|---|---|
| PEP → PDP transport | Direct HTTP/XML to AuthzForce | gRPC to authz-pdp (authorize.proto) |
| Resource encoding | `"service@provider"` string | Separate `service` + `provider` fields |
| Namespace separation | Two string namespaces (`svc@prov` vs `svc`) | Two action values (`orchestrate` vs `consume`) |
| Policy format | `resource="telemetry@robot-fleet-site-1"` | `resource="telemetry", provider="robot-fleet-site-1"` |
| Interface spec | Implicit (Go interface only) | Explicit (authorize.proto — canonical, AI-generatable) |
| grpcurl inspection | Not possible | `grpcurl -plaintext localhost:9550 list` |

## The interface: authorize.proto

`core-evol/proto/authorize/authorize.proto` is the canonical spec. Its inline
comments describe every field, its XACML attribute mapping, caching guidance,
and the namespace separation strategy. See also the human README at
`core-evol/proto/authorize/README.md`.

### XACML attribute mapping

| Proto field | XACML attribute URN                                      |
|---|---|
| `subject`   | `urn:oasis:names:tc:xacml:1.0:subject:subject-id`       |
| `service`   | `urn:oasis:names:tc:xacml:1.0:resource:resource-id`     |
| `provider`  | `urn:arrowhead:attribute:provider-id` (when non-empty)  |
| `action`    | `urn:oasis:names:tc:xacml:1.0:action:action-id`         |

### Namespace separation (action-based)

| Plane          | action        | provider | Matched by                     |
|---|---|---|---|
| Orchestration  | `orchestrate` | set      | authz-pdp / DynamicOrch-XACML  |
| Enforcement    | `consume`     | empty    | kafka-authz, pki-rest-authz    |

An orchestration policy (`action=orchestrate`) never matches an enforcement
request (`action=consume`) and vice versa. No `@`-encoding needed.

## Architecture

```
PAP (9505) ──push XACML──▶ AuthzForce PDP ◀── authz-pdp ◀──gRPC── DynamicOrch-XACML (8893)
                           AuthzForce PDP ◀── kafka-authz (9101)
                           AuthzForce PDP ◀── topic-auth-xacml (9090)
                           AuthzForce PDP ◀── pki-rest-authz (9208)
```

One policy store. One authority. Two action namespaces. All planes consistent.

## Key properties

- **gRPC interface** — `authorize.proto` is the PEP↔PDP contract. Any XACML-compatible
  PDP can implement it. The interface is discoverable via gRPC reflection.
- **Separate fields** — `service` and `provider` are distinct XACML attributes, not
  a concatenated string. Policies can match either field independently.
- **Action-based namespacing** — `action=orchestrate` and `action=consume` separate
  orchestration from enforcement policies without string encoding.
- **Per-provider granularity** — a policy can permit `portal-cloud-ml` to access
  `telemetry` from `robot-fleet-site-1` while denying `robot-fleet-site-2`.
- **Pluggable backend** — `AUTH_BACKEND=consumerauth` switches DynamicOrch-XACML to
  AH5 ConsumerAuthorization without any orchestration logic change.
- **Fail-closed per provider** — gRPC error or Deny for Pi → Pi excluded; others unaffected.
- **Instant revocation** — PAP DELETE of an orchestration policy stops that provider being
  returned at the next orchestration request (no cache in authz-pdp).
- **Reflection enabled** — `grpcurl -plaintext localhost:9550 list` works out of the box.

## Quick start

```bash
cd experiments/experiment-12
docker compose up --build
```

Services available:
- Dashboard: http://localhost:3012
- PAP admin: http://localhost:3012/admin.html
- DynamicOrch-XACML: http://localhost:8893
- authz-pdp gRPC: localhost:9550
- PAP: http://localhost:9505
- PIP: http://localhost:9506
- AuthzForce: http://localhost:8596/authzforce-ce

## grpcurl inspection

```bash
# Install grpcurl: https://github.com/fullstorydev/grpcurl

# List services (reflection enabled)
grpcurl -plaintext localhost:9550 list
# → arrowhead.authz.v1.AuthorizationPDP
# → grpc.reflection.v1alpha.ServerReflection

# Describe the service schema
grpcurl -plaintext localhost:9550 describe arrowhead.authz.v1.AuthorizationPDP

# Orchestration decision: portal-cloud-ml → robot-fleet-site-1
grpcurl -plaintext \
  -d '{"subject":"portal-cloud-ml","service":"telemetry","provider":"robot-fleet-site-1","action":"orchestrate"}' \
  localhost:9550 arrowhead.authz.v1.AuthorizationPDP/Decide
# → {"decision":"PERMIT","statusCode":"urn:oasis:names:tc:xacml:1.0:status:ok"}

# Enforcement decision: portal-cloud-ml → telemetry (no provider)
grpcurl -plaintext \
  -d '{"subject":"portal-cloud-ml","service":"telemetry","action":"consume"}' \
  localhost:9550 arrowhead.authz.v1.AuthorizationPDP/Decide
# → {"decision":"PERMIT",...}
```

## Test orchestration manually

```bash
# Authorized consumer → portal-cloud-ml provider returned
curl -s -X POST http://localhost:8893/orchestration/dynamic \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"service-partner-1","address":"1.2.3.4","port":8000},
       "requestedService":{"serviceDefinition":"telemetry-rest"}}' | jq .

# Unauthorized consumer → empty list (no orchestration policy)
curl -s -X POST http://localhost:8893/orchestration/dynamic \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"unauthorized","address":"1.2.3.4","port":8000},
       "requestedService":{"serviceDefinition":"telemetry-rest"}}' | jq .

# portal-cloud-ml: gets all 3 robot sites
curl -s -X POST http://localhost:8893/orchestration/dynamic \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"portal-cloud-ml","address":"1.2.3.4","port":8000},
       "requestedService":{"serviceDefinition":"telemetry"}}' | jq '.response | length'
# → 3

# Revoke access to site-2 only — site-1 and site-3 still returned
POL_ID=$(curl -s http://localhost:9505/policies | jq -r \
  '.policies[] | select(.subject=="portal-cloud-ml" and .resource=="telemetry" and .provider=="robot-fleet-site-2") | .id')
curl -s -X DELETE http://localhost:9505/policies/$POL_ID
curl -s -X POST http://localhost:8893/orchestration/dynamic \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"portal-cloud-ml","address":"1.2.3.4","port":8000},
       "requestedService":{"serviceDefinition":"telemetry"}}' | jq '.response | map(.provider.systemName)'
# → ["robot-fleet-site-1","robot-fleet-site-3"]
```

## Switch to ConsumerAuth backend (for comparison)

To see the same orchestration loop with AH5 ConsumerAuthorization instead of XACML:

```yaml
# docker-compose.yml — dynamicorch-xacml service
environment:
  AUTH_BACKEND: "consumerauth"
  CA_URL: "http://consumerauth:8082"
```

ConsumerAuth grants must be seeded manually for this to work. The orchestration
logic — SR query, per-provider loop, fail-closed semantics — is identical.

## System test

```bash
cd experiments/experiment-12
docker compose up -d --build
bash test-system.sh
```

## File structure

```
experiments/experiment-12/
├── docker-compose.yml
├── dockerfiles/
│   ├── authz-pdp.Dockerfile           ← NEW: gRPC PDP server
│   ├── dynamicorch-xacml.Dockerfile   ← updated: uses gRPC client
│   ├── pap.Dockerfile                 ← reuses exp-10 PAP (now with provider field)
│   ├── pip.Dockerfile                 ← reuses exp-10 PIP service
│   ├── core.Dockerfile                ← AH5 core systems
│   └── …                            ← shared from exp-10
├── AUTH_ALTERNATIVES.md
├── AH5_EVOL.md
├── DIAGRAMS.md
└── test-system.sh

core-evol/
├── proto/
│   └── authorize/
│       ├── authorize.proto            ← canonical PEP↔PDP interface spec
│       ├── authorize.pb.go            ← generated message types
│       ├── authorize_grpc.pb.go       ← generated service interfaces
│       ├── Makefile                   ← regeneration command
│       └── README.md                 ← human interface docs with examples
├── internal/
│   ├── orchestration/
│   │   ├── service.go                ← XACMLOrchestrator + GRPCDecider + CADecider
│   │   ├── service_test.go
│   │   ├── handler.go
│   │   └── types.go
│   └── pdpserver/
│       ├── server.go                 ← AuthorizationPDPServer implementation
│       └── server_test.go
└── cmd/
    ├── authz-pdp/
    │   └── main.go                   ← gRPC server binary
    └── dynamicorch-xacml/
        └── main.go                   ← updated: AUTH_BACKEND selection
```

## Design decisions

See `AUTH_ALTERNATIVES.md` for the four-approach comparison and why Approach B was chosen.
See `AH5_EVOL.md` for the AH5 specification deviation analysis.
See `DIAGRAMS.md` for architecture diagrams.
See `core-evol/proto/authorize/README.md` for the gRPC interface documentation.
