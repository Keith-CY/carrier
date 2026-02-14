#!/usr/bin/env bash
# Test verify-checksum.sh including filenames with spaces and relative paths.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
VERIFY="$SCRIPT_DIR/verify-checksum.sh"
pass=0
fail=0

assert_exit() {
  local name="$1"
  local expected="$2"
  shift 2
  local actual=0
  "$@" >/dev/null 2>&1 || actual=$?
  if [ "$actual" -eq "$expected" ]; then
    echo "  PASS: $name"
    pass=$((pass + 1))
  else
    echo "  FAIL: $name (expected $expected, got $actual)"
    fail=$((fail + 1))
  fi
}

echo "Testing verify-checksum.sh..."

tmpdir=$(mktemp -d)

# --- Basic valid case ---
echo "hello world" > "$tmpdir/test.bin"
HASH=$(sha256sum "$tmpdir/test.bin" | awk '{print $1}')
echo "$HASH  test.bin" > "$tmpdir/checksums.txt"
assert_exit "basic valid checksum" 0 bash "$VERIFY" "$tmpdir/test.bin" "$tmpdir/checksums.txt"

# --- Mismatch case ---
echo "aaaa  test.bin" > "$tmpdir/bad-checksums.txt"
assert_exit "checksum mismatch fails" 1 bash "$VERIFY" "$tmpdir/test.bin" "$tmpdir/bad-checksums.txt"

# --- Filename with spaces ---
echo "space content" > "$tmpdir/my file name.tar.gz"
HASH_SPACE=$(sha256sum "$tmpdir/my file name.tar.gz" | awk '{print $1}')
echo "$HASH_SPACE  my file name.tar.gz" > "$tmpdir/checksums-space.txt"
assert_exit "filename with spaces" 0 bash "$VERIFY" "$tmpdir/my file name.tar.gz" "$tmpdir/checksums-space.txt"

# --- Relative path case ---
echo "relative content" > "$tmpdir/rel.bin"
HASH_REL=$(sha256sum "$tmpdir/rel.bin" | awk '{print $1}')
echo "$HASH_REL  rel.bin" > "$tmpdir/checksums-rel.txt"
pushd /tmp >/dev/null
assert_exit "relative path from different cwd" 0 bash "$VERIFY" "$tmpdir/rel.bin" "$tmpdir/checksums-rel.txt"
popd >/dev/null

# --- Missing file ---
assert_exit "missing target file" 1 bash "$VERIFY" "$tmpdir/nonexistent" "$tmpdir/checksums.txt"

# --- Missing checksum file ---
assert_exit "missing checksum file" 1 bash "$VERIFY" "$tmpdir/test.bin" "$tmpdir/nonexistent.txt"

# --- No matching entry ---
echo "deadbeef  other.bin" > "$tmpdir/checksums-other.txt"
assert_exit "no matching entry in checksum file" 1 bash "$VERIFY" "$tmpdir/test.bin" "$tmpdir/checksums-other.txt"

# --- Malformed input (empty checksum file) ---
> "$tmpdir/empty-checksums.txt"
assert_exit "empty checksum file" 1 bash "$VERIFY" "$tmpdir/test.bin" "$tmpdir/empty-checksums.txt"

rm -rf "$tmpdir"

echo ""
echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
