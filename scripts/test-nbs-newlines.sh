#!/usr/bin/env bash
# Tests for NBS extraction stability across newline variants.
# Verifies that \n, \r\n, and trailing whitespace produce identical results.
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

# Extract NBS suggestions, normalize output (trim each line)
extract_nbs() {
  # Normalize \r\n to \n, then extract
  printf '%s' "$1" | tr -d '\r' | sed -n 's/^[[:space:]]*NBS:[[:space:]]*\(.\{1,\}\)/\1/p' | sed 's/[[:space:]]*$//'
}

echo "=== NBS extraction across newline variants ==="

SUGGESTION="Add edge-case test for missing request_id."

echo ""
echo "--- Unix newlines (LF) ---"
input_unix=$(printf 'Some comment\nNBS: %s\nAnother line\n' "$SUGGESTION")
result_unix=$(extract_nbs "$input_unix")
assert_eq "Unix LF extraction" "$SUGGESTION" "$result_unix"

echo ""
echo "--- Windows newlines (CRLF) ---"
input_crlf=$(printf 'Some comment\r\nNBS: %s\r\nAnother line\r\n' "$SUGGESTION")
result_crlf=$(extract_nbs "$input_crlf")
assert_eq "Windows CRLF extraction" "$SUGGESTION" "$result_crlf"

echo ""
echo "--- Lines with trailing spaces ---"
input_trailing=$(printf 'NBS: %s   \n' "$SUGGESTION")
result_trailing=$(extract_nbs "$input_trailing")
assert_eq "Trailing spaces stripped" "$SUGGESTION" "$result_trailing"

echo ""
echo "--- Lines with trailing tabs ---"
input_tabs=$(printf 'NBS: %s\t\t\n' "$SUGGESTION")
result_tabs=$(extract_nbs "$input_tabs")
assert_eq "Trailing tabs stripped" "$SUGGESTION" "$result_tabs"

echo ""
echo "--- Cross-variant consistency ---"
assert_eq "Unix == CRLF" "$result_unix" "$result_crlf"
assert_eq "Unix == trailing spaces" "$result_unix" "$result_trailing"
assert_eq "Unix == trailing tabs" "$result_unix" "$result_tabs"

echo ""
echo "--- Multiple NBS lines with mixed newlines ---"
input_mixed=$(printf 'NBS: First suggestion\r\nNBS: Second suggestion\nNBS: Third suggestion   \r\n')
result_multi=$(extract_nbs "$input_mixed")
expected_multi=$(printf 'First suggestion\nSecond suggestion\nThird suggestion')
assert_eq "Multiple NBS with mixed newlines" "$expected_multi" "$result_multi"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[[ "$FAIL" -eq 0 ]]
