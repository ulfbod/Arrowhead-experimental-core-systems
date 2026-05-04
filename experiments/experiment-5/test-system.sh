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

green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }

pass() { green "  PASS  $1"; PASS=$((PASS+1)); }

fail() {
  red "  FAIL  $1"
  echo "         expected: $2"
  echo "         actual:   $3"
  FAIL=$((FAIL+1))
}

check_eq() {
  local desc="$1" expected="$2" actual="$3"
  if [ "$actual" = "$expected" ]; then pass "$desc"; else fail "$desc" "$expected" "$actual"; fi
}

http_code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
http_body() { curl -s "$@"; }

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

if echo "$status" | grep -q '"synced":true'; then
  pass "synced=true"
else
  fail "synced=true" '"synced":true' "$status"
fi

if echo "$status" | grep -qE '"version":[1-9]'; then
  pass "version ≥ 1"
else
  fail "version ≥ 1" '"version":N≥1' "$status"
fi

# BUG FIX: policy-sync /status now includes lastSyncedAt and grants fields.
if echo "$status" | grep -q '"lastSyncedAt":"20'; then
  pass "lastSyncedAt field present  [fix: added to policy-sync /status]"
else
  fail "lastSyncedAt field present" '"lastSyncedAt":"2026-..."' "$status"
fi

if echo "$status" | grep -qE '"grants":[1-9]'; then
  pass "grants field ≥ 1  [fix: added to policy-sync /status]"
else
  fail "grants field ≥ 1" '"grants":N≥1' "$status"
fi

# ── Section 3: kafka-authz authorization ───────────────────────────────────────
echo
echo "=== 3. kafka-authz /auth/check (localhost:9091) ==="

body=$(http_body -X POST http://localhost:9091/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"demo-consumer-1","service":"telemetry"}')
echo "  demo-consumer-1:    $body"
if echo "$body" | grep -q '"decision":"Permit"'; then
  pass "demo-consumer-1 → Permit"
else
  fail "demo-consumer-1 → Permit" '"decision":"Permit"' "$body"
fi

body=$(http_body -X POST http://localhost:9091/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"analytics-consumer","service":"telemetry"}')
echo "  analytics-consumer: $body"
if echo "$body" | grep -q '"decision":"Permit"'; then
  pass "analytics-consumer → Permit"
else
  fail "analytics-consumer → Permit" '"decision":"Permit"' "$body"
fi

body2=$(http_body -X POST http://localhost:9091/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unknown-consumer","service":"telemetry"}')
echo "  unknown-consumer:   $body2"
if echo "$body2" | grep -q '"decision":"Deny"'; then
  pass "unknown-consumer → Deny"
else
  fail "unknown-consumer → Deny" '"decision":"Deny"' "$body2"
fi

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

if echo "$sse" | grep -q 'not authorized'; then
  fail "SSE test-probe: not denied (403)" "no 'not authorized'" "$preview"
else
  pass "SSE test-probe: not denied (403)"
fi

if echo "$sse" | grep -q '^data:'; then
  pass "SSE test-probe: data lines received from Kafka"
else
  fail "SSE test-probe: data lines received from Kafka" "data: {...}" "$preview"
fi

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

if [ "${count:-0}" -gt 0 ]; then
  pass "msgCount > 0 (got $count)"
else
  fail "msgCount > 0" ">0" "${count:-0}"
fi

if echo "$stats" | grep -q '"lastDeniedAt":""'; then
  pass "lastDeniedAt is empty (never denied)"
else
  fail "lastDeniedAt is empty" '""' "$stats"
fi

# ── Section 6: kafka-authz /status activeStreams is a number ──────────────────
echo
echo "=== 6. kafka-authz /status (activeStreams type) ==="
# BUG FIX: activeStreams was returned as map[string]int64 (renders as
# [object Object] in the dashboard); now returned as a plain integer.

kstatus=$(http_body http://localhost:3005/api/kafka-authz/status)
echo "  raw: $kstatus"
if echo "$kstatus" | grep -qE '"activeStreams":[0-9]'; then
  pass "activeStreams is a number  [fix: changed map→int in handleStatus]"
else
  fail "activeStreams is a number" '"activeStreams":N' "$kstatus"
fi

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
  if echo "$deny_body" | grep -q '"decision":"Deny"'; then
    pass "consumer-1 → Deny in AuthzForce after revocation"
  else
    fail "consumer-1 → Deny after revocation" '"decision":"Deny"' "$deny_body"
  fi

  regrant=$(http_body -X POST http://localhost:8082/authorization/grant \
    -H 'Content-Type: application/json' \
    -d '{"consumerSystemName":"demo-consumer-1","providerSystemName":"robot-fleet","serviceDefinition":"telemetry"}')
  if echo "$regrant" | grep -qE '"id":|already exists'; then
    pass "consumer-1 grant restored"
  else
    fail "consumer-1 grant restored" '"id":N' "$regrant"
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
