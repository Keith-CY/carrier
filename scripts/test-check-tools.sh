#!/usr/bin/env bash
# Test the check-tools helper by stubbing PATH.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK_SCRIPT="$SCRIPT_DIR/check-tools.sh"
pass=0
fail=0

echo "Testing check-tools.sh..."

# Test 1: With full PATH, should succeed
if bash "$CHECK_SCRIPT" >/dev/null 2>&1; then
  echo "  PASS: exits 0 with all tools present"
  pass=$((pass + 1))
else
  echo "  FAIL: expected exit 0 with all tools present"
  fail=$((fail + 1))
fi

# Test 2: With empty PATH, should fail (missing required tools)
if PATH="/nonexistent" bash "$CHECK_SCRIPT" >/dev/null 2>&1; then
  echo "  FAIL: expected exit 1 with empty PATH"
  fail=$((fail + 1))
else
  echo "  PASS: exits non-zero with missing tools"
  pass=$((pass + 1))
fi

echo ""
echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
