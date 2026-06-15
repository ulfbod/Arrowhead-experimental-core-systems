#!/usr/bin/env bash
# test-system.sh — system test for experiment-15 running under Docker.
#
# Experiment 15: CA-as-PIP — profile-ca serves PIP endpoints directly.
#
# New in exp-15 vs exp-14:
# - No separate pip service; profile-ca at :8787 serves /pip/attributes/{cn},
#   /pip/subjects, and /pip/status directly from its cert store (D4).
# - Cert issuance immediately creates a PIP entry (no gRPC propagation lag).
# - Cert revocation immediately marks the PIP entry as valid=false.
#
# Run from experiments/experiment-15/ with the stack already up:
#
#   cd experiments/experiment-15
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

smoke_http "profile-ca /health"              http://localhost:8787/health
smoke_http "AuthzForce /health"              http://localhost:8896/health
smoke_http "PAP /health"                     http://localhost:9805/health
smoke_http "dynamicorch-xacml /status"       http://localhost:9193/status
smoke_http "kafka-authz /health"             http://localhost:9801/health
smoke_http "pki-rest-authz /health (HTTP)"   http://localhost:9809/health
smoke_http "portal-cloud-ml /health"         http://localhost:9807/health

# Verify no separate pip service (CA-as-PIP design).
# We expect profile-ca PIP endpoints to be reachable, NOT a separate pip:
# (We do NOT call smoke_http for pip since it doesn't exist in exp-15.)

# ── Section 1: profile-ca HTTP endpoints ─────────────────────────────────────
echo
echo "=== 1. profile-ca (localhost:8787) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:8787/health)"

