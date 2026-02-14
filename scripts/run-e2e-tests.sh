#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

cd "$repo_root"

echo "Running end-to-end test suite..."

if ! command -v carrier >/dev/null 2>&1; then
  echo "Error: carrier CLI is not available in PATH."
  echo "Expected e2e command: carrier test e2e --report test-results/"

  if [[ "${CI:-}" == "true" ]]; then
    echo "CI mode: failing fast to avoid silently skipping end-to-end validation."
    echo "Install/configure carrier CLI in CI before running this workflow."
    echo "Hint: see CONTRIBUTING.md for carrier CLI setup guidance."
    exit 1
  fi

  echo "Install/configure the carrier CLI before running e2e checks locally."
  echo "Hint: see CONTRIBUTING.md for carrier CLI setup guidance."
  exit 1
fi

carrier test e2e --report test-results/
