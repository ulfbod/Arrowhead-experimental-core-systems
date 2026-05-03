# Experiment 4 — Authorization Architecture Analysis

This document analyses the design decisions behind the broker authorization architecture
in experiment-4, the revocation latency properties of each approach, and how the
implementation relates to established design patterns and open-source alternatives.

---

## 1. The revocation-delay problem

Experiment-4 uses a message broker (RabbitMQ) as the transport layer for telemetry
data flowing between a robot-fleet provider and multiple analytics consumers. The
Arrowhead ConsumerAuthorization (CA) service is the single source of truth for access
policy: which consumer may access which service from which provider.

The problem is that RabbitMQ maintains its own internal user and permission state.
When an operator revokes a grant in CA, that change must propagate to RabbitMQ for
the revocation to have any effect on live broker connections.

### Original approach: polling sync (topic-auth-sync)

The original `topic-auth-sync` service solved this by polling CA at a fixed interval
`T_sync` (default 10 s) and reconciling RabbitMQ's user and topic-permission state
with the current CA rule set. This gives a bounded but non-zero revocation window:

```
δ_revocation ≤ T_sync    (under normal network conditions)
```

The bound is conditional: if `topic-auth-sync` cannot reach CA or RabbitMQ within
one poll cycle (network partition, CA unavailability), the bound extends to
2×T_sync, 3×T_sync, and so on until connectivity is restored.

---

## 2. RabbitMQ HTTP auth backend

RabbitMQ's `rabbitmq_auth_backend_http` plugin [1] allows any authorization decision
to be delegated to an external HTTP service. RabbitMQ calls configured endpoints for:

| Operation | Endpoint | When called |
|---|---|---|
| Client connects | `/auth/user` | At connection establishment (authn only if HTTP handles authn) |
| Vhost access | `/auth/vhost` | At connection establishment |
| Resource operation | `/auth/resource` | On queue.declare, exchange.declare, etc. |
| Topic publish/bind | `/auth/topic` | On every basic.publish and queue.bind |

The plugin supports separate backends for authentication (authn) and authorization (authz),
configurable in `rabbitmq.conf` as [2]:

```ini
auth_backends.1.authn = rabbit_auth_backend_internal
auth_backends.1.authz = rabbit_auth_backend_http
```

Experiment-4 uses this split model. The internal backend validates passwords; the HTTP
backend (served by `topic-auth-http`) handles all authorization decisions by querying CA live.

---

## 3. Implementation: topic-auth-http

`topic-auth-http` is a Go service at `support/topic-auth-http/` that:

1. **Serves the RabbitMQ HTTP authz backend API** — handles `/auth/vhost`,
   `/auth/resource`, and `/auth/topic` endpoints. Each handler fetches the current
   CA rule set and makes an allow/deny decision for the requesting user.

2. **Runs a background reconciliation sync** — at `SYNC_INTERVAL` (default 60 s),
   creates and deletes RabbitMQ internal users to match the current CA rule set.
   This is the safety net: it ensures that internal-backend authentication continues
   to work for valid consumers and that stale users are eventually cleaned up.

### Revocation behaviour

| Event | Mechanism | Timing |
|---|---|---|
| Consumer reconnects after revocation | Vhost authz check → CA returns no grant → deny | Immediate (sub-second) |
| Publisher publishes after revocation of its grant | Topic authz check on each publish → deny | Within next publish cycle |
| Active subscriber (connection never drops) | Reconciliation sync deletes user | Within SYNC_INTERVAL (60 s) |
| Consumer reconnects after user deletion | Internal authn fails (user gone) | Immediate |

The primary improvement over polling-only: a revoked consumer that attempts to
reconnect is denied immediately by the vhost check, without waiting for the next
sync cycle.

### Cache (CACHE_TTL)

`topic-auth-http` caches CA responses for `CACHE_TTL` (default `0s` — no caching).
With zero TTL, every authorization check hits CA live. Operators may set a short TTL
(e.g., `1s` or `5s`) to reduce CA load at the cost of a small additional revocation
window bounded by the TTL.

---

## 4. Design patterns

### 4.1 Externalized Authorization / Policy Decision Point

The architecture is an instance of the **Externalized Authorization** pattern,
formalized in XACML as a separation between the Policy Administration Point (PAP),
Policy Decision Point (PDP), and Policy Enforcement Points (PEP):

- **PAP + PDP**: ConsumerAuthorization — the sole authority that defines and evaluates policy
- **PEP 1**: DynamicOrchestration — enforces at control-plane (orchestration time)
- **PEP 2**: RabbitMQ + topic-auth-http — enforces at data-plane (broker operation time)

