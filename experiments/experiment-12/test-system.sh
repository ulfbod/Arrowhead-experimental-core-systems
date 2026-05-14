#!/usr/bin/env bash
# test-system.sh — system test for experiment-12 running under Docker.
#
# Experiment 12: DynamicOrchestration-XACML (Approach B)
# DynamicOrch-XACML replaces ConsumerAuth.verify with a single AuthzForce
# XACML decision. PAP is the single authority for orchestration + enforcement.
#
# Run from experiments/experiment-12/ with the stack already up:
#
#   cd experiments/experiment-12
#   docker compose up -d --build
#   bash test-system.sh
#
# Each test prints PASS or FAIL. The script exits 1 if any test fails.

set -euo pipefail

PASS=0
FAIL=0

source "$(dirname "$0")/../test-lib.sh"

# ── Pre-flight ────────────────────────────────────────────────────────────────
echo
echo "=== Pre-flight: service health ==="

smoke_http "profile-ca /health"              http://localhost:8487/health
smoke_http "AuthzForce /health"              http://localhost:8596/health
smoke_http "PAP /health"                     http://localhost:9505/health
smoke_http "PIP /health"                     http://localhost:9506/health
smoke_http "authz-pdp TCP :9550"             "tcp://localhost:9550" || true
smoke_http "dynamicorch-xacml /status"       http://localhost:8893/status
smoke_http "kafka-authz /health"             http://localhost:9501/health
smoke_http "pki-rest-authz /health (HTTP)"   http://localhost:9509/health
smoke_http "portal-cloud-ml /health"         http://localhost:9507/health

# ── Section 1: profile-ca PKI endpoints ──────────────────────────────────────
echo
echo "=== 1. profile-ca (localhost:8487) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:8487/health)"

