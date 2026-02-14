#!/usr/bin/env bash
# Run ShellCheck on all repository shell scripts for local CI parity.
# Usage: ./scripts/run-shellcheck.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

mapfile -t SCRIPTS < <(find "$REPO_ROOT/scripts" -name '*.sh' -type f | sort)

if [[ ${#SCRIPTS[@]} -eq 0 ]]; then
  echo "No shell scripts found under scripts/."
  exit 0
fi

echo "Running ShellCheck on ${#SCRIPTS[@]} script(s)..."

FAIL=0
for s in "${SCRIPTS[@]}"; do
  if ! shellcheck "$s"; then
    FAIL=1
  fi
done

echo "Checked ${#SCRIPTS[@]} file(s)."

if [[ $FAIL -ne 0 ]]; then
  echo "ShellCheck found errors."
  exit 1
fi

echo "All scripts passed."
