#!/bin/bash
# Test script for openclaw-installer.sh checksum verification
# All tests run the installer in a subshell so that `exit` inside the
# script does not kill the test runner.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALLER="$SCRIPT_DIR/openclaw-installer.sh"
PASS=0
FAIL=0

pass() { echo "PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

echo "Testing openclaw-installer.sh checksum verification..."

# Test 1: No checksum at all → must exit non-zero
echo "Test 1: Should fail when no checksum provided"
if env -u OPENCLAW_CHECKSUM bash "$INSTALLER" "1.0.0" >/dev/null 2>&1; then
    fail "Script should exit non-zero when no checksum provided"
else
    pass "Script rejects missing checksum"
fi

# Test 2: Checksum via $2 → script proceeds (will fail at download, which is fine)
echo "Test 2: Should accept checksum from second argument"
OUT=$(bash "$INSTALLER" "1.0.0" "abc123" 2>&1 || true)
if echo "$OUT" | grep -q "Downloading"; then
    pass "Script accepts checksum from \$2 and reaches download step"
else
    fail "Script did not reach download step with \$2 checksum"
fi

# Test 3: Checksum via OPENCLAW_CHECKSUM env var → same behaviour
echo "Test 3: Should accept checksum from OPENCLAW_CHECKSUM env var"
OUT=$(OPENCLAW_CHECKSUM="abc123" bash "$INSTALLER" "1.0.0" 2>&1 || true)
if echo "$OUT" | grep -q "Downloading"; then
    pass "Script accepts checksum from env var and reaches download step"
else
    fail "Script did not reach download step with env-var checksum"
fi

# Test 4: Error message mentions both input methods
echo "Test 4: Error message should mention OPENCLAW_CHECKSUM"
ERR=$(env -u OPENCLAW_CHECKSUM bash "$INSTALLER" "1.0.0" 2>&1 || true)
if echo "$ERR" | grep -q "OPENCLAW_CHECKSUM"; then
    pass "Error message mentions OPENCLAW_CHECKSUM"
else
    fail "Error message does not mention OPENCLAW_CHECKSUM"
fi

echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
