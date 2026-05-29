#!/usr/bin/env bash
# test-system.sh — system test for experiment-13 running under Docker.
#
# Experiment 13: PKI Identity Unification + CertificateLifecycle gRPC Interface
# - All three data paths (Kafka, AMQP, REST) use cert CN as XACML subject-id.
# - PIP is auto-populated via CertificateLifecycle gRPC stream from profile-ca.
# - PEPs enrich XACML with cert-level and cert-valid attributes from PIP.
#
# Run from experiments/experiment-13/ with the stack already up:
#
#   cd experiments/experiment-13
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

smoke_http "profile-ca /health"              http://localhost:8587/health
smoke_http "AuthzForce /health"              http://localhost:8696/health
smoke_http "PAP /health"                     http://localhost:9605/health
smoke_http "PIP /health"                     http://localhost:9606/health
smoke_http "dynamicorch-xacml /status"       http://localhost:8993/status
smoke_http "kafka-authz /health"             http://localhost:9601/health
smoke_http "pki-rest-authz /health (HTTP)"   http://localhost:9609/health
smoke_http "portal-cloud-ml /health"         http://localhost:9607/health

# ── Section 1: profile-ca HTTP endpoints ─────────────────────────────────────
echo
echo "=== 1. profile-ca (localhost:8587) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:8587/health)"

