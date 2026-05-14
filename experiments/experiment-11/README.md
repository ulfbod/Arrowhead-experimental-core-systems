# Experiment 11 — Hybrid PAP/PIP/PDP (Strategy A)

## Goal

Implement **Strategy A** of the PAP/PIP/PDP split: a hybrid architecture where two policy sources
are merged into a single XACML PolicySet at push time.

| Source | Path to AuthzForce | Latency |
|---|---|---|
| PAP-native policies | PAP → AuthzForce (direct push on every Create/Delete) | Instant |
| ConsumerAuth grants | ConsumerAuth → PIP (poll) → PAP SyncFromPIP() → AuthzForce | ≤ sync interval (10 s) |

This contrasts with **experiment-10 (Strategy B)** where PAP is the exclusive source of truth and there is no PIP sync loop — only PAP-native policies exist, with zero sync delay but requiring manual policy management.

## Architecture

```
ConsumerAuth ──(poll /authorization/lookup every 10 s)──▶ PIP
                                                           │
                          ┌────────────────────────────────┘
                          │  SyncFromPIP() when version changes
                          ▼
Robot Fleet ──Kafka TLS──▶ kafka-authz ─────────────────▶ AuthzForce (PDP)
Robot Fleet ──AMQP TLS──▶  topic-auth-xacml ─────────────▶ AuthzForce
Service Partners ─mTLS──▶  pki-rest-authz ───────────────▶ AuthzForce
                                                                ▲
PAP ─────────(native policy Create/Delete — instant push)───────┘
```

### PAP (Policy Administration Point)

- Stores native XACML policies in memory (subject, resource, action, effect).
- On every Create/Delete, calls `triggerPush()`: fetches PIP grants and merges both sources, then calls `authzforce.SetPolicy()`.
- Background sync loop: calls `SyncFromPIP()` every `SYNC_INTERVAL`. Only rebuilds XACML if PIP version has changed.
- Deduplicates by `subject+resource` key when merging — Deny-effect native policies take precedence over PIP grants but are excluded from the XACML grant list (only Permit rules are sent).

### PIP (Policy Information Point)

- Polls ConsumerAuth `/authorization/lookup` at configurable intervals.
- Stores a versioned grant cache: version increments only when grant content actually changes (order-insensitive comparison).
- Exposes `/grants` with optional `?subject=X` and `?subject=X&resource=Y` filtering.
- `/status` reports: `synced`, `version`, `grants`, `lastSyncAt`, `consumerAuthURL`.

## Ports

| Service | Host port |
|---|---|
| profile-ca HTTP | 8387 |
| profile-ca mTLS | 8388 |
| AuthzForce | 8496 |
| ServiceRegistry TLS | 8790 |
| Authentication TLS | 8791 |
| ConsumerAuthorization TLS | 8792 |
| DynamicOrchestration TLS | 8793 |
| PAP | 9405 |
| PIP | 9406 |
| kafka-authz | 9401 |
| pki-rest-authz mTLS | 9408 |
| pki-rest-authz HTTP | 9409 |
| portal-cloud-ml | 9407 |
| service-partner-1 | 9411 |
| service-partner-2 | 9412 |
| robot-fleet-site-1 | 9416 |
| robot-fleet-site-2 | 9417 |
| robot-fleet-site-3 | 9418 |
| RabbitMQ management | 15879 |
| Dashboard | 3011 |

## Running

```bash
cd experiments/experiment-11
docker compose up --build
```

Dashboard: http://localhost:3011  
Admin UI: http://localhost:3011/admin.html

## Testing

```bash
cd experiments/experiment-11
docker compose up -d --build
bash test-system.sh
```

Unit/integration tests (no Docker):

```bash
go test arrowhead/pap11 -cover
go test arrowhead/pip11 -cover
```

## Key design decisions

**Why merge at push time?**  
XACML evaluation is stateless — AuthzForce evaluates against a single uploaded PolicySet. Merging at push time avoids adding PIP attribute lookups to the hot path (every authorization request).

**Why version-based PIP sync?**  
ConsumerAuth grants rarely change. A version check avoids re-uploading the same XACML PolicySet on every sync tick.

**Graceful degradation**  
If PIP is temporarily unavailable, `triggerPush()` proceeds with empty PIP grants (native policies still enforced). `SyncFromPIP()` silently skips failed poll cycles.

**Deduplication**  
When the same subject+resource appears in both a native Permit policy and a PIP grant, only one rule is generated. This prevents redundant XACML Policy elements and keeps the PolicySet minimal.
