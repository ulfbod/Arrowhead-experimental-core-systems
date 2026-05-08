#!/usr/bin/env bash
# test-system.sh — system test for experiment-6 running under Docker.
#
# Run from experiments/experiment-6/ with the stack already up:
#
#   cd experiments/experiment-6
#   docker compose up -d --build
#   bash test-system.sh
#
# Each test prints PASS or FAIL. The script exits 1 if any test fails.

set -euo pipefail

PASS=0
FAIL=0

source "$(dirname "$0")/../test-lib.sh"

# ── TypeScript type-check (no Docker required) ────────────────────────────────
# Run tsc --noEmit before any Docker checks so type errors are caught before a
# slow rebuild.  Skipped gracefully when node_modules has not been installed.
echo
echo "=== TypeScript type-check ==="
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if command -v npm &>/dev/null && [ -d "$SCRIPT_DIR/dashboard/node_modules" ]; then
  if npm --prefix "$SCRIPT_DIR/dashboard" run typecheck 2>&1; then
    pass "dashboard tsc --noEmit"
  else
    red "  FAIL  dashboard tsc --noEmit"
    red "  TypeScript type errors found — fix before running docker compose up --build"
    exit 2
  fi
else
  printf '\033[33m  SKIP  dashboard type-check (cd dashboard && npm install, then re-run)\033[0m\n'
fi

# ── Pre-flight: dashboard reachable ───────────────────────────────────────────
# Fail fast if nginx is not up — every /api/* test would also fail, making the
# output misleading.  Wait up to 15 s for the container to become ready.
echo
echo "=== Pre-flight: dashboard nginx (localhost:3006) ==="

