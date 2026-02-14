#!/usr/bin/env bash
# Tests for verify-checksum.sh malformed input handling.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
VERIFY="$SCRIPT_DIR/verify-checksum.sh"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

PASS=0
FAIL=0

assert_fail() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    echo "FAIL: $desc (expected non-zero exit)"
    FAIL=$((FAIL + 1))
  else
    echo "PASS: $desc"
    PASS=$((PASS + 1))
  fi
}

assert_pass() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    echo "PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc (expected zero exit)"
    FAIL=$((FAIL + 1))
  fi
}

# Test: empty lines are skipped, valid entry passes
echo "hello" > "$TMPDIR/hello.txt"
VALID_SUM="$(sha256sum "$TMPDIR/hello.txt" | awk '{print $1}')"
printf '\n\n%s  %s\n\n' "$VALID_SUM" "$TMPDIR/hello.txt" > "$TMPDIR/with-blanks.txt"
assert_pass "empty lines skipped" bash "$VERIFY" "$TMPDIR/with-blanks.txt"

# Test: missing checksum token (line with only whitespace after trimming is skipped)
echo "   " > "$TMPDIR/only-spaces.txt"
assert_pass "whitespace-only line skipped" bash "$VERIFY" "$TMPDIR/only-spaces.txt"

# Test: missing filename (single token on line)
echo "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890" > "$TMPDIR/no-filename.txt"
assert_fail "missing filename" bash "$VERIFY" "$TMPDIR/no-filename.txt"

# Test: non-hex checksum
printf 'ZZZZZZ  %s\n' "$TMPDIR/hello.txt" > "$TMPDIR/bad-hex.txt"
assert_fail "non-hex checksum" bash "$VERIFY" "$TMPDIR/bad-hex.txt"

# Test: extra whitespace normalization (tabs between fields)
printf '%s\t\t%s\n' "$VALID_SUM" "$TMPDIR/hello.txt" > "$TMPDIR/tabs.txt"
assert_pass "tab-separated fields" bash "$VERIFY" "$TMPDIR/tabs.txt"

# Test: comment lines are skipped
printf '# comment\n%s  %s\n' "$VALID_SUM" "$TMPDIR/hello.txt" > "$TMPDIR/comments.txt"
assert_pass "comment lines skipped" bash "$VERIFY" "$TMPDIR/comments.txt"

# Test: no arguments
assert_fail "no arguments" bash "$VERIFY"

# Test: nonexistent checksum file
assert_fail "nonexistent file" bash "$VERIFY" "$TMPDIR/nonexistent.txt"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
