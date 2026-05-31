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

---

## Per-System Ratings

Ratings reflect all resolved steps through Step 39 (Phase 3 complete).

| System | Endpoint% | Model% | Behavior% | Overall | Key open gaps |
|--------|-----------|--------|-----------|---------|---------------|
| ServiceRegistry | 87 | 80 | 85 | **~84%** | — (G10, G34 resolved) |
| Authentication | 85 | 80 | 78 | **~81%** | — |
| ConsumerAuthorization | 80 | 75 | 75 | **~77%** | — (G23 variants resolved) |
| DynamicOrchestration | 90 | 82 | 88 | **~87%** | — (G40 QoS filtering resolved) |
| SimpleStoreOrchestration | 80 | 75 | 83 | **~79%** | — |
| Blacklist | 100 | 85 | 90 | **~92%** | — |
| GeneralManagement (cross-cutting) | 100 | 85 | 90 | **~92%** | — (G34 MQTT adapter resolved) |
| FlexibleStoreOrchestration | N/A | N/A | N/A | Extension | No spec (G1) |
| CertificateAuthority | N/A | N/A | N/A | Extension | Not in spec (G9) |
| DeviceQoSEvaluator | 90 | 90 | 85 | **~88%** | — (new in Phase 3) |
| TranslationManager | 90 | 90 | 85 | **~88%** | — (new in Phase 3) |

**Notes:**
- G11, G25 (intercloud), G40 (result fields), G41, G43 resolved in Phase 1 (Steps E1–E5).
- G37, G42, G20, G38, G39, G26 resolved in Phase 2 (Steps 27–31).
- G10, G23 (variants), G34 (MQTT adapter), G35, G36, G40 (QoS filtering) resolved in Phase 3 (Steps 33–38).
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

*No open conformance gaps remain. All gaps from Phases 1–3 are resolved.*

| Gap | Description | PoC | Teaching | Prototyping | Production | Phase |
|-----|-------------|-----|----------|-------------|------------|-------|
| **G34** (MQTTS) | MQTT over TLS not implemented (plain MQTT adapter is in place) | Low | Low | Medium | High | — |

---

## Resolved Gaps (Steps 1–21 and Phase 1)

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
| G11 | System revoke derives identity from Bearer token (fail-closed) | E1 |
| G41 | Blacklist Bearer enforcement on discovery; `mode` enum in mgmt/query | E2 |
| G40 (result fields) | `cloudIdentifier`, `exclusiveUntil`, `interfaces[]` in OrchestrationResult | E3 |
| G25 (intercloud) | ALLOW_INTERCLOUD / ONLY_INTERCLOUD → 501 Not Implemented | E4 |
| G43 | `credentials` validated as `{"password":"..."}` object; plain string → 400 | E5 |
| G37 | Management access policy — sysop Bearer guard on all `/mgmt/*` endpoints via `MGMT_AUTH_URL` | 27 |
| G42 | Blacklist integration — `BlacklistClient` wired into SR, Orchestration, ConsumerAuth, CA | 28 |
| G20 | Pagination — `model.Paginate[T]` applied to all query/list endpoints across all systems | 29 |
| G38 | ConsumerAuth `authorization-token/mgmt` bulk endpoints (5 endpoints) | 30 |
| G39 | ConsumerAuth `authorization/mgmt` bulk endpoints (4 endpoints) | 30 |
| G26 (delivery) | Push trigger delivers HTTP POST to subscriber `notifyInterface`; DELIVERED/FAILED history | 31 |
| G10 | Registration identity enforcement — `REGISTER_AUTH_URL`; Bearer token name must match request body `name`/`systemName` | 33 |
| G23 (variants) | `USAGE_LIMITED_TOKEN` (counter-based) and `BASE64_SELF_CONTAINED` (HMAC-SHA256) token variants | 34 |
| G35 | Device QoS Evaluator — TCP RTT probe, measurement store, management query; `core/cmd/deviceqoseval` (port 8088) | 35 |
| G40 (QoS filtering) | `qualityRequirements[]` in OrchestrationRequest; DynamicOrch filters candidates by latency via QoS client | 36 |
| G36 | Translation Manager — JSON field-remapping bridges, CRUD, translate endpoint; `core/cmd/translationmgr` (port 8089) | 37 |
| G34 | MQTT adapter (`core/internal/mqttutil`) — subscribe/publish with AH5 topic conventions; `MQTT-INSECURE-JSON` interface | 38 |

---

## Phase Plan

