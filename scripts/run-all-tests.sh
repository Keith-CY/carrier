#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

printf '\n=== Daemon tests ===\n'
(
  cd "$repo_root/daemon"
  go test ./...
)

printf '\n=== Gateway tests ===\n'
(
  cd "$repo_root/daemon"
  go test ./internal/gateway/...
)

printf '\n=== Start script systemd regression test ===\n'
"$repo_root/scripts/start_systemd_test.sh"

printf '\n=== End-to-end tests ===\n'
"$repo_root/scripts/run-e2e-tests.sh"
