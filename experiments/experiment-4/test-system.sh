#!/usr/bin/env bash
# test-system.sh — system test for experiment-4 running under Docker.
#
# Run from experiments/experiment-4/ with the stack already up:
#
#   cd experiments/experiment-4
#   docker compose up -d --build
#   bash test-system.sh
#
# Each test prints PASS or FAIL. Exits 1 if any test fails.
# On completion, any revoked grants are restored.

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

# Tracks grant IDs that we revoke so we can restore them at the end.
REVOKED_IDS=""
restore_all() {
  for name in demo-consumer-1 demo-consumer-2 demo-consumer-3; do
    http_body -s -X POST http://localhost:8082/authorization/grant \
      -H 'Content-Type: application/json' \
      -d "{\"consumerSystemName\":\"$name\",\"providerSystemName\":\"robot-fleet\",\"serviceDefinition\":\"telemetry\"}" \
      > /dev/null 2>&1 || true
  done
}
trap restore_all EXIT

# ── Section 1: Core service health ────────────────────────────────────────────
echo
echo "=== 1. Core service health ==="

for svc in \
  "ServiceRegistry:http://localhost:8080/health" \
  "Authentication:http://localhost:8081/health" \
  "ConsumerAuth:http://localhost:8082/health" \
  "DynamicOrch:http://localhost:8083/health"; do
  name="${svc%%:*}"
  url="${svc#*:}"
  check_eq "$name /health → 200" "200" "$(http_code "$url")"
done

check_eq "topic-auth-http /health → 200" "200" "$(http_code http://localhost:9090/health)"
check_eq "robot-fleet /health → 200" "200" "$(http_code http://localhost:9104/health)"

# ── Section 2: RabbitMQ management reachable ──────────────────────────────────
echo
echo "=== 2. RabbitMQ management API ==="
check_eq "RabbitMQ management /api/overview → 200" "200" \
  "$(http_code -u admin:admin http://localhost:15674/api/overview)"

