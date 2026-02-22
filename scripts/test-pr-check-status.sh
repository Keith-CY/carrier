#!/usr/bin/env bash
# Tests for PR check status parsing logic used by automation scripts.
# Validates that 'skipping' is non-blocking while 'fail'/'cancel' block.
set -euo pipefail

PASS=0
FAIL=0

assert_eq() {
  local desc="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "  PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $desc (expected='$expected', got='$actual')"
    FAIL=$((FAIL + 1))
  fi
}

# Simulate PR check eligibility decision:
# eligible = no 'fail' or 'cancelled' statuses
# 'pass' and 'skipping' are non-blocking
check_eligible() {
  local statuses="$1"
  local eligible="true"
  for status in $statuses; do
    case "$status" in
      pass|skipping) ;;
      fail|cancelled) eligible="false"; break ;;
      pending) eligible="pending"; break ;;
      *) eligible="unknown"; break ;;
    esac
  done
  echo "$eligible"
}

echo "=== PR check status parsing tests ==="

echo ""
echo "--- All pass ---"
assert_eq "All pass is eligible" "true" "$(check_eligible "pass pass pass")"

echo ""
echo "--- Pass + skipping (non-blocking) ---"
assert_eq "Pass + skipping is eligible" "true" "$(check_eligible "pass skipping pass")"
assert_eq "Only skipping is eligible" "true" "$(check_eligible "skipping skipping")"

echo ""
echo "--- Fail blocks ---"
assert_eq "One fail blocks" "false" "$(check_eligible "pass fail pass")"
assert_eq "Fail + skipping blocks" "false" "$(check_eligible "skipping fail")"

echo ""
echo "--- Cancelled blocks ---"
assert_eq "Cancelled blocks" "false" "$(check_eligible "pass cancelled")"
assert_eq "Cancelled + skipping blocks" "false" "$(check_eligible "skipping cancelled pass")"

echo ""
echo "--- Mixed pass + skipping + fail ---"
assert_eq "Mixed with fail blocks" "false" "$(check_eligible "pass skipping fail pass")"

echo ""
echo "--- Pending is not eligible yet ---"
assert_eq "Pending returns pending" "pending" "$(check_eligible "pass pending skipping")"

echo ""
echo "--- JSON fixture simulation ---"
# Simulate parsing gh pr checks JSON output
fixture='[
  {"name":"Daemon Tests","status":"pass"},
  {"name":"Gateway Check","status":"pass"},
  {"name":"E2E Tests","status":"skipping"}
]'
statuses=$(echo "$fixture" | jq -r '.[].status' | tr '\n' ' ')
assert_eq "JSON fixture with skipping is eligible" "true" "$(check_eligible "$statuses")"

fixture_fail='[
  {"name":"Daemon Tests","status":"pass"},
  {"name":"Gateway Check","status":"fail"},
  {"name":"E2E Tests","status":"skipping"}
]'
statuses_fail=$(echo "$fixture_fail" | jq -r '.[].status' | tr '\n' ' ')
assert_eq "JSON fixture with fail is not eligible" "false" "$(check_eligible "$statuses_fail")"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[[ "$FAIL" -eq 0 ]]
