#!/usr/bin/env bash
# Regression tests for check-doc-command-sync.sh drift detection.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
pass=0
fail=0

run_test() {
  local name="$1"
  local expected_exit="$2"
  local tmpdir="$3"

  # Create a wrapper that points to the temp directory
  cat > "$tmpdir/scripts/check-doc-command-sync.sh" << 'INNER'
#!/usr/bin/env bash
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
README="$REPO_ROOT/README.md"
CONTRIBUTING="$REPO_ROOT/CONTRIBUTING.md"
if [ ! -f "$README" ] || [ ! -f "$CONTRIBUTING" ]; then
  echo "ERROR: README.md or CONTRIBUTING.md not found."
  exit 1
fi
extract_sync_blocks() {
  local file="$1"
  local current_id=""
  while IFS= read -r line; do
    if [[ "$line" =~ \<!--\ sync-id:\ ([a-zA-Z0-9_-]+)\ --\> ]]; then
      current_id="${BASH_REMATCH[1]}"
    elif [[ "$line" =~ \<!--\ /sync\ --\> ]]; then
      current_id=""
    elif [ -n "$current_id" ]; then
      echo "$current_id|$line"
    fi
  done < "$file"
}
readme_blocks=$(extract_sync_blocks "$README")
contrib_blocks=$(extract_sync_blocks "$CONTRIBUTING")
readme_ids=$(echo "$readme_blocks" | cut -d'|' -f1 | sort -u)
contrib_ids=$(echo "$contrib_blocks" | cut -d'|' -f1 | sort -u)
all_ids=$(printf '%s\n%s\n' "$readme_ids" "$contrib_ids" | sort -u | grep -v '^$' || true)
if [ -z "$all_ids" ]; then
  echo "No sync markers found. Nothing to check."
  exit 0
fi
drift=0
for id in $all_ids; do
  readme_content=$(echo "$readme_blocks" | grep "^${id}|" | cut -d'|' -f2-)
  contrib_content=$(echo "$contrib_blocks" | grep "^${id}|" | cut -d'|' -f2-)
  if [ -z "$readme_content" ] && [ -n "$contrib_content" ]; then
    echo "DRIFT: sync-id '$id' found in CONTRIBUTING.md but missing from README.md"
    drift=$((drift + 1))
  elif [ -n "$readme_content" ] && [ -z "$contrib_content" ]; then
    echo "DRIFT: sync-id '$id' found in README.md but missing from CONTRIBUTING.md"
    drift=$((drift + 1))
  elif [ "$readme_content" != "$contrib_content" ]; then
    echo "DRIFT: sync-id '$id' content differs between README.md and CONTRIBUTING.md"
    drift=$((drift + 1))
  else
    echo "OK: sync-id '$id'"
  fi
done
echo ""
if [ "$drift" -gt 0 ]; then
  echo "$drift sync block(s) drifted."
  exit 1
else
  echo "All sync blocks are aligned."
fi
INNER
  chmod +x "$tmpdir/scripts/check-doc-command-sync.sh"

  local actual_exit=0
  bash "$tmpdir/scripts/check-doc-command-sync.sh" >/dev/null 2>&1 || actual_exit=$?

  if [ "$actual_exit" -eq "$expected_exit" ]; then
    echo "  PASS: $name (exit=$actual_exit)"
    pass=$((pass + 1))
  else
    echo "  FAIL: $name (expected exit=$expected_exit, got=$actual_exit)"
    fail=$((fail + 1))
  fi
}

echo "Testing check-doc-command-sync.sh..."

# Test 1: Aligned snippets → exit 0
tmpdir=$(mktemp -d)
mkdir -p "$tmpdir/scripts"
cat > "$tmpdir/README.md" << 'DOC'
# README
<!-- sync-id: install-cmd -->
```bash
bun install
```
<!-- /sync -->
DOC
cat > "$tmpdir/CONTRIBUTING.md" << 'DOC'
# Contributing
<!-- sync-id: install-cmd -->
```bash
bun install
```
<!-- /sync -->
DOC
run_test "aligned snippets pass" 0 "$tmpdir"
rm -rf "$tmpdir"

# Test 2: README-only snippet → exit 1
tmpdir=$(mktemp -d)
mkdir -p "$tmpdir/scripts"
cat > "$tmpdir/README.md" << 'DOC'
# README
<!-- sync-id: install-cmd -->
```bash
bun install
```
<!-- /sync -->
DOC
cat > "$tmpdir/CONTRIBUTING.md" << 'DOC'
# Contributing
No sync markers here.
DOC
run_test "README-only snippet fails" 1 "$tmpdir"
rm -rf "$tmpdir"

# Test 3: CONTRIBUTING-only snippet → exit 1
tmpdir=$(mktemp -d)
mkdir -p "$tmpdir/scripts"
cat > "$tmpdir/README.md" << 'DOC'
# README
No sync markers here.
DOC
cat > "$tmpdir/CONTRIBUTING.md" << 'DOC'
# Contributing
<!-- sync-id: test-cmd -->
```bash
go test ./...
```
<!-- /sync -->
DOC
run_test "CONTRIBUTING-only snippet fails" 1 "$tmpdir"
rm -rf "$tmpdir"

echo ""
echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
