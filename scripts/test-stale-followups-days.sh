#!/usr/bin/env bash
# Tests for --days argument parsing in stale-followups.sh.
# Validates that invalid/missing values are rejected with clear errors.
set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")" && pwd)/stale-followups.sh"
PASS=0
FAIL=0

assert_fails() {
  local desc="$1"
  shift
  if output=$("$SCRIPT" "$@" 2>&1); then
    echo "  FAIL: $desc (expected non-zero exit, got success)"
    FAIL=$((FAIL + 1))
  else
    echo "  PASS: $desc (exit non-zero)"
    PASS=$((PASS + 1))
  fi
}

assert_error_contains() {
  local desc="$1" needle="$2"
  shift 2
  local output
  output=$("$SCRIPT" "$@" 2>&1) || true
  if echo "$output" | grep -qi "$needle"; then
    echo "  PASS: $desc (message contains '$needle')"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $desc (message missing '$needle', got: $output)"
    FAIL=$((FAIL + 1))
  fi
}

echo "=== --days argument edge case tests ==="

echo ""
echo "--- --days with no value ---"
assert_fails "--days with no value" --days
assert_error_contains "--days no value error message" "requires a value" --days

echo ""
echo "--- --days with non-numeric value ---"
assert_fails "--days abc" --days abc
assert_error_contains "--days abc error message" "positive integer" --days abc

echo ""
echo "--- --days=0 ---"
assert_fails "--days 0" --days 0
assert_error_contains "--days 0 error message" "positive integer" --days 0

echo ""
echo "--- --days negative ---"
assert_fails "--days -5" --days -5
assert_error_contains "--days -5 error message" "positive integer" --days -5

echo ""
echo "--- --days valid positive integer (default path check) ---"
# We can't fully run the script without gh, but we can check it gets past arg parsing
# by verifying it doesn't fail with an arg-parsing error
output=$("$SCRIPT" --days 7 2>&1) || true
if echo "$output" | grep -q "Scanning"; then
  echo "  PASS: --days 7 passes arg parsing"
  PASS=$((PASS + 1))
else
  echo "  FAIL: --days 7 did not reach scanning phase"
  FAIL=$((FAIL + 1))
fi

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[[ "$FAIL" -eq 0 ]]
