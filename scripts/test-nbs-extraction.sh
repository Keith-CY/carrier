#!/usr/bin/env bash
# Tests for NBS extraction regex used by review-nbs-followup workflow.
# Mirrors the JS regex: /^\s*NBS:\s*(.+)\s*$/gmi
set -euo pipefail

PASS=0
FAIL=0

assert_match() {
  local desc="$1" input="$2" expected="$3"
  # Use grep with the same pattern the workflow uses
  local result
  result=$(echo "$input" | sed -n 's/^[[:space:]]*NBS:[[:space:]]*\(.\{1,\}\)[[:space:]]*$/\1/p' | sed 's/[[:space:]]*$//')
  if [[ "$result" == "$expected" ]]; then
    echo "  PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $desc (expected='$expected', got='$result')"
    FAIL=$((FAIL + 1))
  fi
}

assert_no_match() {
  local desc="$1" input="$2"
  local result
  result=$(echo "$input" | sed -n 's/^[[:space:]]*NBS:[[:space:]]*\(.\{1,\}\)[[:space:]]*$/\1/p' | sed 's/[[:space:]]*$//')
  if [[ -z "$result" ]]; then
    echo "  PASS: $desc (no match)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $desc (expected no match, got='$result')"
    FAIL=$((FAIL + 1))
  fi
}

echo "=== NBS extraction boundary tests ==="
echo ""

echo "--- Punctuation trailing ---"
assert_match "NBS line ending with period" \
  "NBS: Add edge-case test for missing request_id." \
  "Add edge-case test for missing request_id."

assert_match "NBS line ending with semicolon" \
  "NBS: Clarify fallback behavior in README;" \
  "Clarify fallback behavior in README;"

assert_match "NBS line ending with closing paren" \
  "NBS: Consider renaming the label (e.g., FILE_COVERAGE)" \
  "Consider renaming the label (e.g., FILE_COVERAGE)"

echo ""
echo "--- Bullet prefixes (non-whitespace before NBS: must NOT match) ---"
assert_no_match "Dash bullet before NBS" \
  "- NBS: suggestion after dash bullet"

assert_no_match "Star bullet before NBS" \
  "* NBS: suggestion after star bullet"

assert_no_match "Numbered bullet before NBS" \
  "1. NBS: suggestion after number"

echo ""
echo "--- Whitespace variants ---"
assert_match "Leading spaces" \
  "   NBS: indented suggestion" \
  "indented suggestion"

assert_match "Leading tab" \
  "	NBS: tab-indented suggestion" \
  "tab-indented suggestion"

assert_match "Trailing spaces" \
  "NBS: suggestion with trailing spaces   " \
  "suggestion with trailing spaces"

echo ""
echo "--- Non-NBS lines (must NOT match) ---"
assert_no_match "Plain text" "This is not an NBS line"
assert_no_match "NBS in middle of text" "Some text NBS: embedded"
assert_no_match "Empty NBS" "NBS:   "
assert_no_match "Just NBS keyword" "NBS:"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [[ "$FAIL" -gt 0 ]]; then
  exit 1
fi
