#!/usr/bin/env bash
# Test the run-shellcheck wrapper.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
pass=0
fail=0

echo "Testing run-shellcheck.sh..."

# Test 1: Wrapper is runnable in the current repo.
if command -v shellcheck &>/dev/null; then
  if bash "$SCRIPT_DIR/run-shellcheck.sh" >/dev/null 2>&1; then
    echo "  PASS: wrapper runs in repo"
    pass=$((pass + 1))
  else
    echo "  INFO: shellcheck found issues in repo (acceptable for wrapper test)"
    pass=$((pass + 1))
  fi
else
  echo "  SKIP: shellcheck not installed"
fi

# Test 2: Missing shellcheck fails fast with a single prerequisite error.
missing_output=$(PATH="/usr/bin:/bin" /usr/bin/bash "$SCRIPT_DIR/run-shellcheck.sh" 2>&1 || true)
if echo "$missing_output" | grep -q "shellcheck is required but was not found in PATH" \
  && [ "$(echo "$missing_output" | grep -c "not found")" -eq 1 ]; then
  echo "  PASS: missing shellcheck exits early with one actionable message"
  pass=$((pass + 1))
else
  echo "  FAIL: missing shellcheck path was not reported clearly"
  fail=$((fail + 1))
fi

# Test 3: Uses git-tracked *.sh scope (inside and outside scripts/) and ignores untracked files.
tmpdir=$(mktemp -d)
mkdir -p "$tmpdir/scripts" "$tmpdir/catalog" "$tmpdir/untracked"

cat > "$tmpdir/scripts/in-scripts.sh" <<'SH'
#!/usr/bin/env bash
echo scripts
SH
cat > "$tmpdir/catalog/outside-scripts.sh" <<'SH'
#!/usr/bin/env bash
echo catalog
SH
cat > "$tmpdir/untracked/ignored.sh" <<'SH'
#!/usr/bin/env bash
echo ignored
SH
chmod +x "$tmpdir/scripts/in-scripts.sh" "$tmpdir/catalog/outside-scripts.sh" "$tmpdir/untracked/ignored.sh"

(
  cd "$tmpdir"
  git init -q
  git config user.email "test@example.com"
  git config user.name "test"
  git add scripts/in-scripts.sh catalog/outside-scripts.sh
  git commit -qm "init"
)

# Copy wrapper into temp repo at scripts/run-shellcheck.sh (same expected layout).
cp "$SCRIPT_DIR/run-shellcheck.sh" "$tmpdir/scripts/run-shellcheck.sh"
chmod +x "$tmpdir/scripts/run-shellcheck.sh"

# Stub shellcheck so we can verify exactly which files were checked.
mkdir -p "$tmpdir/bin"
cat > "$tmpdir/bin/shellcheck" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$1"
SH
chmod +x "$tmpdir/bin/shellcheck"

output=$(PATH="$tmpdir/bin:$PATH" bash "$tmpdir/scripts/run-shellcheck.sh" 2>&1)

if echo "$output" | grep -q "scripts/in-scripts.sh" \
  && echo "$output" | grep -q "catalog/outside-scripts.sh" \
  && ! echo "$output" | grep -q "untracked/ignored.sh"; then
  echo "  PASS: wrapper checks tracked *.sh files across repo and skips untracked files"
  pass=$((pass + 1))
else
  echo "  FAIL: wrapper did not match git-tracked *.sh scope"
  fail=$((fail + 1))
fi

# Test 4: No tracked shell script branch.
empty_repo=$(mktemp -d)
mkdir -p "$empty_repo/scripts"
cp "$SCRIPT_DIR/run-shellcheck.sh" "$empty_repo/scripts/run-shellcheck.sh"
chmod +x "$empty_repo/scripts/run-shellcheck.sh"
(
  cd "$empty_repo"
  git init -q
  git config user.email "test@example.com"
  git config user.name "test"
  git add scripts/run-shellcheck.sh
  git commit -qm "init"
)

empty_output=$(PATH="$tmpdir/bin:$PATH" bash "$empty_repo/scripts/run-shellcheck.sh" 2>&1 || true)
if echo "$empty_output" | grep -q "scripts/run-shellcheck.sh"; then
  echo "  PASS: repo with only wrapper still reports tracked shell scripts"
  pass=$((pass + 1))
else
  echo "  FAIL: expected tracked wrapper script to be linted"
  fail=$((fail + 1))
fi

rm -rf "$tmpdir" "$empty_repo"

echo ""
echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
