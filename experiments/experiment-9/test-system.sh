#!/usr/bin/env bash
# test-system.sh — system test for experiment-9 running under Docker.
#
# UC3 Lawn Mowing as a Service: robot sites → portal-cloud-ml → service partners.
#
# Run from experiments/experiment-9/ with the stack already up:
#
#   cd experiments/experiment-9
#   docker compose up -d --build
#   bash test-system.sh
#
# Each test prints PASS or FAIL. The script exits 1 if any test fails.

set -euo pipefail

PASS=0
FAIL=0

source "$(dirname "$0")/../test-lib.sh"

# ── Pre-flight: core services reachable ───────────────────────────────────────
echo
echo "=== Pre-flight: core and PEP service health ==="

smoke_http "profile-ca /health (HTTP)"           http://localhost:8187/health
smoke_http "AuthzForce /health"                   http://localhost:8296/health
smoke_http "kafka-authz /health"                  http://localhost:9201/health
smoke_http "pki-rest-authz /health (HTTP)"        http://localhost:9209/health
smoke_http "portal-cloud-ml /health (HTTP)"       http://localhost:9207/health

# ── Section 1: profile-ca PKI endpoints ──────────────────────────────────────
echo
echo "=== 1. profile-ca (localhost:8187) ==="

check_eq "GET /health → 200"  "200" "$(http_code http://localhost:8187/health)"

