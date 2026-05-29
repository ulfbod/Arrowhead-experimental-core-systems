# AH5 Conformance Assessment — ArrowheadCore Go Implementation

**Purpose:** Research/assessment view of AH5 specification conformance.
Tracks ratings, context-based impact, and phase priorities.

**Gap detail:** All gap descriptions, root causes, design decisions, and fix notes live
exclusively in `core/GAP_ANALYSIS.md`. This document references those G-IDs; it does not
duplicate their content.

**Spec reviewed:** AH5 documentation as of May 2026
(https://aitia-iiot.github.io/ah5-docs-java-spring/ — full site traversal).

---

## Rating Methodology

Ratings assess three orthogonal dimensions:

| Dimension | Weight | What it measures |
|-----------|--------|-----------------|
| **Endpoint coverage** | 40% | Fraction of spec-defined endpoints implemented with correct HTTP method, path, and response codes |
| **Data model accuracy** | 30% | Closeness of request/response field names, types, and structures to spec |
| **Behavioral conformance** | 30% | Correctness of semantic behaviour (validation, filtering, error codes, lifecycle) |

**Overall** = 0.4 × Endpoint% + 0.3 × Model% + 0.3 × Behavior%

Ratings reflect all resolved steps through Step 21. Open gaps are annotated with their G-ID.

---

## Per-System Ratings

| System | Endpoint% | Model% | Behavior% | Overall | Key open gaps |
|--------|-----------|--------|-----------|---------|---------------|
| ServiceRegistry | 75 | 70 | 65 | **~70%** | G11, G20, G34, G37 |
| Authentication | 80 | 72 | 65 | **~73%** | G20, G37, G43 |
| ConsumerAuthorization | 60 | 65 | 55 | **~60%** | G20, G23 (partial), G37, G38, G39, G42 |
| DynamicOrchestration | 80 | 65 | 60 | **~69%** | G20, G25 (intercloud), G26 (delivery), G37, G40 (result fields) |
| SimpleStoreOrchestration | 75 | 65 | 60 | **~67%** | G20, G25 (intercloud), G37, G40 (result fields) |
| Blacklist | 100 | 70 | 50 | **~76%** | G41, G42 |
| GeneralManagement (cross-cutting) | 100 | 85 | 75 | **~88%** | G37 |
| FlexibleStoreOrchestration | N/A | N/A | N/A | Extension | No spec (G1) |
| CertificateAuthority | N/A | N/A | N/A | Extension | Not in spec (G9) |

**Notes:**
- Blacklist endpoint% is 100% (all five spec endpoints implemented); overall dragged down by
  missing enforcement integration (G42) and Bearer/mode gaps (G41).
- GeneralManagement is a cross-cutting capability, not an independent system.
- FlexibleStoreOrchestration and CertificateAuthority are extensions with no AH5 spec
  counterpart; conformance ratings are not applicable.

---

## Context Impact Key

| Level | Meaning |
|-------|---------|
| **None** | No observable effect in this context |
| **Low** | Minor inconvenience; workaround exists |
| **Medium** | Limits capability or requires adaptation |
| **High** | Materially impairs the use case |
| **Blocker** | Makes the use case impossible or unsafe |

**Contexts:** PoC · Teaching · Prototyping · Production

---

## Open Gaps — Impact and Phase

| Gap | Description | PoC | Teaching | Prototyping | Production | Phase |
|-----|-------------|-----|----------|-------------|------------|-------|
| **G11** | System revoke derives identity from token, not `?name=` | Low | Medium | High | Blocker | **1** |
| **G43** | `credentials` not validated as `{"password":"..."}` object | None | Low | Low | Medium | **1** |
| **G25** (intercloud) | ALLOW_INTERCLOUD/ONLY_INTERCLOUD silently ignored; should 501 | None | Low | Medium | High | **1** |
| **G40** (result fields) | OrchestrationResult missing `cloudIdentifier`, `exclusiveUntil`, `interfaces[]` | None | Low | Medium | High | **1** |
| **G41** | Blacklist Bearer not enforced on discovery; `mode` enum mismatch | None | Medium | Medium | High | **1** |
| **G20** | No pagination on any query/list endpoint | None | Low | Medium | High | **2** |
| **G26** (delivery) | Push notification delivery is a stub; no HTTP call to subscriber | Medium | Medium | High | Blocker | **2** |
| **G37** | Management endpoints open to any caller; no access policy | None | Medium | High | Blocker | **2** |
| **G38** | authorizationTokenManagement bulk endpoints absent | None | None | Medium | High | **2** |
| **G39** | authorizationManagement bulk endpoints absent | None | None | Medium | High | **2** |
| **G42** | Blacklist not integrated with SR, Orchestration, ConsumerAuth | Low | Medium | High | Blocker | **2** |
| **G23** (variants) | Token variants USAGE_LIMITED, JWT not implemented | None | Low | Medium | High | **3** |
| **G34** | No MQTT/MQTTS communication profiles | Low | Medium | High | High | **3** |
| **G35** | Device QoS Evaluator support system not implemented | None | Medium | Medium | High | **3** |
| **G36** | Translation Manager support system not implemented | None | Low | Medium | High | **3** |

---

## Resolved Gaps (Steps 1–21)

| Gap | Description | Step |
|-----|-------------|------|
| G3 | Predictable tokens → crypto/rand UUID v4 | 1 |
| G13 | ServiceInstanceID composite string format | 2 |
| G14 | Version normalisation (empty → `1.0.0`) | 2 |
| G17 | `alivesAt` filtering in service lookup | 3 |
| G18 | 423 Locked on device revoke with dependents | 3 |
| G19 | Naming convention validation (PascalCase, UPPER_SNAKE, etc.) | 4 |
| G16 | Metadata query operators (six operators + shorthand) | 5 |
| G12 | ConsumerAuth path prefix `/consumerauthorization` | 6 |
| G15 | Authentication HTTP method and response-field alignment | 7 |
| G24 | Orchestration path `/serviceorchestration/orchestration/pull` | 8 |
| G25 | `orchestrationFlags` (MATCHMAKING, ONLY_PREFERRED) | 8 |
| G5 | Persistence — SQLite backend via `DB_PATH` | 9 |
| G8 | Expired-token background cleanup | 12 |
| G2 | Credential verification (bcrypt) | 13 |
| G21 | Authentication management endpoints (identity + session mgmt) | 13 |
| G22 | ConsumerAuth provider-centric policy model | 14 |
| G23 | TIME_LIMITED token generation and verify | 15 (partial) |
| G30 | Service interface model + ServiceLookupRequest empty-filter guard | 16 |
| G7 | DynamicOrchestration uses AH5 SR lookup endpoint | 17 |
| G27 | Lock management + orchestration history | 18 |
| G26 | Subscribe/unsubscribe + push management endpoints | 19 (partial) |
| G28 | Blacklist system (all five endpoints) | 20 |
| G29 | GeneralManagement on all systems | 21 |

---

## Phase Plan

| Phase | Steps | Focus |
|-------|-------|-------|
| **Phase 1** | 22–26 | Wire-compatibility: five low-effort gaps that break spec-compliant clients |
| **Phase 2** | 27–31 (TBD) | Functional completeness: pagination, push delivery, access policy, bulk endpoints |
| **Phase 3** | 32+ (TBD) | Advanced features: additional token types, Blacklist integration, support systems |

See `CONFORMANCE_UPDATE_PLAN.md` for the detailed TDD execution plan.

---

## Extensions beyond AH5 (not conformance gaps)

| Feature | Gap | Description |
|---------|-----|-------------|
| Certificate Authority (`/ca`) | G9 | Issues and revokes X.509 ECDSA certs; no AH5 counterpart |
| FlexibleStore Orchestration | G1 | Priority-weighted, metadata-filtered orchestration; AH5 spec page "Coming soon" |
| DynamicOrch identity check (`ENABLE_IDENTITY_CHECK`) | D8 | Optional token-gated pull; explicit extension beyond spec |

---

*Last updated: 2026-05-29 (restructured — G-IDs only; gap detail moved to core/GAP_ANALYSIS.md)*