ca_info=$(http_body http://localhost:8787/ca/info)
assert_json_field "GET /ca/info → commonName field"  "commonName"  "$ca_info"
assert_json_field "GET /ca/info → certificate field" "certificate" "$ca_info"
assert_contains   "GET /ca/info → PEM block"         "BEGIN CERTIFICATE" "$ca_info"

on_resp=$(http_body -X POST http://localhost:8787/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"test-probe"}')
assert_json_field "POST /bootstrap/onboarding-cert → certificate" "certificate" "$on_resp"
assert_json_value "POST /bootstrap/onboarding-cert → profile=on"  "profile"     "on" "$on_resp"

# ── Section 2: PKI lifecycle (on → de → sy) ──────────────────────────────────
echo
echo "=== 2. PKI lifecycle ==="

on_cert=$(echo "$on_resp" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)
de_resp=$(http_body -X POST http://localhost:8787/bootstrap/device-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"test-probe\",\"onboardingCertificate\":\"${on_cert}\"}")
assert_json_value "POST /bootstrap/device-cert → profile=de" "profile" "de" "$de_resp"

de_cert=$(echo "$de_resp" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)
sy_resp=$(http_body -X POST http://localhost:8787/bootstrap/system-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"test-probe\",\"deviceCertificate\":\"${de_cert}\"}")
assert_json_value "POST /bootstrap/system-cert → profile=sy" "profile" "sy" "$sy_resp"

# ── Section 3: CA-as-PIP — cert issuance immediately visible in PIP ──────────
echo
echo "=== 3. CA-as-PIP: issuance immediately visible (D4) ==="

# Issue certs for pip-test system
pip_on=$(http_body -X POST http://localhost:8787/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"pip-test"}')
pip_on_cert=$(echo "$pip_on" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)

pip_de=$(http_body -X POST http://localhost:8787/bootstrap/device-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"pip-test\",\"onboardingCertificate\":\"${pip_on_cert}\"}")
pip_de_cert=$(echo "$pip_de" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)

pip_sy=$(http_body -X POST http://localhost:8787/bootstrap/system-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"pip-test\",\"deviceCertificate\":\"${pip_de_cert}\"}")
assert_json_value "POST system-cert pip-test → profile=sy" "profile" "sy" "$pip_sy"

# NO SLEEP needed — CA-as-PIP: cert store is the PIP, no async propagation
pip_attr=$(http_body http://localhost:8787/pip/attributes/pip-test)
assert_json_field  "CA-as-PIP: pip-test has certLevel" "certLevel" "$pip_attr"
assert_json_value  "CA-as-PIP: pip-test certLevel=sy"  "certLevel" "sy" "$pip_attr"
assert_contains    "CA-as-PIP: pip-test valid=true"    "true"       "$pip_attr"

# ── Section 4: CA-as-PIP — /pip/subjects endpoint ────────────────────────────
echo
echo "=== 4. CA-as-PIP: /pip/subjects includes all records ==="

pip_subjects=$(http_body http://localhost:8787/pip/subjects)
assert_json_field "GET /pip/subjects → subjects field" "subjects"     "$pip_subjects"
assert_contains   "GET /pip/subjects has pip-test"     "pip-test"     "$pip_subjects"
assert_contains   "GET /pip/subjects has portal-cloud-ml" "portal-cloud-ml" "$pip_subjects"

pip_subject_detail=$(http_body http://localhost:8787/pip/subjects/pip-test)
assert_json_field "GET /pip/subjects/pip-test → cn field" "cn" "$pip_subject_detail"

pip_subject_missing_code=$(http_code http://localhost:8787/pip/subjects/no-such-system)
check_eq "GET /pip/subjects/unknown → 404" "404" "$pip_subject_missing_code"

# ── Section 5: CA-as-PIP — /pip/status endpoint ──────────────────────────────
echo
echo "=== 5. CA-as-PIP: /pip/status ==="

pip_status=$(http_body http://localhost:8787/pip/status)
assert_json_field "GET /pip/status → subjectCount field" "subjectCount" "$pip_status"
assert_json_gt    "GET /pip/status → subjectCount > 0"   "subjectCount" 0 "$pip_status"

# ── Section 6: CA-as-PIP — cert revocation immediately visible ───────────────
echo
echo "=== 6. CA-as-PIP: revocation immediately reflected (no gRPC lag) ==="

check_eq "DELETE /ca/certificates/pip-test → 204" "204" \
  "$(http_code -X DELETE http://localhost:8787/ca/certificates/pip-test)"

# NO SLEEP needed — CA-as-PIP: revocation is immediate in the cert store
pip_revoked=$(http_body http://localhost:8787/pip/attributes/pip-test)
assert_json_value "CA-as-PIP: pip-test valid=false after revocation" \
  "valid" "false" "$pip_revoked"

# /pip/subjects still includes revoked record (GetAllRecords includes revoked)
pip_subjects_after=$(http_body http://localhost:8787/pip/subjects)
assert_contains "GET /pip/subjects still has revoked pip-test" "pip-test" "$pip_subjects_after"

# ── Section 7: /pip/attributes for unknown CN → 404 ─────────────────────────
echo
echo "=== 7. CA-as-PIP: unknown CN is fail-closed ==="

pip_unknown_code=$(http_code http://localhost:8787/pip/attributes/nonexistent-system)
check_eq "GET /pip/attributes/nonexistent → 404" "404" "$pip_unknown_code"

# ── Section 8: Connection-time cert revocation enforcement (D2') ─────────────
echo
echo "=== 8. Connection-time revocation enforcement (D2') ==="

# Issue and revoke a test cert for revocation pre-gate tests
revoke_on=$(http_body -X POST http://localhost:8787/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"stream-test"}')
revoke_on_cert=$(echo "$revoke_on" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)

revoke_de=$(http_body -X POST http://localhost:8787/bootstrap/device-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"stream-test\",\"onboardingCertificate\":\"${revoke_on_cert}\"}")
revoke_de_cert=$(echo "$revoke_de" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)

revoke_sy=$(http_body -X POST http://localhost:8787/bootstrap/system-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"stream-test\",\"deviceCertificate\":\"${revoke_de_cert}\"}")
assert_json_value "system-cert stream-test → profile=sy" "profile" "sy" "$revoke_sy"

# Verify PIP shows valid immediately (CA-as-PIP)
stream_attr=$(http_body http://localhost:8787/pip/attributes/stream-test)
assert_json_value "CA-as-PIP: stream-test valid=true before revoke" "valid" "true" "$stream_attr"

# Revoke stream-test
check_eq "DELETE stream-test → 204" "204" \
  "$(http_code -X DELETE http://localhost:8787/ca/certificates/stream-test)"

# PIP should immediately reflect revocation (no gRPC lag)
stream_revoked=$(http_body http://localhost:8787/pip/attributes/stream-test)
assert_json_value "CA-as-PIP: stream-test valid=false immediately after revoke" \
  "valid" "false" "$stream_revoked"

# kafka-authz: revoked cert must be denied
kafka_revoked=$(http_body -X POST http://localhost:9801/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"stream-test","service":"telemetry"}')
assert_contains "kafka-authz stream-test (revoked) → Deny" "Deny" "$kafka_revoked"

# kafka-authz: valid cert must be permitted
kafka_permit=$(http_body -X POST http://localhost:9801/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"portal-cloud-ml","service":"telemetry"}')
assert_json_field "kafka-authz Permit → decision field" "decision" "$kafka_permit"
assert_contains   "kafka-authz Permit → Permit"         "Permit"   "$kafka_permit"

# ── Section 9: PAP status ─────────────────────────────────────────────────────
echo
echo "=== 9. PAP (localhost:9805) ==="

pap_status=$(http_body http://localhost:9805/status)
assert_json_field "GET /status → domainExternalId"  "domainExternalId"  "$pap_status"
assert_json_field "GET /status → policies"          "policies"          "$pap_status"
check_eq          "GET /health → 200"               "200"               "$(http_code http://localhost:9805/health)"

pap_list=$(http_body http://localhost:9805/policies)
assert_json_field "GET /policies → policies array" "policies" "$pap_list"
assert_contains   "PAP has portal-cloud-ml policy" "portal-cloud-ml"   "$pap_list"

check_eq "POST /policies with missing fields → 400" "400" \
  "$(http_code -X POST http://localhost:9805/policies \
    -H 'Content-Type: application/json' -d '{}')"

# ── Section 10: AuthzForce ────────────────────────────────────────────────────
echo
echo "=== 10. AuthzForce (localhost:8896) ==="
check_eq "GET /health → 200" "200" "$(http_code http://localhost:8896/health)"

# ── Section 11: DynamicOrch-XACML ────────────────────────────────────────────
echo
echo "=== 11. DynamicOrch-XACML (localhost:9193) ==="

orch_status=$(http_body http://localhost:9193/status)
assert_json_field "GET /status → status"    "status"   "$orch_status"
assert_json_value "GET /status → status=UP" "status"   "UP" "$orch_status"

orch_unauth=$(http_body -X POST http://localhost:9193/serviceorchestration/orchestration/pull \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"unauthorized","address":"1.1.1.1","port":8000},"requestedService":{"serviceDefinition":"telemetry"}}')
assert_json_field "Unauthorized consumer → response field" "response" "$orch_unauth"
assert_contains   "Unauthorized consumer → empty"         '"response":[]' "$(echo "$orch_unauth" | tr -d ' \n')"

orch_sp1=$(http_body -X POST http://localhost:9193/serviceorchestration/orchestration/pull \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"service-partner-1","address":"1.1.1.1","port":8000},"requestedService":{"serviceDefinition":"telemetry-rest"}}')
assert_json_field "service-partner-1 telemetry-rest → response field" "response" "$orch_sp1"
assert_contains   "service-partner-1 → portal-cloud-ml returned" "portal-cloud-ml" "$orch_sp1"

# ── Section 12: kafka-authz ───────────────────────────────────────────────────
echo
echo "=== 12. kafka-authz (localhost:9801) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:9801/health)"

kafka_deny=$(http_body -X POST http://localhost:9801/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unauthorized","service":"telemetry"}')
assert_contains "kafka-authz Deny → non-Permit" "Deny" "$kafka_deny"

# ── Section 13: pki-rest-authz ────────────────────────────────────────────────
echo
echo "=== 13. pki-rest-authz (localhost:9809) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:9809/health)"

rest_permit=$(http_body -X POST http://localhost:9809/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"service-partner-1","service":"telemetry-rest"}')
assert_json_field "pki-rest-authz Permit → permit field" "permit" "$rest_permit"
assert_contains   "pki-rest-authz Permit → permit=true"  "true"   "$rest_permit"

rest_revoked=$(http_body -X POST http://localhost:9809/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"stream-test","service":"telemetry-rest"}')
assert_contains "pki-rest-authz revoked cert → Deny" "false" "$rest_revoked"

# ── Section 14: Security invariants (exp-15) ─────────────────────────────────
echo
echo "=== 14. Security invariants (exp-15) ==="

# D4 invariant: PIP is the cert store — unknown CN → 404 (fail-closed)
pip_ghost_code=$(http_code http://localhost:8787/pip/attributes/ghost-system)
check_eq "CA-as-PIP D4 invariant: unknown CN → 404" "404" "$pip_ghost_code"

# D4 invariant: revoked cert shows valid=false in PIP immediately
pip_invariant=$(http_body http://localhost:8787/pip/attributes/stream-test)
assert_json_value "D4 invariant: stream-test valid=false in CA-as-PIP" "valid" "false" "$pip_invariant"

# PAP does not expose raw AuthzForce proxy
check_eq "PAP /domains → 404 (no AF proxy)" "404" \
  "$(http_code http://localhost:9805/domains 2>/dev/null || echo 404)"

# kafka-authz unknown CN fails closed
kafka_ghost=$(http_body -X POST http://localhost:9801/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"ghost-system","service":"telemetry"}')
assert_contains "kafka-authz unknown CN → fail-closed (Deny)" "Deny" "$kafka_ghost"

# ── Section 15: Core TLS ports reachable ─────────────────────────────────────
echo
echo "=== 15. Core TLS ports (9190–9192) ==="

check_eq "ServiceRegistry TLS :9190 → responds" "200" \
  "$(http_code --insecure https://localhost:9190/health 2>/dev/null || echo 000)"
check_eq "Authentication TLS :9191 → responds"  "200" \
  "$(http_code --insecure https://localhost:9191/health 2>/dev/null || echo 000)"
check_eq "ConsumerAuth TLS :9192 → responds"    "200" \
  "$(http_code --insecure https://localhost:9192/health 2>/dev/null || echo 000)"

# ── Section 16: portal-cloud-ml stats ────────────────────────────────────────
echo
echo "=== 16. portal-cloud-ml (localhost:9807) ==="

ml_stats=$(http_body http://localhost:9807/stats)
assert_json_field "GET /stats → messagesReceived" "messagesReceived" "$ml_stats"

# ── Summary ───────────────────────────────────────────────────────────────────
echo
echo "════════════════════════════════"
printf "  PASS: %d   FAIL: %d\n" "$PASS" "$FAIL"
echo "════════════════════════════════"
[ "$FAIL" -eq 0 ] || exit 1