ca_info=$(http_body http://localhost:8187/ca/info)
assert_json_field "GET /ca/info → commonName field"   "commonName"   "$ca_info"
assert_json_field "GET /ca/info → certificate field"  "certificate"  "$ca_info"
assert_contains   "GET /ca/info → PEM block"          "BEGIN CERTIFICATE" "$ca_info"

on_resp=$(http_body -X POST http://localhost:8187/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"test-probe"}')
assert_json_field "POST /bootstrap/onboarding-cert → certificate" "certificate" "$on_resp"
assert_json_value "POST /bootstrap/onboarding-cert → profile=on"  "profile"     "on" "$on_resp"

# ── Section 2: Full PKI lifecycle (on → de → sy) ─────────────────────────────
echo
echo "=== 2. Full PKI lifecycle (on → de → sy) ==="

CA_FILE=/tmp/exp9-ca.crt
ON_CRT=/tmp/exp9-on.crt ; ON_KEY=/tmp/exp9-on.key
DE_CRT=/tmp/exp9-de.crt ; DE_KEY=/tmp/exp9-de.key
SY_CRT=/tmp/exp9-sy.crt ; SY_KEY=/tmp/exp9-sy.key

_ca_pem=$(curl -s http://localhost:8187/ca/info \
  | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
[ -z "$_ca_pem" ] && smoke_fail "PKI lifecycle: CA cert" "empty"
echo "$_ca_pem" > "$CA_FILE"
pass "PKI step 1: CA cert fetched"

_on_resp=$(curl -s -X POST http://localhost:8187/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"lifecycle-test"}')
_on_crt=$(echo "$_on_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
_on_key=$(echo "$_on_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")
[ -z "$_on_crt" ] && smoke_fail "PKI step 2" "empty onboarding cert"
echo "$_on_crt" > "$ON_CRT" ; echo "$_on_key" > "$ON_KEY"
pass "PKI step 2: onboarding cert (OU=on)"

_de_resp=$(curl -s -X POST \
  --cert "$ON_CRT" --key "$ON_KEY" --cacert "$CA_FILE" \
  --resolve "profile-ca:8088:127.0.0.1" \
  https://profile-ca:8088/ca/device-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"lifecycle-test"}' 2>/dev/null || echo "")
_de_crt=$(echo "$_de_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
_de_key=$(echo "$_de_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")
[ -z "$_de_crt" ] && smoke_fail "PKI step 3" "empty device cert"
echo "$_de_crt" > "$DE_CRT" ; echo "$_de_key" > "$DE_KEY"
pass "PKI step 3: device cert (OU=de)"

_sy_resp=$(curl -s -X POST \
  --cert "$DE_CRT" --key "$DE_KEY" --cacert "$CA_FILE" \
  --resolve "profile-ca:8088:127.0.0.1" \
  https://profile-ca:8088/ca/system-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"lifecycle-test"}' 2>/dev/null || echo "")
_sy_crt=$(echo "$_sy_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
_sy_key=$(echo "$_sy_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")
[ -z "$_sy_crt" ] && smoke_fail "PKI step 4" "empty system cert"
echo "$_sy_crt" > "$SY_CRT" ; echo "$_sy_key" > "$SY_KEY"
pass "PKI step 4: system cert (OU=sy)"

# ── Section 3: Profile enforcement ───────────────────────────────────────────
echo
echo "=== 3. Profile enforcement ==="

_bad_de=$(curl -s -o /dev/null -w "%{http_code}" \
  --cert "$SY_CRT" --key "$SY_KEY" --cacert "$CA_FILE" \
  --resolve "profile-ca:8088:127.0.0.1" \
  https://profile-ca:8088/ca/device-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"bypass"}' 2>/dev/null; echo -n "")
[ "$_bad_de" = "403" ] && pass "System cert rejected for /ca/device-cert (403)" \
  || fail "System cert rejected for /ca/device-cert" "403" "$_bad_de"

# ── Section 4: policy-sync ───────────────────────────────────────────────────
echo
echo "=== 4. policy-sync /status (localhost:9205) ==="

echo "  Waiting for policy-sync first sync (up to 30s)..."
ps_status=""
for i in $(seq 1 6); do
  ps_status=$(curl -s http://localhost:9205/status 2>/dev/null || echo "{}")
  if echo "$ps_status" | grep -q '"synced":true'; then break; fi
  echo "  ... attempt $i/6, sleeping 5s"
  sleep 5
done
echo "$ps_status" | grep -q '"synced":true' \
  && pass "policy-sync synced=true" \
  || smoke_fail "policy-sync synced=true" "not synced after 30s"

echo "$ps_status" | grep -q '"domainExternalId":"arrowhead-exp9"' \
  && pass "policy-sync domainExternalId=arrowhead-exp9" \
  || smoke_fail "policy-sync domainExternalId=arrowhead-exp9" "AUTHZFORCE_DOMAIN mismatch (EXP-001)"

status=$(http_body http://localhost:9205/status)
echo "  raw: $status"
assert_json_value "synced=true"                           "synced"           "true"             "$status"
assert_json_gt    "version ≥ 1"                           "version"          0                  "$status"
assert_json_gt    "grants ≥ 1"                            "grants"           0                  "$status"
assert_json_value "domainExternalId=arrowhead-exp9"       "domainExternalId" "arrowhead-exp9"   "$status"

# ── Section 5: AuthzForce ─────────────────────────────────────────────────────
echo
echo "=== 5. AuthzForce (localhost:8296) ==="

check_eq "GET /health → 200"               "200" "$(http_code http://localhost:8296/health)"
check_eq "GET /authzforce-ce/domains → 200" "200" "$(http_code http://localhost:8296/authzforce-ce/domains)"

# ── Section 6: kafka-authz authorization ─────────────────────────────────────
echo
echo "=== 6. kafka-authz /auth/check (localhost:9201) ==="

body=$(http_body -X POST http://localhost:9201/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"portal-cloud-ml","service":"telemetry"}')
echo "  portal-cloud-ml: $body"
assert_json_value "portal-cloud-ml → Permit (Kafka/SSE)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9201/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"test-probe","service":"telemetry"}')
echo "  test-probe:      $body"
assert_json_value "test-probe → Permit (Kafka/SSE)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9201/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unauthorized","service":"telemetry"}')
echo "  unauthorized:    $body"
assert_json_value "unauthorized → Deny (Kafka/SSE)" "decision" "Deny" "$body"

# ── Section 7: pki-rest-authz /auth/check ────────────────────────────────────
echo
echo "=== 7. pki-rest-authz /auth/check (HTTP localhost:9209) ==="

body=$(http_body -X POST http://localhost:9209/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"service-partner-1","service":"telemetry-rest"}')
echo "  service-partner-1: $body"
assert_json_value "service-partner-1 → Permit (REST mTLS)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9209/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"service-partner-2","service":"telemetry-rest"}')
echo "  service-partner-2: $body"
assert_json_value "service-partner-2 → Permit (REST mTLS)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9209/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"test-probe","service":"telemetry-rest"}')
echo "  test-probe:        $body"
assert_json_value "test-probe → Permit (REST mTLS)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9209/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unauthorized","service":"telemetry-rest"}')
echo "  unauthorized:      $body"
assert_json_value "unauthorized → Deny (REST mTLS)" "decision" "Deny" "$body"

# ── Section 8: portal-cloud-ml health and stats ───────────────────────────────
echo
echo "=== 8. portal-cloud-ml (localhost:9207) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:9207/health)"
pm_stats=$(http_body http://localhost:9207/stats)
echo "  raw: $pm_stats"
assert_json_field "portal-cloud-ml /stats → msgCount field"    "msgCount"    "$pm_stats"
assert_json_field "portal-cloud-ml /stats → transport field"   "transport"   "$pm_stats"
assert_json_value "portal-cloud-ml transport=kafka-sse"        "transport"   "kafka-sse" "$pm_stats"

# ── Section 9: mTLS — system cert for pki-rest-authz ─────────────────────────
echo
echo "=== 9. mTLS — PKI-lifecycle system cert for REST access to portal-cloud-ml ==="

PROBE_SY_CRT=/tmp/exp9-probe-sy.crt
PROBE_SY_KEY=/tmp/exp9-probe-sy.key

_p_on=$(curl -s -X POST http://localhost:8187/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"test-probe"}')
_p_on_crt=$(echo "$_p_on" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
_p_on_key=$(echo "$_p_on" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")

if [ -z "$_p_on_crt" ]; then
  fail "mTLS: test-probe onboarding cert" "certificate" "empty"
else
  echo "$_p_on_crt" > /tmp/exp9-probe-on.crt
  echo "$_p_on_key" > /tmp/exp9-probe-on.key

  _p_de=$(curl -s -X POST \
    --cert /tmp/exp9-probe-on.crt --key /tmp/exp9-probe-on.key \
    --cacert "$CA_FILE" --resolve "profile-ca:8088:127.0.0.1" \
    https://profile-ca:8088/ca/device-cert \
    -H 'Content-Type: application/json' -d '{"systemName":"test-probe"}' 2>/dev/null || echo "")
  _p_de_crt=$(echo "$_p_de" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
  _p_de_key=$(echo "$_p_de" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")
  echo "$_p_de_crt" > /tmp/exp9-probe-de.crt
  echo "$_p_de_key" > /tmp/exp9-probe-de.key

  _p_sy=$(curl -s -X POST \
    --cert /tmp/exp9-probe-de.crt --key /tmp/exp9-probe-de.key \
    --cacert "$CA_FILE" --resolve "profile-ca:8088:127.0.0.1" \
    https://profile-ca:8088/ca/system-cert \
    -H 'Content-Type: application/json' -d '{"systemName":"test-probe"}' 2>/dev/null || echo "")
  _p_sy_crt=$(echo "$_p_sy" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
  _p_sy_key=$(echo "$_p_sy" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")

  if [ -z "$_p_sy_crt" ]; then
    fail "mTLS: test-probe system cert" "certificate" "empty"
  else
    echo "$_p_sy_crt" > "$PROBE_SY_CRT"
    echo "$_p_sy_key" > "$PROBE_SY_KEY"
    pass "mTLS: test-probe PKI lifecycle (on → de → sy) completed"

    # Use system cert to access pki-rest-authz mTLS port → proxied to portal-cloud-ml
    mtls_code="000"
    for i in $(seq 1 12); do
      mtls_code=$(curl -s -o /dev/null -w "%{http_code}" \
        --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
        --cacert "$CA_FILE" \
        --resolve "pki-rest-authz:9208:127.0.0.1" \
        https://pki-rest-authz:9208/telemetry/latest 2>/dev/null; echo -n "")
      if [ "$mtls_code" = "200" ]; then break; fi
      echo "  waiting for portal-cloud-ml data... (attempt $i/12, HTTP $mtls_code)"
      sleep 5
    done
    check_eq "mTLS test-probe GET /telemetry/latest via pki-rest-authz → 200" "200" "$mtls_code"

    no_cert_code=$(curl -s -o /dev/null -w "%{http_code}" \
      --cacert "$CA_FILE" \
      --resolve "pki-rest-authz:9208:127.0.0.1" \
      https://pki-rest-authz:9208/telemetry/latest 2>/dev/null; echo -n "")
    if [ "$no_cert_code" = "400" ] || [ "$no_cert_code" = "000" ] || [ "$no_cert_code" = "401" ]; then
      pass "mTLS: request without client cert rejected (got $no_cert_code)"
    else
      fail "mTLS: request without client cert rejected" "400 or 000" "$no_cert_code"
    fi
  fi
fi

# ── Section 10: service-partner stats ────────────────────────────────────────
echo
echo "=== 10. service-partner-1 /stats (localhost:9211) ==="

sp_stats=""
sp_count=0
for i in $(seq 1 12); do
  sp_stats=$(http_body http://localhost:9211/stats 2>/dev/null || echo "{}")
  sp_count=$(echo "$sp_stats" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  if [ "${sp_count:-0}" -gt 0 ]; then break; fi
  echo "  waiting for service-partner-1 messages... (attempt $i/12)"
  sleep 5
done
echo "  raw: $sp_stats"
assert_json_gt    "service-partner-1 msgCount > 0"                 "msgCount"    0                "$sp_stats"
assert_json_value "service-partner-1 transport=rest-mtls-pki"      "transport"   "rest-mtls-pki"  "$sp_stats"
assert_json_value "service-partner-1 lastDeniedAt empty"           "lastDeniedAt" ""              "$sp_stats"

# ── Section 11: portal-cloud-ml msgCount > 0 ─────────────────────────────────
echo
echo "=== 11. portal-cloud-ml receives telemetry from robot sites ==="

pm_stats=""
pm_count=0
for i in $(seq 1 12); do
  pm_stats=$(http_body http://localhost:9207/stats 2>/dev/null || echo "{}")
  pm_count=$(echo "$pm_stats" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  if [ "${pm_count:-0}" -gt 0 ]; then break; fi
  echo "  waiting for portal-cloud-ml SSE data... (attempt $i/12)"
  sleep 5
done
echo "  raw: $pm_stats"
assert_json_gt "portal-cloud-ml msgCount > 0 (SSE from robot sites)" "msgCount" 0 "$pm_stats"

# ── Section 12: pki-rest-authz /status ───────────────────────────────────────
echo
echo "=== 12. pki-rest-authz /status (localhost:9209) ==="

ra_status=$(http_body http://localhost:9209/status)
echo "  raw: $ra_status"
assert_json_field "requestsTotal" "requestsTotal" "$ra_status"
assert_json_field "permitted"     "permitted"     "$ra_status"
assert_json_field "denied"        "denied"        "$ra_status"

# ── Section 13: kafka-authz SSE stream ────────────────────────────────────────
echo
echo "=== 13. kafka-authz SSE stream (test-probe) ==="

sse=$(timeout 4 curl -sN \
  "http://localhost:9201/stream/test-probe?service=telemetry" 2>/dev/null || true)
echo "  first 300 chars: ${sse:0:300}"
assert_not_contains "SSE test-probe: not denied" "not authorized" "$sse"
assert_contains     "SSE test-probe: data lines" "data: {" "$sse"

# ── Section 14: Revocation propagation ────────────────────────────────────────
echo
echo "=== 14. Revocation: service-partner-1 denied after grant removed ==="

if [ -f "$PROBE_SY_CRT" ] && [ -f "$PROBE_SY_KEY" ]; then
  # Look up the telemetry-rest policy (instanceId is provider|targetType|target).
  lookup=$(curl -s -X POST \
    --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
    --cacert "$CA_FILE" \
    --resolve "consumerauth:8492:127.0.0.1" \
    https://consumerauth:8492/consumerauthorization/authorization/lookup \
    -H 'Content-Type: application/json' \
    -d '{"targetNames":["telemetry-rest"],"targetType":"SERVICE_DEF"}')
  grant_id=$(echo "$lookup" | grep -oE '"instanceId":"[^"]*"' | head -1 | grep -oE '"[^"]*"$' | tr -d '"')

  if [ -z "$grant_id" ]; then
    fail "telemetry-rest grant in ConsumerAuth" "grant" "not found — check setup container logs"
    echo "  Skipping revocation test."
  else
    pass "telemetry-rest grant found (instanceId=$grant_id)"

    permit_before=$(http_body -X POST http://localhost:9209/auth/check \
      -H 'Content-Type: application/json' \
      -d '{"consumer":"service-partner-1","service":"telemetry-rest"}')
    assert_json_value "service-partner-1 → Permit before revocation" "decision" "Permit" "$permit_before"

    encoded_id=$(echo "$grant_id" | sed 's/|/%7C/g')
    revoke_code=$(curl -s -o /dev/null -w "%{http_code}" \
      -X DELETE \
      --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
      --cacert "$CA_FILE" \
      --resolve "consumerauth:8492:127.0.0.1" \
      "https://consumerauth:8492/consumerauthorization/authorization/revoke/$encoded_id"; echo -n "")
    check_eq "revoke telemetry-rest policy via TLS → 200" "200" "$revoke_code"

    echo "  waiting 30s for policy-sync to propagate revocation..."
    sleep 30

    deny_body=$(http_body -X POST http://localhost:9209/auth/check \
      -H 'Content-Type: application/json' \
      -d '{"consumer":"service-partner-1","service":"telemetry-rest"}')
    echo "  auth check after revoke: $deny_body"
    assert_json_value "service-partner-1 → Deny after revocation" "decision" "Deny" "$deny_body"

    regrant=$(curl -s -X POST \
      --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
      --cacert "$CA_FILE" \
      --resolve "consumerauth:8492:127.0.0.1" \
      https://consumerauth:8492/consumerauthorization/authorization/grant \
      -H 'Content-Type: application/json' \
      -d '{"provider":"portal-cloud-ml","targetType":"SERVICE_DEF","target":"telemetry-rest","defaultPolicy":{"policyType":"WHITELIST","policyList":["service-partner-1","service-partner-2","test-probe"]}}')
    if [[ "$regrant" == *'"instanceId":'* ]] || [[ "$regrant" == *"already exists"* ]]; then
      pass "telemetry-rest grant restored"
    else
      fail "telemetry-rest grant restored" '"instanceId": or already exists' "$regrant"
    fi

    echo "  waiting 15s for grant restoration to propagate..."
    sleep 15

    permit_after=$(http_body -X POST http://localhost:9209/auth/check \
      -H 'Content-Type: application/json' \
      -d '{"consumer":"service-partner-1","service":"telemetry-rest"}')
    assert_json_value "service-partner-1 → Permit after re-grant" "decision" "Permit" "$permit_after"
  fi
else
  fail "Revocation test" "PROBE_SY_CRT" "PKI lifecycle failed — skipping section 14"
fi

# ── Section 15: Core TLS ports accessible (mTLS) ─────────────────────────────
echo
echo "=== 15. G4 closure — core service mTLS ports ==="

if [ -f "$PROBE_SY_CRT" ] && [ -f "$PROBE_SY_KEY" ]; then
  for _svc_port in "serviceregistry:8590:serviceregistry:8490" "authentication:8591:authentication:8491" "consumerauth:8592:consumerauth:8492" "dynamicorch:8593:dynamicorch:8493"; do
    _svc=$(echo "$_svc_port" | cut -d: -f1)
    _host_port=$(echo "$_svc_port" | cut -d: -f2)
    _container_port=$(echo "$_svc_port" | cut -d: -f4)
    _code=$(curl -s -o /dev/null -w "%{http_code}" \
      --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
      --cacert "$CA_FILE" \
      --resolve "${_svc}:${_container_port}:127.0.0.1" \
      "https://${_svc}:${_container_port}/health" 2>/dev/null; echo -n "")
    check_eq "G4: ${_svc} TLS /health → 200" "200" "$_code"
  done
else
  fail "G4 mTLS tests" "PROBE_SY_CRT" "PKI lifecycle failed — skipping section 15"
fi

# ── Section 16: Core plain HTTP ports NOT exposed to host ────────────────────
echo
echo "=== 16. Security: core plain HTTP ports not exposed to host ==="

for _plain_port in 8080 8081 8082 8083; do
  _code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 2 \
    http://localhost:$_plain_port/health 2>/dev/null; echo -n "")
  if [ "$_code" = "000" ]; then
    pass "Core port $_plain_port not accessible from host (connection refused)"
  else
    fail "Core port $_plain_port not accessible from host" "000" "HTTP $_code"
  fi
done

# ── Cleanup ───────────────────────────────────────────────────────────────────
rm -f "$CA_FILE" "$ON_CRT" "$ON_KEY" "$DE_CRT" "$DE_KEY" "$SY_CRT" "$SY_KEY" \
      "$PROBE_SY_CRT" "$PROBE_SY_KEY" \
      /tmp/exp9-probe-on.crt /tmp/exp9-probe-on.key \
      /tmp/exp9-probe-de.crt /tmp/exp9-probe-de.key 2>/dev/null || true

# ── Summary ───────────────────────────────────────────────────────────────────
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