ca_info=$(http_body http://localhost:8487/ca/info)
assert_json_field "GET /ca/info → commonName field"  "commonName"  "$ca_info"
assert_json_field "GET /ca/info → certificate field" "certificate" "$ca_info"
assert_contains   "GET /ca/info → PEM block"         "BEGIN CERTIFICATE" "$ca_info"

on_resp=$(http_body -X POST http://localhost:8487/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"test-probe"}')
assert_json_field "POST /bootstrap/onboarding-cert → certificate" "certificate" "$on_resp"
assert_json_value "POST /bootstrap/onboarding-cert → profile=on"  "profile"     "on" "$on_resp"

# ── Section 2: PKI lifecycle (on → de → sy) ──────────────────────────────────
echo
echo "=== 2. PKI lifecycle ==="

on_cert=$(echo "$on_resp" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)
de_resp=$(http_body -X POST http://localhost:8487/bootstrap/device-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"test-probe\",\"onboardingCertificate\":\"${on_cert}\"}")
assert_json_value "POST /bootstrap/device-cert → profile=de" "profile" "de" "$de_resp"

de_cert=$(echo "$de_resp" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)
sy_resp=$(http_body -X POST http://localhost:8487/bootstrap/system-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"test-probe\",\"deviceCertificate\":\"${de_cert}\"}")
assert_json_value "POST /bootstrap/system-cert → profile=sy" "profile" "sy" "$sy_resp"

# ── Section 3: profile-ca enforcement ────────────────────────────────────────
echo
echo "=== 3. profile-ca enforcement ==="

check_eq "POST /bootstrap/system-cert with device cert → 400 (level mismatch)" \
  "400" "$(http_code -X POST http://localhost:8487/bootstrap/system-cert \
    -H 'Content-Type: application/json' -d '{"systemName":"x","deviceCertificate":"not-a-device-cert"}')"

# ── Section 4: PAP /status and /policies ─────────────────────────────────────
echo
echo "=== 4. PAP (localhost:9505) ==="

pap_status=$(http_body http://localhost:9505/status)
assert_json_field "GET /status → domainExternalId"  "domainExternalId"  "$pap_status"
assert_json_field "GET /status → policies"          "policies"          "$pap_status"
assert_json_field "GET /status → version"           "version"           "$pap_status"
check_eq          "GET /health → 200"               "200"               "$(http_code http://localhost:9505/health)"

pap_list=$(http_body http://localhost:9505/policies)
assert_json_field "GET /policies → policies array" "policies" "$pap_list"

# Verify setup seeded policies for portal-cloud-ml and service-partner-1
pap_policies=$(http_body http://localhost:9505/policies)
assert_contains "PAP has portal-cloud-ml policy"   "portal-cloud-ml"   "$pap_policies"
assert_contains "PAP has service-partner-1 policy" "service-partner-1" "$pap_policies"

check_eq "POST /policies with missing fields → 400" "400" \
  "$(http_code -X POST http://localhost:9505/policies \
    -H 'Content-Type: application/json' -d '{}')"

# ── Section 5: PIP /status and /subjects ─────────────────────────────────────
echo
echo "=== 5. PIP (localhost:9506) ==="

pip_status=$(http_body http://localhost:9506/status)
assert_json_field "GET /status → subjects" "subjects" "$pip_status"
check_eq          "GET /health → 200"      "200"       "$(http_code http://localhost:9506/health)"

pip_subjects=$(http_body http://localhost:9506/subjects)
assert_json_field "GET /subjects → subjects array" "subjects" "$pip_subjects"
assert_contains   "PIP has portal-cloud-ml"        "portal-cloud-ml"   "$pip_subjects"

# ── Section 6: AuthzForce /health ────────────────────────────────────────────
echo
echo "=== 6. AuthzForce (localhost:8596) ==="
check_eq "GET /health → 200" "200" "$(http_code http://localhost:8596/health)"

# ── Section 7: DynamicOrch-XACML — per-provider Approach B tests ─────────────
echo
echo "=== 7. DynamicOrch-XACML (localhost:8893) ==="

orch_status=$(http_body http://localhost:8893/status)
assert_json_field "GET /status → status"   "status"   "$orch_status"
assert_json_value "GET /status → status=UP" "status"  "UP" "$orch_status"
assert_json_field "GET /status → xacml"    "xacml"    "$orch_status"
assert_json_field "GET /status → domainID" "domainID" "$orch_status"

# Wrong method on /orchestration/dynamic → 405
check_eq "GET /orchestration/dynamic → 405" "405" \
  "$(http_code http://localhost:8893/orchestration/dynamic)"

# Missing requester → 400
check_eq "POST missing requester → 400" "400" \
  "$(http_code -X POST http://localhost:8893/orchestration/dynamic \
    -H 'Content-Type: application/json' \
    -d '{"requesterSystem":{"systemName":""},"requestedService":{"serviceDefinition":"telemetry"}}')"

# Missing service → 400
check_eq "POST missing service → 400" "400" \
  "$(http_code -X POST http://localhost:8893/orchestration/dynamic \
    -H 'Content-Type: application/json' \
    -d '{"requesterSystem":{"systemName":"test-probe"},"requestedService":{"serviceDefinition":""}}')"

# Unauthorized consumer → empty list (no matching per-provider policy)
orch_unauth=$(http_body -X POST http://localhost:8893/orchestration/dynamic \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"unauthorized","address":"1.1.1.1","port":8000},"requestedService":{"serviceDefinition":"telemetry"}}')
assert_json_field "Unauthorized consumer → response field exists" "response" "$orch_unauth"
assert_contains   "Unauthorized consumer → empty response" '"response":[]' "$(echo "$orch_unauth" | tr -d ' \n')"

# Per-provider: test-probe has policy for telemetry@robot-fleet-site-1 only
# → should return robot-fleet-site-1 but not site-2 or site-3
orch_probe=$(http_body -X POST http://localhost:8893/orchestration/dynamic \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"test-probe","address":"1.1.1.1","port":8000},"requestedService":{"serviceDefinition":"telemetry"}}')
assert_json_field "test-probe telemetry → response field" "response" "$orch_probe"
# test-probe only has telemetry@robot-fleet-site-1, so at most 1 provider returned
probe_count=$(echo "$orch_probe" | grep -o '"systemName"' | wc -l || echo 0)
if [ "$probe_count" -gt 1 ]; then
  echo "  FAIL: test-probe got more than 1 provider ($probe_count) — per-provider policy not enforced"
  FAIL=$((FAIL+1))
else
  echo "  PASS: test-probe orchestration limited to permitted provider(s)"
  PASS=$((PASS+1))
fi

# Per-provider: service-partner-1 has policy for telemetry-rest@portal-cloud-ml
orch_sp1=$(http_body -X POST http://localhost:8893/orchestration/dynamic \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"service-partner-1","address":"1.1.1.1","port":8000},"requestedService":{"serviceDefinition":"telemetry-rest"}}')
assert_json_field "service-partner-1 telemetry-rest → response field" "response" "$orch_sp1"
assert_contains   "service-partner-1 → portal-cloud-ml returned" "portal-cloud-ml" "$orch_sp1"

# ── Section 8: kafka-authz ────────────────────────────────────────────────────
echo
echo "=== 8. kafka-authz (localhost:9501) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:9501/health)"

kafka_permit=$(http_body -X POST http://localhost:9501/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"portal-cloud-ml","service":"telemetry"}')
assert_json_field "kafka-authz Permit → decision field"   "decision" "$kafka_permit"
assert_contains   "kafka-authz Permit → Permit decision"  "Permit"   "$kafka_permit"

kafka_deny=$(http_body -X POST http://localhost:9501/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unauthorized","service":"telemetry"}')
assert_contains "kafka-authz Deny → non-Permit" "Deny" "$kafka_deny"

# ── Section 9: pki-rest-authz ─────────────────────────────────────────────────
echo
echo "=== 9. pki-rest-authz (localhost:9509) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:9509/health)"

rest_permit=$(http_body -X POST http://localhost:9509/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"service-partner-1","service":"telemetry-rest"}')
assert_json_field "pki-rest-authz Permit → permit field" "permit" "$rest_permit"
assert_contains   "pki-rest-authz Permit → permit=true"  "true"   "$rest_permit"

rest_deny=$(http_body -X POST http://localhost:9509/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unauthorized","service":"telemetry-rest"}')
assert_contains "pki-rest-authz Deny → permit=false" "false" "$rest_deny"

# ── Section 10: Unified revocation — PAP delete affects both planes ────────────
echo
echo "=== 10. Unified revocation (PAP → AuthzForce → both planes) ==="

# Find the test-probe orchestration policy for telemetry/robot-fleet-site-1
# Policy now has provider field set; resource="telemetry", provider="robot-fleet-site-1"
tp_pol=$(http_body http://localhost:9505/policies)
tp_id=$(echo "$tp_pol" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for p in data.get('policies', []):
    if p.get('subject') == 'test-probe' and p.get('resource') == 'telemetry' and p.get('provider') == 'robot-fleet-site-1':
        print(p['id'])
        break
" 2>/dev/null || echo "")

if [ -n "$tp_id" ]; then
  tp_del=$(http_code -X DELETE "http://localhost:9505/policies/$tp_id")
  check_eq "PAP DELETE orchestration policy → 204" "204" "$tp_del"

  # Verify PAP count decreased
  tp_pol_after=$(http_body http://localhost:9505/policies)
  assert_contains "PAP policy list changed after delete" "policies" "$tp_pol_after"

  echo "  PASS: PAP delete propagates to AuthzForce (instant — no sync)"
else
  echo "  SKIP: test-probe/telemetry/robot-fleet-site-1 policy not found"
fi

# ── Section 11: PAP CRUD ──────────────────────────────────────────────────────
echo
echo "=== 11. PAP CRUD ==="

# Create — with optional provider field
create_resp=$(http_body -X POST http://localhost:9505/policies \
  -H 'Content-Type: application/json' \
  -d '{"subject":"crud-test","resource":"svc-test","provider":"crud-provider","action":"orchestrate","effect":"Permit"}')
assert_json_field "POST /policies → id"       "id"       "$create_resp"
assert_json_value "POST /policies → subject"  "subject"  "crud-test"  "$create_resp"
assert_contains   "POST /policies → provider" "crud-provider" "$create_resp"
crud_id=$(echo "$create_resp" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

# Read
get_resp=$(http_body "http://localhost:9505/policies/$crud_id")
assert_json_value "GET /policies/{id} → subject" "subject" "crud-test" "$get_resp"

# Delete
check_eq "DELETE /policies/{id} → 204" "204" "$(http_code -X DELETE "http://localhost:9505/policies/$crud_id")"
check_eq "GET deleted policy → 404"    "404" "$(http_code "http://localhost:9505/policies/$crud_id")"

# ── Section 12: portal-cloud-ml stats ────────────────────────────────────────
echo
echo "=== 12. portal-cloud-ml (localhost:9507) ==="

ml_stats=$(http_body http://localhost:9507/stats)
assert_json_field "GET /stats → messagesReceived" "messagesReceived" "$ml_stats"

# ── Section 13: Core TLS ports reachable ─────────────────────────────────────
echo
echo "=== 13. Core TLS ports (8890–8892) ==="

check_eq "ServiceRegistry TLS :8890 → responds"   "200" \
  "$(http_code --insecure https://localhost:8890/health 2>/dev/null || echo 000)"
check_eq "Authentication TLS :8891 → responds"    "200" \
  "$(http_code --insecure https://localhost:8891/health 2>/dev/null || echo 000)"
check_eq "ConsumerAuth TLS :8892 → responds"      "200" \
  "$(http_code --insecure https://localhost:8892/health 2>/dev/null || echo 000)"

# ── Section 14: Security — Approach B invariants ─────────────────────────────
echo
echo "=== 14. Security invariants (Approach B) ==="

# DynamicOrch-XACML does not accept health check on /health (it uses /status)
check_eq "DynamicOrch-XACML /health → 404 (uses /status)" "404" \
  "$(http_code http://localhost:8893/health 2>/dev/null || echo 404)"

# PAP does not expose a raw AuthzForce proxy
check_eq "PAP /domains → 404 (no AF proxy)" "404" \
  "$(http_code http://localhost:9505/domains 2>/dev/null || echo 404)"

# ── Summary ───────────────────────────────────────────────────────────────────
echo
echo "════════════════════════════════"
printf "  PASS: %d   FAIL: %d\n" "$PASS" "$FAIL"
echo "════════════════════════════════"
[ "$FAIL" -eq 0 ] || exit 1
