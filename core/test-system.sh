#!/usr/bin/env bash
# test-system.sh — system test for core/.
#
# Runs the full Go build and test suite, including the in-process E2E integration
# test that wires all seven core systems together.
#
# Usage (from repo root or core/):
#
#   bash core/test-system.sh
#
# No Docker required — all systems run in-process via httptest.

set -euo pipefail

PASS=0
FAIL=0

green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }

pass() { green "  PASS  $1"; PASS=$((PASS+1)); }
fail() { red   "  FAIL  $1"; echo "         $2"; FAIL=$((FAIL+1)); }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo
echo "=== Core system test ==="
echo "  Directory: $(pwd)"
echo

# ── 1. Build ──────────────────────────────────────────────────────────────────
echo "--- 1. go build ./... ---"
if go build ./...; then
  pass "go build ./..."
else
  fail "go build ./..." "build failed — see output above"
fi

# ── 2. Vet ────────────────────────────────────────────────────────────────────
echo
echo "--- 2. go vet ./... ---"
if go vet ./...; then
  pass "go vet ./..."
else
  fail "go vet ./..." "vet issues found — see output above"
fi

# ── 3. Unit + integration tests ───────────────────────────────────────────────
echo
echo "--- 3. go test ./... (includes E2E integration test) ---"
if go test -count=1 -race ./...; then
  pass "go test ./... (all systems, including E2E)"
else
  fail "go test ./..." "one or more tests failed — see output above"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo
echo "=============================="
echo "  PASS: $PASS  FAIL: $FAIL"
echo "=============================="

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
