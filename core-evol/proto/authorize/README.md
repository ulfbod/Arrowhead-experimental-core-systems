# AuthorizationPDP gRPC Interface

`authorize.proto` is the canonical API contract between a Policy Enforcement
Point (PEP) and the Policy Decision Point (PDP) in the Arrowhead experiment-12
authorization architecture.

## Motivation

Arrowhead 5's ConsumerAuthorization uses a three-field Boolean grant store:
`(consumer, provider, service) → {authorized: true/false}`. Replacing the
transport with gRPC and the encoding with native XACML attributes (instead of
the `service@provider` string hack) gives:

- Each field is typed and named — no parsing of concatenated strings
- XACML attribute IDs appear explicitly in the policy; the proto is the schema
- Any XACML-compatible PDP can implement this interface
- The proto file alone is enough for another implementer (human or AI) to build
  a conforming PDP from scratch

## XACML attribute mapping

| Proto field | XACML attribute URN                                          |
|---|---|
| `subject`   | `urn:oasis:names:tc:xacml:1.0:subject:subject-id`           |
| `service`   | `urn:oasis:names:tc:xacml:1.0:resource:resource-id`         |
| `provider`  | `urn:arrowhead:attribute:provider-id` (when non-empty)      |
| `action`    | `urn:oasis:names:tc:xacml:1.0:action:action-id`             |

When `provider` is absent the request carries only `resource-id`, matching
service-level enforcement policies. When `provider` is present the request
carries both `resource-id` and `provider-id`, matching per-provider
orchestration policies.

## Namespace separation

Orchestration and enforcement use the same AuthzForce domain but different
`action` values, which naturally separates policy namespaces:

| Plane          | action        | provider | Policy example                                     |
|---|---|---|---|
| Orchestration  | `orchestrate` | set      | `portal-cloud-ml, telemetry, robot-fleet-site-1`   |
| Enforcement    | `consume`     | empty    | `portal-cloud-ml, telemetry`                       |

A policy with `action=orchestrate` never matches an enforcement request
(`action=consume`), and vice versa. No `@`-encoding needed.

## Decision semantics

| Decision          | Meaning                                      | PEP action              |
|---|---|---|
| `PERMIT`          | Policy grants access                         | Allow / include provider |
| `DENY`            | Policy denies access                         | Block / exclude provider |
| `NOT_APPLICABLE`  | No policy matched; default deny-unless-permit| Block / exclude provider |
| `INDETERMINATE`   | Evaluation error (missing attr, syntax err)  | Block (fail-closed)      |
| gRPC error        | PDP unreachable or internal failure          | Block (fail-closed)      |

**Fail-closed per provider**: a gRPC error for provider Pᵢ excludes Pᵢ from the
orchestration result. Other providers may still succeed. The caller never
receives a partial error — the exclusion is silent.

## Cache / TTL guidance

| Use case                  | Recommended TTL | Rationale                              |
|---|---|---|
| Orchestration (DynamicOrch) | 0 (no cache)  | Stale permit survives PAP revocation   |
| Enforcement (kafka-authz)   | ≤ 5 s         | Bounded lag; reduces PDP round-trips   |
| Enforcement (pki-rest-authz)| ≤ 5 s         | Same                                   |

The `authz-pdp` server itself does not cache. Caching is the PEP's
responsibility.

## Examples

### grpcurl — list and inspect

```bash
# Requires authz-pdp running with reflection; see docker-compose.yml

# List services
grpcurl -plaintext localhost:9550 list
# → arrowhead.authz.v1.AuthorizationPDP
# → grpc.reflection.v1alpha.ServerReflection

# Describe the service
grpcurl -plaintext localhost:9550 describe arrowhead.authz.v1.AuthorizationPDP

# Describe message types
grpcurl -plaintext localhost:9550 describe arrowhead.authz.v1.DecisionRequest
grpcurl -plaintext localhost:9550 describe arrowhead.authz.v1.Decision
```

### grpcurl — make decisions

```bash
# Orchestration: portal-cloud-ml may consume telemetry from robot-fleet-site-1
grpcurl -plaintext \
  -d '{"subject":"portal-cloud-ml","service":"telemetry","provider":"robot-fleet-site-1","action":"orchestrate"}' \
  localhost:9550 arrowhead.authz.v1.AuthorizationPDP/Decide
# → {"decision":"PERMIT","statusCode":"urn:oasis:names:tc:xacml:1.0:status:ok"}

# Orchestration: unknown consumer — no policy → NOT_APPLICABLE → deny
grpcurl -plaintext \
  -d '{"subject":"unknown","service":"telemetry","provider":"robot-fleet-site-1","action":"orchestrate"}' \
  localhost:9550 arrowhead.authz.v1.AuthorizationPDP/Decide
# → {"decision":"NOT_APPLICABLE","statusCode":"urn:oasis:names:tc:xacml:1.0:status:ok"}

# Enforcement: portal-cloud-ml may consume telemetry (no provider constraint)
grpcurl -plaintext \
  -d '{"subject":"portal-cloud-ml","service":"telemetry","action":"consume"}' \
  localhost:9550 arrowhead.authz.v1.AuthorizationPDP/Decide
# → {"decision":"PERMIT","statusCode":"urn:oasis:names:tc:xacml:1.0:status:ok"}
```

### Per-provider revocation flow

```bash
# Revoke site-2 access only (orchestration plane)
POL_ID=$(curl -s http://localhost:9505/policies | \
  jq -r '.policies[] | select(.subject=="portal-cloud-ml" and .resource=="telemetry" and .provider=="robot-fleet-site-2") | .id')
curl -s -X DELETE http://localhost:9505/policies/$POL_ID

# Confirm: site-2 now denied, site-1 and site-3 still permitted
grpcurl -plaintext \
  -d '{"subject":"portal-cloud-ml","service":"telemetry","provider":"robot-fleet-site-2","action":"orchestrate"}' \
  localhost:9550 arrowhead.authz.v1.AuthorizationPDP/Decide
# → {"decision":"NOT_APPLICABLE",...}
```

## Regenerating Go code

```bash
# From the proto directory (requires protoc + plugins):
make -C core-evol/proto/authorize gen

# Or via Docker (no local protoc needed):
docker run --rm -v "$(pwd)/core-evol/proto/authorize:/workspace" \
  -w /workspace golang:1.22-alpine sh -c \
  "apk add -q --no-cache protoc &&
   go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2 &&
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.4.0 &&
   protoc --go_out=. --go_opt=paths=source_relative \
          --go-grpc_out=. --go-grpc_opt=paths=source_relative authorize.proto"
```

## Implementing a new PDP

To build a conforming PDP from this proto:

1. Implement `AuthorizationPDPServer` (generated interface in `authorize_grpc.pb.go`).
2. In `Decide`: evaluate the request against your policy store.
   - Map `subject` → XACML `subject-id`
   - Map `service` → XACML `resource-id`
   - Map `provider` (when non-empty) → XACML `urn:arrowhead:attribute:provider-id`
   - Map `action` → XACML `action-id`
3. Return `PERMIT`, `DENY`, `NOT_APPLICABLE`, or `INDETERMINATE`.
4. Enable gRPC reflection: `reflection.Register(grpcServer)`.
5. Respect fail-closed: return a gRPC error on internal failure, not `PERMIT`.
