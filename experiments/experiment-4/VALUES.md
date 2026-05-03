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

### 3. ConsumerAuthorization remains the single policy authority

Without the integration in experiment-4, a broker-mediated deployment has two
independent policy authorities: CA (controlling which consumers may orchestrate)
and the broker's native ACLs (controlling which consumers may connect). These
must be kept in sync manually, and revocation in one does not affect the other.

Experiment-4 eliminates the independent broker policy. RabbitMQ's
`rabbitmq-auth-backend-http` plugin delegates all authorization decisions to
`topic-auth-sync`, which derives every permission from CA and from nothing else.
The broker has no autonomous policy authority. An operator managing grants in CA
is managing the complete access-control state for both the orchestration layer
and the broker layer.

### 4. Revocation has a formal, bounded guarantee across all enforcement points

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

- **Layer 2 — Data plane (bounded, δ ≤ T_sync = 10 s).**
  `topic-auth-sync` polls CA at interval `T_sync` and reconciles RabbitMQ user
  and permission state. A revoked grant causes the corresponding RabbitMQ user
  to be deleted. The broker terminates the active AMQP connection for that user.
  The consumer is disconnected from the broker within one `T_sync` cycle.

Together, these layers provide a revocation guarantee that is impossible to
achieve with a standalone broker or with CA and broker ACLs managed
independently:

| Scenario | Standalone broker | Experiment-4 |
|---|---|---|
| Consumer calls DO after revocation | DO denies (CA checked) | DO denies (immediate) |
| Consumer already connected to broker | Still connected (broker unaware) | Disconnected within T_sync |
| Consumer reconnects to broker with old credentials | Succeeds if credentials are valid | Fails (user deleted by topic-auth-sync) |
| Consumer reconnects via DO after T_sync | DO denies | DO denies |

### 5. No broker configuration knowledge required by consumers or operators

In a standalone-broker deployment, every consumer must be pre-configured with
the broker address, port, virtual host, exchange name, and its own credentials.
Adding a new consumer means updating broker ACLs and providing all of those
values. Adding a new provider means informing every consumer of the new
exchange or routing key.

In experiment-4, consumers learn broker coordinates from the orchestration
response. Adding a second `robot-fleet` instance requires only a new
ServiceRegistry registration; existing consumers discover it at their next
orchestration call without any reconfiguration. Adding a new consumer requires
only a new CA grant; `topic-auth-sync` provisions the corresponding RabbitMQ
user within one `T_sync` cycle.

### 6. Identity verification closes the credential-theft attack vector

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

| Concern | Broker used separately | Experiment-4 |
|---|---|---|
| **Cross-network connectivity** | Requires bilateral reachability or VPN mesh | Both sides connect outward to broker; no inbound exposure needed |
| **Service discovery** | Hardcoded broker coordinates | Broker address and routing metadata returned by DynamicOrchestration |
| **Policy authority** | CA + broker ACLs (two independent sources) | CA only; broker permissions are derived projections |
| **Revocation — control plane** | Immediate at DO | Immediate at DO |
| **Revocation — data plane** | Unbounded (active connections persist) | Bounded (≤ T_sync = 10 s) |
| **Consumer configuration** | Broker address, exchange, credentials required | Identity credentials and orchestration URL only |
| **Adding a provider** | Notify all consumers of new coordinates | Register in SR; consumers adapt at next orchestration call |
| **Adding a consumer** | Configure broker ACLs + distribute credentials | Add CA grant; topic-auth-sync provisions broker user |
| **Impersonation resistance** | Credential-based only | Identity token verified by DO before CA check |
| **AHF spec compliance** | Partial (CA only) | Full: SR + AUTH + CA + DO all in the governance loop |

---

## What experiment-4 does not address

- **mTLS transport security.** The CertificateAuthority service is declared and
  running but not wired into the service code. All inter-system calls are plain
  HTTP. This is Phase 5 in the integration plan and is identified as the
  remaining open gap.
- **Multi-broker federation.** The architecture assumes a single broker reachable
  by all parties. Extending the transformation to federated topologies with
  cross-cloud grant scoping is left for future work.
- **Push-based revocation.** `topic-auth-sync` polls CA on a fixed interval.
  Replacing polling with CA-emitted change events would reduce the data-plane
  revocation window from `T_sync` to near-zero while retaining reconciliation as
  a safety net.
