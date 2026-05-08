#!/usr/bin/env bash
# test-system.sh — system test for experiment-7 running under Docker.
#
# Run from experiments/experiment-7/ with the stack already up:
#
#   cd experiments/experiment-7
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

smoke_http "CA /health"                     http://localhost:8086/health
smoke_http "AuthzForce /health"             http://localhost:8186/health
smoke_http "kafka-authz /health"            http://localhost:9091/health
smoke_http "cert-rest-authz /health (HTTP)" http://localhost:9099/health

# ── Shared TLS cert setup ─────────────────────────────────────────────────────
# Issued once here; reused by sections 7, 11, 13, and 14.
# Core system plain HTTP ports (8080-8083) are NOT exposed to the host —
# all core service access from this test script goes through TLS ports (8480-8483).
SHARED_CA_FILE=/tmp/exp7-shared-ca.crt
SHARED_PROBE_CRT=/tmp/exp7-shared-probe.crt
SHARED_PROBE_KEY=/tmp/exp7-shared-probe.key
SHARED_CERTS_OK=false

_shared_ca_pem=$(curl -s http://localhost:8086/ca/info \
  | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
if [ -z "$_shared_ca_pem" ]; then
  smoke_fail "Shared TLS cert setup (CA cert fetch)" "CA /ca/info returned empty — is the CA container running?"
fi
echo "$_shared_ca_pem" > "$SHARED_CA_FILE"

_shared_probe_resp=$(curl -s -X POST http://localhost:8086/ca/certificate/issue \
  -H 'Content-Type: application/json' -d '{"systemName":"test-probe"}')
_shared_probe_crt=$(echo "$_shared_probe_resp" | python3 -c \
  'import sys,json; d=json.load(sys.stdin); print(d["certificate"])' 2>/dev/null || echo "")
_shared_probe_key=$(echo "$_shared_probe_resp" | python3 -c \
  'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])' 2>/dev/null || echo "")
if [ -z "$_shared_probe_crt" ] || [ -z "$_shared_probe_key" ]; then
  smoke_fail "Shared TLS cert setup (test-probe cert issue)" "failed to parse cert/key from CA response"
fi
echo "$_shared_probe_crt" > "$SHARED_PROBE_CRT"
echo "$_shared_probe_key" > "$SHARED_PROBE_KEY"
SHARED_CERTS_OK=true
pass "Shared TLS test-probe cert issued (reused for all mTLS checks)"

# ConsumerAuth health check via TLS port 8482 (plain HTTP port 8082 not host-exposed).
_ca_health=$(curl -s -o /dev/null -w "%{http_code}" \
  --cert "$SHARED_PROBE_CRT" --key "$SHARED_PROBE_KEY" \
  --cacert "$SHARED_CA_FILE" \
  --resolve "consumerauth:8482:127.0.0.1" \
  https://consumerauth:8482/health 2>/dev/null; echo -n "")
if [ "$_ca_health" = "200" ]; then
  pass "ConsumerAuth TLS /health → 200 (port 8482)"
else
  smoke_fail "ConsumerAuth TLS /health → 200 (port 8482)" \
    "HTTP $_ca_health — is consumerauth container running with TLS_PORT=8482?"
fi

# policy-sync must be synced to arrowhead-exp7 domain (EXP-001).
echo "  Waiting for policy-sync first sync (up to 30s)..."
ps_status=""
for i in $(seq 1 6); do
  ps_status=$(curl -s http://localhost:9095/status 2>/dev/null || echo "{}")
  if echo "$ps_status" | grep -q '"synced":true'; then break; fi
  echo "  ... attempt $i/6, sleeping 5s"
  sleep 5
done
if echo "$ps_status" | grep -q '"synced":true'; then
  pass "policy-sync synced=true"
else
  smoke_fail "policy-sync synced=true" "not synced after 30s — check policy-sync container logs"
fi
if echo "$ps_status" | grep -q '"domainExternalId":"arrowhead-exp7"'; then
  pass "policy-sync domainExternalId=arrowhead-exp7"
else
  smoke_fail "policy-sync domainExternalId=arrowhead-exp7" \
    "expected arrowhead-exp7 — AUTHZFORCE_DOMAIN mismatch causes all auth checks to return Deny (see EXP-001)"
fi

# ── Section 1: CertificateAuthority endpoints ─────────────────────────────────
echo
echo "=== 1. CertificateAuthority (localhost:8086) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:8086/health)"
check_eq "GET /ca/health → 200" "200" "$(http_code http://localhost:8086/ca/health)"

ca_info=$(http_body http://localhost:8086/ca/info)
echo "  /ca/info: ${ca_info:0:120}..."
assert_json_field "GET /ca/info → commonName field present" "commonName" "$ca_info"
assert_json_field "GET /ca/info → certificate field present" "certificate" "$ca_info"
assert_contains   "GET /ca/info → certificate is PEM" "BEGIN CERTIFICATE" "$ca_info"

# Issue a test certificate to verify CA is functional.
issue_resp=$(http_body -X POST http://localhost:8086/ca/certificate/issue \
  -H 'Content-Type: application/json' \
  -d '{"systemName":"test-probe"}')
echo "  issue test-probe cert: ${issue_resp:0:120}..."
assert_json_field "POST /ca/certificate/issue → certificate field" "certificate" "$issue_resp"
assert_json_field "POST /ca/certificate/issue → privateKey field"  "privateKey"  "$issue_resp"
assert_contains   "POST /ca/certificate/issue → cert is PEM" "BEGIN CERTIFICATE" "$issue_resp"
assert_contains   "POST /ca/certificate/issue → key is PEM"  "BEGIN"              "$issue_resp"

# Verify the issued certificate.
cert_pem=$(echo "$issue_resp" | grep -o '"certificate":"[^"]*"' | sed 's/"certificate":"//;s/"$//' | sed 's/\\n/\n/g')
verify_resp=$(http_body -X POST http://localhost:8086/ca/certificate/verify \
  -H 'Content-Type: application/json' \
  --data-binary "{\"certificate\":$(echo "$issue_resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(json.dumps(d["certificate"]))')}")
echo "  verify test-probe cert: $verify_resp"
assert_json_value "POST /ca/certificate/verify → valid=true" "valid" "true" "$verify_resp"
assert_json_value "POST /ca/certificate/verify → systemName=test-probe" "systemName" "test-probe" "$verify_resp"

# ── Section 2: cert-provisioner — certs written to volume ─────────────────────
echo
echo "=== 2. cert-provisioner — certificate volume ==="

# Verify cert-provisioner completed (kafka and rabbitmq are healthy as a proxy).
# Kafka uses SSL on 9092 — not HTTP-reachable from host. Verify via container health status.
kafka_health=$(docker compose ps kafka --format json 2>/dev/null | python3 -c \
  'import sys,json; rows=json.load(sys.stdin); print(rows[0].get("Health","") if isinstance(rows,list) else rows.get("Health",""))' \
  2>/dev/null || echo "")
if [ "$kafka_health" = "healthy" ]; then
  pass "Kafka container healthy (cert-provisioner completed)"
else
  fail "Kafka container healthy (cert-provisioner completed)" "healthy" "${kafka_health:-not healthy}"
fi

smoke_http "kafka-authz healthy (implies cert-provisioner succeeded)" http://localhost:9091/health

# ── Section 3: AuthzForce server endpoints ────────────────────────────────────
echo
echo "=== 3. AuthzForce server (localhost:8186) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:8186/health)"
check_eq "GET /authzforce-ce/health → 200" "200" "$(http_code http://localhost:8186/authzforce-ce/health)"
check_eq "GET /authzforce-ce/domains → 200" "200" "$(http_code http://localhost:8186/authzforce-ce/domains)"

# ── Section 4: policy-sync /status ───────────────────────────────────────────
echo
echo "=== 4. policy-sync /status (localhost:9095) ==="

status=$(http_body http://localhost:9095/status)
echo "  raw: $status"

assert_json_value "synced=true"                        "synced"           "true"           "$status"
assert_json_gt    "version ≥ 1"                        "version"          0                "$status"
assert_json_gt    "grants field ≥ 1"                   "grants"           0                "$status"
assert_json_value "domainExternalId=arrowhead-exp7"    "domainExternalId" "arrowhead-exp7" "$status"

# ── Section 5: kafka-authz authorization ──────────────────────────────────────
echo
echo "=== 5. kafka-authz /auth/check (localhost:9091) ==="

body=$(http_body -X POST http://localhost:9091/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"analytics-consumer","service":"telemetry"}')
echo "  analytics-consumer: $body"
assert_json_value "analytics-consumer → Permit (Kafka)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9091/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unknown-consumer","service":"telemetry"}')
echo "  unknown-consumer:   $body"
assert_json_value "unknown-consumer → Deny (Kafka)" "decision" "Deny" "$body"

# ── Section 6: cert-rest-authz /auth/check ───────────────────────────────────
echo
echo "=== 6. cert-rest-authz /auth/check (HTTP localhost:9099) ==="

body=$(http_body -X POST http://localhost:9099/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"cert-consumer","service":"telemetry-rest"}')
echo "  cert-consumer:   $body"
assert_json_value "cert-consumer → Permit (cert-rest-authz)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9099/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"test-probe","service":"telemetry-rest"}')
echo "  test-probe:      $body"
assert_json_value "test-probe → Permit (cert-rest-authz)" "decision" "Permit" "$body"

body=$(http_body -X POST http://localhost:9099/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unauthorized","service":"telemetry-rest"}')
echo "  unauthorized:    $body"
assert_json_value "unauthorized → Deny (cert-rest-authz)" "decision" "Deny" "$body"

# ── Section 7: mTLS — cert-consumer uses client certificate ──────────────────
echo
echo "=== 7. mTLS — cert-based REST authentication ==="

# Use shared certs set up in pre-flight (no re-issuance needed).
if [ "$SHARED_CERTS_OK" != "true" ]; then
  fail "mTLS test" "shared certs available" "cert setup failed in pre-flight — skipping section 7"
else
  # Use client cert to access cert-rest-authz mTLS port.
  # cert-rest-authz has SAN=cert-rest-authz (not localhost). Use --resolve so
  # curl maps cert-rest-authz:9098 → 127.0.0.1 without DNS, letting TLS hostname
  # verification use the correct name from the server certificate.
  # Wait for data to be available (up to 60s).
  mtls_code="000"
  for i in $(seq 1 12); do
    mtls_code=$(curl -s -o /dev/null -w "%{http_code}" \
      --cert "$SHARED_PROBE_CRT" \
      --key  "$SHARED_PROBE_KEY" \
      --cacert "$SHARED_CA_FILE" \
      --resolve "cert-rest-authz:9098:127.0.0.1" \
      https://cert-rest-authz:9098/telemetry/latest 2>/dev/null || echo "000")
    if [ "$mtls_code" = "200" ] || [ "$mtls_code" = "404" ]; then
      break
    fi
    echo "  waiting for data-provider-tls data... (attempt $i/12, HTTP $mtls_code)"
    sleep 5
  done
  check_eq "mTLS test-probe GET /telemetry/latest → 200 (authorized)" "200" "$mtls_code"

  # Request WITHOUT client cert → 400 or connection error (server requires client cert).
  no_cert_code=$(curl -s -o /dev/null -w "%{http_code}" \
    --cacert "$SHARED_CA_FILE" \
    --resolve "cert-rest-authz:9098:127.0.0.1" \
    https://cert-rest-authz:9098/telemetry/latest 2>/dev/null; echo -n "")
  if [ "$no_cert_code" = "400" ] || [ "$no_cert_code" = "000" ] || [ "$no_cert_code" = "401" ]; then
    pass "mTLS: request without client cert rejected (got $no_cert_code)"
  else
    fail "mTLS: request without client cert rejected" "400 or connection error" "$no_cert_code"
  fi
fi

# ── Section 8: cert-consumer message count ────────────────────────────────────
echo
echo "=== 8. cert-consumer /stats (localhost:9096) ==="

rcstats=""
count=0
for i in $(seq 1 12); do
  rcstats=$(http_body http://localhost:9096/stats 2>/dev/null || echo "{}")
  count=$(echo "$rcstats" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  if [ "${count:-0}" -gt 0 ]; then
    break
  fi
  echo "  waiting for cert-consumer to accumulate messages... (attempt $i/12, sleeping 5s)"
  sleep 5
done
echo "  raw: $rcstats"

assert_json_gt    "cert-consumer msgCount > 0"                             "msgCount"     0  "$rcstats"
assert_json_value "cert-consumer transport=rest-mtls"                      "transport"    "rest-mtls" "$rcstats"
assert_json_value "cert-consumer lastDeniedAt is empty (never denied)"     "lastDeniedAt" ""          "$rcstats"

# ── Section 9: analytics-consumer (Kafka path) ───────────────────────────────
echo
echo "=== 9. analytics-consumer /stats (localhost:9004) ==="

stats=""
ac_count=0
for i in $(seq 1 12); do
  stats=$(http_body http://localhost:9004/stats 2>/dev/null || echo "{}")
  ac_count=$(echo "$stats" | grep -oE '"msgCount":[0-9]+' | grep -oE '[0-9]+' || echo "0")
  if [ "${ac_count:-0}" -gt 0 ]; then
    break
  fi
  echo "  waiting for analytics-consumer... (attempt $i/12, sleeping 5s)"
  sleep 5
done
echo "  raw: $stats"
assert_json_gt "analytics-consumer msgCount > 0" "msgCount" 0 "$stats"

# ── Section 10: cert-rest-authz status counters ───────────────────────────────
echo
echo "=== 10. cert-rest-authz /status (HTTP localhost:9099) ==="

ra_status=$(http_body http://localhost:9099/status)
echo "  raw: $ra_status"
assert_json_field "cert-rest-authz requestsTotal field" "requestsTotal" "$ra_status"
assert_json_field "cert-rest-authz permitted field"     "permitted"     "$ra_status"
assert_json_field "cert-rest-authz denied field"        "denied"        "$ra_status"

# ── Section 11: Revocation propagation (cert consumer) ───────────────────────
echo
echo "=== 11. Revocation sync-delay: cert-consumer denied after SYNC_INTERVAL ==="
# ConsumerAuthorization plain HTTP port (8082) is not host-exposed.
# All admin API calls go through the mTLS port (8482) using the shared test-probe cert.

if [ "$SHARED_CERTS_OK" != "true" ]; then
  fail "Revocation test" "shared certs available" "cert setup failed in pre-flight — skipping section 11"
else
  lookup=$(curl -s \
    --cert "$SHARED_PROBE_CRT" --key "$SHARED_PROBE_KEY" \
    --cacert "$SHARED_CA_FILE" \
    --resolve "consumerauth:8482:127.0.0.1" \
    https://consumerauth:8482/authorization/lookup)
  cc_grant_id=$(echo "$lookup" \
    | grep -oE '"id":[0-9]+[^}]*"consumerSystemName":"cert-consumer"' \
    | grep -oE '"id":[0-9]+' | grep -oE '[0-9]+' | head -1)

  if [ -z "$cc_grant_id" ]; then
    fail "cert-consumer grant exists in ConsumerAuth" "grant with id" "not found"
    echo "  Skipping revocation test."
  else
    pass "cert-consumer grant exists in ConsumerAuth (id=$cc_grant_id)"

    permit_before=$(http_body -X POST http://localhost:9099/auth/check \
      -H 'Content-Type: application/json' \
      -d '{"consumer":"cert-consumer","service":"telemetry-rest"}')
    assert_json_value "cert-consumer → Permit before revocation" "decision" "Permit" "$permit_before"

    revoke_code=$(curl -s -o /dev/null -w "%{http_code}" \
      -X DELETE \
      --cert "$SHARED_PROBE_CRT" --key "$SHARED_PROBE_KEY" \
      --cacert "$SHARED_CA_FILE" \
      --resolve "consumerauth:8482:127.0.0.1" \
      "https://consumerauth:8482/authorization/revoke/$cc_grant_id"; echo -n "")
    check_eq "revoke cert-consumer grant via TLS port 8482 → 200" "200" "$revoke_code"

    echo "  waiting 30 s for policy-sync cycle to propagate revocation..."
    sleep 30

    deny_body=$(http_body -X POST http://localhost:9099/auth/check \
      -H 'Content-Type: application/json' \
      -d '{"consumer":"cert-consumer","service":"telemetry-rest"}')
    echo "  cert-consumer AuthzForce after revoke: $deny_body"
    assert_json_value "cert-consumer → Deny in AuthzForce after revocation" "decision" "Deny" "$deny_body"

    regrant=$(curl -s \
      -X POST \
      --cert "$SHARED_PROBE_CRT" --key "$SHARED_PROBE_KEY" \
      --cacert "$SHARED_CA_FILE" \
      --resolve "consumerauth:8482:127.0.0.1" \
      https://consumerauth:8482/authorization/grant \
      -H 'Content-Type: application/json' \
      -d '{"consumerSystemName":"cert-consumer","providerSystemName":"data-provider-tls","serviceDefinition":"telemetry-rest"}')
    if [[ "$regrant" == *'"id":'* ]] || [[ "$regrant" == *"already exists"* ]]; then
      pass "cert-consumer grant restored via TLS port 8482"
    else
      fail "cert-consumer grant restored" '"id":N or already exists' "$regrant"
    fi

    echo "  waiting 15 s for grant restoration to propagate..."
    sleep 15

    permit_after=$(http_body -X POST http://localhost:9099/auth/check \
      -H 'Content-Type: application/json' \
      -d '{"consumer":"cert-consumer","service":"telemetry-rest"}')
    assert_json_value "cert-consumer → Permit again after grant restored" "decision" "Permit" "$permit_after"
  fi
fi

# ── Section 12: kafka-authz SSE stream (test-probe) ──────────────────────────
echo
echo "=== 12. kafka-authz SSE stream (test-probe consumer) ==="

sse=$(timeout 4 curl -sN \
  "http://localhost:9091/stream/test-probe?service=telemetry" 2>/dev/null || true)
preview="${sse:0:300}"
echo "  first 300 chars: $preview"

assert_not_contains "SSE test-probe: not denied (403)" "not authorized" "$sse"
assert_contains     "SSE test-probe: data lines received from Kafka" "data: {" "$sse"

# ── Section 13: G4 closure — core service mTLS ports ─────────────────────────
echo
echo "=== 13. G4 closure — core service mTLS ports (localhost 8480-8483) ==="
# Uses shared test-probe cert set up in pre-flight.

if [ "$SHARED_CERTS_OK" != "true" ]; then
  fail "G4 mTLS tests" "shared certs available" "cert setup failed in pre-flight — skipping section 13"
else
  # ServiceRegistry TLS port (8480) — mTLS client cert → 200.
  code=$(curl -s -o /dev/null -w "%{http_code}" \
    --cert "$SHARED_PROBE_CRT" --key "$SHARED_PROBE_KEY" \
    --cacert "$SHARED_CA_FILE" \
    --resolve "serviceregistry:8480:127.0.0.1" \
    https://serviceregistry:8480/health 2>/dev/null; echo -n "")
  check_eq "G4: ServiceRegistry TLS port /health (mTLS) → 200" "200" "$code"

  # ServiceRegistry TLS port — no client cert → rejected (000 or 400).
  nocc=$(curl -s -o /dev/null -w "%{http_code}" \
    --cacert "$SHARED_CA_FILE" \
    --resolve "serviceregistry:8480:127.0.0.1" \
    https://serviceregistry:8480/health 2>/dev/null; echo -n "")
  if [ "$nocc" = "000" ] || [ "$nocc" = "400" ] || [ "$nocc" = "403" ]; then
    pass "G4: ServiceRegistry TLS port rejects request without client cert (got $nocc)"
  else
    fail "G4: ServiceRegistry TLS port rejects request without client cert" "000 or 400" "$nocc"
  fi

  # ConsumerAuthorization TLS port (8482) — mTLS client cert → 200.
  code=$(curl -s -o /dev/null -w "%{http_code}" \
    --cert "$SHARED_PROBE_CRT" --key "$SHARED_PROBE_KEY" \
    --cacert "$SHARED_CA_FILE" \
    --resolve "consumerauth:8482:127.0.0.1" \
    https://consumerauth:8482/health 2>/dev/null; echo -n "")
  check_eq "G4: ConsumerAuthorization TLS port /health (mTLS) → 200" "200" "$code"

  # Authentication TLS port (8481) — mTLS client cert → 200.
  code=$(curl -s -o /dev/null -w "%{http_code}" \
    --cert "$SHARED_PROBE_CRT" --key "$SHARED_PROBE_KEY" \
    --cacert "$SHARED_CA_FILE" \
    --resolve "authentication:8481:127.0.0.1" \
    https://authentication:8481/health 2>/dev/null; echo -n "")
  check_eq "G4: Authentication TLS port /health (mTLS) → 200" "200" "$code"

  # DynamicOrchestration TLS port (8483) — mTLS client cert → 200.
  code=$(curl -s -o /dev/null -w "%{http_code}" \
    --cert "$SHARED_PROBE_CRT" --key "$SHARED_PROBE_KEY" \
    --cacert "$SHARED_CA_FILE" \
    --resolve "dynamicorch:8483:127.0.0.1" \
    https://dynamicorch:8483/health 2>/dev/null; echo -n "")
  check_eq "G4: DynamicOrchestration TLS port /health (mTLS) → 200" "200" "$code"
fi

# ── Section 14: Core plain HTTP ports NOT accessible from host ────────────────
echo
echo "=== 14. Security: core plain HTTP ports not exposed to host ==="
# Plain HTTP ports (8080-8083) are Docker-internal only (no host port binding).
# A connection refused (curl code 000) or no-route confirms they are not reachable.
# If any of these return an HTTP response, the port binding is incorrectly exposed.

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

# ── Cleanup shared TLS cert files ────────────────────────────────────────────
rm -f "$SHARED_CA_FILE" "$SHARED_PROBE_CRT" "$SHARED_PROBE_KEY" 2>/dev/null || true

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