# ── Section 3: CA grants seeded by setup ──────────────────────────────────────
echo
echo "=== 3. ConsumerAuth grants ==="
grants_body=$(http_body http://localhost:8082/authorization/lookup)
echo "  $grants_body"
grant_count=$(echo "$grants_body" | grep -oE '"count":[0-9]+' | grep -oE '[0-9]+' || echo "0")
if [ "${grant_count:-0}" -ge 3 ]; then
  pass "at least 3 grants seeded (count=$grant_count)"
else
  fail "at least 3 grants seeded" "≥3" "${grant_count:-0}"
fi

# ── Section 4: All 3 consumers receiving messages ─────────────────────────────
echo
echo "=== 4. Consumers receiving messages ==="
for port_name in "9002:demo-consumer-1" "9004:demo-consumer-2" "9005:demo-consumer-3"; do
  port="${port_name%%:*}"
  cname="${port_name#*:}"
  stats=""
  count=0
  for i in $(seq 1 12); do
    stats=$(http_body "http://localhost:${port}/stats" 2>/dev/null || echo '{"msgCount":0}')
    count=$(echo "$stats" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
    if [ "${count:-0}" -gt 0 ]; then break; fi
    echo "  waiting for $cname messages... ($i/12)"
    sleep 5
  done
  if [ "${count:-0}" -gt 0 ]; then
    pass "$cname receiving messages (msgCount=$count)"
  else
    fail "$cname receiving messages" "msgCount>0" "${count:-0}"
  fi
done

# ── Section 5: topic-auth-http live auth check ────────────────────────────────
echo
echo "=== 5. topic-auth-http live auth checks ==="

# Simulate what RabbitMQ sends: /auth/user for a valid consumer
auth_resp=$(http_body -X POST http://localhost:9090/auth/user \
  -d "username=demo-consumer-1&password=consumer-secret")
check_eq "auth/user demo-consumer-1 → allow" "allow" "$auth_resp"

# Revoked / unknown consumer should be denied
auth_deny=$(http_body -X POST http://localhost:9090/auth/user \
  -d "username=unknown-system&password=consumer-secret")
check_eq "auth/user unknown-system → deny" "deny" "$auth_deny"

# Admin auth
auth_admin=$(http_body -X POST http://localhost:9090/auth/user \
  -d "username=admin&password=admin")
if echo "$auth_admin" | grep -q "^allow"; then
  pass "auth/user admin → allow (with tags)"
else
  fail "auth/user admin → allow" "allow ..." "$auth_admin"
fi

# ── Section 6: Revocation — consumer-2 ────────────────────────────────────────
echo
echo "=== 6. Revocation: consumer-2 AMQP connection closed after grant revoked ==="

# Find demo-consumer-2 grant ID.
lookup=$(http_body http://localhost:8082/authorization/lookup)
grant_id=$(echo "$lookup" | grep -oE '"id":[0-9]+[^}]*"consumerSystemName":"demo-consumer-2"' | grep -oE '"id":[0-9]+' | grep -oE '[0-9]+' | head -1)
if [ -z "$grant_id" ]; then
  # Try the other field order
  grant_id=$(echo "$lookup" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for r in data.get('rules', []):
    if r.get('consumerSystemName') == 'demo-consumer-2':
        print(r['id'])
        break
" 2>/dev/null || echo "")
fi

if [ -z "$grant_id" ]; then
  fail "demo-consumer-2 grant found in CA" "grant id" "not found"
  echo "  Skipping revocation test (no grant to revoke)."
else
  pass "demo-consumer-2 grant found in CA (id=$grant_id)"

  # Confirm connection exists before revocation.
  conns_before=$(http_body -u admin:admin http://localhost:15674/api/connections)
  c2_before=$(echo "$conns_before" | grep -o '"demo-consumer-2"' | wc -l | tr -d ' ')
  if [ "${c2_before:-0}" -gt 0 ]; then
    pass "demo-consumer-2 has active AMQP connection before revocation"
  else
    fail "demo-consumer-2 has active AMQP connection before revocation" ">0" "0"
  fi

  # Record baseline message count.
  stats_before=$(http_body http://localhost:9004/stats 2>/dev/null || echo '{"msgCount":0}')
  count_before=$(echo "$stats_before" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")

  # Revoke the grant.
  revoke_code=$(http_code -X DELETE "http://localhost:8082/authorization/revoke/$grant_id")
  check_eq "DELETE /authorization/revoke/$grant_id → 200" "200" "$revoke_code"

  # Verify auth/user now denies consumer-2 (live CA check).
  check_eq "auth/user demo-consumer-2 → deny after revoke" "deny" \
    "$(http_body -X POST http://localhost:9090/auth/user -d 'username=demo-consumer-2&password=consumer-secret')"

  # Wait for revocation loop to fire (REVOCATION_INTERVAL=3s + buffer).
  echo "  waiting 8 s for revocation loop..."
  sleep 8

  # Confirm connection is gone.
  conns_after=$(http_body -u admin:admin http://localhost:15674/api/connections)
  c2_after=$(echo "$conns_after" | grep -o '"demo-consumer-2"' | wc -l | tr -d ' ')
  check_eq "demo-consumer-2 AMQP connection closed after revocation" "0" "${c2_after:-0}"

  # Confirm stats have NOT grown significantly (consumer is disconnected).
  sleep 5
  stats_after=$(http_body http://localhost:9004/stats 2>/dev/null || echo '{"msgCount":0}')
  count_after=$(echo "$stats_after" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  delta=$((count_after - count_before))
  # With 3 robots at 10Hz and 5s window, a connected consumer would get ~150 msgs.
  # Allow up to 10 as slack for messages already in flight when revocation fired.
  if [ "$delta" -le 10 ]; then
    pass "demo-consumer-2 msgCount not growing after revocation (delta=$delta)"
  else
    fail "demo-consumer-2 msgCount not growing after revocation" "delta≤10" "delta=$delta"
  fi

  # ── Section 7: Re-grant restores access ─────────────────────────────────────
  echo
  echo "=== 7. Re-grant: consumer-2 reconnects ==="

  regrant=$(http_body -X POST http://localhost:8082/authorization/grant \
    -H 'Content-Type: application/json' \
    -d '{"consumerSystemName":"demo-consumer-2","providerSystemName":"robot-fleet","serviceDefinition":"telemetry"}')
  if echo "$regrant" | grep -qE '"id":|already exists'; then
    pass "demo-consumer-2 grant restored"
  else
    fail "demo-consumer-2 grant restored" '"id":N' "$regrant"
  fi

  # Verify auth/user allows consumer-2 again.
  check_eq "auth/user demo-consumer-2 → allow after re-grant" "allow" \
    "$(http_body -X POST http://localhost:9090/auth/user -d 'username=demo-consumer-2&password=consumer-secret')"

  # Wait for consumer-2 to retry and reconnect (retry loop: 5s).
  echo "  waiting 20 s for consumer-2 to reconnect..."
  sleep 20

  # Confirm connection is re-established.
  conns_reconnect=$(http_body -u admin:admin http://localhost:15674/api/connections)
  c2_reconnect=$(echo "$conns_reconnect" | grep -o '"demo-consumer-2"' | wc -l | tr -d ' ')
  if [ "${c2_reconnect:-0}" -gt 0 ]; then
    pass "demo-consumer-2 AMQP connection re-established after re-grant"
  else
    fail "demo-consumer-2 AMQP connection re-established after re-grant" ">0" "0"
  fi

  # Confirm stats are growing again (record count, wait, check delta).
  count_regrant_before=$(http_body http://localhost:9004/stats 2>/dev/null | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  sleep 5
  count_regrant_after=$(http_body http://localhost:9004/stats 2>/dev/null | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  delta_regrant=$((count_regrant_after - count_regrant_before))
  if [ "$delta_regrant" -gt 5 ]; then
    pass "demo-consumer-2 msgCount growing after re-grant (delta=$delta_regrant)"
  else
    fail "demo-consumer-2 msgCount growing after re-grant" "delta>5" "delta=$delta_regrant"
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
