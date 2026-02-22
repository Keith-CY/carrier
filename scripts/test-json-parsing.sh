#!/usr/bin/env bash
# Regression tests for GH JSON parsing edge cases used in repo scripts.
# Verifies safe handling of missing/empty fields in jq-based extraction.
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

echo "=== GH JSON parsing edge cases ==="

# --- Empty assignees array ---
echo ""
echo "--- Empty assignees ---"
json_empty_assignees='{"number":1,"title":"test","assignees":[]}'
result=$(echo "$json_empty_assignees" | jq -r '.assignees[]?.login // empty' 2>/dev/null || echo "")
assert_eq "Empty assignees returns empty" "" "$result"

# --- Missing assignees field ---
json_no_assignees='{"number":2,"title":"test"}'
result=$(echo "$json_no_assignees" | jq -r '.assignees[]?.login // empty' 2>/dev/null || echo "")
assert_eq "Missing assignees returns empty" "" "$result"

# --- Null title ---
json_null_title='{"number":3,"title":null,"assignees":[]}'
result=$(echo "$json_null_title" | jq -r '.title // ""')
assert_eq "Null title returns empty string" "" "$result"

# --- Empty title ---
json_empty_title='{"number":4,"title":"","assignees":[]}'
result=$(echo "$json_empty_title" | jq -r '.title // ""')
assert_eq "Empty title returns empty string" "" "$result"

# --- Missing number field ---
json_no_number='{"title":"orphan issue"}'
result=$(echo "$json_no_number" | jq -r '.number // empty')
assert_eq "Missing number returns empty" "" "$result"

# --- Valid record with all fields ---
json_valid='{"number":5,"title":"good issue","assignees":[{"login":"dev01lay2"}]}'
num=$(echo "$json_valid" | jq -r '.number')
title=$(echo "$json_valid" | jq -r '.title')
assignee=$(echo "$json_valid" | jq -r '.assignees[0].login')
assert_eq "Valid number" "5" "$num"
assert_eq "Valid title" "good issue" "$title"
assert_eq "Valid assignee" "dev01lay2" "$assignee"

# --- Array with mixed valid/invalid records ---
echo ""
echo "--- Mixed array processing ---"
json_array='[{"number":1,"title":"ok"},{"number":null,"title":"bad"},{"number":3,"title":null}]'
count=$(echo "$json_array" | jq '[.[] | select(.number != null and .title != null)] | length')
assert_eq "Filter valid records from mixed array" "1" "$count"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[[ "$FAIL" -eq 0 ]]
