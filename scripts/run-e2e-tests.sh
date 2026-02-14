#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

cd "$repo_root"

echo "Running end-to-end test suite..."

if ! command -v carrier >/dev/null 2>&1; then
  if [[ "${CI:-}" == "true" ]]; then
    echo "Skipping E2E: carrier CLI is not available in CI PATH."
    echo "If CLI is expected in CI, install/configure it before running this workflow."
    exit 0
  fi

  echo "Error: carrier CLI is not available in PATH."
  echo "Expected e2e command: carrier test e2e --report test-results/"
  echo "Install/configure the carrier CLI before running e2e checks locally."
  exit 1
fi

carrier test e2e --report test-results/
