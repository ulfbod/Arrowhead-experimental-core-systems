# Experiment 12 — DynamicOrchestration-XACML (Approach B)

## What this experiment demonstrates

Experiment-12 implements **Approach B** from `AUTH_ALTERNATIVES.md`:
DynamicOrchestration uses AuthzForce (the XACML PDP) instead of ConsumerAuthorization
to decide which service endpoints to return.

The key insight: in experiments 10–11, a PAP policy revocation would stop data-plane
traffic immediately (via kafka-authz, topic-auth-xacml, pki-rest-authz) but would not
affect orchestration — a consumer would still receive provider endpoints even after
being denied. This split creates an inconsistency between what orchestration says
is allowed and what enforcement enforces.

Experiment-12 eliminates this split by routing both decisions through AuthzForce.

## What changed from experiment-10

| Component | Experiment-10 | Experiment-12 |
|---|---|---|
| DynamicOrch binary | `core/cmd/dynamicorch` | `core-evol/cmd/dynamicorch-xacml` |
| Orchestration auth | `ConsumerAuth.verify` (per provider) | `AuthzForce.Decide` (single call) |
| ConsumerAuth role | Authorization + Orchestration | AH5 spec presence only |
| Setup: CA grants | Required | Not needed |
| Policy authority | PAP (enforcement) + CA (orchestration) | PAP only |

## Architecture

```
PAP (9505) ──push XACML──▶ AuthzForce PDP ──decide──▶ DynamicOrch-XACML (8083)
                                           ──decide──▶ kafka-authz (9101)
                                           ──decide──▶ topic-auth-xacml (9090)
                                           ──decide──▶ pki-rest-authz (9208)
```

One policy store. One authority. All planes consistent.

## Key properties

- **Single XACML call per orchestration request** — `AuthzForce.Decide(domainID, consumer, service, "consume")` replaces N per-provider `CA.verify` calls.
- **All-or-nothing result** — Permit returns all providers of the service; Deny returns an empty list. The XACML policy is per `(consumer, service)`, not per `(consumer, provider, service)`.
- **Fail-closed** — AuthzForce unavailable → empty orchestration result (same as Deny).
- **Instant revocation on both planes** — PAP DELETE → AuthzForce updated → DynamicOrch-XACML and all PEPs deny simultaneously.
- **No ConsumerAuth grants needed** — setup only seeds PAP policies + PIP subjects.

## Quick start

```bash
cd experiments/experiment-12
docker compose up --build
```

Services available:
- Dashboard: http://localhost:3012
- PAP admin: http://localhost:3012/admin.html
- DynamicOrch-XACML: http://localhost:8893
- PAP: http://localhost:9505
- PIP: http://localhost:9506
- AuthzForce: http://localhost:8596/authzforce-ce

## Test orchestration manually

```bash
# Authorized consumer → providers returned
curl -s -X POST http://localhost:8893/orchestration/dynamic \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"service-partner-1","address":"1.2.3.4","port":8000},
       "requestedService":{"serviceDefinition":"telemetry-rest"}}' | jq .

# Unauthorized consumer → empty list
curl -s -X POST http://localhost:8893/orchestration/dynamic \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"unauthorized","address":"1.2.3.4","port":8000},
       "requestedService":{"serviceDefinition":"telemetry-rest"}}' | jq .

# Revoke SP1 via PAP → check orchestration (empty) + enforcement (deny)
POL_ID=$(curl -s http://localhost:9505/policies | jq -r '.policies[] | select(.subject=="service-partner-1" and .resource=="telemetry-rest") | .id')
curl -s -X DELETE http://localhost:9505/policies/$POL_ID
curl -s -X POST http://localhost:8893/orchestration/dynamic \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"service-partner-1","address":"1.2.3.4","port":8000},
       "requestedService":{"serviceDefinition":"telemetry-rest"}}' | jq '.response | length'
# → 0
```

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
│   ├── dynamicorch-xacml.Dockerfile   ← new: builds core-evol binary
│   ├── pap.Dockerfile                  ← reuses exp-10 PAP service
│   ├── pip.Dockerfile                  ← reuses exp-10 PIP service
│   ├── core.Dockerfile                 ← AH5 core systems
│   └── …                              ← shared from exp-10
├── dashboard/
│   ├── index.html                      ← monitoring dashboard
│   ├── admin.html                      ← PAP + DynamicOrch-XACML admin
│   └── nginx.conf
├── rabbitmq/
├── AUTH_ALTERNATIVES.md               ← four-approach pros/cons table
├── AH5_EVOL.md                        ← AH5 spec evolution analysis
├── DIAGRAMS.md                        ← architecture diagrams
└── test-system.sh

core-evol/                             ← AH5-evolved systems (new module)
├── go.mod                             ← module: arrowhead/core-evol
├── internal/orchestration/
│   ├── types.go                       ← duplicated AH5 model types
│   ├── service.go                     ← XACMLOrchestrator + srClient
│   ├── service_test.go                ← TDD tests
│   ├── handler.go                     ← HTTP handler
│   ├── handler_test.go                ← handler tests
│   └── srclient_test.go               ← srClient integration tests
└── cmd/dynamicorch-xacml/
    └── main.go                        ← binary entry point
```

## Design decisions

See `AUTH_ALTERNATIVES.md` for the four-approach comparison and why Approach B was chosen.
See `AH5_EVOL.md` for the AH5 specification deviation analysis and trade-offs.
See `DIAGRAMS.md` for architecture diagrams and flow comparisons.
