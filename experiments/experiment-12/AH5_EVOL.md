# AH5_EVOL — DynamicOrchestration XACML Extension

## What AH5 specifies

The AH5 DynamicOrchestration service (as specified in `core/SPEC.md`) performs:

1. Accept an `OrchestrationRequest` from a consumer.
2. Query ServiceRegistry for providers of the requested service.
3. Optionally filter results through ConsumerAuthorization.verify — one HTTP call per provider.
4. Return the filtered `OrchestrationResponse`.

ConsumerAuthorization.verify is a simple boolean gate: given `(consumer, provider, service)`, it returns `{ "authorized": true/false }`. It has no knowledge of XACML or attribute-based policies.

## What Approach B changes

DynamicOrchestration-XACML replaces step 3 with a single XACML PDP call:

```
AuthzForce.Decide(domainID, consumerSystemName, serviceDefinition, "consume")
```

### Differences from the AH5 spec

| Dimension | AH5 (core/) | Approach B (core-evol/) |
|---|---|---|
| Authorization authority | ConsumerAuthorization | AuthzForce XACML PDP |
| Granularity | (consumer, provider, service) per provider | (consumer, service) — single call for all providers |
| Policy language | Boolean grant list | XACML 3.0 — attribute-based, rule-combining algorithms, conditions |
| Policy source | ConsumerAuthorization internal store | PAP → AuthzForce (same source as data-plane enforcement) |
| Result semantics | Per-provider filter | All-or-nothing: Permit → all providers, Deny → empty |
| Failure mode | Per-provider: skip unauthorized providers | Fail-closed: AF unavailable → empty result |
| New dependencies | None (uses spec-defined CA) | Requires AuthzForce to be reachable |

### Why it moves beyond the spec

The AH5 spec defines ConsumerAuthorization as the authorization system for orchestration.
Replacing it with an XACML PDP is architecturally incompatible with the spec because:

1. ConsumerAuthorization is a separate, spec-defined core system. Bypassing it removes a
   spec-mandated component from the orchestration flow.

2. The authorization granularity changes. AH5 authorization is per-provider — the orchestrator
   constructs an individual authorization check for each candidate provider. XACML policies in
   this architecture are per-service — the decision covers all providers of a service equally.

3. The result semantics change. AH5 returns a filtered list (each provider independently
   evaluated). Approach B returns all-or-nothing — if the consumer is authorized for the
   service, it gets all providers.

## Pros of this evolution

- **Policy unification**: orchestration and data-plane enforcement share a single policy store
  (PAP → AuthzForce). A revocation in PAP instantly affects both planes without sync lag or
  dual-write.
- **Expressiveness**: XACML allows conditions, attribute-based rules, and combining algorithms
  that CA grants cannot express.
- **Efficiency**: one XACML call replaces N CA calls (N = number of candidate providers),
  reducing orchestration latency at scale.
- **Auditability**: a single AuthzForce decision log covers both planes.

## Cons of this evolution

- **Spec non-compliance**: the resulting system does not conform to AH5.
  It cannot interoperate with AH5-compliant orchestrators or CA systems without adaptation.
- **Coarser granularity**: the AH5 model allows granting access to a specific provider of a
  service (e.g. allow consumer A to access provider X but not provider Y for the same service).
  Approach B loses this per-provider distinction.
- **New infrastructure dependency**: DynamicOrch now depends on AuthzForce being available.
  In the AH5 model, orchestration and enforcement are decoupled — an enforcement failure does
  not affect orchestration.
- **All-or-nothing semantics**: a consumer is either authorised for a service (all providers)
  or not. This may be too coarse for deployments where providers are distinguished by location,
  capability, or trust level.

## Where the evolved code lives

```
core-evol/
└── internal/orchestration/
│   ├── types.go          — duplicated AH5 model types (cannot import core/internal/)
│   ├── service.go        — XACMLOrchestrator, srClient, AuthDecider interface
│   ├── handler.go        — HTTP handler and route registration
└── cmd/dynamicorch-xacml/
    └── main.go           — binary entry point, env config
```

The `core/` directory is untouched. The AH5-compliant systems remain available and
could serve as a fallback or reference implementation.
