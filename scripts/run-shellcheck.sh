#!/usr/bin/env bash
# Run shellcheck consistently across all repository shell scripts.
# Usage: bash scripts/run-shellcheck.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

mapfile -t scripts < <(find "$REPO_ROOT/scripts" -name '*.sh' -type f | sort)

if [ ${#scripts[@]} -eq 0 ]; then
  echo "No shell scripts found under scripts/. Nothing to check."
  exit 0
fi

echo "Running shellcheck on ${#scripts[@]} script(s)..."
echo ""

errors=0
for script in "${scripts[@]}"; do
  rel="${script#"$REPO_ROOT/"}"
  if shellcheck "$script"; then
    echo "  ✓ $rel"
  else
    echo "  ✗ $rel"
    errors=$((errors + 1))
  fi
done

echo ""
echo "Checked ${#scripts[@]} file(s), $errors with findings."

if [ "$errors" -gt 0 ]; then
  exit 1
fi
