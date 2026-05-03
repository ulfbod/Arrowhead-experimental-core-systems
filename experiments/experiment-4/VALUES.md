# Experiment 4 — Key Values

This document explains what experiment-4 adds beyond naively attaching a message
broker to an Arrowhead local cloud, and why those additions matter specifically
for geo-distributed deployments.

---

## Baseline: broker used separately from Arrowhead core

Using a message broker alongside an Arrowhead local cloud without integration
means the two systems operate independently:

- The broker has its own native access-control lists (ACLs).
- Consumers learn the broker address from environment configuration, not from
  ServiceRegistry or DynamicOrchestration.
- Authorization grants in ConsumerAuthorization (CA) and broker ACLs are
  managed separately: revoking a grant in CA has no effect on the broker.
- Any process with the right broker credentials can connect, bypassing
  Arrowhead's identity and orchestration layer entirely.
- Service discovery and broker connectivity are orthogonal concerns with no
  shared governance.

This is operationally convenient within a single local cloud where all services
share a network. It becomes structurally problematic once the local cloud spans
multiple network domains.

---

## Problem context: geo-distributed local clouds

In a geo-distributed deployment, providers and consumers reside on separate
networks with no direct path between them — typical for industrial servitization
scenarios where robots, gateways, and analytics platforms belong to different
sites or operators.

Two problems arise that neither plain Arrowhead nor a standalone broker
addresses:

**P1 — Endpoint unreachability.**
A provider behind cellular NAT or a closed site network cannot expose a directly
reachable endpoint. Arrowhead's service lifecycle assumes the provider registers
its own `address:port`; the consumer opens a direct connection to that address.
That connection fails when the provider is unreachable from the consumer's
network. VPN overlays solve the reachability problem but require bilateral
provisioning for every producer-consumer pair, which scales poorly.

**P2 — Policy fragmentation.**
Introducing a broker creates a second policy authority. Revoking a grant in CA
does not terminate the broker connection. The broker's native ACLs must be kept
in sync manually, undermining auditability and the principle that the local cloud
has a single source of truth for access control.

---

## Values delivered by experiment-4

### 1. Outward connectivity eliminates direct-path requirements

A message broker placed at a mutually reachable point (cloud DMZ, shared
infrastructure) is reachable by both provider and consumers via outbound
connections. Neither side needs to expose an inbound port or establish a
bilateral VPN. Experiment-4 realizes this by registering the broker's address
`e_b` in ServiceRegistry in place of the provider's own `address:port`
(transformation `T`). From the Arrowhead governance layer, service registration
and discovery work exactly as before; what changes is only what is placed in the
address and metadata fields.

### 2. Standard consumer orchestration flow is preserved

Consumers call DynamicOrchestration exactly as they would for any Arrowhead
service. They receive a broker address, exchange name, and routing-key pattern
from the orchestration response and derive their subscription key locally.
Consumers do not need to know that the underlying transport is a broker rather
than a direct endpoint. The full four-step cycle — identity login, orchestration,
authorization token, AMQP connect — is identical in structure to what a
consumer would execute against a direct REST provider.

This means the same consumer code works across local clouds where some services
use direct HTTP and others are broker-mediated.

### 3. Single source of truth: policy defined once, enforced at multiple independent points

The core architectural principle of experiment-4 is a strict separation between
*where policy is defined* and *where policy is enforced*.

**Policy** — the set of rules expressing which consumer may use which service —
is defined exclusively in ConsumerAuthorization as `(consumer, provider, service)`
triples. No other component can add or remove rules. CA is the single source of
truth.

**Enforcement points** are components that make access-control decisions. In
experiment-4 there are two: DynamicOrchestration (Layer 1) and RabbitMQ via
`topic-auth-http` (Layer 2). Neither enforcement point holds an independent
policy. Each maintains only a *derived projection* of the CA rule set — a
representation suited to its own access-control model (orchestration result
filtering and broker topic permissions, respectively). When CA changes, every
projection converges to reflect that change within a bounded window.

Without this integration, a broker-mediated deployment has two *independent*
policy authorities: CA controls orchestration access; the broker's native ACLs
control connection access. These are managed separately. Revoking a grant in CA
does not affect the broker, and the broker's permissions cannot express
Arrowhead-level semantics such as `(consumer, provider, service)` triples.

The practical consequences of the single-source architecture:

- **Operators manage policy in one place.** A grant added or revoked in CA is
  the complete management action. There is no second step to update broker ACLs.
- **Audit is centralized.** All access-control decisions trace back to CA. There
  is no shadow policy at the broker that could diverge from the CA record.
- **New enforcement points do not create new policy authorities.** Adding a
  second broker, a gateway, or any other component that needs to enforce access
  control means connecting it as a new derived projection of CA — not creating a
  new policy store. The pattern generalises: the same CA rule set can be
  projected into any number of heterogeneous enforcement mechanisms without
  splitting policy authority.

### 4. The broker decouples providers from consumers

In a direct service-oriented model without a broker, a consumer opens a
persistent connection to the provider's own endpoint. This creates three forms
of tight coupling:

