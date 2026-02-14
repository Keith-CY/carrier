#!/usr/bin/env bash
# Tests for review-followup issue creation logic with malformed GH payloads.
# Validates the extractNbsLines regex and shortTitle helper behavior
# mirroring the workflow in .github/workflows/review-nbs-followup.yml.
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

# Simulate extractNbsLines: extract NBS suggestion text from input
extract_nbs() {
  echo "$1" | sed -n 's/^[[:space:]]*NBS:[[:space:]]*\(.\{1,\}\)/\1/p' | sed 's/[[:space:]]*$//'
}

# Simulate shortTitle: truncate to 80 chars
short_title() {
  local s="$1"
  s=$(echo "$s" | tr -s ' ' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
  if [[ ${#s} -gt 80 ]]; then
    echo "${s:0:77}..."
  else
    echo "$s"
  fi
}

echo "=== Malformed payload field tests ==="

echo ""
echo "--- Missing assignees array ---"
payload='{"number":10,"title":"test PR","body":"NBS: do something"}'
assignees=$(echo "$payload" | jq -r '.assignees // [] | length')
assert_eq "Missing assignees defaults to empty array" "0" "$assignees"

echo ""
echo "--- Null/empty title ---"
payload_null='{"number":11,"title":null,"body":"NBS: suggestion"}'
title=$(echo "$payload_null" | jq -r '.title // ""')
assert_eq "Null title yields empty" "" "$title"

payload_empty='{"number":12,"title":"","body":"NBS: suggestion"}'
title=$(echo "$payload_empty" | jq -r 'if .title == "" then "EMPTY" else .title end')
assert_eq "Empty title detected" "EMPTY" "$title"

echo ""
echo "--- Missing issue number ---"
payload_no_num='{"title":"orphan"}'
num=$(echo "$payload_no_num" | jq -r '.number // "MISSING"')
assert_eq "Missing number detected" "MISSING" "$num"

echo ""
echo "--- Body with no valid NBS lines ---"
result=$(extract_nbs "This is a normal comment without NBS markers")
assert_eq "No NBS in plain text" "" "$result"

result=$(extract_nbs "NBS:")
assert_eq "Bare NBS: with no content" "" "$result"

result=$(extract_nbs "NBS:   ")
assert_eq "NBS: with only whitespace" "" "$result"

echo ""
echo "--- Valid NBS extraction ---"
result=$(extract_nbs "NBS: Add edge-case test for missing request_id.")
assert_eq "Standard NBS line" "Add edge-case test for missing request_id." "$result"

echo ""
echo "--- shortTitle truncation ---"
long="This is a very long suggestion that exceeds eighty characters and should be truncated with an ellipsis at the end"
result=$(short_title "$long")
assert_eq "Long title truncated to 80 chars" "80" "${#result}"
assert_eq "Long title ends with ellipsis" "..." "${result: -3}"

short="Short suggestion"
result=$(short_title "$short")
assert_eq "Short title unchanged" "Short suggestion" "$result"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[[ "$FAIL" -eq 0 ]]
