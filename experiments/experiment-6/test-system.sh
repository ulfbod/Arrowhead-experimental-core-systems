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

if echo "$status" | grep -q '"lastSyncedAt":"20'; then
  pass "lastSyncedAt field present"
else
  fail "lastSyncedAt field present" '"lastSyncedAt":"2026-..."' "$status"
fi

if echo "$status" | grep -qE '"grants":[1-9]'; then
  pass "grants field ≥ 1"
else
  fail "grants field ≥ 1" '"grants":N≥1' "$status"
fi

if echo "$status" | grep -q '"syncInterval"'; then
  pass "syncInterval field present in /status"
else
  fail "syncInterval field present" '"syncInterval":"..."' "$status"
fi

if echo "$status" | grep -q '"domainExternalId":"arrowhead-exp6"'; then
  pass "policy-sync using domain arrowhead-exp6"
else
  fail "policy-sync using domain arrowhead-exp6" '"domainExternalId":"arrowhead-exp6"' "$status"
fi

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
if echo "$body" | grep -q '"decision":"Permit"'; then
  pass "demo-consumer-1 → Permit (Kafka)"
else
  fail "demo-consumer-1 → Permit (Kafka)" '"decision":"Permit"' "$body"
fi

body=$(http_body -X POST http://localhost:9091/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unknown-consumer","service":"telemetry"}')
echo "  unknown-consumer:   $body"
if echo "$body" | grep -q '"decision":"Deny"'; then
  pass "unknown-consumer → Deny (Kafka)"
else
  fail "unknown-consumer → Deny (Kafka)" '"decision":"Deny"' "$body"
fi

# ── Section 5: rest-authz /auth/check ─────────────────────────────────────────
echo
echo "=== 5. rest-authz /auth/check (localhost:9093) ==="

body=$(http_body -X POST http://localhost:9093/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"rest-consumer","service":"telemetry-rest"}')
echo "  rest-consumer:      $body"
if echo "$body" | grep -q '"decision":"Permit"'; then
  pass "rest-consumer → Permit (REST)"
else
  fail "rest-consumer → Permit (REST)" '"decision":"Permit"' "$body"
fi

body=$(http_body -X POST http://localhost:9093/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"test-probe","service":"telemetry-rest"}')
echo "  test-probe:         $body"
if echo "$body" | grep -q '"decision":"Permit"'; then
  pass "test-probe → Permit (REST)"
else
  fail "test-probe → Permit (REST)" '"decision":"Permit"' "$body"
fi

body=$(http_body -X POST http://localhost:9093/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unauthorized","service":"telemetry-rest"}')
echo "  unauthorized:       $body"
if echo "$body" | grep -q '"decision":"Deny"'; then
  pass "unauthorized → Deny (REST)"
else
  fail "unauthorized → Deny (REST)" '"decision":"Deny"' "$body"
fi

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

if [ "${count:-0}" -gt 0 ]; then
  pass "rest-consumer msgCount > 0 (got $count)"
else
  fail "rest-consumer msgCount > 0" ">0" "${count:-0}"
fi

if echo "$rcstats" | grep -q '"lastDeniedAt":""'; then
  pass "rest-consumer lastDeniedAt is empty (never denied)"
else
  fail "rest-consumer lastDeniedAt is empty" '""' "$rcstats"
fi

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

if [ "${ac_count:-0}" -gt 0 ]; then
  pass "analytics-consumer msgCount > 0 (got $ac_count)"
else
  fail "analytics-consumer msgCount > 0" ">0" "${ac_count:-0}"
fi

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
  if echo "$permit_before" | grep -q '"decision":"Permit"'; then
    pass "rest-consumer → Permit before revocation"
  else
    fail "rest-consumer → Permit before revocation" '"decision":"Permit"' "$permit_before"
  fi

  revoke_code=$(http_code -X DELETE "http://localhost:8082/authorization/revoke/$rc_grant_id")
  check_eq "revoke rest-consumer grant → 200" "200" "$revoke_code"

  echo "  waiting 30 s for policy-sync cycle to propagate revocation..."
  sleep 30

  deny_body=$(http_body -X POST http://localhost:9093/auth/check \
    -H 'Content-Type: application/json' \
    -d '{"consumer":"rest-consumer","service":"telemetry-rest"}')
  echo "  rest-consumer AuthzForce after revoke: $deny_body"
  if echo "$deny_body" | grep -q '"decision":"Deny"'; then
    pass "rest-consumer → Deny in AuthzForce after revocation"
  else
    fail "rest-consumer → Deny after revocation" '"decision":"Deny"' "$deny_body"
  fi

  http_code_after=$(http_code -H 'X-Consumer-Name: rest-consumer' \
    http://localhost:9093/telemetry/latest)
  check_eq "rest-consumer REST request → 403 after revocation" "403" "$http_code_after"

  regrant=$(http_body -X POST http://localhost:8082/authorization/grant \
    -H 'Content-Type: application/json' \
    -d '{"consumerSystemName":"rest-consumer","providerSystemName":"data-provider","serviceDefinition":"telemetry-rest"}')
  if echo "$regrant" | grep -qE '"id":|already exists'; then
    pass "rest-consumer grant restored"
  else
    fail "rest-consumer grant restored" '"id":N' "$regrant"
  fi

  echo "  waiting 15 s for grant restoration to propagate..."
  sleep 15

  permit_after=$(http_body -X POST http://localhost:9093/auth/check \
    -H 'Content-Type: application/json' \
    -d '{"consumer":"rest-consumer","service":"telemetry-rest"}')
  if echo "$permit_after" | grep -q '"decision":"Permit"'; then
    pass "rest-consumer → Permit again after grant restored"
  else
    fail "rest-consumer → Permit after grant restored" '"decision":"Permit"' "$permit_after"
  fi
fi

# ── Section 10: kafka-authz SSE stream (test-probe) ───────────────────────────
echo
echo "=== 10. kafka-authz SSE stream (test-probe consumer) ==="

sse=$(timeout 4 curl -sN \
  "http://localhost:9091/stream/test-probe?service=telemetry" 2>/dev/null || true)
preview="${sse:0:300}"
echo "  first 300 chars: $preview"

if echo "$sse" | grep -q 'not authorized'; then
  fail "SSE test-probe: not denied" "no 'not authorized'" "$preview"
else
  pass "SSE test-probe: not denied (403)"
fi

if [[ "$sse" == *"data: {"* ]]; then
  pass "SSE test-probe: data lines received from Kafka"
else
  fail "SSE test-probe: data lines received from Kafka" "data: {...}" "$preview"
fi

# ── Section 11: Dashboard proxy — Kafka and REST tab endpoints ────────────────
echo
echo "=== 11. Dashboard proxy — Kafka and REST tab API endpoints (localhost:3006) ==="

# kafka-authz /status via nginx
kstatus=$(http_body http://localhost:3006/api/kafka-authz/status)
echo "  kafka-authz /status: $kstatus"
if echo "$kstatus" | grep -q '"activeStreams"'; then
  pass "GET /api/kafka-authz/status via nginx → activeStreams field"
else
  fail "GET /api/kafka-authz/status via nginx" '"activeStreams":N' "$kstatus"
fi
if echo "$kstatus" | grep -q '"totalServed"'; then
  pass "GET /api/kafka-authz/status via nginx → totalServed field"
else
  fail "GET /api/kafka-authz/status via nginx" '"totalServed":N' "$kstatus"
fi

# kafka-authz /auth/check via nginx
kcheck=$(http_body -X POST http://localhost:3006/api/kafka-authz/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"analytics-consumer","service":"telemetry"}')
echo "  kafka-authz /auth/check (analytics-consumer): $kcheck"
if echo "$kcheck" | grep -q '"decision":"Permit"'; then
  pass "POST /api/kafka-authz/auth/check via nginx → analytics-consumer Permit"
else
  fail "POST /api/kafka-authz/auth/check via nginx" '"decision":"Permit"' "$kcheck"
fi

kcheck_deny=$(http_body -X POST http://localhost:3006/api/kafka-authz/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unknown-consumer","service":"telemetry"}')
echo "  kafka-authz /auth/check (unknown-consumer): $kcheck_deny"
if echo "$kcheck_deny" | grep -q '"decision":"Deny"'; then
  pass "POST /api/kafka-authz/auth/check via nginx → unknown-consumer Deny"
else
  fail "POST /api/kafka-authz/auth/check via nginx" '"decision":"Deny"' "$kcheck_deny"
fi

# data-provider /stats via nginx
dpstats=$(http_body http://localhost:3006/api/data-provider/stats)
echo "  data-provider /stats: $dpstats"
if echo "$dpstats" | grep -q '"msgCount"'; then
  pass "GET /api/data-provider/stats via nginx → msgCount field"
else
  fail "GET /api/data-provider/stats via nginx" '"msgCount":N' "$dpstats"
fi
if echo "$dpstats" | grep -q '"robotCount"'; then
  pass "GET /api/data-provider/stats via nginx → robotCount field"
else
  fail "GET /api/data-provider/stats via nginx" '"robotCount":N' "$dpstats"
fi

# rest-authz /status via nginx
rastatus=$(http_body http://localhost:3006/api/rest-authz/status)
echo "  rest-authz /status: $rastatus"
if echo "$rastatus" | grep -q '"requestsTotal"'; then
  pass "GET /api/rest-authz/status via nginx → requestsTotal field"
else
  fail "GET /api/rest-authz/status via nginx" '"requestsTotal":N' "$rastatus"
fi
if echo "$rastatus" | grep -q '"permitted"'; then
  pass "GET /api/rest-authz/status via nginx → permitted field"
else
  fail "GET /api/rest-authz/status via nginx" '"permitted":N' "$rastatus"
fi

# ServiceRegistry query for telemetry-rest via nginx
srquery=$(http_body -X POST http://localhost:3006/api/serviceregistry/serviceregistry/query \
  -H 'Content-Type: application/json' \
  -d '{"serviceDefinition":"telemetry-rest"}')
echo "  SR query for telemetry-rest (first 200 chars): ${srquery:0:200}"
if echo "$srquery" | grep -q '"serviceInstances"'; then
  pass "POST /api/serviceregistry/query telemetry-rest via nginx → serviceInstances field"
else
  fail "POST /api/serviceregistry/query via nginx" '"serviceInstances":[...]' "$srquery"
fi

# REST data access via nginx proxy (authorized)
rest_auth_code=$(http_code -H 'X-Consumer-Name: test-probe' \
  http://localhost:3006/api/rest-authz/telemetry/latest)
check_eq "GET /api/rest-authz/telemetry/latest via nginx (test-probe) → 200" "200" "$rest_auth_code"

# REST data access via nginx proxy (unauthorized)
rest_deny_code=$(http_code -H 'X-Consumer-Name: unauthorized' \
  http://localhost:3006/api/rest-authz/telemetry/latest)
check_eq "GET /api/rest-authz/telemetry/latest via nginx (unauthorized) → 403" "403" "$rest_deny_code"

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
