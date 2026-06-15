# Experiment 13 gRPC Interfaces — Authorization Stack

Two gRPC interfaces are used in the experiment-13 authorization stack:

| Interface | Package | Server | Client | Pattern |
|-----------|---------|--------|--------|---------|
| **AuthorizationPDP** | `arrowhead.authz.v1` | `authz-pdp` :9550 (host :9650) | `DynamicOrchestration` (GRPCDecider) | Unary RPC |
| **CertificateLifecycle** | `arrowhead.ca.v1` | `profile-ca` :8089 (host :8589) | `PIP` subscriber | Server-streaming RPC |

---

## 1. AuthorizationPDP

### Service

- **Package**: `arrowhead.authz.v1`
- **Proto**: `core-evol/proto/authorize/authorize.proto`
- **Server**: `authz-pdp` — gRPC adapter in front of AuthzForce CE
- **Client**: `DynamicOrchestration` via `GRPCDecider` (`core-evol/internal/orchestration/service.go`)
- **Transport**: gRPC plaintext
- **Architecture position**: This is a canonical PDP–PEP interface with three non-standard aspects.
  The PEP is a **control-plane** actor (service discovery), not a data-plane enforcement point — it decides
  which provider addresses to *disclose*, not whether to allow a connection. The `action` field acts as a
  **namespace separator** (`"orchestrate"` vs `"consume"`) rather than an operation verb. The `provider`
  field makes per-provider IoT policies possible, which has no direct equivalent in web-service XACML.
  See `grpc-summary.md` for full analysis.
- **grpcurl inspection**:

```bash
grpcurl -plaintext localhost:9650 list
grpcurl -plaintext localhost:9650 describe arrowhead.authz.v1.AuthorizationPDP
grpcurl -plaintext -d '{"subject":"portal-cloud-ml","service":"telemetry","provider":"robot-fleet-site-1","action":"orchestrate"}' \
  localhost:9650 arrowhead.authz.v1.AuthorizationPDP/Decide
```

---

### RPC: Decide

```
AuthorizationPDP
└── Decide
     ├── Purpose
     ├── Request schema
     ├── Response schema
     ├── Example request
     ├── Example response
     └── Error codes
```

#### Purpose

Evaluates a single access-control request and returns PERMIT or DENY. The
DynamicOrchestrator calls `Decide` once per provider candidate found in the
ServiceRegistry. Only providers that receive `PERMIT` are included in the
final orchestration response. Implements XACML 3.0 Request/Response semantics
over gRPC. Fail-closed: any error or non-PERMIT decision causes the provider
to be excluded.

Two action namespaces keep orchestration and enforcement policies from
colliding:

- `action = "orchestrate"` — orchestration PEP (this service); `provider` is
  set, enabling per-provider policy evaluation.
- `action = "consume"` — enforcement PEPs (kafka-authz, pki-rest-authz);
  `provider` is empty.

#### Request schema: `DecisionRequest`

| Field | Type | Required | XACML mapping | Description |
|-------|------|----------|---------------|-------------|
| `domain_id` | string | No | AuthzForce domain UUID | Policy domain. May be empty for single-domain deployments. |
| `subject` | string | Yes | `subject-id` | Consumer system name as registered in ServiceRegistry. |
| `service` | string | Yes | `resource-id` | Service definition name (e.g. `"telemetry"`). |
| `provider` | string | Orchestration only | `urn:arrowhead:attribute:provider-id` | Provider system name. Set by orchestration PEPs; empty for enforcement PEPs. |
| `action` | string | Yes | `action-id` | `"orchestrate"` for this service; `"consume"` for enforcement. |

#### Response schema: `DecisionResponse`

| Field | Type | Description |
|-------|------|-------------|
| `decision` | `Decision` enum | `PERMIT`, `DENY`, `INDETERMINATE`, or `NOT_APPLICABLE`. |
| `status_code` | string | XACML status URN (informational only — act on `decision`). |

#### Example request (orchestration, with provider)

```json
{
  "domain_id": "ah5-domain",
  "subject":   "portal-cloud-ml",
  "service":   "telemetry",
  "provider":  "robot-fleet-site-1",
  "action":    "orchestrate"
}
```

#### Example response (permit)

```json
{
  "decision":    "PERMIT",
  "status_code": "urn:oasis:names:tc:xacml:1.0:status:ok"
}
```

#### Example response (deny)

```json
{
  "decision":    "DENY",
  "status_code": "urn:oasis:names:tc:xacml:1.0:status:ok"
}
```

#### Error codes

