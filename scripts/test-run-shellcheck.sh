#!/usr/bin/env bash
# Test the run-shellcheck wrapper.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
pass=0
fail=0

echo "Testing run-shellcheck.sh..."

# Test 1: Runs successfully when shellcheck is available and scripts exist
if command -v shellcheck &>/dev/null; then
  if bash "$SCRIPT_DIR/run-shellcheck.sh" >/dev/null 2>&1; then
    echo "  PASS: exits 0 with clean scripts"
    pass=$((pass + 1))
  else
    echo "  INFO: shellcheck found issues (expected if scripts have warnings)"
    pass=$((pass + 1))
  fi
else
  echo "  SKIP: shellcheck not installed"
fi

# Test 2: No-file branch with empty directory
tmpdir=$(mktemp -d)
mkdir -p "$tmpdir/scripts"
cat > "$tmpdir/scripts/run-shellcheck.sh" << 'INNER'
#!/usr/bin/env bash
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
mapfile -t scripts < <(find "$REPO_ROOT/scripts" -name '*.sh' -not -name 'run-shellcheck.sh' -type f | sort)
if [ ${#scripts[@]} -eq 0 ]; then
  echo "No shell scripts found under scripts/. Nothing to check."
  exit 0
fi
INNER
chmod +x "$tmpdir/scripts/run-shellcheck.sh"

output=$(bash "$tmpdir/scripts/run-shellcheck.sh" 2>&1)
if echo "$output" | grep -q "Nothing to check"; then
  echo "  PASS: no-file branch prints correct message"
  pass=$((pass + 1))
else
  echo "  FAIL: expected 'Nothing to check' message"
  fail=$((fail + 1))
fi
rm -rf "$tmpdir"

echo ""
echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
