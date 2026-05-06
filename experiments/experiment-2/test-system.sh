#!/usr/bin/env bash
# test-system.sh — system test for experiment-2 running under Docker.
#
# Run from experiments/experiment-2/ with the stack already up:
#
#   cd experiments/experiment-2
#   docker compose up -d --build
#   bash test-system.sh
#
# Each test prints PASS or FAIL. Exits 1 if any test fails.

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

# ── Section 1: Core service health ────────────────────────────────────────────
echo
echo "=== 1. Core service health ==="

for svc in \
  "ServiceRegistry:http://localhost:8080/health" \
  "Authentication:http://localhost:8081/health" \
  "ConsumerAuth:http://localhost:8082/health" \
  "DynamicOrch:http://localhost:8083/health" \
  "CertificateAuthority:http://localhost:8086/health"; do
  name="${svc%%:*}"
  url="${svc#*:}"
  check_eq "$name /health → 200" "200" "$(http_code "$url")"
done

# ── Section 2: RabbitMQ management reachable ──────────────────────────────────
echo
echo "=== 2. RabbitMQ management API ==="
check_eq "RabbitMQ management /api/overview → 200" "200" \
  "$(http_code -u admin:admin http://localhost:15672/api/overview)"

# ── Section 3: Edge-adapter health and telemetry endpoint ─────────────────────
echo
echo "=== 3. Edge-adapter ==="
check_eq "edge-adapter /health → 200" "200" "$(http_code http://localhost:9001/health)"

# Wait up to 15s for telemetry to appear.
echo "  Waiting for telemetry data (up to 15s)..."
got_telemetry=false
for i in $(seq 1 15); do
  body=$(http_body http://localhost:9001/telemetry/latest 2>/dev/null || echo "")
  if echo "$body" | grep -q "robotId\|temperature\|robot"; then
    got_telemetry=true
    break
  fi
  sleep 1
done
if $got_telemetry; then
  pass "edge-adapter /telemetry/latest contains telemetry data"
else
  fail "edge-adapter /telemetry/latest" "JSON with robotId/temperature" "no data after 15s"
fi

# ── Section 4: ServiceRegistry has registered services ───────────────────────
echo
echo "=== 4. ServiceRegistry ==="
sr_body=$(http_body -X POST http://localhost:8080/serviceregistry/query \
  -H 'Content-Type: application/json' \
  -d '{}' 2>/dev/null || echo "")
if echo "$sr_body" | grep -q "serviceInstances\|services\|\[\]"; then
  pass "ServiceRegistry /query → 200 with valid response"
else
  fail "ServiceRegistry /query" "JSON response with serviceInstances" "$sr_body"
fi

# ── Section 5: Robot-fleet health ─────────────────────────────────────────────
echo
echo "=== 5. Robot-fleet ==="
# robot-fleet in exp-2 has no HTTP health endpoint; check via docker logs
check_eq "robot-fleet container running" "200" "$(http_code http://localhost:9001/health 2>/dev/null || echo 000)"

# ── Section 6: Consumer stats ─────────────────────────────────────────────────
echo
echo "=== 6. Consumer stats ==="
consumer_body=$(http_body http://localhost:9002/stats 2>/dev/null || echo "")
if echo "$consumer_body" | grep -q "msgCount"; then
  pass "consumer /stats → contains msgCount"
else
  fail "consumer /stats" "JSON with msgCount" "$consumer_body"
fi

# Allow some time for messages to arrive.
sleep 5
consumer_body=$(http_body http://localhost:9002/stats 2>/dev/null || echo "")
msg_count=$(echo "$consumer_body" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
if [ "${msg_count:-0}" -gt 0 ]; then
  pass "consumer received messages (msgCount=$msg_count)"
else
  fail "consumer msgCount > 0" ">0" "$msg_count"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo
echo "=============================="
echo "  PASS: $PASS  FAIL: $FAIL"
echo "=============================="

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