| gRPC status | Decision value | Meaning | PEP action |
|-------------|----------------|---------|------------|
| `OK` | `PERMIT` | Policy grants access | Include provider in result |
| `OK` | `DENY` | Policy denies access | Exclude provider (fail-closed) |
| `OK` | `INDETERMINATE` | Evaluation error or missing attribute | Exclude provider (fail-closed) |
| `OK` | `NOT_APPLICABLE` | No policy matched | Exclude provider (fail-closed) |
| `UNAVAILABLE` | — | authz-pdp unreachable | Exclude provider (fail-closed) |
| `DEADLINE_EXCEEDED` | — | RPC timeout | Exclude provider (fail-closed) |
| `INTERNAL` | — | authz-pdp internal error | Exclude provider (fail-closed) |

---

### Message definitions

```protobuf
// authorize.proto — arrowhead.authz.v1

service AuthorizationPDP {
  rpc Decide(DecisionRequest) returns (DecisionResponse);
}

message DecisionRequest {
  string domain_id = 1;  // AuthzForce domain UUID (may be empty)
  string subject   = 2;  // consumer system name → XACML subject-id
  string service   = 3;  // service definition   → XACML resource-id
  string provider  = 4;  // provider system name → XACML provider-id (orchestration only)
  string action    = 5;  // "orchestrate" | "consume"
}

message DecisionResponse {
  Decision decision    = 1;  // PERMIT | DENY | INDETERMINATE | NOT_APPLICABLE
  string   status_code = 2;  // XACML status URN (informational)
}

enum Decision {
  DECISION_UNSPECIFIED = 0;  // zero value; treat as DENY
  PERMIT               = 1;  // access granted
  DENY                 = 2;  // access denied
  INDETERMINATE        = 3;  // evaluation error; treat as DENY
  NOT_APPLICABLE       = 4;  // no policy matched; treat as DENY
}
```

---

---

## 2. CertificateLifecycle

### Service

- **Package**: `arrowhead.ca.v1`
- **Proto**: `core-evol/proto/certlifecycle/certlifecycle.proto`
- **Server**: `profile-ca` — CA service with gRPC reflection enabled
- **Client**: `PIP` (Policy Information Point) — subscribes on startup
- **Transport**: gRPC plaintext, server-side streaming (persistent connection)
- **Architecture position**: This is **not a PDP–PEP interface**. No authorization decision is made here.
  It is a **CA-to-PIP attribute provisioning channel**: it synchronises authentication state (cert validity,
  cert tier) into the PIP SubjectStore that feeds downstream XACML subject attributes. Classic XACML has the
  PDP pull attributes from the PIP synchronously per request; here the model is **inverted** — the PIP
  subscribes and pre-populates, and enforcement PEPs query the PIP before calling the PDP. Certificate
  issuance and revocation events are causally coupled to authorization outcomes: a revoked certificate
  causes XACML DENY even if the TLS handshake somehow passed. See `grpc-summary.md` for full analysis.
- **grpcurl inspection**:

```bash
grpcurl -plaintext localhost:8589 list
grpcurl -plaintext localhost:8589 describe arrowhead.ca.v1.CertificateLifecycle
grpcurl -plaintext -d '{"include_snapshot":true}' \
  localhost:8589 arrowhead.ca.v1.CertificateLifecycle/Subscribe
```

---

### RPC: Subscribe

```
CertificateLifecycle
└── Subscribe
     ├── Purpose
     ├── Request schema
     ├── Response schema (stream)
     ├── Example request
     ├── Example stream responses
     └── Reconnect / error semantics
```

#### Purpose

Opens a persistent server-side streaming RPC from `profile-ca` to a subscriber
(PIP, audit log, or monitoring). The stream carries `CertEvent` messages as
certificate lifecycle transitions occur (issued, revoked, expired).

When `include_snapshot = true`, the server first sends one `SNAPSHOT` event per
currently-valid certificate before switching to live events. This bootstraps
the subscriber from scratch without a separate REST call, which is critical for
PIP on startup.

The stream remains open until the client cancels or the server shuts down.
Subscribers run the call in a reconnect loop with exponential backoff
(initial 1 s, max 30 s). On reconnect, `include_snapshot = true` re-baselines
the subscriber's state.

Fail-closed semantics: if the stream is interrupted, the subscriber retains its
last known state and does **not** purge its store. Authorization decisions made
while the stream is down use stale-but-non-empty cert-level attributes, which
is safer than denying everything.

