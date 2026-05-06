#!/usr/bin/env bash
# test-system.sh — system test for experiment-3 running under Docker.
#
# Run from experiments/experiment-3/ with the stack already up:
#
#   cd experiments/experiment-3
#   docker compose up -d --build
#   bash test-system.sh
#
# Each test prints PASS or FAIL. Exits 1 if any test fails.
# Any revoked grants are restored on exit.

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

restore_all() {
  for name in demo-consumer-1 demo-consumer-2 demo-consumer-3; do
    http_body -X POST http://localhost:8082/authorization/grant \
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
  "ConsumerAuth:http://localhost:8082/health"; do
  name="${svc%%:*}"
  url="${svc#*:}"
  check_eq "$name /health → 200" "200" "$(http_code "$url")"
done

# ── Section 2: RabbitMQ management reachable ──────────────────────────────────
echo
echo "=== 2. RabbitMQ management API ==="
check_eq "RabbitMQ management /api/overview → 200" "200" \
  "$(http_code -u admin:admin http://localhost:15673/api/overview)"

# ── Section 3: topic-auth-sync health ─────────────────────────────────────────
echo
echo "=== 3. topic-auth-sync ==="
check_eq "topic-auth-sync /health → 200" "200" "$(http_code http://localhost:9090/health)"

# ── Section 4: CA grants seeded ───────────────────────────────────────────────
echo
echo "=== 4. ConsumerAuth grants ==="
grants_body=$(http_body http://localhost:8082/authorization/lookup)
grant_count=$(echo "$grants_body" | grep -oE '"count":[0-9]+' | grep -oE '[0-9]+' || echo "0")
if [ "${grant_count:-0}" -ge 3 ]; then
  pass "at least 3 grants seeded (count=$grant_count)"
else
  fail "grant count ≥ 3" "≥3" "${grant_count}"
fi

# ── Section 5: robot-fleet health ─────────────────────────────────────────────
echo
echo "=== 5. robot-fleet ==="
check_eq "robot-fleet /health → 200" "200" "$(http_code http://localhost:9103/health)"

# ── Section 6: Consumers receiving messages ───────────────────────────────────
echo
echo "=== 6. Consumer message receipt ==="
sleep 5  # Allow time for messages to flow through broker

for i in 1 2 3; do
  body=$(http_body "http://localhost:$((9002 + i - 1))/stats" 2>/dev/null || echo "")
  msg_count=$(echo "$body" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  if [ "${msg_count:-0}" -gt 0 ]; then
    pass "consumer-$i received messages (msgCount=$msg_count)"
  else
    fail "consumer-$i msgCount > 0" ">0" "${msg_count}"
  fi
done

# ── Section 7: topic-auth-sync authorization check ────────────────────────────
echo
echo "=== 7. topic-auth-sync authorization ==="
# Verify an authorized user is allowed.
allow_resp=$(http_body -X POST http://localhost:9090/auth/topic \
  -d "username=demo-consumer-1&vhost=arrowhead&resource=topic&name=telemetry.robot-1&permission=read" \
  2>/dev/null || echo "")
if echo "$allow_resp" | grep -q "allow"; then
  pass "topic-auth-sync allows authorized consumer"
else
  fail "topic-auth-sync allow" "allow" "$allow_resp"
fi

# Verify an unauthorized user is denied.
deny_resp=$(http_body -X POST http://localhost:9090/auth/topic \
  -d "username=unknown-consumer&vhost=arrowhead&resource=topic&name=telemetry.robot-1&permission=read" \
  2>/dev/null || echo "")
if echo "$deny_resp" | grep -q "deny"; then
  pass "topic-auth-sync denies unauthorized consumer"
else
  fail "topic-auth-sync deny" "deny" "$deny_resp"
fi

# ── Section 8: Revocation flow ────────────────────────────────────────────────
echo
echo "=== 8. Revocation flow ==="

# Get consumer-2's grant ID.
grant_id=$(http_body http://localhost:8082/authorization/lookup |
  python3 -c "import sys,json; rules=json.load(sys.stdin).get('rules',[]); print(next((r['id'] for r in rules if r.get('consumerSystemName')=='demo-consumer-2'), ''))" 2>/dev/null || echo "")

if [ -z "$grant_id" ]; then
  fail "find consumer-2 grant" "non-empty grant id" "empty"
else
  pass "found consumer-2 grant id=$grant_id"

  # Revoke consumer-2.
  revoke_code=$(http_code -X DELETE "http://localhost:8082/authorization/revoke/$grant_id")
  check_eq "revoke consumer-2 → 200" "200" "$revoke_code"

  # Wait for topic-auth-sync to pick up the revocation.
  sleep 12

  # topic-auth-sync should now deny consumer-2.
  deny_after=$(http_body -X POST http://localhost:9090/auth/topic \
    -d "username=demo-consumer-2&vhost=arrowhead&resource=topic&name=telemetry.robot-1&permission=read" \
    2>/dev/null || echo "")
  if echo "$deny_after" | grep -q "deny"; then
    pass "topic-auth-sync denies consumer-2 after revocation"
  else
    fail "topic-auth-sync deny after revocation" "deny" "$deny_after"
  fi
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo
echo "=============================="
echo "  PASS: $PASS  FAIL: $FAIL"
echo "=============================="

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