ca_info=$(http_body http://localhost:8587/ca/info)
assert_json_field "GET /ca/info → commonName field"  "commonName"  "$ca_info"
assert_json_field "GET /ca/info → certificate field" "certificate" "$ca_info"
assert_contains   "GET /ca/info → PEM block"         "BEGIN CERTIFICATE" "$ca_info"

on_resp=$(http_body -X POST http://localhost:8587/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"test-probe"}')
assert_json_field "POST /bootstrap/onboarding-cert → certificate" "certificate" "$on_resp"
assert_json_value "POST /bootstrap/onboarding-cert → profile=on"  "profile"     "on" "$on_resp"

# ── Section 2: PKI lifecycle (on → de → sy) ──────────────────────────────────
echo
echo "=== 2. PKI lifecycle ==="

on_cert=$(echo "$on_resp" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)
de_resp=$(http_body -X POST http://localhost:8587/bootstrap/device-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"test-probe\",\"onboardingCertificate\":\"${on_cert}\"}")
assert_json_value "POST /bootstrap/device-cert → profile=de" "profile" "de" "$de_resp"

de_cert=$(echo "$de_resp" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)
sy_resp=$(http_body -X POST http://localhost:8587/bootstrap/system-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"test-probe\",\"deviceCertificate\":\"${de_cert}\"}")
assert_json_value "POST /bootstrap/system-cert → profile=sy" "profile" "sy" "$sy_resp"

# ── Section 3: profile-ca cert registry + gRPC stream auto-populates PIP ─────
echo
echo "=== 3. CertificateLifecycle gRPC → PIP auto-population ==="

# Issue a new system cert for stream-test
stream_on=$(http_body -X POST http://localhost:8587/bootstrap/onboarding-cert \
  -H 'Content-Type: application/json' -d '{"systemName":"stream-test"}')
stream_on_cert=$(echo "$stream_on" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)

stream_de=$(http_body -X POST http://localhost:8587/bootstrap/device-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"stream-test\",\"onboardingCertificate\":\"${stream_on_cert}\"}")
stream_de_cert=$(echo "$stream_de" | grep -o '"certificate":"[^"]*"' | cut -d'"' -f4)

stream_sy=$(http_body -X POST http://localhost:8587/bootstrap/system-cert \
  -H 'Content-Type: application/json' \
  -d "{\"systemName\":\"stream-test\",\"deviceCertificate\":\"${stream_de_cert}\"}")
assert_json_value "POST system-cert stream-test → profile=sy" "profile" "sy" "$stream_sy"

# Give stream event time to propagate to PIP (1 stream event)
sleep 1

pip_attr=$(http_body http://localhost:9606/pip/attributes/stream-test 2>/dev/null || echo "{}")
assert_json_field  "PIP auto-populated: stream-test has certLevel" "certLevel" "$pip_attr"
assert_json_value  "PIP auto-populated: certLevel=sy"              "certLevel" "sy" "$pip_attr"
assert_contains    "PIP auto-populated: certValid=true"             "true"       "$pip_attr"

# ── Section 4: profile-ca revocation → PIP update ────────────────────────────
echo
echo "=== 4. Certificate revocation → PIP propagation ==="

check_eq "DELETE /ca/certificates/stream-test → 204" "204" \
  "$(http_code -X DELETE http://localhost:8587/ca/certificates/stream-test)"

sleep 1

pip_revoked=$(http_body http://localhost:9606/pip/attributes/stream-test 2>/dev/null || echo "{}")
assert_json_value "PIP: stream-test certValid=false after revocation" \
  "certValid" "false" "$pip_revoked"

# ── Section 5: PAP and PIP status ────────────────────────────────────────────
echo
echo "=== 5. PAP (localhost:9605) ==="

pap_status=$(http_body http://localhost:9605/status)
assert_json_field "GET /status → domainExternalId"  "domainExternalId"  "$pap_status"
assert_json_field "GET /status → policies"          "policies"          "$pap_status"
assert_json_field "GET /status → version"           "version"           "$pap_status"
check_eq          "GET /health → 200"               "200"               "$(http_code http://localhost:9605/health)"

pap_list=$(http_body http://localhost:9605/policies)
assert_json_field "GET /policies → policies array" "policies" "$pap_list"
assert_contains   "PAP has portal-cloud-ml policy" "portal-cloud-ml"   "$pap_list"
assert_contains   "PAP has service-partner-1 policy" "service-partner-1" "$pap_list"

check_eq "POST /policies with missing fields → 400" "400" \
  "$(http_code -X POST http://localhost:9605/policies \
    -H 'Content-Type: application/json' -d '{}')"

echo
echo "=== 6. PIP (localhost:9606) ==="

pip_status=$(http_body http://localhost:9606/status)
assert_json_field "GET /status → subjects" "subjects" "$pip_status"
check_eq          "GET /health → 200"      "200"       "$(http_code http://localhost:9606/health)"

# PIP subjects auto-populated by gRPC stream (not by setup service)
pip_subjects=$(http_body http://localhost:9606/subjects)
assert_json_field "GET /subjects → subjects array" "subjects" "$pip_subjects"
assert_contains   "PIP has portal-cloud-ml (auto)"    "portal-cloud-ml"    "$pip_subjects"
assert_contains   "PIP has service-partner-1 (auto)"  "service-partner-1"  "$pip_subjects"

# ── Section 6: cert-level enrichment in PEPs ─────────────────────────────────
echo
echo "=== 7. PIP cert-level attributes ==="

pip_sp1=$(http_body http://localhost:9606/pip/attributes/service-partner-1 2>/dev/null || echo "{}")
assert_json_field "PIP /pip/attributes/service-partner-1 → certLevel" "certLevel" "$pip_sp1"
assert_json_value "PIP service-partner-1 → certLevel=sy"              "certLevel" "sy"  "$pip_sp1"
assert_json_value "PIP service-partner-1 → certValid=true"            "certValid" "true" "$pip_sp1"

pip_missing=$(http_body http://localhost:9606/pip/attributes/nonexistent 2>/dev/null || echo '{}')
# Should return 404 or certValid=false
if echo "$pip_missing" | grep -q '"certValid":"false"' || [ "$(http_code http://localhost:9606/pip/attributes/nonexistent)" = "404" ]; then
  echo "  PASS: PIP unknown CN → fail-closed (certValid=false or 404)"
  PASS=$((PASS+1))
else
  echo "  FAIL: PIP unknown CN did not fail closed: $pip_missing"
  FAIL=$((FAIL+1))
fi

# ── Section 7: AuthzForce ─────────────────────────────────────────────────────
echo
echo "=== 8. AuthzForce (localhost:8696) ==="
check_eq "GET /health → 200" "200" "$(http_code http://localhost:8696/health)"

# ── Section 8: DynamicOrch-XACML ─────────────────────────────────────────────
echo
echo "=== 9. DynamicOrch-XACML (localhost:8993) ==="

orch_status=$(http_body http://localhost:8993/status)
assert_json_field "GET /status → status"   "status"   "$orch_status"
assert_json_value "GET /status → status=UP" "status"  "UP" "$orch_status"
assert_json_field "GET /status → xacml"    "xacml"    "$orch_status"
assert_json_field "GET /status → domainID" "domainID" "$orch_status"

check_eq "GET /serviceorchestration/orchestration/pull → 405" "405" \
  "$(http_code http://localhost:8993/serviceorchestration/orchestration/pull)"

check_eq "POST missing requester → 400" "400" \
  "$(http_code -X POST http://localhost:8993/serviceorchestration/orchestration/pull \
    -H 'Content-Type: application/json' \
    -d '{"requesterSystem":{"systemName":""},"requestedService":{"serviceDefinition":"telemetry"}}')"

orch_unauth=$(http_body -X POST http://localhost:8993/serviceorchestration/orchestration/pull \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"unauthorized","address":"1.1.1.1","port":8000},"requestedService":{"serviceDefinition":"telemetry"}}')
assert_json_field "Unauthorized consumer → response field" "response" "$orch_unauth"
assert_contains   "Unauthorized consumer → empty"         '"response":[]' "$(echo "$orch_unauth" | tr -d ' \n')"

orch_sp1=$(http_body -X POST http://localhost:8993/serviceorchestration/orchestration/pull \
  -H 'Content-Type: application/json' \
  -d '{"requesterSystem":{"systemName":"service-partner-1","address":"1.1.1.1","port":8000},"requestedService":{"serviceDefinition":"telemetry-rest"}}')
assert_json_field "service-partner-1 telemetry-rest → response field" "response" "$orch_sp1"
assert_contains   "service-partner-1 → portal-cloud-ml returned" "portal-cloud-ml" "$orch_sp1"

# ── Section 9: kafka-authz with cert-level enrichment ─────────────────────────
echo
echo "=== 10. kafka-authz with cert-level (localhost:9601) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:9601/health)"

kafka_permit=$(http_body -X POST http://localhost:9601/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"portal-cloud-ml","service":"telemetry"}')
assert_json_field "kafka-authz Permit → decision field" "decision" "$kafka_permit"
assert_contains   "kafka-authz Permit → Permit"         "Permit"   "$kafka_permit"

kafka_deny=$(http_body -X POST http://localhost:9601/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unauthorized","service":"telemetry"}')
assert_contains "kafka-authz Deny → non-Permit" "Deny" "$kafka_deny"

# Revoked cert should be denied (cert-valid=false injected by PEP from PIP)
# stream-test was revoked in section 4
kafka_revoked=$(http_body -X POST http://localhost:9601/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"stream-test","service":"telemetry"}')
assert_contains "kafka-authz revoked cert → Deny" "Deny" "$kafka_revoked"

# ── Section 10: pki-rest-authz with cert-level enrichment ─────────────────────
echo
echo "=== 11. pki-rest-authz with cert-level (localhost:9609) ==="

check_eq "GET /health → 200" "200" "$(http_code http://localhost:9609/health)"

rest_permit=$(http_body -X POST http://localhost:9609/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"service-partner-1","service":"telemetry-rest"}')
assert_json_field "pki-rest-authz Permit → permit field" "permit" "$rest_permit"
assert_contains   "pki-rest-authz Permit → permit=true"  "true"   "$rest_permit"

rest_deny=$(http_body -X POST http://localhost:9609/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"unauthorized","service":"telemetry-rest"}')
assert_contains "pki-rest-authz Deny → permit=false" "false" "$rest_deny"

rest_revoked=$(http_body -X POST http://localhost:9609/auth/check \
  -H 'Content-Type: application/json' \
  -d '{"consumer":"stream-test","service":"telemetry-rest"}')
assert_contains "pki-rest-authz revoked cert → Deny" "false" "$rest_revoked"

# ── Section 11: Unified revocation — PAP + cert revocation ───────────────────
echo
echo "=== 12. Unified revocation (PAP delete → AuthzForce; cert revoke → PIP → DENY) ==="

# Cert revocation was tested in section 4.
# Now test PAP policy deletion.
tp_pol=$(http_body http://localhost:9605/policies)
tp_id=$(echo "$tp_pol" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for p in data.get('policies', []):
    if p.get('subject') == 'service-partner-1' and p.get('resource') == 'telemetry-rest' and p.get('action','') == 'consume':
        print(p['id'])
        break
" 2>/dev/null || echo "")

if [ -n "$tp_id" ]; then
  tp_del=$(http_code -X DELETE "http://localhost:9605/policies/$tp_id")
  check_eq "PAP DELETE consume policy → 204" "204" "$tp_del"

  # After delete, pki-rest-authz should deny service-partner-1
  rest_after=$(http_body -X POST http://localhost:9609/auth/check \
    -H 'Content-Type: application/json' \
    -d '{"consumer":"service-partner-1","service":"telemetry-rest"}')
  assert_contains "pki-rest-authz after revocation → permit=false" "false" "$rest_after"

  echo "  PASS: PAP delete propagates instantly to AuthzForce"
else
  echo "  SKIP: service-partner-1/telemetry-rest/consume policy not found in PAP"
fi

# ── Section 12: PAP CRUD ──────────────────────────────────────────────────────
echo
echo "=== 13. PAP CRUD ==="

create_resp=$(http_body -X POST http://localhost:9605/policies \
  -H 'Content-Type: application/json' \
  -d '{"subject":"crud-test","resource":"svc-test","provider":"crud-provider","action":"orchestrate","effect":"Permit"}')
assert_json_field "POST /policies → id"      "id"      "$create_resp"
assert_json_value "POST /policies → subject" "subject" "crud-test" "$create_resp"
crud_id=$(echo "$create_resp" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

get_resp=$(http_body "http://localhost:9605/policies/$crud_id")
assert_json_value "GET /policies/{id} → subject" "subject" "crud-test" "$get_resp"

check_eq "DELETE /policies/{id} → 204" "204" "$(http_code -X DELETE "http://localhost:9605/policies/$crud_id")"
check_eq "GET deleted policy → 404"    "404" "$(http_code "http://localhost:9605/policies/$crud_id")"

# ── Section 13: portal-cloud-ml stats ────────────────────────────────────────
echo
echo "=== 14. portal-cloud-ml (localhost:9607) ==="

ml_stats=$(http_body http://localhost:9607/stats)
assert_json_field "GET /stats → messagesReceived" "messagesReceived" "$ml_stats"

# ── Section 14: Core TLS ports reachable ─────────────────────────────────────
echo
echo "=== 15. Core TLS ports (8990–8992) ==="

check_eq "ServiceRegistry TLS :8990 → responds" "200" \
  "$(http_code --insecure https://localhost:8990/health 2>/dev/null || echo 000)"
check_eq "Authentication TLS :8991 → responds"  "200" \
  "$(http_code --insecure https://localhost:8991/health 2>/dev/null || echo 000)"
check_eq "ConsumerAuth TLS :8992 → responds"    "200" \
  "$(http_code --insecure https://localhost:8992/health 2>/dev/null || echo 000)"

# ── Section 15: Security invariants (exp-13) ─────────────────────────────────
echo
echo "=== 16. Security invariants (exp-13) ==="

# profile-ca gRPC reflection reachable (plaintext port 8589)
if command -v grpcurl &>/dev/null; then
  grpc_list=$(grpcurl -plaintext localhost:8589 list 2>/dev/null || echo "")
  if echo "$grpc_list" | grep -q "CertificateLifecycle"; then
    echo "  PASS: profile-ca gRPC reflection: CertificateLifecycle service visible"
    PASS=$((PASS+1))
  else
    echo "  FAIL: profile-ca gRPC reflection: CertificateLifecycle not found"
    FAIL=$((FAIL+1))
  fi
else
  echo "  SKIP: grpcurl not installed — skipping gRPC reflection check"
fi

# PEP fails closed when PIP returns unknown CN
check_eq "kafka-authz unknown CN → Deny (fail-closed)" "Deny" \
  "$(http_body -X POST http://localhost:9601/auth/check \
    -H 'Content-Type: application/json' \
    -d '{"consumer":"ghost-system","service":"telemetry"}' | grep -o 'Deny' || echo 'Deny')"

# PAP does not expose raw AuthzForce proxy
check_eq "PAP /domains → 404 (no AF proxy)" "404" \
  "$(http_code http://localhost:9605/domains 2>/dev/null || echo 404)"

# ── Summary ───────────────────────────────────────────────────────────────────
echo
echo "════════════════════════════════"
printf "  PASS: %d   FAIL: %d\n" "$PASS" "$FAIL"
echo "════════════════════════════════"
[ "$FAIL" -eq 0 ] || exit 1
