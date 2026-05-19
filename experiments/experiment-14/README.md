# Experiment 14 — Connection-Time Certificate Revocation Enforcement

## What this experiment demonstrates

Experiment-14 extends experiment-13 with **connection-time certificate revocation
enforcement** across both Kafka and RabbitMQ, implementing design decision D2'.

### The gap in experiment-13

In experiment-13, the PEP passes `certValid` as a XACML subject attribute to
AuthzForce, which evaluates it as part of the policy. However, the custom
AuthzForce server in the experiment-13 stack does not enforce `certValid` in the
XACML policy — it only checks whether a grant (Permit policy) exists for the
subject. This means a consumer with a revoked cert can still receive Permit if a
grant policy exists. The cert is structurally valid (mTLS succeeds), but PIP says
it is revoked.

### The fix: D2' — cert-valid pre-gate at connection time

Experiment-14 adds a pre-gate layer before AuthzForce is ever called:

**RabbitMQ (topic-auth-xacml):**
- `handleUser` and `handleVhost` query PIP immediately after password verification.
- If `certValid=false` (revoked, unknown, or PIP unreachable), the AMQP connection
  is rejected with "deny" — without calling AuthzForce at all.
- This enforces revocation at AMQP connection setup, not only at message time.

**Kafka (ArrowheadPrincipalBuilder plugin):**
- A Java `KafkaPrincipalBuilder` plugin is installed in the Kafka broker.
- After the TLS handshake completes (cert is structurally valid, signed by CA),
  the plugin queries PIP before returning a principal.
- If `certValid=false`, an `AuthenticationException` is thrown, rejecting the
  connection before any Kafka protocol exchange.
- The PIP URL is passed via `KAFKA_OPTS=-Darrowhead.pip.url=http://pip:9506`.

### Fail-closed behavior

Both enforcement points are fail-closed:
- If PIP is unreachable → connection denied (not permitted).
- If PIP returns 404 (unknown CN) → connection denied.
- If PIP JSON is malformed → connection denied.

## Architecture

See [DIAGRAMS.md](DIAGRAMS.md) for sequence diagrams.

```
Client → Kafka TLS handshake
      → ArrowheadPrincipalBuilder.build()
         → GET /pip/attributes/{cn}
         → certValid=false? → AuthenticationException (connection rejected)
         → certValid=true?  → KafkaPrincipal(cn)
      → kafka-authz /auth/check (message-level enforcement continues)

Client → RabbitMQ AMQP connect (mTLS)
      → topic-auth-xacml /auth/user
         → password check
         → GET /pip/attributes/{cn}
         → certValid=false? → "deny" (connection rejected, PDP not called)
         → certValid=true?  → AuthzForce PDP → "allow"/"deny"
      → /auth/vhost (same pre-gate pattern)
```

## Components

| Component | Language | Change from exp-13 |
|---|---|---|
| `topic-auth-xacml` | Go | D2' pre-gate in `handleUser` / `handleVhost` |
| `kafka-principal-builder` | Java (Maven) | New: KafkaPrincipalBuilder plugin |
| All others | Go | Unchanged — use exp-13 sources |

## Design decisions

| ID | Decision | Where enforced |
|---|---|---|
| D1 | No PEP-side caching of PIP responses | topic-auth-xacml, kafka-authz |
| D2' | cert-valid pre-gate at connection time (replaces D2) | topic-auth-xacml (handleUser/handleVhost), Kafka plugin |
| D3 | Hard mTLS requirement at broker level | RabbitMQ config, Kafka config |

D2' replaces D2 (exp-13's "cert-valid forwarded to AuthzForce"). The PDP is no
longer responsible for cert-validity enforcement at connection time.

## How to run

```bash
cd experiments/experiment-14
docker compose up --build -d
bash test-system.sh
```

Dashboard: http://localhost:3014

## Host ports

All ports are experiment-13 host ports + 100:

| Service | Host port |
|---|---|
| profile-ca HTTP | 8687 |
| profile-ca mTLS | 8688 |
| profile-ca gRPC | 8689 |
| AuthzForce | 8796 |
| ServiceRegistry TLS | 9090 |
| Authentication | 9091 |
| ConsumerAuthorization | 9092 |
| DynamicOrch-XACML | 9093 |
| authz-pdp gRPC | 9750 |
| PAP | 9705 |
| PIP | 9706 |
| kafka-authz | 9701 |
| pki-rest-authz mTLS | 9708 |
| pki-rest-authz HTTP | 9709 |
| portal-cloud-ml | 9707 |
| robot-fleet-site-1 | 9716 |
| robot-fleet-site-2 | 9717 |
| robot-fleet-site-3 | 9718 |
| service-partner-1 | 9711 |
| service-partner-2 | 9712 |
| RabbitMQ management | 16179 |
| Kafdrop | 9014 |
| Dashboard | 3014 |

## Dashboard

Dashboard at **http://localhost:3014**. Three views:

- **Index** — live health status for all services; shows which cert identities are registered in PIP
- **Demo** — issue a cert via profile-ca, trigger an orchestration request, and observe connection-time rejection vs. message-level enforcement
- **Admin** — revoke a certificate via `DELETE /ca/certificates/{cn}` and verify that the next Kafka or AMQP connection is rejected before the PDP is ever called

## Testing

Go unit tests for topic-auth-xacml:

```bash
cd experiments/experiment-14/services/topic-auth-xacml
GOWORK=off go test ./...
```

Java unit tests for kafka-principal-builder (run inside Maven build during Docker build):

```bash
cd experiments/experiment-14/services/kafka-principal-builder
mvn test
```

Key new tests in `server_test.go`:
- `TestHandleUser_consumerCertRevoked_deniedBeforePDP` — revoked cert denied before PDP
- `TestHandleUser_consumerCertValid_proceedsToPDP` — valid cert proceeds to PDP
- `TestHandleVhost_consumerCertRevoked_deniedAtConnection` — revoked cert denied at vhost
- `TestHandleUser_pipUnreachable_failClosed` — PIP unreachable → deny (fail-closed)
