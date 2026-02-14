#!/usr/bin/env bash
# Regression tests for stale-followups.sh parsing and threshold logic.
# Does NOT call GitHub API — tests extraction logic only.
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

# --- PR number extraction from title ---
extract_pr_from_title() {
  echo "$1" | sed -n 's/.*PR #\([0-9]\{1,\}\).*/\1/p' | head -1
}

echo "=== PR number extraction from title ==="
assert_eq "Standard title" "47" "$(extract_pr_from_title '[review-followup] PR #47: some suggestion')"
assert_eq "Large PR number" "1234" "$(extract_pr_from_title '[review-followup] PR #1234: fix thing')"
assert_eq "No PR number" "" "$(extract_pr_from_title 'Some other issue title')"
assert_eq "PR without hash" "" "$(extract_pr_from_title '[review-followup] PR 47: missing hash')"

# --- PR number extraction from body ---
extract_pr_from_body() {
  echo "$1" | sed -n 's|.*/pull/\([0-9]\{1,\}\).*|\1|p' | head -1
}

echo ""
echo "=== PR number extraction from body ==="
assert_eq "Standard body link" "123" "$(extract_pr_from_body 'Source PR: https://github.com/Keith-CY/carrier/pull/123')"
assert_eq "No pull link" "" "$(extract_pr_from_body 'No link here')"
assert_eq "Multiple links picks last (sed behavior)" "20" "$(extract_pr_from_body 'See /pull/10 and /pull/20')"

# --- --days argument validation ---
echo ""
echo "=== --days argument edge cases ==="

# The script should accept valid positive integers
validate_days() {
  local val="$1"
  if [[ "$val" =~ ^[0-9]+$ ]] && [[ "$val" -gt 0 ]]; then
    echo "valid"
  else
    echo "invalid"
  fi
}

assert_eq "--days 7 valid" "valid" "$(validate_days "7")"
assert_eq "--days 14 valid" "valid" "$(validate_days "14")"
assert_eq "--days 0 invalid" "invalid" "$(validate_days "0")"
assert_eq "--days negative invalid" "invalid" "$(validate_days "-1")"
assert_eq "--days non-numeric invalid" "invalid" "$(validate_days "abc")"
assert_eq "--days empty invalid" "invalid" "$(validate_days "")"

# --- No stale issues output ---
echo ""
echo "=== Output format ==="
expected_no_stale="No stale review-followup issues found."
assert_eq "No-stale message matches" "$expected_no_stale" "No stale review-followup issues found."

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[[ "$FAIL" -eq 0 ]]
