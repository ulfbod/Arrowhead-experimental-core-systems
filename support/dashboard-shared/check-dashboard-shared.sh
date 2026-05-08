#!/usr/bin/env bash
# check-dashboard-shared.sh
#
# Verifies that the 10 shared dashboard files in support/dashboard-shared/ are
# properly symlinked from both experiment-5 and experiment-6.
#
# Checks:
#   1. Each expected symlink exists and points to support/dashboard-shared/.
#   2. The symlink target resolves to the canonical file (no dangling links).
#   3. The resolved content is identical to the canonical file in support/dashboard-shared/.
#
# Usage:
#   bash support/dashboard-shared/check-dashboard-shared.sh
# Run from the repo root.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARED="$REPO_ROOT/support/dashboard-shared"

PASS=0; FAIL=0

pass() { echo "  PASS  $1"; PASS=$((PASS + 1)); }
fail() { echo "  FAIL  $1"; echo "        $2"; FAIL=$((FAIL + 1)); }

# The 10 shared files, expressed as paths relative to dashboard/src/
SHARED_FILES=(
  "main.tsx"
  "hooks/usePolling.ts"
  "config/context.tsx"
  "config/defaults.ts"
  "config/types.ts"
  "components/StatusDot.tsx"
  "views/HealthView.tsx"
  "views/GrantsView.tsx"
  "views/PolicyView.tsx"
  "views/LiveDataView.tsx"
)

check_experiment() {
  local exp="$1"
  local src="$REPO_ROOT/experiments/$exp/dashboard/src"

  echo "=== $exp ==="

  for rel in "${SHARED_FILES[@]}"; do
    local link="$src/$rel"
    local canonical="$SHARED/$rel"

    # 1. Exists
    if [[ ! -e "$link" ]]; then
      fail "$rel" "file missing at $link"
      continue
    fi

    # 2. Is a symlink
    if [[ ! -L "$link" ]]; then
      fail "$rel" "exists but is a regular file, not a symlink — should point to support/dashboard-shared/$rel"
      continue
    fi

    # 3. Symlink target contains 'support/dashboard-shared'
    local target
    target="$(readlink "$link")"
    if [[ "$target" != *"support/dashboard-shared"* ]]; then
      fail "$rel" "symlink points to '$target', expected a path containing 'support/dashboard-shared'"
      continue
    fi

    # 4. Resolves (not dangling)
    local resolved
    resolved="$(readlink -f "$link" 2>/dev/null)" || { fail "$rel" "dangling symlink — target does not exist"; continue; }

    # 5. Content matches canonical
    if ! diff -q "$resolved" "$canonical" > /dev/null 2>&1; then
      fail "$rel" "content differs from support/dashboard-shared/$rel — canonical file may have been edited in one location only"
      continue
    fi

    pass "$rel"
  done
}

check_experiment "experiment-5"
echo ""
check_experiment "experiment-6"

echo ""
echo "────────────────────────────────────────"
echo "  Passed: $PASS / $((PASS + FAIL))"
echo "────────────────────────────────────────"

if [[ $FAIL -gt 0 ]]; then
  echo "  FAIL — $FAIL check(s) failed"
  echo ""
  echo "  To fix a missing or broken symlink:"
  echo "    cd experiments/<exp>/dashboard/src/<subdir>"
  echo "    ln -sf <relative-path-to>/support/dashboard-shared/<file> <file>"
  echo ""
  echo "  To fix a content divergence:"
  echo "    Edit the canonical file in support/dashboard-shared/"
  echo "    The symlinks in both experiments will reflect the change immediately."
  exit 1
fi

echo "  PASS — all shared dashboard files are correctly symlinked"
