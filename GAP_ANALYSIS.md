# AH5 Gap Analysis

**Date:** 2026-04-29  
**Scope:** Comparison of current implementation against Arrowhead 5 (java-spring) core systems documentation.

---

## Current State

Only the **ServiceRegistry** is implemented:

| Endpoint | Method | Status |
|---|---|---|
| `/serviceregistry/register` | POST | ✓ Implemented |
| `/serviceregistry/query` | POST | ✓ Implemented (AH4-style; AH5 uses GET lookup) |
| `/health` | GET | ✓ Implemented |
| `/serviceregistry/unregister` | DELETE | ✗ Missing |
| `/serviceregistry/lookup` | GET | ✗ Missing |

---

## Missing Core Systems

| AH5 System | Status | Notes |
|---|---|---|
| ServiceRegistry | Partial | Missing revoke + lookup; no system/device discovery |
| ConsumerAuthorization | ✗ Missing | Full new system |
| Authentication | ✗ Missing | Full new system |
| DynamicServiceOrchestration | ✗ Missing | Full new system |
| SimpleStoreServiceOrchestration | ✗ Missing | Full new system |
| FlexibleStoreServiceOrchestration | ✗ Missing | AH5 docs say "coming soon" — designed from first principles |

---

## Detailed Gaps

### ServiceRegistry

**Missing from AH5:**
- `DELETE /serviceregistry/unregister` (revoke operation)
- `GET /serviceregistry/lookup` (AH5-style read-only lookup)
- System discovery (`POST/DELETE/GET /systemdiscovery/...`)
- Device discovery (`POST/DELETE/GET /devicediscovery/...`)
- Management service (bulk admin operations)

**Kept as-is (backward compat):**
- `POST /serviceregistry/register` — backward compatible with existing experiments
- `POST /serviceregistry/query` — kept alongside new GET lookup

### ConsumerAuthorization

AH5 responsibilities: authorization rules (grant/revoke/lookup/verify) + token generation.

**All missing:**
- `POST /authorization/grant`
- `DELETE /authorization/revoke/{id}`
- `GET /authorization/lookup`
- `POST /authorization/verify`
- `POST /authorization/token/generate`
- `GET /authorization/token/verify`

### Authentication

AH5 responsibilities: identity token management (login/logout/verify).

**All missing:**
- `POST /authentication/identity/login`
- `DELETE /authentication/identity/logout`
- `GET /authentication/identity/verify`

### DynamicServiceOrchestration

AH5: finds matching service instances dynamically (SR lookup + auth check).

**All missing:**
- `POST /orchestration/dynamic` (pull)

### SimpleStoreServiceOrchestration

AH5: returns providers from pre-configured peer-to-peer store rules.

**All missing:**
- `POST /orchestration/simplestore` (pull)
- `GET /orchestration/simplestore/rules`
- `POST /orchestration/simplestore/rules`
- `DELETE /orchestration/simplestore/rules/{id}`

### FlexibleStoreServiceOrchestration

AH5 docs: "Coming soon." Designed based on SimpleStore with priority ordering.

**All missing:**
- `POST /orchestration/flexiblestore` (pull, priority-ordered)
- `GET /orchestration/flexiblestore/rules`
- `POST /orchestration/flexiblestore/rules`
- `DELETE /orchestration/flexiblestore/rules/{id}`

---

## Architectural Gaps

| Issue | Current | AH5 |
|---|---|---|
| Number of binaries | 1 (service registry only) | 6 distinct core systems |
| Orchestration | None | Three separate strategies |
| Authorization | None | Consumer-specific authorization + tokens |
| Authentication | None | Identity token lifecycle |
| Service revocation | None | DELETE revoke |
| Separation of concerns | SR only | 6 independent systems communicating via HTTP |

---

## Design Decisions and Assumptions

1. **Go, not Spring Boot.** The repository uses Go. All new systems are implemented in Go.
2. **In-memory storage.** All new systems use in-memory repositories, consistent with the existing SR.
3. **DynamicOrchestration calls SR + ConsumerAuth.** When `enable.authorization=true` (configurable), it cross-checks ConsumerAuth. By default this check is skipped for standalone use.
4. **FlexibleStore design.** Since AH5 docs are "coming soon", FlexibleStore is designed as SimpleStore + priority field + metadata filter matching on rules. Documented assumption in SPEC.md.
5. **Backward compatibility.** All existing SR endpoints remain unchanged. Experiments require zero changes.
6. **Token format.** Authentication tokens are opaque UUIDs. No JWT/cryptographic signing (experimental implementation).
7. **No database.** All state is in-memory and lost on restart, consistent with existing SR.
8. **System discovery.** AH5 also defines systemDiscovery and deviceDiscovery on the SR. These are deferred — they represent significant scope beyond the primary task objectives.

---

## Port Assignments

| System | Default Port | Env Var |
|---|---|---|
| ServiceRegistry | 8080 | PORT |
| Authentication | 8081 | PORT |
| ConsumerAuthorization | 8082 | PORT |
| DynamicOrchestration | 8083 | PORT |
| SimpleStoreOrchestration | 8084 | PORT |
| FlexibleStoreOrchestration | 8085 | PORT |
