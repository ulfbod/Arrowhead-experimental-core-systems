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

Ratings reflect the implemented state after Phase 3 (Steps 1–39, E1–E5) and account for all
open gaps identified through the Phase 4/5 audit. Ratings will improve as Phases 4 and 5 are completed.

| System | Endpoint% | Model% | Behavior% | Overall | Key open gaps |
|--------|-----------|--------|-----------|---------|---------------|
| ServiceRegistry | 82 | 75 | 82 | **~80%** | G44, G45 |
| Authentication | 85 | 80 | 76 | **~81%** | G52 |
| ConsumerAuthorization | 78 | 70 | 68 | **~72%** | G46, G47, G6 (partial) |
| DynamicOrchestration | 88 | 82 | 84 | **~85%** | G48, G49, G25 (ONLY_EXCLUSIVE) |
| SimpleStoreOrchestration | 77 | 75 | 82 | **~78%** | G51 |
| Blacklist | 100 | 85 | 85 | **~91%** | G50 |
| GeneralManagement (cross-cutting) | 100 | 85 | 87 | **~91%** | — |
| FlexibleStoreOrchestration | N/A | N/A | N/A | Extension | No spec (G1) |
| CertificateAuthority | N/A | N/A | N/A | Extension | Not in spec (G9) |
| DeviceQoSEvaluator | 85 | 80 | 80 | **~82%** | G53 |
| TranslationManager | 85 | 85 | 80 | **~84%** | Phase 5 roadmap |

**Notes:**
- G11, G25 (intercloud), G40 (result fields), G41, G43 resolved in Phase 1 (Steps E1–E5).
- G37, G42, G20, G38, G39, G26 resolved in Phase 2 (Steps 27–31).
- G10, G23 (variants), G34 (MQTT adapter), G35, G36, G40 (QoS filtering) resolved in Phase 3 (Steps 33–38).
- Ratings revised downward after Phase 4/5 audit revealed G44–G53 and residual partial gaps.
- GeneralManagement is a cross-cutting capability, not an independent system.
- FlexibleStoreOrchestration and CertificateAuthority are extensions with no AH5 spec
  counterpart; conformance ratings are not applicable.

**Projected ratings after Phase 4 (Steps 40–49):**

| System | Endpoint% | Model% | Behavior% | Overall |
|--------|-----------|--------|-----------|---------|
| ServiceRegistry | 92 | 85 | 90 | **~89%** |
| Authentication | 90 | 83 | 85 | **~86%** |
| ConsumerAuthorization | 83 | 78 | 80 | **~81%** |
| DynamicOrchestration | 92 | 85 | 90 | **~89%** |
| SimpleStoreOrchestration | 85 | 80 | 87 | **~84%** |
| Blacklist | 100 | 88 | 92 | **~94%** |
| GeneralManagement | 100 | 88 | 92 | **~94%** |
| DeviceQoSEvaluator | 90 | 85 | 87 | **~87%** |
| TranslationManager | 90 | 88 | 85 | **~88%** |

**Projected ratings after Phase 5 (Steps 50–56):**

| System | Endpoint% | Model% | Behavior% | Overall |
|--------|-----------|--------|-----------|---------|
| ServiceRegistry | 95 | 92 | 95 | **~94%** |
| Authentication | 95 | 90 | 92 | **~93%** |
| ConsumerAuthorization | 95 | 92 | 92 | **~93%** |
| DynamicOrchestration | 97 | 90 | 95 | **~94%** |
| SimpleStoreOrchestration | 92 | 85 | 92 | **~90%** |
| Blacklist | 100 | 92 | 95 | **~96%** |
| GeneralManagement | 100 | 92 | 95 | **~96%** |
| DeviceQoSEvaluator | 95 | 92 | 92 | **~93%** |
| TranslationManager | 92 | 90 | 88 | **~90%** |

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

Phases 1–3 are complete. The gaps below were identified during the Phase 4/5 audit and represent
the remaining work to reach full AH5 conformance.

### Partial gaps (implementation exists but incomplete)