| Phase | Steps | Focus | Status |
|-------|-------|-------|--------|
| **Phase 1** | E1–E5 | Wire-compatibility: five gaps that break spec-compliant clients | **Complete** |
| **Phase 2** | 27–32 | Functional completeness: access policy, Blacklist integration, pagination, bulk endpoints, push delivery | **Complete** |
| **Phase 3** | 33–39 | Advanced conformance: registration identity, token variants, QoS evaluation, support systems, MQTT | **Complete** |

### Phase 2 — Step breakdown

| Step | Gap(s) | Focus | Priority | Affects core-evol |
|------|--------|-------|----------|-------------------|
| 27 | G37 | Management access policy (`MGMT_ACCESS_POLICY`) — sysop-only Bearer guard on all `/mgmt/*` endpoints | **Blocker** (Production) | Yes — handler.go |
| 28 | G42 | Blacklist integration — `BlacklistClient` wired into SR register, Orchestration filter, ConsumerAuth grant/verify | **Blocker** (Production) | Yes — service.go |
| 29 | G20 | Pagination — generic `Paginate[T]` helper applied to all query/list endpoints | High | Minimal |
| 30 | G38, G39 | ConsumerAuth bulk endpoints — `mgmt/grant-policies`, `mgmt/revoke-policies`, `mgmt/query-policies`, `mgmt/check-policies`, `mgmt/generate-tokens`, `mgmt/revoke-tokens`, `mgmt/query-tokens` | High | No |
| 31 | G26 | Push notification HTTP delivery — actual HTTP call to subscriber `notifyInterface` on trigger | High | Yes — service.go |
| 32 | — | Phase 2 documentation update — CONFORMANCE.md, CONFORMANCE_UPDATE_PLAN.md, GAP_ANALYSIS.md, SPEC.md, EXAMPLES.md, README.md | — | — |

See `CONFORMANCE_UPDATE_PLAN.md` for the detailed TDD execution plan (Steps 27–32).

### Phase 3 — Step breakdown

**Order:** Step 33 (G10) is a Production blocker and should be done first. Steps 34 and 35 are independent of each other. Step 36 is blocked on Step 35 (G35). Steps 37 and 38 are independent of all others. Step 38 (MQTT) is the highest-effort item and should be done last. Step 39 is the documentation sweep.

| Step | Gap(s) | Focus | Priority | Affects core-evol |
|------|--------|-------|----------|-------------------|
| 33 | G10 | Registration identity enforcement — `REGISTER_AUTH_URL` env var; system/service register verifies Bearer matches request `name` | **Blocker** (Production) | No |
| 34 | G23 | Token variants — `USAGE_LIMITED_TOKEN` (counter-based) and `BASE64_SELF_CONTAINED` (HMAC-signed); JWT variants remain 501 | Medium (Prototyping) | No |
| 35 | G35 | Device QoS Evaluator — new `core/cmd/deviceqoseval` binary; TCP RTT probe; measurement store; management query endpoint | Medium (Prototyping) | No |
| 36 | G40 | QoS filtering — `qualityRequirements[]` in `OrchestrationRequest`; DynamicOrch calls QoS Evaluator and filters candidates | Medium (Prototyping) | No |
| 37 | G36 | Translation Manager — new `core/cmd/translationmgr` binary; JSON field-remapping bridge; minimal management endpoints | Low (Research) | No |
| 38 | G34 | MQTT profiles — MQTT listener alongside HTTP when `MQTT_BROKER_URL` set; register MQTT interfaces in SR | Low (Research) | Yes |
| 39 | — | Phase 3 documentation update — CONFORMANCE.md, CONFORMANCE_UPDATE_PLAN.md, GAP_ANALYSIS.md, SPEC.md, EXAMPLES.md, README.md | — | — |

See `CONFORMANCE_UPDATE_PLAN.md` for the detailed TDD execution plan (Steps 33–39).

---

## Extensions beyond AH5 (not conformance gaps)

| Feature | Gap | Description |
|---------|-----|-------------|
| Certificate Authority (`/ca`) | G9 | Issues and revokes X.509 ECDSA certs; no AH5 counterpart |
| FlexibleStore Orchestration | G1 | Priority-weighted, metadata-filtered orchestration; AH5 spec page "Coming soon" |
| DynamicOrch identity check (`ENABLE_IDENTITY_CHECK`) | D8 | Optional token-gated pull; explicit extension beyond spec |

---

*Last updated: 2026-05-31 (Phase 3 complete — Steps 33–39 implemented; G10, G23, G34, G35, G36, G40 resolved)*