**Spatial coupling.** The provider must expose a reachable address. Every
consumer must know that address. If the provider moves, is replaced, or scales
to multiple instances, every consumer must be reconfigured or re-orchestrated.

**Logical coupling.** The provider is aware of each incoming consumer connection.
Scaling to N consumers means N simultaneous connections to the provider process.
The provider's load scales directly with the number of active consumers. Adding
or removing consumers is observable by the provider.

**Temporal coupling.** The provider and consumer must be online simultaneously
for data to flow. A consumer that is temporarily unavailable (restart, network
partition) misses all data produced during the outage, because there is nowhere
for the data to be buffered.

A message broker eliminates all three forms of coupling:

- The provider publishes to the broker and has no knowledge of how many
  consumers exist, who they are, or where they are located. Its load is
  one outbound connection regardless of consumer count.
- Consumers subscribe independently. Adding a new consumer is transparent to
  the provider; no provider configuration or code changes.
- The broker buffers messages in queues. A consumer that reconnects after a
  transient disconnection can resume from the last acknowledged message (subject
  to queue durability and retention settings).

**In the context of geo-distributed local clouds,** this decoupling is
especially valuable. A telemetry provider in a factory local cloud publishes to
a shared broker. Analytics consumers in a cloud local cloud subscribe
independently. The factory and cloud sites are operationally independent: either
can restart, scale, or relocate without the other being aware, as long as both
maintain connectivity to the broker.

Without experiment-4's integration, this decoupling would exist at the transport
layer but not at the governance layer: consumers would still need out-of-band
knowledge of broker credentials and exchange names, and authorization would
still be managed separately at the broker. Experiment-4 extends the decoupling
into the governance layer: consumers learn all broker coordinates from
DynamicOrchestration, and access control is managed solely in CA.

### 5. Revocation has a formal, bounded guarantee across all enforcement points

In a standalone-broker setup, revoking a grant in CA stops future
re-orchestrations but does not affect active broker connections.
A consumer already connected continues to receive messages until it reconnects.
The window is unbounded in the absence of forced disconnection.

Experiment-4 provides two independently enforced revocation effects:

- **Layer 1 — Control plane (immediate, δ = 0).**
  DynamicOrchestration reads CA directly on every orchestration request
  (`ENABLE_AUTH=true`, `ENABLE_IDENTITY_CHECK=true`). A revoked grant causes DO
  to return an empty provider list on the consumer's next attempt (within ≤ 5 s,
  set by the consumer retry interval). The consumer cannot obtain a broker
  address regardless of what credentials it holds.

- **Layer 2 — Data plane (live HTTP authz backend).**
  `topic-auth-http` serves the RabbitMQ HTTP authorization backend API. RabbitMQ
  delegates all authentication and authorization to `topic-auth-http`
  (`auth_backends.1 = rabbit_auth_backend_http`). Every vhost access, resource
  operation, and topic routing-key check is evaluated against CA live. A revoked
  grant causes the next broker operation by that consumer to be denied immediately
  — there is no polling delay.

  **Revocation timing by scenario:**

  | Scenario | Mechanism | Window |
  |---|---|---|
  | Consumer reconnects after revocation | Vhost authz check → CA no grant → deny connection | Immediate (sub-second) |
  | Publisher publish after revocation | Topic authz check on each basic.publish → deny | Within next publish cycle |
  | Active idle subscriber (connection never drops) | No current mechanism | Unbounded (open issue) |

  **Cache TTL.** `topic-auth-http` caches CA responses for `CACHE_TTL`
  (default `0s` — no caching). Zero TTL means every authorization check hits CA
  live, at the cost of one CA HTTP call per broker operation. Operators may set a
  short TTL (e.g., `1s`–`5s`) to reduce CA load; the revocation window for
  reconnect scenarios then extends by at most one TTL period.

  **CA availability.** With all auth delegated to the HTTP backend, a CA outage
  causes authorization checks to fail with `deny` (safe default). New consumer
  connections and publish operations will be refused until CA is reachable again.
  This is the correct behaviour for a security-critical system but must be
  accounted for in the CA availability SLA.

  The conditional-bound caveat from the polling-only architecture still applies:
  if `topic-auth-http` cannot reach CA, authz checks return `deny` by default,
  which means revocation is over-enforced (safe) rather than under-enforced
  during a partition. This is the opposite failure mode from the polling-only
  approach, where a partition left stale grants in force.

Together, these layers provide a revocation guarantee that is impossible to
achieve with a standalone broker or with CA and broker ACLs managed
independently:

| Scenario | Standalone broker | Experiment-4 |
|---|---|---|
| Consumer calls DO after revocation | DO denies (CA checked) | DO denies (immediate) |
| Consumer reconnects to broker after revocation | Succeeds if credentials are valid | Denied at vhost check (immediate) |
| Publisher publishes after its grant is revoked | Continues until ACL updated | Denied on next publish (sub-second) |
| Active subscriber (connection never drops) | Still subscribed indefinitely | No mechanism (open issue — see below) |
| Consumer reconnects via DO after grant revoked | DO denies | DO denies (immediate) |

