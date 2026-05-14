# Experiment 12 — Architecture Diagrams

## System overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Arrowhead Cloud (UC3)                               │
│                                                                             │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐                    │
│  │ServiceRegistry│  │Authentication│   │ConsumerAuth  │  (AH5 spec only —  │
│  │  TLS :8490   │  │  TLS :8491   │   │  TLS :8492   │   not called for   │
│  └──────────────┘  └──────────────┘   └──────────────┘   authorization)   │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │         DynamicOrch-XACML  :8083  (core-evol — Approach B)          │  │
│  │                                                                      │  │
│  │  1. Validate request                                                 │  │
│  │  2. POST /serviceregistry/query → providers P1, P2, … Pn            │  │
│  │  3. For each Pi:                                                     │  │
│  │       gRPC Decide(subject=consumer, service=svc, provider=Pi,       │  │
│  │                   action="orchestrate")                              │  │
│  │       PERMIT → include Pi     DENY/error → exclude Pi (fail-closed) │  │
│  │  4. Return filtered provider list                                    │  │
│  └──────────────────────────┬───────────────────────────────────────────┘  │
│                             │ N × gRPC Decide (authorize.proto)            │
│                             ▼                                               │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │  authz-pdp  :9550  (gRPC server — authorize.proto)                 │    │
│  │                                                                    │    │
│  │  Translates gRPC DecisionRequest to XACML 3.0 Request:            │    │
│  │    resource-id              = service                              │    │
│  │    urn:arrowhead:…:provider-id = provider (when non-empty)         │    │
│  │    action-id                = action                               │    │
│  │                                                                    │    │
│  │  gRPC reflection enabled: grpcurl -plaintext localhost:9550 list  │    │
│  └──────────────────────────┬───────────────────────────────────────-┘    │
│                             │ HTTP/XML XACML evaluate                      │
│                             ▼                                               │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                    Policy Plane (Strategy B — exp-10)                 │ │
│  │                                                                       │ │
│  │  ┌──────────────────────────────────┐   ┌──────────────────────────┐ │ │
│  │  │  PAP :9505                       │   │  PIP :9506               │ │ │
│  │  │  (Policy Admin Point)            │   │  cert level registry     │ │ │
│  │  │                                  │   └──────────────────────────┘ │ │
│  │  │  Two namespaces by action:       │                                 │ │
│  │  │   action=orchestrate + provider  │                                 │ │
│  │  │   action=consume (no provider)   │                                 │ │
│  │  │                                  │                                 │ │
│  │  │  On Create/Delete: push to AF    │                                 │ │
│  │  └──────────────────────────────────┘                                 │ │
│  │             │ SetPolicy()                                              │ │
│  │             ▼                                                          │ │
│  │  ┌──────────────────────────────┐                                     │ │
│  │  │  AuthzForce PDP :8080        │                                     │ │
│  │  │  XACML PolicySet             │                                     │ │
│  │  │  (urn:arrowhead:exp12:pap)   │ ◀─── authz-pdp  (orchestration)    │ │
│  │  │                              │ ◀─── kafka-authz (enforcement)      │ │
│  │  │                              │ ◀─── topic-auth-xacml (enforcement) │ │
│  │  │                              │ ◀─── pki-rest-authz (enforcement)   │ │
│  │  └──────────────────────────────┘                                     │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                       Enforcement Plane (PEP)                         │  │
│  │  ┌──────────────┐  ┌──────────────────┐  ┌───────────────────────┐   │  │
│  │  │ kafka-authz  │  │ topic-auth-xacml │  │ pki-rest-authz        │   │  │
│  │  │ :9101        │  │ :9090            │  │ :9208 (TLS)/:9209     │   │  │
│  │  │ action=consume│  │ action=consume  │  │ action=consume        │   │  │
│  │  │ no provider   │  │ no provider     │  │ no provider           │   │  │
│  │  └──────────────┘  └──────────────────┘  └───────────────────────┘   │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Per-provider orchestration flow

```
Consumer: POST /orchestration/dynamic
  { requesterSystem: {systemName: "service-partner-1"},
    requestedService: {serviceDefinition: "telemetry-rest"} }

  Step 1: POST /serviceregistry/query {serviceDefinition: "telemetry-rest"}
          → [{provider: "portal-cloud-ml", …}]

  Step 2: gRPC authz-pdp.Decide(
            subject="service-partner-1",
            service="telemetry-rest",
            provider="portal-cloud-ml",
            action="orchestrate"
          )
          → PERMIT (policy: subject=service-partner-1, resource=telemetry-rest,
                    provider=portal-cloud-ml, action=orchestrate)

  Result: [{provider: "portal-cloud-ml", serviceUri: "/telemetry", …}]

---

Consumer: "portal-cloud-ml" requesting "telemetry" (3 providers)

  Step 1: SR query → [robot-fleet-site-1, robot-fleet-site-2, robot-fleet-site-3]

  Step 2: Decide(portal-cloud-ml, telemetry, robot-fleet-site-1, orchestrate) → PERMIT
  Step 3: Decide(portal-cloud-ml, telemetry, robot-fleet-site-2, orchestrate) → PERMIT
  Step 4: Decide(portal-cloud-ml, telemetry, robot-fleet-site-3, orchestrate) → PERMIT

  Result: all 3 providers returned

---

Partial grant example (site-2 access revoked):

  PAP DELETE {subject:"portal-cloud-ml", resource:"telemetry", provider:"robot-fleet-site-2", action:"orchestrate"}

  Step 2: Decide(…, robot-fleet-site-1, …) → PERMIT
  Step 3: Decide(…, robot-fleet-site-2, …) → NOT_APPLICABLE (no policy → default deny)
  Step 4: Decide(…, robot-fleet-site-3, …) → PERMIT

  Result: [robot-fleet-site-1, robot-fleet-site-3]  ← site-2 excluded
```

