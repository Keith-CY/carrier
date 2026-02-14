#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

cd "$repo_root"

echo "Running end-to-end test suite..."

if command -v carrier >/dev/null 2>&1; then
  carrier test e2e --report test-results/
  exit 0
fi

echo "carrier CLI is not available in PATH."
echo "Falling back to gateway e2e smoke tests (bun test src/providers/parsers.e2e.test.ts)."

if ! command -v bun >/dev/null 2>&1; then
  echo "Error: bun is required to run fallback end-to-end tests."
  exit 1
fi

if [[ ! -d "$repo_root/gateway/node_modules" ]]; then
  echo "gateway/node_modules not found; installing gateway dependencies..."
  (
    cd "$repo_root/gateway"
    bun install --no-progress
  )
fi

(
  cd "$repo_root/gateway"
  bun test src/providers/parsers.e2e.test.ts
)