#### Request schema: `SubscribeRequest`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `include_snapshot` | bool | No (default `false`) | If `true`, server pre-streams one `SNAPSHOT` event per currently-valid certificate before live events. Use on first connect and after reconnects. |

#### Response schema: stream `CertEvent`

| Field | Type | Present when | Description |
|-------|------|-------------|-------------|
| `cn` | string | Always | Certificate Common Name — the system identity. Maps to XACML `subject-id`. |
| `ou` | string | Always | Arrowhead cert level encoded in Subject OU: `lo`, `on`, `de`, `sy`. Maps to XACML `urn:arrowhead:attribute:cert-level`. |
| `type` | `EventType` enum | Always | `ISSUED`, `REVOKED`, `EXPIRED`, or `SNAPSHOT`. |
| `issued_at` | string | `ISSUED`, `SNAPSHOT` | RFC3339 issuance timestamp. |
| `expires_at` | string | `ISSUED`, `SNAPSHOT` | RFC3339 expiry timestamp. |

#### Cert level (`ou`) values

| Value | Tier | Privilege |
|-------|------|-----------|
| `lo` | Local Cloud CA | Root; not used for service identity |
| `on` | Onboarding | May request Device certs |
| `de` | Device | May request System certs |
| `sy` | System | Used for service-to-service mTLS ← most common in experiment-13 |

#### Example request (first connect)

```json
{ "include_snapshot": true }
```

#### Example stream response sequence

```
// ── Phase 1: snapshot of existing certificates ──────────────────────────────
{ "cn": "robot-fleet-site-1", "ou": "sy", "type": "SNAPSHOT",
  "issued_at": "2026-06-10T08:00:00Z", "expires_at": "2027-06-10T08:00:00Z" }

{ "cn": "portal-cloud-ml",    "ou": "sy", "type": "SNAPSHOT",
  "issued_at": "2026-06-10T08:01:00Z", "expires_at": "2027-06-10T08:01:00Z" }

// ── Phase 2: live events ─────────────────────────────────────────────────────
{ "cn": "new-sensor-1",       "ou": "sy", "type": "ISSUED",
  "issued_at": "2026-06-15T10:00:00Z", "expires_at": "2027-06-15T10:00:00Z" }

{ "cn": "old-sensor",         "ou": "sy", "type": "REVOKED",
  "issued_at": "", "expires_at": "" }

{ "cn": "expired-device",     "ou": "de", "type": "EXPIRED",
  "issued_at": "", "expires_at": "" }
```

#### Reconnect and error semantics

| Condition | EventType | Subscriber action |
|-----------|-----------|------------------|
| Stream open; snapshot event received | `SNAPSHOT` | `Register(cn, ou, valid=true)` |
| New certificate issued | `ISSUED` | `Register(cn, ou, valid=true)` |
| Certificate revoked | `REVOKED` | `Register(cn, ou, valid=false)` |
| Certificate expired | `EXPIRED` | `Register(cn, ou, valid=false)` |
| Stream EOF or network error | — | Reconnect with exponential backoff (1 s → 30 s max); retain last known state |
| profile-ca unreachable at startup | — | Retry loop; serve empty store; fail-closed on first auth check |
| `EVENT_UNSPECIFIED` received | — | Ignore |

---

### Message definitions

```protobuf
// certlifecycle.proto — arrowhead.ca.v1

service CertificateLifecycle {
  // Subscribe opens a persistent server-side stream of CertEvents.
  // The stream remains open until the client cancels or the server shuts down.
  rpc Subscribe(SubscribeRequest) returns (stream CertEvent);
}

message SubscribeRequest {
  // include_snapshot=true: server sends SNAPSHOT events for all currently
  // valid certs before switching to live events.
  bool include_snapshot = 1;
}

message CertEvent {
  string    cn         = 1;  // cert Common Name  → XACML subject-id
  string    ou         = 2;  // cert level: "lo" | "on" | "de" | "sy"
  EventType type       = 3;  // lifecycle event type
  string    issued_at  = 4;  // RFC3339; present for ISSUED/SNAPSHOT only
  string    expires_at = 5;  // RFC3339; present for ISSUED/SNAPSHOT only
}

enum EventType {
  EVENT_UNSPECIFIED = 0;  // zero value; ignore
  ISSUED            = 1;  // new cert issued     → Register(cn, ou, valid=true)
  REVOKED           = 2;  // cert revoked        → Register(cn, ou, valid=false)
  EXPIRED           = 3;  // cert expired        → Register(cn, ou, valid=false)
  SNAPSHOT          = 4;  // initial state event → Register(cn, ou, valid=true)
}
```
