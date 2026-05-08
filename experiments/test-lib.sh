#!/usr/bin/env bash
# experiments/test-lib.sh — shared assertion library for experiment test-system.sh scripts.
#
# Source near the top of any test-system.sh, after PASS=0 / FAIL=0:
#
#   source "$(dirname "$0")/../test-lib.sh"
#
# The sourcing script must declare PASS=0 and FAIL=0 before any pass/fail calls.

# ── Colour helpers ────────────────────────────────────────────────────────────
green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }

# ── Basic pass / fail ─────────────────────────────────────────────────────────
pass() { green "  PASS  $1"; PASS=$((PASS+1)); }

fail() {
  red "  FAIL  $1"
  echo "         expected: $2"
  echo "         actual:   $3"
  FAIL=$((FAIL+1))
}

# check_eq "desc" "expected" "actual"
check_eq() {
  local desc="$1" expected="$2" actual="$3"
  if [ "$actual" = "$expected" ]; then pass "$desc"; else fail "$desc" "$expected" "$actual"; fi
}

# ── HTTP helpers ──────────────────────────────────────────────────────────────
http_code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
http_body() { curl -s "$@"; }

# ── Smoke-check helpers (exit immediately on failure) ─────────────────────────
smoke_fail() {
  red "  FAIL  $1"
  echo "         $2"
  red "  Cannot continue — fix the above, then re-run."
  red "  Is the stack up?  Run: docker compose up -d --build"
  exit 2
}

# smoke_http "desc" url [extra_curl_args...]
# Checks HTTP 200; exits immediately if the request fails.
smoke_http() {
  local desc="$1" url="$2"; shift 2
  local code
  code=$(http_code "$url" "$@" 2>/dev/null || echo "000")
  if [ "$code" = "200" ]; then
    pass "$desc → 200"
  else
    smoke_fail "$desc → 200" "HTTP $code — is the container running?"
  fi
}

# ── Standard assertions ───────────────────────────────────────────────────────

# assert_http "desc" expected_status url [extra_curl_args...]
# Asserts that the HTTP status code matches expected_status.
assert_http() {
  local desc="$1" expected="$2" url="$3"; shift 3
  local actual
  actual=$(http_code "$url" "$@" 2>/dev/null || echo "000")
  check_eq "$desc" "$expected" "$actual"
}

# assert_contains "desc" "needle" "haystack"
# Asserts that haystack contains needle.
# Uses bash [[ ]] matching — NOT echo|grep — to avoid the SSE false-negative
# trap documented in EXPERIENCES.md EXP-004 and EXP-006.
assert_contains() {
  local desc="$1" needle="$2" haystack="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    pass "$desc"
  else
    fail "$desc" "(contains) $needle" "${haystack:0:200}"
  fi
}

# assert_not_contains "desc" "needle" "haystack"
# Asserts that haystack does NOT contain needle.
# Uses bash [[ ]] matching for the same reason as assert_contains.
assert_not_contains() {
  local desc="$1" needle="$2" haystack="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    pass "$desc"
  else
    fail "$desc" "(not contains) $needle" "${haystack:0:200}"
  fi
}

# assert_json_field "desc" "field" "json"
# Asserts that json contains the key "field" with any value.
# Example: assert_json_field "has msgCount" "msgCount" "$stats"
assert_json_field() {
  local desc="$1" field="$2" json="$3"
  if [[ "$json" == *"\"$field\":"* ]]; then
    pass "$desc"
  else
    fail "$desc" "\"$field\":... present" "${json:0:200}"
  fi
}

# assert_json_value "desc" "field" "value" "json"
# Asserts that json contains "field":"value" (string) or "field":value (non-string).
# Example: assert_json_value "synced=true"    "synced"   "true"   "$status"
# Example: assert_json_value "decision=Permit" "decision" "Permit" "$body"
assert_json_value() {
  local desc="$1" field="$2" value="$3" json="$4"
  if [[ "$json" == *"\"$field\":\"$value\""* ]] || \
     [[ "$json" == *"\"$field\":$value"* ]]; then
    pass "$desc"
  else
    fail "$desc" "\"$field\":\"$value\"" "${json:0:200}"
  fi
}

# assert_json_gt "desc" "field" minimum "json"
# Asserts that the numeric value of field in json is strictly greater than minimum.
# Example: assert_json_gt "msgCount > 0" "msgCount" 0 "$stats"
# Example: assert_json_gt "grants ≥ 3"   "count"    2 "$grants"
assert_json_gt() {
  local desc="$1" field="$2" minimum="$3" json="$4"
  local val
  val=$(printf '%s' "$json" | grep -oE "\"$field\":[0-9]+" | grep -oE '[0-9]+$' || echo "0")
  if [ "${val:-0}" -gt "$minimum" ]; then
    pass "$desc (got $val)"
  else
    fail "$desc" ">$minimum" "${val:-0}"
  fi
}