### 6. No broker configuration knowledge required by consumers or operators

In a standalone-broker deployment, every consumer must be pre-configured with
the broker address, port, virtual host, exchange name, and its own credentials.
Adding a new consumer means updating broker ACLs and providing all of those
values. Adding a new provider means informing every consumer of the new
exchange or routing key.

In experiment-4, consumers learn broker coordinates from the orchestration
response. Adding a second `robot-fleet` instance requires only a new
ServiceRegistry registration; existing consumers discover it at their next
orchestration call without any reconfiguration. Adding a new consumer requires
only a new CA grant; `topic-auth-http` provisions the corresponding RabbitMQ
user within one `SYNC_INTERVAL` cycle (default 60 s).

### 7. Identity verification closes the credential-theft attack vector

In a standalone-broker deployment, broker credentials are the sole identity
proof. A stolen password gives access equivalent to the legitimate consumer.
Revoking broker access requires knowing that the credentials were compromised.

In experiment-4, DO verifies the consumer's identity token via Authentication
(`ENABLE_IDENTITY_CHECK=true`) before answering. The verified identity from the
token replaces the self-reported system name for all CA checks. A compromised
consumer cannot impersonate another, and a consumer that has had its CA grant
revoked cannot orchestrate regardless of what credentials it presents to the
broker.

---

## Summary table

| Concern | Direct SOA (no broker) | Broker used separately | Experiment-4 |
|---|---|---|---|
| **Cross-network connectivity** | Provider must be reachable by consumer | Both sides connect outward; no inbound exposure | Both sides connect outward; no inbound exposure |
| **Service discovery** | Provider address from DO | Hardcoded broker coordinates | Broker address and routing metadata from DO |
| **Policy authority** | CA only | CA + broker ACLs (two independent sources) | CA only; broker permissions are derived projections |
| **Policy management** | One place (CA) | Two places (CA + broker ACLs) | One place (CA) |
| **Audit trail** | Centralized in CA | Split across CA and broker logs | Centralized in CA |
| **Revocation — control plane** | Immediate at DO | Immediate at DO | Immediate at DO |
| **Revocation — data plane** | n/a (no persistent connection after request) | Unbounded (active connections persist) | Immediate on reconnect/publish (HTTP authz); unbounded for active idle subscribers (open issue) |
| **Provider load** | Scales with N consumers | One outbound connection to broker | One outbound connection to broker |
| **Adding a consumer** | Add CA grant | Configure broker ACLs + distribute credentials | Add CA grant; RabbitMQ user is not required (HTTP auth backend handles auth live) |
| **Consumer knows provider address** | Yes (direct connection) | No (connects to broker) | No (connects to broker) |
| **Temporal coupling** | Provider and consumer must be online together | Broker queues buffer messages | Broker queues buffer messages |
| **Consumer configuration** | Provider address from DO | Broker address, exchange, credentials required | Identity credentials and orchestration URL only |
| **Impersonation resistance** | Identity token verified by DO | Credential-based only | Identity token verified by DO before CA check |
| **AHF spec compliance** | Full | Partial (CA only) | Full: SR + AUTH + CA + DO all in the governance loop |

---

## What experiment-4 does not address

- **mTLS transport security.** The CertificateAuthority service is declared and
  running but not wired into the service code. All inter-system calls are plain
  HTTP. This is Phase 5 in the integration plan and is identified as the
  remaining open gap.
- **Multi-broker federation.** The architecture assumes a single broker reachable
  by all parties. Extending the transformation to federated topologies with
  cross-cloud grant scoping is left for future work.
- **Active subscriber termination without reconnect.** An active AMQP consumer
  that never disconnects will not be denied by the HTTP authz backend until it
  attempts a new operation (bind or reconnect). There is currently no backstop
  mechanism: no periodic sync, no force-close. A targeted force-close of the AMQP
  connection via the RabbitMQ management API after CA revocation would close this
  residual window but is not yet implemented.

---

## References

[1] RabbitMQ HTTP auth backend — plugin source, endpoint protocol, and
    configuration options:
    https://github.com/rabbitmq/rabbitmq-auth-backend-http

[2] RabbitMQ Access Control guide — authn/authz split (`auth_backends.N.authn` /
    `auth_backends.N.authz`), HTTP backend, and topic authorization:
    https://www.rabbitmq.com/docs/access-control

[3] Open Policy Agent — externalized authorization / Policy Decision Point pattern:
    https://www.openpolicyagent.org/docs/latest/

[4] Kubernetes controller pattern — level-triggered reconciliation (desired vs.
    current state, self-healing control loop):
    https://kubernetes.io/docs/concepts/architecture/controller/

[5] HashiCorp Vault RabbitMQ secrets engine — dynamic credential generation with
    TTL-based revocation as an alternative to live authz queries:
    https://developer.hashicorp.com/vault/docs/secrets/rabbitmq
