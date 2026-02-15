#!/usr/bin/env bash
# Test the check-tools helper by stubbing PATH.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK_SCRIPT="$SCRIPT_DIR/check-tools.sh"
pass=0
fail=0

echo "Testing check-tools.sh..."

TMP_BIN="$(mktemp -d)"
trap 'rm -rf "$TMP_BIN"' EXIT

make_stub() {
  local tool="$1"
  cat >"$TMP_BIN/$tool" <<'STUB'
#!/usr/bin/env bash
echo "stub 0.0.0"
STUB
  chmod +x "$TMP_BIN/$tool"
}

# Hermetic stubs for required tools used by check-tools.sh.
make_stub "gh"
make_stub "jq"
make_stub "go"
make_stub "bun"
ln -sf "$(command -v bash)" "$TMP_BIN/bash"

# Test 1: Hermetic success path with stubbed required tools.
if PATH="$TMP_BIN:/usr/bin:/bin" /bin/bash "$CHECK_SCRIPT" >/dev/null 2>&1; then
  echo "  PASS: exits 0 with stubbed required tools"
  pass=$((pass + 1))
else
  echo "  FAIL: expected exit 0 with stubbed required tools"
  fail=$((fail + 1))
fi

# Test 2: With empty/nonexistent PATH, should fail (missing required tools)
if PATH="/nonexistent" /bin/bash "$CHECK_SCRIPT" >/dev/null 2>&1; then
  echo "  FAIL: expected exit 1 with empty PATH"
  fail=$((fail + 1))
else
  echo "  PASS: exits non-zero with missing tools"
  pass=$((pass + 1))
fi

echo ""
echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