Open Policy Agent (OPA) [3] is the dominant open-source realization of this pattern.
OPA holds policy as Rego rules; enforcement points query OPA per-request or receive
cached "bundle" projections. The CA + topic-auth-http combination is structurally
identical: CA is the policy store and evaluator; topic-auth-http is the adapter that
translates RabbitMQ's auth API into CA queries.

### 4.2 Control-plane / Data-plane separation

The Layer 1 (DO) / Layer 2 (broker) split mirrors the architecture of service meshes
such as Istio/Envoy, where a control plane holds policy and pushes it via xDS to data-plane
sidecar proxies that enforce it on live traffic.

### 4.3 Level-triggered reconciliation (Kubernetes controller pattern)

The background sync loop in `topic-auth-http` follows the **level-triggered controller
pattern** [4] used by Kubernetes controllers: periodically reconcile observed state (RabbitMQ
users) with desired state (CA rules), regardless of whether a change event was received.
This is more robust than a purely event-driven model because it self-heals from missed
events, partial failures, and drift.

The HTTP authz backend provides the **edge-triggered** (low-latency) path; the reconciliation
loop provides the **level-triggered** (safety-net) path. The combination is the same principle
used in Kubernetes operators.

---

## 5. Alternatives

### HashiCorp Consul

Consul's service catalog [see https://developer.hashicorp.com/consul/docs] can replace
ServiceRegistry; Consul Intentions can replace CA grants for mTLS-based service mesh
enforcement. However, Consul has no native mechanism to project Intentions into RabbitMQ
ACLs. A `topic-auth-http`-style adapter that reads Consul Intentions and serves the
RabbitMQ HTTP auth backend API would realize the same architecture with Consul as the
policy source.

### HashiCorp Vault dynamic secrets for RabbitMQ

Vault's RabbitMQ secrets engine [5] generates short-lived, lease-bound credentials on
demand. When a consumer's Vault token is revoked, RabbitMQ credentials are revoked
with it. The revocation window is bounded by the credential TTL (configurable down to
seconds), and forced revocation is immediate via Vault's lease revocation API.

This is a fundamentally different approach: rather than querying a policy source at
each operation, credentials expire. It addresses the revocation problem without
requiring an HTTP auth backend, but it does not model the Arrowhead
`(consumer, provider, service)` semantics — Vault roles map to RabbitMQ virtual hosts
and permissions, not to service-definition triples.

### EMQX (MQTT broker)

EMQX [see https://www.emqx.io/docs] has a significantly more mature external
authorization model than RabbitMQ. It supports HTTP, JWT, LDAP, and database backends
natively, with configurable result caching TTLs. For MQTT-based IIoT deployments,
EMQX is a closer out-of-the-box match for the experiment-4 governance requirements.

---

## 6. What remains open

- **Active subscriber termination without reconnect.** An active subscriber whose
  grant is revoked will not be cut off by the HTTP authz backend until they attempt
  an operation that triggers a topic or vhost check. The reconciliation sync (at
  SYNC_INTERVAL) is the backstop, but this window is longer than the near-instant
  revocation for reconnect attempts. A force-connection-close API call to RabbitMQ
  management after CA revocation would eliminate this residual window.

- **CA availability as a hard dependency for authz.** When authn=internal and
  authz=HTTP, a CA outage causes all topic and vhost checks to fail (the handler
  returns `deny` on error). This is the safe default, but it means a CA outage
  prevents new consumer connections and any publish/bind operations. Operators
  should account for this in their CA availability SLA.

- **mTLS transport security.** The CertificateAuthority service is declared but not
  wired into the service code. All inter-system calls remain plain HTTP.

---

## References

[1] RabbitMQ HTTP auth backend plugin — source, configuration options, and endpoint
    protocol: https://github.com/rabbitmq/rabbitmq-auth-backend-http

[2] RabbitMQ Access Control guide — authn/authz split configuration, backend chaining,
    and topic authorization: https://www.rabbitmq.com/docs/access-control

[3] Open Policy Agent — externalized policy decision point pattern and bundle-based
    policy distribution: https://www.openpolicyagent.org/docs/latest/

[4] Kubernetes controller pattern — level-triggered reconciliation (desired vs. current
    state): https://kubernetes.io/docs/concepts/architecture/controller/

[5] HashiCorp Vault RabbitMQ secrets engine — dynamic credential generation with TTL
    and lease revocation: https://developer.hashicorp.com/vault/docs/secrets/rabbitmq