| Gap | Description | PoC | Teaching | Prototyping | Production | Phase |
|-----|-------------|-----|----------|-------------|------------|-------|
| **G4** (completion) | mTLS not enforced by default; plain HTTP is the default transport | None | Low | High | **Blocker** | **5** |
| **G6** (completion) | ConsumerAuth does not require a prior Authentication token for authz token generation | None | Low | Medium | High | **5** |
| **G23** (JWT) | JWT token variants (RSA_SHA256, RSA_SHA512, TRANSLATION_BRIDGE) return 501 | None | Low | Medium | High | **5** |
| **G25** (ONLY_EXCLUSIVE) | `ONLY_EXCLUSIVE` flag accepted but not enforced against the lock store | None | Low | Medium | Medium | **4** |
| **G26** (auto-push) | Push delivery is manual-trigger only; no provider-change auto-polling | None | Medium | Medium | High | **5** |
| **G34** (MQTTS) | MQTT over TLS not implemented; plain MQTT adapter is in place | Low | Low | Medium | High | **5** |

### New gaps (Phase 4/5 audit)

| Gap | Description | PoC | Teaching | Prototyping | Production | Phase |
|-----|-------------|-----|----------|-------------|------------|-------|
| **G44** | ServiceRegistry missing PUT update operations for systems, devices, interface templates, service definitions | Low | Low | Medium | High | **4** |
| **G45** | `securityPolicy` accepted but not validated on service registration; `authenticationInfo` stored but not enforced | None | Low | Medium | High | **4** |
| **G46** | `scopedPolicies` stored but not evaluated in ConsumerAuthorization verify; only `defaultPolicy` is checked | None | Medium | High | **Blocker** | **4** |
| **G47** | JWT token signing infrastructure absent; RSA key pair not managed; `/authorization-token/public-key` returns 404 | None | Low | High | **Blocker** | **5** |
| **G48** | `ONLY_EXCLUSIVE` orchestration flag does not consult lock store to exclude locked providers | None | Low | Medium | Medium | **4** |
| **G49** | Orchestration history query has no filtering by consumer, service, status, or date range | None | Low | Medium | High | **4** |
| **G50** | Blacklist expired entries never auto-purged; accumulate unboundedly in store | None | None | Low | Medium | **4** |
| **G51** | SimpleStore full rule update endpoint missing; only priority reordering is available | None | Low | Medium | Medium | **4** |
| **G52** | Authentication identity creation does not enforce PascalCase naming convention (G19 applies to SR only) | None | Low | Low | Medium | **4** |
| **G53** | QoS measurements limited to TCP RTT; bandwidth, jitter, packet-loss not modelled or measured | None | Low | High | High | **5** |

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
| **Phase 4** | 40–49 | Behavioral completeness: model correctness gaps, missing CRUD operations, scoped policy evaluation | Planned |
| **Phase 5** | 50–56 | Full protocol compliance: JWT token signing, mTLS by default, auth coupling, MQTTS, QoS dimensions | Planned |

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

### Phase 4 — Behavioral completeness (Steps 40–49)

**Goal:** Close model-correctness and missing-CRUD gaps identified in the Phase 4/5 audit.
No new external dependencies. All steps are independent of each other except where noted.

**Order:** Step 40 (G44 — SR PUT operations) first because it unblocks SR management tooling. Steps 41–47 are independent. Step 48 (G46 — scoped policies) should come before Step 50 (G6 completion) in Phase 5 since they share the ConsumerAuth verify path. Step 49 is the documentation sweep.

| Step | Gap(s) | Focus | Priority | Systems affected |
|------|--------|-------|----------|-----------------|
| 40 | G44 | ServiceRegistry PUT update endpoints for systems, devices, interface templates, service definitions | High | SR |
| 41 | G45 | `securityPolicy` enum validation on service registration; `authenticationInfo` field wired into verification | Medium | SR |
| 42 | G46 | Scoped policy evaluation in ConsumerAuth verify — consult `ScopedPolicies[scope]` before falling back to `DefaultPolicy` | **Blocker** (Production) | ConsumerAuth |
| 43 | G48 | `ONLY_EXCLUSIVE` flag — exclude candidates whose `exclusiveUntil` is in the future from orchestration results | Medium | DynamicOrch |
| 44 | G49 | Orchestration history query filtering — consumer, service definition, status, date range | Medium | DynamicOrch |
| 45 | G50 | Blacklist expired-entry auto-purge — background goroutine; configurable `BLACKLIST_PURGE_INTERVAL_SECONDS` | Low | Blacklist |
| 46 | G51 | SimpleStore full rule update endpoint — `PUT /mgmt/simple-store/update/{id}` replacing all rule fields | Medium | SimpleStoreOrch |
| 47 | G52 | Authentication identity creation naming convention — enforce PascalCase on `POST /mgmt/identities` | Low | Authentication |
| 48 | G25 | `ONLY_EXCLUSIVE` flag returns 501 if used without lock support; update stub behaviour | Low | DynamicOrch, SimpleStoreOrch |
| 49 | — | Phase 4 documentation update — CONFORMANCE.md, GAP_ANALYSIS.md, SPEC.md, EXAMPLES.md, README.md | — | — |

