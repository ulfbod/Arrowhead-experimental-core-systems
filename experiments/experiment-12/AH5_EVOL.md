# AH5_EVOL — DynamicOrchestration XACML Extension

## What AH5 specifies

The AH5 DynamicOrchestration service (as specified in `core/SPEC.md`) performs:

1. Accept an `OrchestrationRequest` from a consumer.
2. Query ServiceRegistry for providers of the requested service.
3. Optionally filter results through ConsumerAuthorization.verify — one HTTP call per provider,
   with signature `(consumer, provider, service) → {authorized: bool}`.
4. Return the filtered `OrchestrationResponse`.

ConsumerAuthorization.verify is a simple boolean gate that evaluates each
`(consumer, provider, service)` triple independently. It has no knowledge of XACML
or attribute-based policies.

## What Approach B changes

DynamicOrchestration-XACML replaces step 3 with per-provider gRPC PDP calls:

```
For each provider P returned by ServiceRegistry:
  authz-pdp.Decide(subject=consumer, service=svcDef, provider=P.systemName, action="orchestrate")
  → PERMIT:      include P in result
  → DENY or err: exclude P (fail-closed)
```

The gRPC call goes to `authz-pdp`, which translates it to a XACML request with
separate `resource-id` (service) and `urn:arrowhead:attribute:provider-id` (provider)
attributes before calling AuthzForce CE. The PEP never speaks XACML directly.

### Differences from the AH5 spec

| Dimension | AH5 (core/) | Approach B (core-evol/) |
|---|---|---|
| Authorization authority | ConsumerAuthorization | authz-pdp gRPC service → AuthzForce XACML PDP |
| Policy language | Boolean grant list (consumer, provider, service) | XACML 3.0 — attribute-based, rule-combining algorithms |
| PEP→PDP transport | HTTP to ConsumerAuth | gRPC to authz-pdp (authorize.proto) |
| Resource encoding | N/A (separate `providerSystemName` field) | Separate `service` + `provider` XACML attributes |
| Namespace separation | N/A | `action=orchestrate` vs `action=consume` |
| Policy source | ConsumerAuthorization internal store | PAP → AuthzForce (same source as data-plane PEPs) |
| Call count per request | N × CA.verify (one per provider) | N × gRPC.Decide (one per provider) |
| Failure mode | Per-provider: skip unauthorized providers | Per-provider fail-closed: gRPC error → skip (same semantics) |
| Per-provider granularity | Yes (evaluated per (consumer, provider, service)) | Yes (separate XACML attributes; policies match per-provider) |
| New dependencies | None (uses spec-defined CA) | Requires authz-pdp and AuthzForce to be reachable |
| Backend switchable | No | Yes: AUTH_BACKEND=consumerauth restores CA path |

### Why it moves beyond the spec

The AH5 spec defines ConsumerAuthorization as the authorization system for orchestration.
Replacing it with an XACML PDP is architecturally incompatible with the spec because:

1. ConsumerAuthorization is a separate, spec-defined core system. Bypassing it removes a
   spec-mandated component from the orchestration flow.

2. The authorization policy model changes. AH5 stores boolean `(consumer, provider, service)`
   grants in ConsumerAuthorization. Approach B stores XACML policies with a
   `service@provider` resource string in PAP. The two stores are not interchangeable.

3. The authorization infrastructure changes. ConsumerAuthorization is an AH5 core service;
   AuthzForce is a third-party XACML engine. Replacing one with the other is not an in-place
   upgrade — it requires deploying and operating a new component.

## Policy namespace separation (action-based)

Data-plane PEPs use `action=consume` (no provider) while DynamicOrch-XACML uses
`action=orchestrate` (with provider). PAP manages two namespaces distinguished by action:

```
PAP policy store
├── action=orchestrate + provider set  →  orchestration decisions (DynamicOrch-XACML via authz-pdp)
└── action=consume     + provider empty →  enforcement decisions  (kafka-authz, topic-auth-xacml, pki-rest-authz)
```

Both namespaces coexist in the same PAP and AuthzForce domain. A single PAP DELETE removes
the policy immediately. Because `action` values differ, an orchestration policy cannot
accidentally match an enforcement request and vice versa — no string encoding required.

## Pros of this evolution

- **Policy unification**: orchestration and data-plane enforcement share a single policy store
  (PAP → AuthzForce). A revocation in PAP instantly affects the relevant plane.
- **Per-provider granularity**: the `service@provider` encoding restores the AH5 level of
  per-provider control, while adding XACML expressiveness.
- **Expressiveness**: XACML allows conditions, attribute-based rules, and combining algorithms
  that CA grants cannot express.
- **Single operational system**: operators manage one policy store instead of two (PAP vs CA).
- **Auditability**: a single AuthzForce decision log covers all planes.

## Cons of this evolution

- **Spec non-compliance**: the resulting system does not conform to AH5.
  It cannot interoperate with AH5-compliant orchestrators or CA systems without adaptation.
- **Dual policy namespace**: per-provider policies (`service@provider`) for orchestration and
  service-level policies (`service`) for enforcement must both be maintained. Granting access
  requires two PAP entries per `(consumer, service, provider)` triple.
- **N XACML calls per orchestration request**: same call count as AH5 CA.verify, but through
  AuthzForce instead of ConsumerAuthorization. No reduction in request-time latency.
- **New infrastructure dependency**: DynamicOrch now depends on AuthzForce being available.
  In the AH5 model, orchestration and enforcement are decoupled — an enforcement failure does
  not affect orchestration.
- **Enforcement still service-level**: data-plane PEPs do not know which specific provider a
  consumer is accessing. Enforcement granularity below service level requires per-provider
  broker topics or endpoints, which is beyond the scope of this experiment.

## Where the evolved code lives

```
core-evol/
├── proto/authorize/
│   ├── authorize.proto        — canonical PEP↔PDP gRPC interface (XACML-aligned)
│   ├── authorize.pb.go        — generated message types
│   ├── authorize_grpc.pb.go   — generated service interfaces
│   └── README.md             — human interface docs with grpcurl examples
├── internal/
│   ├── orchestration/
│   │   ├── types.go           — duplicated AH5 model types
│   │   ├── service.go         — XACMLOrchestrator + AuthDecider (separate fields)
│   │   │                        GRPCDecider (→ authz-pdp), CADecider (→ ConsumerAuth)
│   │   └── handler.go         — HTTP handler
│   └── pdpserver/
│       └── server.go          — AuthorizationPDPServer impl (gRPC → AuthzForce)
└── cmd/
    ├── authz-pdp/main.go      — gRPC server binary (reflection enabled)
    └── dynamicorch-xacml/main.go — pluggable backend selection
```

The `core/` directory is untouched. The AH5-compliant systems remain available and
could serve as a fallback or reference implementation.
