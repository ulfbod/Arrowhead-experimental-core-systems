#!/usr/bin/env bash
# test-system.sh — system test for experiment-5 running under Docker.
#
# Run from experiments/experiment-5/ with the stack already up:
#
#   cd experiments/experiment-5
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

# ── Pre-flight: smoke-check ───────────────────────────────────────────────────
# Verify fundamental preconditions before running application-level tests.
# Any failure here exits immediately so cascade failures do not obscure the root cause.
echo
echo "=== Pre-flight: smoke-check ==="

smoke_http "ServiceRegistry /health"  http://localhost:8080/health
smoke_http "ConsumerAuth /health"     http://localhost:8082/health
smoke_http "AuthzForce /health"       http://localhost:8180/health
smoke_http "kafka-authz /health"      http://localhost:9091/health

# Dashboard nginx — required for /api/* proxy paths used throughout this test.
echo "  Waiting for dashboard nginx (up to 15s)..."
dash_up=false
for i in $(seq 1 15); do
  code=$(http_code http://localhost:3005 2>/dev/null || echo "000")
  if [ "$code" = "200" ]; then dash_up=true; break; fi
  echo "  ... attempt $i/15, HTTP $code"
  sleep 1
done
if $dash_up; then
  pass "Dashboard nginx (localhost:3005) → 200"
else
  smoke_fail "Dashboard nginx (localhost:3005) → 200" "nginx not responding after 15s"
fi

# policy-sync must be synced and using the correct AuthzForce domain.
# A domain mismatch causes every PEP decision to return Deny silently (see EXP-001).
echo "  Waiting for policy-sync first sync (up to 30s)..."
ps_status=""
for i in $(seq 1 6); do
  ps_status=$(http_body http://localhost:3005/api/policy-sync/status 2>/dev/null || echo "{}")
  if echo "$ps_status" | grep -q '"synced":true'; then break; fi
  echo "  ... attempt $i/6, sleeping 5s"
  sleep 5
done
if echo "$ps_status" | grep -q '"synced":true'; then
  pass "policy-sync synced=true"
else
  smoke_fail "policy-sync synced=true" "not synced after 30s — check policy-sync container logs"
fi
if echo "$ps_status" | grep -q '"domainExternalId":"arrowhead-exp5"'; then
  pass "policy-sync domainExternalId=arrowhead-exp5"
else
  smoke_fail "policy-sync domainExternalId=arrowhead-exp5" \
    "expected arrowhead-exp5 — AUTHZFORCE_DOMAIN mismatch causes all auth checks to return Deny (see EXP-001)"
fi

# ── Section 1: AuthzForce server endpoints ─────────────────────────────────────
echo
echo "=== 1. AuthzForce server (localhost:8180) ==="

check_eq "GET /health → 200" "200" \
  "$(http_code http://localhost:8180/health)"

# /authzforce-ce/health is the path nginx rewrites /api/authzforce/health to.
# BUG FIX: authzforce-server now registers this path explicitly.
check_eq "GET /authzforce-ce/health → 200  [fix: added handler for nginx-rewritten path]" \
  "200" "$(http_code http://localhost:8180/authzforce-ce/health)"

check_eq "GET /authzforce-ce/domains → 200" "200" \
  "$(http_code http://localhost:8180/authzforce-ce/domains)"

# Exact URL the dashboard Health tab probes for the AuthzForce card.
check_eq "Dashboard nginx GET /api/authzforce/health → 200  [was showing 'down' badge]" \
  "200" "$(http_code http://localhost:3005/api/authzforce/health)"

# ── Section 2: policy-sync /status ─────────────────────────────────────────────
echo
echo "=== 2. policy-sync /status (via nginx localhost:3005) ==="

status=$(http_body http://localhost:3005/api/policy-sync/status)
echo "  raw: $status"

assert_json_value "synced=true" "synced" "true" "$status"
assert_json_gt    "version ≥ 1" "version" 0 "$status"
# BUG FIX: policy-sync /status now includes lastSyncedAt and grants fields.
assert_contains   "lastSyncedAt field present  [fix: added to policy-sync /status]" '"lastSyncedAt":"20' "$status"
assert_json_gt    "grants field ≥ 1  [fix: added to policy-sync /status]" "grants" 0 "$status"

# ── Section 3: kafka-authz authorization ───────────────────────────────────────
echo
echo "=== 3. kafka-authz /auth/check (localhost:9091) ==="

body=$(http_body -X POST http://localhost:9091/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"demo-consumer-1","service":"telemetry"}')
echo "  demo-consumer-1:    $body"
assert_json_value "demo-consumer-1 → Permit" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9091/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"analytics-consumer","service":"telemetry"}')
echo "  analytics-consumer: $body"
assert_json_value "analytics-consumer → Permit" "decision" "Permit" "$body"

body2=$(http_body -X POST http://localhost:9091/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unknown-consumer","service":"telemetry"}')
echo "  unknown-consumer:   $body2"
assert_json_value "unknown-consumer → Deny" "decision" "Deny" "$body2"

# ── Section 4: kafka-authz SSE stream (test-probe consumer) ───────────────────
echo
echo "=== 4. kafka-authz SSE stream (test-probe consumer — isolated group) ==="
# BUG FIX: use "test-probe" (seeded in setup) so the test gets its own
# Kafka consumer group and does NOT compete with analytics-consumer for
# the single arrowhead.telemetry partition.

sse=$(timeout 4 curl -sN \
  "http://localhost:9091/stream/test-probe?service=telemetry" 2>/dev/null || true)
preview="${sse:0:300}"
echo "  first 300 chars: $preview"

assert_not_contains "SSE test-probe: not denied (403)" "not authorized" "$sse"
assert_contains     "SSE test-probe: data lines received from Kafka" "data: {" "$sse"

# ── Section 5: analytics-consumer message count ────────────────────────────────
echo
echo "=== 5. analytics-consumer /stats (via nginx localhost:3005) ==="
# Wait up to 60 s for analytics-consumer to accumulate at least one message.
# This handles the case where analytics-consumer is still backing off from an
# initial denial (readBody bug, now fixed) or waiting for robot-fleet to start
# publishing to Kafka.

stats=""
count=0
for i in $(seq 1 12); do
  stats=$(http_body http://localhost:3005/api/analytics-consumer/stats)
  count=$(echo "$stats" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  if [ "${count:-0}" -gt 0 ]; then
    break
  fi
  echo "  waiting for messages... (attempt $i/12, sleeping 5s)"
  sleep 5
done
echo "  raw: $stats"

assert_json_gt    "msgCount > 0" "msgCount" 0 "$stats"
assert_json_value "lastDeniedAt is empty (never denied)" "lastDeniedAt" "" "$stats"

# ── Section 6: kafka-authz /status activeStreams is a number ──────────────────
echo
echo "=== 6. kafka-authz /status (activeStreams type) ==="
# BUG FIX: activeStreams was returned as map[string]int64 (renders as
# [object Object] in the dashboard); now returned as a plain integer.

kstatus=$(http_body http://localhost:3005/api/kafka-authz/status)
echo "  raw: $kstatus"
assert_json_field "activeStreams is a number  [fix: changed map→int in handleStatus]" "activeStreams" "$kstatus"

# ── Section 7: Revocation — AMQP consumer (consumer-1) ────────────────────────
echo
echo "=== 7. Revocation: consumer-1 AMQP connection closed after grant revoked ==="
# BUG FIX: topic-auth-xacml now runs a revocation loop every 15 s that
# closes AMQP connections for consumers whose grants were removed.

lookup=$(http_body http://localhost:8082/authorization/lookup)
grant_id=$(echo "$lookup" \
  | grep -oE '"id":[0-9]+[^}]*"consumerSystemName":"demo-consumer-1"' \
  | grep -oE '"id":[0-9]+' | grep -oE '[0-9]+' | head -1)

if [ -z "$grant_id" ]; then
  fail "consumer-1 grant exists in CA" "grant with id" "not found"
  echo "  Skipping revocation test."
else
  pass "consumer-1 grant exists in CA (id=$grant_id)"

  conn_before=$(http_body -u admin:admin http://localhost:15675/api/connections)
  c1_before=$(echo "$conn_before" | grep -o '"demo-consumer-1"' | wc -l | tr -d ' ')
  if [ "${c1_before:-0}" -gt 0 ]; then
    pass "consumer-1 has active AMQP connection before revocation"
  else
    fail "consumer-1 has active AMQP connection before revocation" ">0" "0"
  fi

  revoke_code=$(http_code -X DELETE "http://localhost:8082/authorization/revoke/$grant_id")
  check_eq "revoke consumer-1 grant → 200" "200" "$revoke_code"

  echo "  waiting 30 s for policy-sync + revocation loop propagation..."
  sleep 30

  conn_after=$(http_body -u admin:admin http://localhost:15675/api/connections)
  c1_after=$(echo "$conn_after" | grep -o '"demo-consumer-1"' | wc -l | tr -d ' ')
  check_eq "consumer-1 AMQP connection closed after revocation" "0" "${c1_after:-0}"

  deny_body=$(http_body -X POST http://localhost:9091/auth/check \
    -H 'Content-Type: application/json' \
    -d '{"consumer":"demo-consumer-1","service":"telemetry"}')
  echo "  consumer-1 AuthzForce after revoke: $deny_body"
  assert_json_value "consumer-1 → Deny in AuthzForce after revocation" "decision" "Deny" "$deny_body"

  regrant=$(http_body -X POST http://localhost:8082/authorization/grant \
    -H 'Content-Type: application/json' \
    -d '{"consumerSystemName":"demo-consumer-1","providerSystemName":"robot-fleet","serviceDefinition":"telemetry"}')
  if [[ "$regrant" == *'"id":'* ]] || [[ "$regrant" == *"already exists"* ]]; then
    pass "consumer-1 grant restored"
  else
    fail "consumer-1 grant restored" '"id":N or already exists' "$regrant"
  fi
fi

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