## Policy namespace separation (action-based)

```
PAP policy store — two namespaces via action field

  ┌──────────────────────────────────────────────────────────────────────┐
  │ Orchestration namespace: action="orchestrate", provider set          │
  │                                                                      │
  │  portal-cloud-ml  telemetry  robot-fleet-site-1  orchestrate  Permit │
  │  portal-cloud-ml  telemetry  robot-fleet-site-2  orchestrate  Permit │
  │  portal-cloud-ml  telemetry  robot-fleet-site-3  orchestrate  Permit │
  │  service-partner-1  telemetry-rest  portal-cloud-ml  orchestrate  P  │
  │  service-partner-2  telemetry-rest  portal-cloud-ml  orchestrate  P  │
  └──────────────────────────────────────────────────────────────────────┘

  ┌──────────────────────────────────────────────────────────────────────┐
  │ Enforcement namespace: action="consume", no provider                 │
  │                                                                      │
  │  portal-cloud-ml    telemetry      consume  Permit                   │
  │  service-partner-1  telemetry-rest consume  Permit                   │
  │  service-partner-2  telemetry-rest consume  Permit                   │
  └──────────────────────────────────────────────────────────────────────┘

  Both namespaces live in the same PAP and AuthzForce domain.
  XACML action-id match prevents cross-namespace matches.
  No @-encoding. No string parsing. Each field is a typed XACML attribute.
```

## XACML request structure (per-provider orchestration)

```
Orchestration request (service-partner-1 requesting telemetry-rest from portal-cloud-ml):

  <Request>
    <Attributes Category="urn:oasis:names:tc:xacml:1.0:subject-category:access-subject">
      <Attribute AttributeId="urn:oasis:names:tc:xacml:1.0:subject:subject-id">
        <AttributeValue>service-partner-1</AttributeValue>
      </Attribute>
    </Attributes>
    <Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:resource">
      <Attribute AttributeId="urn:oasis:names:tc:xacml:1.0:resource:resource-id">
        <AttributeValue>telemetry-rest</AttributeValue>        ← service
      </Attribute>
      <Attribute AttributeId="urn:arrowhead:attribute:provider-id">
        <AttributeValue>portal-cloud-ml</AttributeValue>       ← provider (separate!)
      </Attribute>
    </Attributes>
    <Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:action">
      <Attribute AttributeId="urn:oasis:names:tc:xacml:1.0:action:action-id">
        <AttributeValue>orchestrate</AttributeValue>           ← namespace separator
      </Attribute>
    </Attributes>
  </Request>

Enforcement request (service-partner-1 consuming telemetry-rest — no provider):

  <Request>
    ... subject = service-partner-1 ...
    <Attributes Category="resource">
      <Attribute AttributeId="resource-id">
        <AttributeValue>telemetry-rest</AttributeValue>        ← service only
      </Attribute>
      ← no provider-id attribute
    </Attributes>
    ... action = consume ...
  </Request>
```

## Authority comparison: experiments 10, 11, 12

```
Exp-10 (split):
  Orchestration → ConsumerAuth   (boolean grants per consumer/provider/service)
  Enforcement   → AuthzForce     (XACML policies per consumer/service)
  Split: two systems to update; possible divergence

Exp-11 (hybrid):
  Orchestration → ConsumerAuth   (boolean grants)
  Enforcement   → AuthzForce     (XACML; PAP merges native + PIP grants with Δt delay)
  ConsumerAuth feeds PIP → PAP → AuthzForce (eventual consistency)

Exp-12 (unified, gRPC interface):
  Orchestration → authz-pdp gRPC → AuthzForce (service+provider separate, action=orchestrate)
  Enforcement   → AuthzForce direct (service only, action=consume)
  Single authority (PAP); two action namespaces; gRPC interface with reflection
  AUTH_BACKEND=consumerauth → same orchestration loop, AH5 CA as decider (for comparison)
```

## Fail-closed behaviour

```
authz-pdp unreachable for provider Pi:
  DynamicOrch-XACML excludes Pi from result (other providers may still succeed)

authz-pdp unreachable for all providers:
  DynamicOrch-XACML returns empty list

ServiceRegistry unavailable:
  DynamicOrch-XACML returns error 500

authz-pdp returns DENY or NOT_APPLICABLE for Pi:
  Pi excluded, no error returned to caller

AUTH_BACKEND=consumerauth, CA unavailable for Pi:
  Pi excluded (same fail-closed semantics, different authority)
```

## Pluggable backend comparison

```
AUTH_BACKEND=grpc (default):
  DynamicOrch-XACML → authz-pdp:9550 (gRPC) → AuthzForce → XACML evaluate
  Interface:  authorize.proto (typed, discoverable, reflection-enabled)
  Policy:     PAP {resource=svc, provider=P, action=orchestrate}
  Granularity: per (consumer, service, provider) via XACML attributes

AUTH_BACKEND=consumerauth:
  DynamicOrch-XACML → ConsumerAuth:8082 (HTTP) → CA grant store → boolean lookup
  Interface:  AH5 CA.verify (POST /consumerauth/verify)
  Policy:     CA grant: (consumerSystemName, providerSystemName, serviceDefinition)
  Granularity: per (consumer, provider, service) — same semantics, different engine

Same orchestration loop in both modes. Same per-provider fail-closed semantics.
Only the authorization call and policy store differ.
```
