# Experiment 15 — CA-as-PIP

## What this experiment demonstrates

Experiment-15 introduces **design decision D4: CA-as-PIP**. The separate `pip` service from experiments 13–14 is removed. Instead, `profile-ca` serves PIP API endpoints (`/pip/attributes/{cn}`, `/pip/subjects`, `/pip/status`) directly from its in-memory certificate store.

This eliminates:
- The gRPC `CertificateLifecycle.Subscribe` subscriber pattern (pip service)
- State replication lag between CA issuance and PIP awareness
- The potential for PIP divergence during gRPC reconnection
- One Docker service in the dependency chain

All PEP services (`kafka-authz`, `topic-auth-xacml`, `pki-rest-authz`) and the Kafka plugin that previously pointed to `http://pip:9506` now point to `http://profile-ca:8787`.

## Architecture

```
profile-ca (:8787)
  │
  ├── /ca/...            — cert issuance & revocation (unchanged)
  ├── /pip/attributes/   — XACML-ready cert validity (CA-as-PIP, D4)
  ├── /pip/subjects      — all cert records including revoked
  ├── /pip/status        — subject count summary
  ├── mTLS :8788         — profile-enforced cert issuance
  └── gRPC :8789         — CertificateLifecycle.Subscribe (kept for compatibility)

PEP services
  ├── kafka-authz        — Kafka message-level enforcement (PIP_URL → profile-ca)
  ├── topic-auth-xacml   — RabbitMQ connection-level enforcement (PIP_URL → profile-ca)
  └── pki-rest-authz     — HTTPS REST proxy enforcement (PIP_URL → profile-ca)

Kafka broker (ArrowheadPrincipalBuilder plugin)
  └── queries profile-ca /pip/attributes/{cn} at TLS handshake time
```

## Design decision D4 — CA-as-PIP

The `ProfileCA` struct already holds the authoritative record of every certificate it has issued, revoked, or expired. Rather than replicating that state into a separate `pip` service over gRPC, D4 exposes the cert store directly as PIP endpoints:

- **Consistency**: PIP responses are always in sync with CA state — there is no replication lag.
- **Simplicity**: One fewer service, one fewer subscriber, one fewer failure mode.
- **Revocation propagation**: Cert revocation is immediately visible to all PEPs without waiting for gRPC event delivery.
- **The gRPC stream is retained** for forward compatibility and for potential future consumers.

## Differences from experiment-14

| Aspect | Experiment-14 | Experiment-15 |
|---|---|---|
| PIP service | Separate `pip` Docker service | Removed |
| PIP data source | gRPC CertificateLifecycle stream → in-memory store | CA cert store directly |
| PIP URL | `http://pip:9506` | `http://profile-ca:8787` |
| Revocation lag | ~1 gRPC event cycle | Zero (cert store IS the PIP) |
| profile-ca HTTP port | 8087 (host: 8687) | 8787 (host: 8787) |
| AUTHZFORCE_DOMAIN | `arrowhead-exp14` | `arrowhead-exp15` |

## Port assignments

| Service | Internal port | Host port |
|---|---|---|
| profile-ca HTTP (+ PIP) | 8787 | 8787 |
| profile-ca mTLS | 8788 | 8788 |
| profile-ca gRPC | 8789 | 8789 |
| AuthzForce | 8080 | 8896 |
| ServiceRegistry TLS | 8490 | 9190 |
| Authentication TLS | 8491 | 9191 |
| ConsumerAuth TLS | 8492 | 9192 |
| DynamicOrch-XACML | 8083 | 9193 |
| authz-pdp gRPC | 9550 | 9850 |
| PAP | 9505 | 9805 |
| kafka-authz | 9101 | 9801 |
| pki-rest-authz mTLS | 9208 | 9808 |
| pki-rest-authz HTTP | 9209 | 9809 |
| portal-cloud-ml | 9207 | 9807 |
| service-partner-1 | 9201 | 9811 |
| service-partner-2 | 9202 | 9812 |
| RobotFleetSite1 | 9003 | 9816 |
| RobotFleetSite2 | 9003 | 9817 |
| RobotFleetSite3 | 9003 | 9818 |
| RabbitMQ management | 15672 | 16279 |
| Kafdrop | 9000 | 9114 |
| Dashboard | 80 | 3015 |

## Startup order

1. `profile-ca` — CA + PIP (no dependencies)
2. `cert-provisioner` — issues all service certs (depends on profile-ca)
3. `rabbitmq`, `authzforce`, `kafka` — infrastructure (depends on cert-provisioner)
4. `serviceregistry`, `authentication`, `consumerauth` — Arrowhead core
5. `pap` — policy administration
6. `setup` — seeds XACML policies (depends on pap, profile-ca, authentication)
7. `authz-pdp` — gRPC PDP (depends on authzforce, pap, setup)
8. `kafka-authz`, `topic-auth-xacml`, `pki-rest-authz` — PEP services
9. `dynamicorch-xacml`, `portal-cloud-ml`, `robot-fleet-site-*`, `service-partner-*`

## Running

```bash
cd experiments/experiment-15
docker compose up -d --build
bash test-system.sh
```
