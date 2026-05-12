#!/usr/bin/env bash
# test-system.sh — system test for experiment-8 running under Docker.
#
# Run from experiments/experiment-8/ with the stack already up:
#
#   cd experiments/experiment-8
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

smoke_http "profile-ca /health (HTTP)"          http://localhost:8087/health
smoke_http "AuthzForce /health"                  http://localhost:8196/health
smoke_http "kafka-authz /health"                 http://localhost:9101/health
smoke_http "pki-rest-authz /health (HTTP)"       http://localhost:9109/health

# ── Section 1: profile-ca PKI endpoints ──────────────────────────────────────
echo
echo "=== 1. profile-ca (localhost:8087) ==="

check_eq "GET /health → 200"  "200" "$(http_code http://localhost:8087/health)"

ca_info=$(http_body http://localhost:8087/ca/info)
echo "  /ca/info: ${ca_info:0:120}..."
assert_json_field "GET /ca/info → commonName field present"    "commonName"   "$ca_info"
assert_json_field "GET /ca/info → certificate field present"   "certificate"  "$ca_info"
assert_contains   "GET /ca/info → certificate is PEM"          "BEGIN CERTIFICATE" "$ca_info"

# Bootstrap onboarding cert (plain HTTP, no auth)
on_resp=$(http_body -X POST http://localhost:8087/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' \
  -d '{"systemName":"test-probe"}')
echo "  bootstrap onboarding cert: ${on_resp:0:120}..."
assert_json_field "POST /bootstrap/onboarding-cert → certificate field" "certificate" "$on_resp"
assert_json_field "POST /bootstrap/onboarding-cert → privateKey field"  "privateKey"  "$on_resp"
assert_json_value "POST /bootstrap/onboarding-cert → profile=on"        "profile"     "on" "$on_resp"

# Infra cert (backward-compat endpoint for cert-provisioner)
infra_resp=$(http_body -X POST http://localhost:8087/ca/certificate/issue \
  -H 'Content-Type: application/json' \
  -d '{"systemName":"test-infra"}')
echo "  infra cert: ${infra_resp:0:120}..."
assert_json_field "POST /ca/certificate/issue → certificate field" "certificate" "$infra_resp"
assert_json_value "POST /ca/certificate/issue → profile=sy"        "profile"     "sy" "$infra_resp"

# ── Section 2: Full PKI lifecycle test (on → de → sy) ────────────────────────
echo
echo "=== 2. Full PKI lifecycle (on → de → sy) ==="

# Extract CA cert from /ca/info for TLS verification
CA_FILE=/tmp/exp8-ca.crt
ON_CRT=/tmp/exp8-on.crt
ON_KEY=/tmp/exp8-on.key
DE_CRT=/tmp/exp8-de.crt
DE_KEY=/tmp/exp8-de.key
SY_CRT=/tmp/exp8-sy.crt
SY_KEY=/tmp/exp8-sy.key

_ca_pem=$(curl -s http://localhost:8087/ca/info \
  | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
if [ -z "$_ca_pem" ]; then
  smoke_fail "PKI lifecycle: CA cert fetch" "CA /ca/info returned empty — is profile-ca running?"
fi
echo "$_ca_pem" > "$CA_FILE"
pass "PKI step 1: CA cert fetched from /ca/info"

# Step 2: Get onboarding cert (plain HTTP, no auth)
_on_resp=$(curl -s -X POST http://localhost:8087/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"lifecycle-test"}')
_on_crt=$(echo "$_on_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
_on_key=$(echo "$_on_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")
_on_profile=$(echo "$_on_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["profile"])' 2>/dev/null || echo "")
if [ -z "$_on_crt" ] || [ -z "$_on_key" ]; then
  smoke_fail "PKI step 2: onboarding cert" "failed to get onboarding cert from bootstrap endpoint"
fi
echo "$_on_crt" > "$ON_CRT"
echo "$_on_key" > "$ON_KEY"
if [ "$_on_profile" = "on" ]; then
  pass "PKI step 2: onboarding cert issued (OU=on)"
else
  fail "PKI step 2: onboarding cert profile" "on" "$_on_profile"
fi

# Step 3: Get device cert (mTLS, presenting onboarding cert)
# Use --resolve to map profile-ca:8088 → 127.0.0.1
_de_resp=$(curl -s -X POST \
  --cert "$ON_CRT" --key "$ON_KEY" \
  --cacert "$CA_FILE" \
  --resolve "profile-ca:8088:127.0.0.1" \
  https://profile-ca:8088/ca/device-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"lifecycle-test"}' 2>/dev/null || echo "")
_de_crt=$(echo "$_de_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
_de_key=$(echo "$_de_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")
_de_profile=$(echo "$_de_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["profile"])' 2>/dev/null || echo "")
if [ -z "$_de_crt" ] || [ -z "$_de_key" ]; then
  smoke_fail "PKI step 3: device cert" "failed to get device cert (presenting onboarding cert to mTLS port 8088)"
fi
echo "$_de_crt" > "$DE_CRT"
echo "$_de_key" > "$DE_KEY"
if [ "$_de_profile" = "de" ]; then
  pass "PKI step 3: device cert issued (OU=de)"
else
  fail "PKI step 3: device cert profile" "de" "$_de_profile"
fi

# Step 4: Get system cert (mTLS, presenting device cert)
_sy_resp=$(curl -s -X POST \
  --cert "$DE_CRT" --key "$DE_KEY" \
  --cacert "$CA_FILE" \
  --resolve "profile-ca:8088:127.0.0.1" \
  https://profile-ca:8088/ca/system-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"lifecycle-test"}' 2>/dev/null || echo "")
_sy_crt=$(echo "$_sy_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
_sy_key=$(echo "$_sy_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")
_sy_profile=$(echo "$_sy_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["profile"])' 2>/dev/null || echo "")
if [ -z "$_sy_crt" ] || [ -z "$_sy_key" ]; then
  smoke_fail "PKI step 4: system cert" "failed to get system cert (presenting device cert to mTLS port 8088)"
fi
echo "$_sy_crt" > "$SY_CRT"
echo "$_sy_key" > "$SY_KEY"
if [ "$_sy_profile" = "sy" ]; then
  pass "PKI step 4: system cert issued (OU=sy)"
else
  fail "PKI step 4: system cert profile" "sy" "$_sy_profile"
fi

# ── Section 3: Profile enforcement (wrong certs rejected) ────────────────────
echo
echo "=== 3. Profile enforcement (wrong certs rejected) ==="

# Try to get device cert using a system cert (should fail with 403)
_bad_de=$(curl -s -o /dev/null -w "%{http_code}" \
  --cert "$SY_CRT" --key "$SY_KEY" \
  --cacert "$CA_FILE" \
  --resolve "profile-ca:8088:127.0.0.1" \
  https://profile-ca:8088/ca/device-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"bypass-attempt"}' 2>/dev/null; echo -n "")
if [ "$_bad_de" = "403" ]; then
  pass "Profile enforcement: system cert rejected for /ca/device-cert (403)"
else
  fail "Profile enforcement: system cert rejected for /ca/device-cert" "403" "$_bad_de"
fi

# Try to get system cert using onboarding cert (skip device step — should fail with 403)
_skip_de=$(curl -s -o /dev/null -w "%{http_code}" \
  --cert "$ON_CRT" --key "$ON_KEY" \
  --cacert "$CA_FILE" \
  --resolve "profile-ca:8088:127.0.0.1" \
  https://profile-ca:8088/ca/system-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"shortcut-attempt"}' 2>/dev/null; echo -n "")
if [ "$_skip_de" = "403" ]; then
  pass "Profile enforcement: onboarding cert rejected for /ca/system-cert (403) — device step not skippable"
else
  fail "Profile enforcement: onboarding cert rejected for /ca/system-cert" "403" "$_skip_de"
fi

# Try to access /ca/device-cert without any client cert (should get 400 or connection error)
_no_cert=$(curl -s -o /dev/null -w "%{http_code}" \
  --cacert "$CA_FILE" \
  --resolve "profile-ca:8088:127.0.0.1" \
  https://profile-ca:8088/ca/device-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"no-cert"}' 2>/dev/null; echo -n "")
if [ "$_no_cert" = "400" ] || [ "$_no_cert" = "000" ] || [ "$_no_cert" = "401" ]; then
  pass "Profile enforcement: /ca/device-cert without client cert rejected (got $_no_cert)"
else
  fail "Profile enforcement: /ca/device-cert without client cert rejected" "400 or 000" "$_no_cert"
fi

# ── Section 4: cert-provisioner — certs written to volume ────────────────────
echo
echo "=== 4. cert-provisioner — certificate volume ==="

kafka_health=$(docker compose ps kafka --format json 2>/dev/null | python3 -c \
  'import sys,json; rows=json.load(sys.stdin); print(rows[0].get("Health","") if isinstance(rows,list) else rows.get("Health",""))' \
  2>/dev/null || echo "")
if [ "$kafka_health" = "healthy" ]; then
  pass "Kafka container healthy (cert-provisioner completed)"
else
  fail "Kafka container healthy (cert-provisioner completed)" "healthy" "${kafka_health:-not healthy}"
fi

smoke_http "kafka-authz healthy (implies cert-provisioner succeeded)" http://localhost:9101/health

# ── Section 5: policy-sync domain check ──────────────────────────────────────
echo
echo "=== 5. policy-sync /status (localhost:9105) ==="

# Wait for policy-sync first sync (up to 30s).
echo "  Waiting for policy-sync first sync (up to 30s)..."
ps_status=""
for i in $(seq 1 6); do
  ps_status=$(curl -s http://localhost:9105/status 2>/dev/null || echo "{}")
  if echo "$ps_status" | grep -q '"synced":true'; then break; fi
  echo "  ... attempt $i/6, sleeping 5s"
  sleep 5
done
if echo "$ps_status" | grep -q '"synced":true'; then
  pass "policy-sync synced=true"
else
  smoke_fail "policy-sync synced=true" "not synced after 30s — check policy-sync container logs"
fi
if echo "$ps_status" | grep -q '"domainExternalId":"arrowhead-exp8"'; then
  pass "policy-sync domainExternalId=arrowhead-exp8"
else
  smoke_fail "policy-sync domainExternalId=arrowhead-exp8" \
    "expected arrowhead-exp8 — AUTHZFORCE_DOMAIN mismatch causes all auth checks to return Deny (EXP-001)"
fi

status=$(http_body http://localhost:9105/status)
echo "  raw: $status"
assert_json_value "synced=true"                         "synced"           "true"           "$status"
assert_json_gt    "version ≥ 1"                         "version"          0                "$status"
assert_json_gt    "grants field ≥ 1"                    "grants"           0                "$status"
assert_json_value "domainExternalId=arrowhead-exp8"     "domainExternalId" "arrowhead-exp8" "$status"

# ── Section 6: AuthzForce server endpoints ────────────────────────────────────
echo
echo "=== 6. AuthzForce server (localhost:8196) ==="

check_eq "GET /health → 200"                 "200" "$(http_code http://localhost:8196/health)"
check_eq "GET /authzforce-ce/health → 200"   "200" "$(http_code http://localhost:8196/authzforce-ce/health)"
check_eq "GET /authzforce-ce/domains → 200"  "200" "$(http_code http://localhost:8196/authzforce-ce/domains)"

# ── Section 7: kafka-authz authorization ──────────────────────────────────────
echo
echo "=== 7. kafka-authz /auth/check (localhost:9101) ==="

body=$(http_body -X POST http://localhost:9101/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"analytics-consumer","service":"telemetry"}')
echo "  analytics-consumer: $body"
assert_json_value "analytics-consumer → Permit (Kafka)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9101/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unknown-consumer","service":"telemetry"}')
echo "  unknown-consumer:   $body"
assert_json_value "unknown-consumer → Deny (Kafka)" "decision" "Deny" "$body"

# ── Section 8: pki-rest-authz /auth/check ─────────────────────────────────────
echo
echo "=== 8. pki-rest-authz /auth/check (HTTP localhost:9109) ==="

body=$(http_body -X POST http://localhost:9109/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"pki-consumer","service":"telemetry-rest"}')
echo "  pki-consumer:   $body"
assert_json_value "pki-consumer → Permit (pki-rest-authz)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9109/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"test-probe","service":"telemetry-rest"}')
echo "  test-probe:      $body"
assert_json_value "test-probe → Permit (pki-rest-authz)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9109/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unauthorized","service":"telemetry-rest"}')
echo "  unauthorized:    $body"
assert_json_value "unauthorized → Deny (pki-rest-authz)" "decision" "Deny" "$body"

# ── Section 9: mTLS — system cert used for pki-rest-authz mTLS ───────────────
echo
echo "=== 9. mTLS — PKI-lifecycle system cert for REST authentication ==="

# Issue a test-probe system cert via full PKI lifecycle
PROBE_SY_CRT=/tmp/exp8-probe-sy.crt
PROBE_SY_KEY=/tmp/exp8-probe-sy.key

# Get onboarding cert
_p_on_resp=$(curl -s -X POST http://localhost:8087/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"test-probe"}')
_p_on_crt=$(echo "$_p_on_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
_p_on_key=$(echo "$_p_on_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")
if [ -z "$_p_on_crt" ]; then
  fail "mTLS test: test-probe onboarding cert" "certificate" "empty response"
else
  echo "$_p_on_crt" > /tmp/exp8-probe-on.crt
  echo "$_p_on_key" > /tmp/exp8-probe-on.key

  # Get device cert
  _p_de_resp=$(curl -s -X POST \
    --cert /tmp/exp8-probe-on.crt --key /tmp/exp8-probe-on.key \
    --cacert "$CA_FILE" \
    --resolve "profile-ca:8088:127.0.0.1" \
    https://profile-ca:8088/ca/device-cert \
    -H 'Content-Type: application/json' -d '{"systemName":"test-probe"}' 2>/dev/null || echo "")
  _p_de_crt=$(echo "$_p_de_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
  _p_de_key=$(echo "$_p_de_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")
  if [ -z "$_p_de_crt" ]; then
    fail "mTLS test: test-probe device cert" "certificate" "empty response"
  else
    echo "$_p_de_crt" > /tmp/exp8-probe-de.crt
    echo "$_p_de_key" > /tmp/exp8-probe-de.key

    # Get system cert
    _p_sy_resp=$(curl -s -X POST \
      --cert /tmp/exp8-probe-de.crt --key /tmp/exp8-probe-de.key \
      --cacert "$CA_FILE" \
      --resolve "profile-ca:8088:127.0.0.1" \
      https://profile-ca:8088/ca/system-cert \
      -H 'Content-Type: application/json' -d '{"systemName":"test-probe"}' 2>/dev/null || echo "")
    _p_sy_crt=$(echo "$_p_sy_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
    _p_sy_key=$(echo "$_p_sy_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")
    if [ -z "$_p_sy_crt" ]; then
      fail "mTLS test: test-probe system cert" "certificate" "empty response"
    else
      echo "$_p_sy_crt" > "$PROBE_SY_CRT"
      echo "$_p_sy_key" > "$PROBE_SY_KEY"
      pass "mTLS test: test-probe PKI lifecycle (on → de → sy) completed"

      # Use system cert to access pki-rest-authz mTLS port
      mtls_code="000"
      for i in $(seq 1 12); do
        mtls_code=$(curl -s -o /dev/null -w "%{http_code}" \
          --cert "$PROBE_SY_CRT" \
          --key  "$PROBE_SY_KEY" \
          --cacert "$CA_FILE" \
          --resolve "pki-rest-authz:9108:127.0.0.1" \
          https://pki-rest-authz:9108/telemetry/latest 2>/dev/null; echo -n "")
        if [ "$mtls_code" = "200" ] || [ "$mtls_code" = "404" ]; then
          break
        fi
        echo "  waiting for data-provider-tls data... (attempt $i/12, HTTP $mtls_code)"
        sleep 5
      done
      check_eq "mTLS test-probe GET /telemetry/latest → 200 (authorized)" "200" "$mtls_code"

      # Request WITHOUT client cert → 400 or connection error
      no_cert_code=$(curl -s -o /dev/null -w "%{http_code}" \
        --cacert "$CA_FILE" \
        --resolve "pki-rest-authz:9108:127.0.0.1" \
        https://pki-rest-authz:9108/telemetry/latest 2>/dev/null; echo -n "")
      if [ "$no_cert_code" = "400" ] || [ "$no_cert_code" = "000" ] || [ "$no_cert_code" = "401" ]; then
        pass "mTLS: request without client cert rejected (got $no_cert_code)"
      else
        fail "mTLS: request without client cert rejected" "400 or connection error" "$no_cert_code"
      fi
    fi
  fi
fi

# ── Section 10: pki-consumer message count ────────────────────────────────────
echo
echo "=== 10. pki-consumer /stats (localhost:9107) ==="

rcstats=""
count=0
for i in $(seq 1 12); do
  rcstats=$(http_body http://localhost:9107/stats 2>/dev/null || echo "{}")
  count=$(echo "$rcstats" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  if [ "${count:-0}" -gt 0 ]; then
    break
  fi
  echo "  waiting for pki-consumer to accumulate messages... (attempt $i/12, sleeping 5s)"
  sleep 5
done
echo "  raw: $rcstats"

assert_json_gt    "pki-consumer msgCount > 0"                            "msgCount"     0  "$rcstats"
assert_json_value "pki-consumer transport=rest-mtls-pki"                 "transport"    "rest-mtls-pki" "$rcstats"
assert_json_value "pki-consumer lastDeniedAt is empty (never denied)"    "lastDeniedAt" ""          "$rcstats"

# ── Section 11: analytics-consumer (Kafka path) ───────────────────────────────
echo
echo "=== 11. analytics-consumer /stats (localhost:9014) ==="

stats=""
ac_count=0
for i in $(seq 1 12); do
  stats=$(http_body http://localhost:9014/stats 2>/dev/null || echo "{}")
  ac_count=$(echo "$stats" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  if [ "${ac_count:-0}" -gt 0 ]; then
    break
  fi
  echo "  waiting for analytics-consumer... (attempt $i/12, sleeping 5s)"
  sleep 5
done
echo "  raw: $stats"
assert_json_gt "analytics-consumer msgCount > 0" "msgCount" 0 "$stats"

# ── Section 12: pki-rest-authz status counters ────────────────────────────────
echo
echo "=== 12. pki-rest-authz /status (HTTP localhost:9109) ==="

ra_status=$(http_body http://localhost:9109/status)
echo "  raw: $ra_status"
assert_json_field "pki-rest-authz requestsTotal field" "requestsTotal" "$ra_status"
assert_json_field "pki-rest-authz permitted field"     "permitted"     "$ra_status"
assert_json_field "pki-rest-authz denied field"        "denied"        "$ra_status"

# ── Section 13: Revocation propagation ───────────────────────────────────────
echo
echo "=== 13. Revocation sync-delay: pki-consumer denied after SYNC_INTERVAL ==="

# Get a client cert for accessing ConsumerAuth TLS port
if [ -f "$PROBE_SY_CRT" ] && [ -f "$PROBE_SY_KEY" ]; then
  lookup=$(curl -s \
    --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
    --cacert "$CA_FILE" \
    --resolve "consumerauth:8492:127.0.0.1" \
    https://consumerauth:8492/authorization/lookup)
  pki_grant_id=$(echo "$lookup" \
    | grep -oE '"id":[0-9]+[^}]*"consumerSystemName":"pki-consumer"' \
    | grep -oE '"id":[0-9]+' | grep -oE '[0-9]+' | head -1)

  if [ -z "$pki_grant_id" ]; then
    fail "pki-consumer grant exists in ConsumerAuth" "grant with id" "not found"
    echo "  Skipping revocation test."
  else
    pass "pki-consumer grant exists in ConsumerAuth (id=$pki_grant_id)"

    permit_before=$(http_body -X POST http://localhost:9109/auth/check \
      -H 'Content-Type: application/json' \
      -d '{"consumer":"pki-consumer","service":"telemetry-rest"}')
    assert_json_value "pki-consumer → Permit before revocation" "decision" "Permit" "$permit_before"

    revoke_code=$(curl -s -o /dev/null -w "%{http_code}" \
      -X DELETE \
      --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
      --cacert "$CA_FILE" \
      --resolve "consumerauth:8492:127.0.0.1" \
      "https://consumerauth:8492/authorization/revoke/$pki_grant_id"; echo -n "")
    check_eq "revoke pki-consumer grant via TLS port 8492 → 200" "200" "$revoke_code"

    echo "  waiting 30 s for policy-sync cycle to propagate revocation..."
    sleep 30

    deny_body=$(http_body -X POST http://localhost:9109/auth/check \
      -H 'Content-Type: application/json' \
      -d '{"consumer":"pki-consumer","service":"telemetry-rest"}')
    echo "  pki-consumer AuthzForce after revoke: $deny_body"
    assert_json_value "pki-consumer → Deny in AuthzForce after revocation" "decision" "Deny" "$deny_body"

    regrant=$(curl -s \
      -X POST \
      --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
      --cacert "$CA_FILE" \
      --resolve "consumerauth:8492:127.0.0.1" \
      https://consumerauth:8492/authorization/grant \
      -H 'Content-Type: application/json' \
      -d '{"consumerSystemName":"pki-consumer","providerSystemName":"data-provider-tls","serviceDefinition":"telemetry-rest"}')
    if [[ "$regrant" == *'"id":'* ]] || [[ "$regrant" == *"already exists"* ]]; then
      pass "pki-consumer grant restored via TLS port 8492"
    else
      fail "pki-consumer grant restored" '"id":N or already exists' "$regrant"
    fi

    echo "  waiting 15 s for grant restoration to propagate..."
    sleep 15

    permit_after=$(http_body -X POST http://localhost:9109/auth/check \
      -H 'Content-Type: application/json' \
      -d '{"consumer":"pki-consumer","service":"telemetry-rest"}')
    assert_json_value "pki-consumer → Permit again after grant restored" "decision" "Permit" "$permit_after"
  fi
else
  fail "Revocation test" "PROBE_SY_CRT available" "PKI lifecycle test failed — skipping section 13"
fi

# ── Section 14: kafka-authz SSE stream ───────────────────────────────────────
echo
echo "=== 14. kafka-authz SSE stream (test-probe consumer) ==="

sse=$(timeout 4 curl -sN \
  "http://localhost:9101/stream/test-probe?service=telemetry" 2>/dev/null || true)
preview="${sse:0:300}"
echo "  first 300 chars: $preview"

assert_not_contains "SSE test-probe: not denied (403)" "not authorized" "$sse"
assert_contains     "SSE test-probe: data lines received from Kafka" "data: {" "$sse"

# ── Section 15: G4 closure — core service mTLS ports ─────────────────────────
echo
echo "=== 15. G4 closure — core service mTLS ports (localhost 8490-8493) ==="

if [ -f "$PROBE_SY_CRT" ] && [ -f "$PROBE_SY_KEY" ]; then
  code=$(curl -s -o /dev/null -w "%{http_code}" \
    --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
    --cacert "$CA_FILE" \
    --resolve "serviceregistry:8490:127.0.0.1" \
    https://serviceregistry:8490/health 2>/dev/null; echo -n "")
  check_eq "G4: ServiceRegistry TLS port /health (mTLS) → 200" "200" "$code"

  code=$(curl -s -o /dev/null -w "%{http_code}" \
    --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
    --cacert "$CA_FILE" \
    --resolve "consumerauth:8492:127.0.0.1" \
    https://consumerauth:8492/health 2>/dev/null; echo -n "")
  check_eq "G4: ConsumerAuthorization TLS port /health (mTLS) → 200" "200" "$code"

  code=$(curl -s -o /dev/null -w "%{http_code}" \
    --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
    --cacert "$CA_FILE" \
    --resolve "authentication:8491:127.0.0.1" \
    https://authentication:8491/health 2>/dev/null; echo -n "")
  check_eq "G4: Authentication TLS port /health (mTLS) → 200" "200" "$code"

  code=$(curl -s -o /dev/null -w "%{http_code}" \
    --cert "$PROBE_SY_CRT" --key "$PROBE_SY_KEY" \
    --cacert "$CA_FILE" \
    --resolve "dynamicorch:8493:127.0.0.1" \
    https://dynamicorch:8493/health 2>/dev/null; echo -n "")
  check_eq "G4: DynamicOrchestration TLS port /health (mTLS) → 200" "200" "$code"
else
  fail "G4 mTLS tests" "PROBE_SY_CRT available" "PKI lifecycle test failed — skipping section 15"
fi

# ── Section 16: Core plain HTTP ports NOT accessible from host ────────────────
echo
echo "=== 16. Security: core plain HTTP ports not exposed to host ==="

for _plain_port in 8080 8081 8082 8083; do
  _plain_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 2 \
    http://localhost:$_plain_port/health 2>/dev/null; echo -n "")
  if [ "$_plain_code" = "000" ]; then
    pass "Core port $_plain_port not accessible from host (connection refused)"
  else
    fail "Core port $_plain_port not accessible from host" \
      "000 (connection refused)" \
      "HTTP $_plain_code — remove the host port binding from docker-compose.yml"
  fi
done

# ── Cleanup ───────────────────────────────────────────────────────────────────
rm -f "$CA_FILE" "$ON_CRT" "$ON_KEY" "$DE_CRT" "$DE_KEY" "$SY_CRT" "$SY_KEY" \
      "$PROBE_SY_CRT" "$PROBE_SY_KEY" \
      /tmp/exp8-probe-on.crt /tmp/exp8-probe-on.key \
      /tmp/exp8-probe-de.crt /tmp/exp8-probe-de.key 2>/dev/null || true

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