### Phase 5 — Full protocol compliance (Steps 50–56)

**Goal:** Reach ≥90% across all dimensions for every spec-defined system. Covers high-effort crypto, transport, and protocol gaps. Each step has external dependencies or significant design risk; careful prerequisite management is required.

**Order:** Step 50 (G46 scoped-policy prerequisite already complete from Phase 4; Step 50 builds on it for G6). Step 51 (G47 JWT) is independent but needs an RSA key-pair bootstrap mechanism. Step 52 (G4 mTLS) builds on the existing TLS infrastructure from experiment-7. Step 53 (G53 QoS dimensions) extends the Device QoS Evaluator. Step 54 (G26 auto-push) extends DynamicOrchestration push. Step 55 (G34 MQTTS) extends the MQTT adapter. Step 56 is the documentation sweep.

| Step | Gap(s) | Focus | Priority | Notes |
|------|--------|-------|----------|-------|
| 50 | G6 | ConsumerAuth token relay — `POST /authorization-token/generate` requires a valid Bearer token from Authentication; verified `systemName` must match request `consumer` | High | Builds on G46 (Phase 4) and REGISTER_AUTH_URL pattern (G10) |
| 51 | G47 | JWT token variants — RSA key-pair managed at startup (`JWT_PRIVATE_KEY_FILE` env var); `RSA_SHA256_JSON_WEB_TOKEN` and `RSA_SHA512_JSON_WEB_TOKEN` generation and verify; `/authorization-token/public-key` endpoint returns PEM | Medium | Needs `crypto/rsa` or `github.com/golang-jwt/jwt/v5` |
| 52 | G4 | mTLS default enforcement — `HTTPS_ONLY` env var makes TLS_PORT the primary listener; plain HTTP only for health endpoints and Docker bootstrap | High | Builds on experiment-7 TLS infrastructure |
| 53 | G53 | QoS full model — `bandwidthBps`, `jitterMs`, `packetLoss` fields on `QoSRecord`; active bandwidth probe; `QOS_PROBE_TIMEOUT_SECONDS` env var; `maxBandwidthBps`, `maxJitterMs` in `QoSRequirement` | Low | Requires iPerf or ICMP probe logic |
| 54 | G26 | Provider-change auto push — `PUSH_POLL_INTERVAL_SECONDS` env var; background goroutine monitors SR for provider changes and fires triggers automatically | Medium | Requires SR event feed or polling loop |
| 55 | G34 | MQTTS — MQTT over TLS; `MQTT_BROKER_TLS_CERT_FILE`, `MQTT_BROKER_TLS_KEY_FILE` env vars; register `MQTT-SECURE-JSON` interface alongside `MQTT-INSECURE-JSON` | Low | Extends mqttutil adapter |
| 56 | — | Phase 5 documentation update — CONFORMANCE.md, GAP_ANALYSIS.md, SPEC.md, EXAMPLES.md, README.md | — | — |

---

## Extensions beyond AH5 (not conformance gaps)

| Feature | Gap | Description |
|---------|-----|-------------|
| Certificate Authority (`/ca`) | G9 | Issues and revokes X.509 ECDSA certs; no AH5 counterpart |
| FlexibleStore Orchestration | G1 | Priority-weighted, metadata-filtered orchestration; AH5 spec page "Coming soon" |
| DynamicOrch identity check (`ENABLE_IDENTITY_CHECK`) | D8 | Optional token-gated pull; explicit extension beyond spec |

---

*Last updated: 2026-05-31 (Phase 4/5 audit — G44–G53 added; per-system ratings revised; Phase 4 and Phase 5 plans added)*