dashboard_up=false
for i in $(seq 1 15); do
  code=$(http_code http://localhost:3006 2>/dev/null || echo "000")
  if [ "$code" = "200" ]; then
    dashboard_up=true
    break
  fi
  echo "  waiting for dashboard... (attempt $i/15, HTTP $code)"
  sleep 1
done

if $dashboard_up; then
  pass "Dashboard nginx → 200"
else
  red "  FAIL  Dashboard nginx → 200 (got $code after 15s)"
  red "  Cannot continue — nginx is not serving.  Is the stack up?"
  red "  Run: docker compose up -d --build"
  exit 1
fi

# Verify the dashboard HTML is the React SPA (not a build error page).
html=$(http_body http://localhost:3006 2>/dev/null || echo "")
if echo "$html" | grep -q '<div id="root">'; then
  pass "Dashboard HTML contains React root element"
else
  fail "Dashboard HTML contains React root element" '<div id="root">' "${html:0:120}"
fi

# Core and PEP service health — exit immediately on any failure so downstream
# tests do not produce misleading cascades.
smoke_http "ConsumerAuth /health"   http://localhost:8082/health
smoke_http "AuthzForce /health"     http://localhost:8186/health
smoke_http "kafka-authz /health"    http://localhost:9091/health
smoke_http "rest-authz /health"     http://localhost:9093/health

# policy-sync must be synced and using the correct AuthzForce domain.
# A domain mismatch causes every PEP decision to return Deny silently (see EXP-001).
echo "  Waiting for policy-sync first sync (up to 30s)..."
ps_status=""
for i in $(seq 1 6); do
  ps_status=$(http_body http://localhost:3006/api/policy-sync/status 2>/dev/null || echo "{}")
  if echo "$ps_status" | grep -q '"synced":true'; then break; fi
  echo "  ... attempt $i/6, sleeping 5s"
  sleep 5
done
if echo "$ps_status" | grep -q '"synced":true'; then
  pass "policy-sync synced=true"
else
  smoke_fail "policy-sync synced=true" "not synced after 30s — check policy-sync container logs"
fi
if echo "$ps_status" | grep -q '"domainExternalId":"arrowhead-exp6"'; then
  pass "policy-sync domainExternalId=arrowhead-exp6"
else
  smoke_fail "policy-sync domainExternalId=arrowhead-exp6" \
    "expected arrowhead-exp6 — AUTHZFORCE_DOMAIN mismatch causes all auth checks to return Deny (see EXP-001)"
fi

# ── Section 1: AuthzForce server endpoints ─────────────────────────────────────
echo
echo "=== 1. AuthzForce server (localhost:8186) ==="

check_eq "GET /health → 200" "200" \
  "$(http_code http://localhost:8186/health)"

check_eq "GET /authzforce-ce/health → 200" \
  "200" "$(http_code http://localhost:8186/authzforce-ce/health)"

check_eq "GET /authzforce-ce/domains → 200" "200" \
  "$(http_code http://localhost:8186/authzforce-ce/domains)"

check_eq "Dashboard nginx GET /api/authzforce/health → 200" \
  "200" "$(http_code http://localhost:3006/api/authzforce/health)"

# ── Section 2: policy-sync /status ─────────────────────────────────────────────
echo
echo "=== 2. policy-sync /status (via nginx localhost:3006) ==="

status=$(http_body http://localhost:3006/api/policy-sync/status)
echo "  raw: $status"

assert_json_value "synced=true"                        "synced"           "true"           "$status"
assert_json_gt    "version ≥ 1"                        "version"          0                "$status"
assert_contains   "lastSyncedAt field present"         '"lastSyncedAt":"20' "$status"
assert_json_gt    "grants field ≥ 1"                   "grants"           0                "$status"
assert_json_field "syncInterval field present"         "syncInterval"                      "$status"
assert_json_value "policy-sync using domain arrowhead-exp6" "domainExternalId" "arrowhead-exp6" "$status"

# ── Section 3: policy-sync /config (runtime SYNC_INTERVAL) ────────────────────
echo
echo "=== 3. policy-sync /config (runtime SYNC_INTERVAL update) ==="

cfg=$(http_body -X POST http://localhost:3006/api/policy-sync/config \
  -H 'Content-Type: application/json' \
  -d '{"syncInterval":"10s"}')
echo "  POST /config {syncInterval:10s}: $cfg"
if echo "$cfg" | grep -q '"syncInterval":"10s"'; then
  pass "SYNC_INTERVAL updated to 10s"
else
  fail "SYNC_INTERVAL updated to 10s" '"syncInterval":"10s"' "$cfg"
fi

bad_cfg=$(http_code -X POST http://localhost:3006/api/policy-sync/config \
  -H 'Content-Type: application/json' \
  -d '{"syncInterval":"0.5s"}')
check_eq "invalid syncInterval → 400" "400" "$bad_cfg"

# ── Section 4: kafka-authz authorization ───────────────────────────────────────
echo
echo "=== 4. kafka-authz /auth/check (localhost:9091) ==="

body=$(http_body -X POST http://localhost:9091/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"demo-consumer-1","service":"telemetry"}')
echo "  demo-consumer-1:    $body"
assert_json_value "demo-consumer-1 → Permit (Kafka)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9091/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unknown-consumer","service":"telemetry"}')
echo "  unknown-consumer:   $body"
assert_json_value "unknown-consumer → Deny (Kafka)" "decision" "Deny" "$body"

# ── Section 5: rest-authz /auth/check ─────────────────────────────────────────
echo
echo "=== 5. rest-authz /auth/check (localhost:9093) ==="

body=$(http_body -X POST http://localhost:9093/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"rest-consumer","service":"telemetry-rest"}')
echo "  rest-consumer:      $body"
assert_json_value "rest-consumer → Permit (REST)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9093/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"test-probe","service":"telemetry-rest"}')
echo "  test-probe:         $body"
assert_json_value "test-probe → Permit (REST)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9093/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unauthorized","service":"telemetry-rest"}')
echo "  unauthorized:       $body"
assert_json_value "unauthorized → Deny (REST)" "decision" "Deny" "$body"

# ── Section 6: REST data access via rest-authz ─────────────────────────────────
echo
echo "=== 6. REST data access via rest-authz (localhost:9093) ==="

# Wait up to 60 s for data-provider to receive data from robot-fleet via Kafka.
telemetry=""
for i in $(seq 1 12); do
  telemetry=$(http_body -H 'X-Consumer-Name: test-probe' \
    http://localhost:9093/telemetry/latest 2>/dev/null || true)
  if [ "$telemetry" != "null" ] && [ -n "$telemetry" ]; then
    break
  fi
  echo "  waiting for data-provider to receive Kafka data... (attempt $i/12, sleeping 5s)"
  sleep 5
done
echo "  telemetry/latest (first 200 chars): ${telemetry:0:200}"

if [ "$telemetry" != "null" ] && [ -n "$telemetry" ] && ! echo "$telemetry" | grep -q '"error"'; then
  pass "GET /telemetry/latest via rest-authz → data received"
else
  fail "GET /telemetry/latest via rest-authz → data received" "non-null JSON without error" "$telemetry"
fi

deny_code=$(http_code -H 'X-Consumer-Name: unauthorized' \
  http://localhost:9093/telemetry/latest)
check_eq "unauthorized REST request → 403" "403" "$deny_code"

# ── Section 7: rest-consumer message count ─────────────────────────────────────
echo
echo "=== 7. rest-consumer /stats (via nginx localhost:3006) ==="

rcstats=""
count=0
for i in $(seq 1 12); do
  rcstats=$(http_body http://localhost:3006/api/rest-consumer/stats)
  count=$(echo "$rcstats" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  if [ "${count:-0}" -gt 0 ]; then
    break
  fi
  echo "  waiting for rest-consumer to accumulate messages... (attempt $i/12, sleeping 5s)"
  sleep 5
done
echo "  raw: $rcstats"

assert_json_gt    "rest-consumer msgCount > 0" "msgCount" 0 "$rcstats"
assert_json_value "rest-consumer lastDeniedAt is empty (never denied)" "lastDeniedAt" "" "$rcstats"

# ── Section 8: analytics-consumer (Kafka path) ─────────────────────────────────
echo
echo "=== 8. analytics-consumer /stats (via nginx localhost:3006) ==="

stats=""
ac_count=0
for i in $(seq 1 12); do
  stats=$(http_body http://localhost:3006/api/analytics-consumer/stats)
  ac_count=$(echo "$stats" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  if [ "${ac_count:-0}" -gt 0 ]; then
    break
  fi
  echo "  waiting for analytics-consumer... (attempt $i/12, sleeping 5s)"
  sleep 5
done
echo "  raw: $stats"

assert_json_gt "analytics-consumer msgCount > 0" "msgCount" 0 "$stats"

# ── Section 9: Sync-delay revocation test (REST path) ─────────────────────────
echo
echo "=== 9. Revocation sync-delay: rest-consumer denied after SYNC_INTERVAL ==="

lookup=$(http_body http://localhost:8082/authorization/lookup)
rc_grant_id=$(echo "$lookup" \
  | grep -oE '"id":[0-9]+[^}]*"consumerSystemName":"rest-consumer"' \
  | grep -oE '"id":[0-9]+' | grep -oE '[0-9]+' | head -1)

if [ -z "$rc_grant_id" ]; then
  fail "rest-consumer grant exists in CA" "grant with id" "not found"
  echo "  Skipping revocation test."
else
  pass "rest-consumer grant exists in CA (id=$rc_grant_id)"

  permit_before=$(http_body -X POST http://localhost:9093/auth/check \
    -H 'Content-Type: application/json' \
    -d '{"consumer":"rest-consumer","service":"telemetry-rest"}')
  assert_json_value "rest-consumer → Permit before revocation" "decision" "Permit" "$permit_before"

  revoke_code=$(http_code -X DELETE "http://localhost:8082/authorization/revoke/$rc_grant_id")
  check_eq "revoke rest-consumer grant → 200" "200" "$revoke_code"

  echo "  waiting 30 s for policy-sync cycle to propagate revocation..."
  sleep 30

  deny_body=$(http_body -X POST http://localhost:9093/auth/check \
    -H 'Content-Type: application/json' \
    -d '{"consumer":"rest-consumer","service":"telemetry-rest"}')
  echo "  rest-consumer AuthzForce after revoke: $deny_body"
  assert_json_value "rest-consumer → Deny in AuthzForce after revocation" "decision" "Deny" "$deny_body"

  http_code_after=$(http_code -H 'X-Consumer-Name: rest-consumer' \
    http://localhost:9093/telemetry/latest)
  check_eq "rest-consumer REST request → 403 after revocation" "403" "$http_code_after"

  regrant=$(http_body -X POST http://localhost:8082/authorization/grant \
    -H 'Content-Type: application/json' \
    -d '{"consumerSystemName":"rest-consumer","providerSystemName":"data-provider","serviceDefinition":"telemetry-rest"}')
  if [[ "$regrant" == *'"id":'* ]] || [[ "$regrant" == *"already exists"* ]]; then
    pass "rest-consumer grant restored"
  else
    fail "rest-consumer grant restored" '"id":N or already exists' "$regrant"
  fi

  echo "  waiting 15 s for grant restoration to propagate..."
  sleep 15

  permit_after=$(http_body -X POST http://localhost:9093/auth/check \
    -H 'Content-Type: application/json' \
    -d '{"consumer":"rest-consumer","service":"telemetry-rest"}')
  assert_json_value "rest-consumer → Permit again after grant restored" "decision" "Permit" "$permit_after"
fi

# ── Section 10: kafka-authz SSE stream (test-probe) ───────────────────────────
echo
echo "=== 10. kafka-authz SSE stream (test-probe consumer) ==="

sse=$(timeout 4 curl -sN \
  "http://localhost:9091/stream/test-probe?service=telemetry" 2>/dev/null || true)
preview="${sse:0:300}"
echo "  first 300 chars: $preview"

assert_not_contains "SSE test-probe: not denied (403)" "not authorized" "$sse"
assert_contains     "SSE test-probe: data lines received from Kafka" "data: {" "$sse"

# ── Section 11: Dashboard proxy — Kafka and REST tab endpoints ────────────────
echo
echo "=== 11. Dashboard proxy — Kafka and REST tab API endpoints (localhost:3006) ==="

# kafka-authz /status via nginx
kstatus=$(http_body http://localhost:3006/api/kafka-authz/status)
echo "  kafka-authz /status: $kstatus"
assert_json_field "GET /api/kafka-authz/status via nginx → activeStreams field" "activeStreams" "$kstatus"
assert_json_field "GET /api/kafka-authz/status via nginx → totalServed field"  "totalServed"   "$kstatus"

# kafka-authz /auth/check via nginx
kcheck=$(http_body -X POST http://localhost:3006/api/kafka-authz/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"analytics-consumer","service":"telemetry"}')
echo "  kafka-authz /auth/check (analytics-consumer): $kcheck"
assert_json_value "POST /api/kafka-authz/auth/check via nginx → analytics-consumer Permit" "decision" "Permit" "$kcheck"

kcheck_deny=$(http_body -X POST http://localhost:3006/api/kafka-authz/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unknown-consumer","service":"telemetry"}')
echo "  kafka-authz /auth/check (unknown-consumer): $kcheck_deny"
assert_json_value "POST /api/kafka-authz/auth/check via nginx → unknown-consumer Deny" "decision" "Deny" "$kcheck_deny"

# data-provider /stats via nginx
dpstats=$(http_body http://localhost:3006/api/data-provider/stats)
echo "  data-provider /stats: $dpstats"
assert_json_field "GET /api/data-provider/stats via nginx → msgCount field"   "msgCount"   "$dpstats"
assert_json_field "GET /api/data-provider/stats via nginx → robotCount field" "robotCount" "$dpstats"

# rest-authz /status via nginx
rastatus=$(http_body http://localhost:3006/api/rest-authz/status)
echo "  rest-authz /status: $rastatus"
assert_json_field "GET /api/rest-authz/status via nginx → requestsTotal field" "requestsTotal" "$rastatus"
assert_json_field "GET /api/rest-authz/status via nginx → permitted field"     "permitted"     "$rastatus"

# ServiceRegistry query for telemetry-rest via nginx.
# data-provider does not self-register in this experiment so serviceQueryData is
# expected to be empty; the test verifies the endpoint responds in the correct
# spec format (serviceQueryData + unfilteredHits), not that instances are present.
srquery=$(http_body -X POST http://localhost:3006/api/serviceregistry/serviceregistry/query \
  -H 'Content-Type: application/json' \
  -d '{"serviceDefinition":"telemetry-rest"}')
echo "  SR query for telemetry-rest (first 200 chars): ${srquery:0:200}"
assert_json_field "POST /api/serviceregistry/query via nginx → serviceQueryData field" "serviceQueryData" "$srquery"
assert_json_field "POST /api/serviceregistry/query via nginx → unfilteredHits field"  "unfilteredHits"  "$srquery"

# REST data access via nginx proxy (authorized / unauthorized)
assert_http "GET /api/rest-authz/telemetry/latest via nginx (test-probe) → 200" 200 \
  http://localhost:3006/api/rest-authz/telemetry/latest -H 'X-Consumer-Name: test-probe'
assert_http "GET /api/rest-authz/telemetry/latest via nginx (unauthorized) → 403" 403 \
  http://localhost:3006/api/rest-authz/telemetry/latest -H 'X-Consumer-Name: unauthorized'

# ── Summary ────────────────────────────────────────────────────────────────────
echo
echo "══════════════════════════════════════"
echo "  PASS: $PASS   FAIL: $FAIL"
echo "══════════════════════════════════════"

if [ "$FAIL" -gt 0 ]; then
  red "  $FAIL test(s) failed."
  exit 1
else
  green "  All $PASS tests passed."
fi
