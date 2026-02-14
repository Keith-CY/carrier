#!/usr/bin/env bash
# Run shellcheck consistently across all tracked repository shell scripts.
# Usage: bash scripts/run-shellcheck.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if ! git -C "$REPO_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "Not inside a git work tree: $REPO_ROOT"
  exit 1
fi

mapfile -t scripts < <(git -C "$REPO_ROOT" ls-files '*.sh' | sort)

if [ ${#scripts[@]} -eq 0 ]; then
  echo "No tracked shell scripts found. Nothing to check."
  exit 0
fi

echo "Running shellcheck on ${#scripts[@]} tracked script(s)..."
echo ""

errors=0
for rel in "${scripts[@]}"; do
  abs="$REPO_ROOT/$rel"
  if shellcheck "$abs"; then
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
