#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

printf '\n=== Daemon tests ===\n'
(
  cd "$repo_root/daemon"
  go test ./...
)

printf '\n=== Gateway checks and tests ===\n'
(
  cd "$repo_root/gateway"
  bun install --no-progress
  bun run check
  bun test
)

printf '\n=== End-to-end tests ===\n'
"$repo_root/scripts/run-e2e-tests.sh"
