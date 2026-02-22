#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

cd "$repo_root"

echo "Running end-to-end test suite..."
"$repo_root/scripts/e2e-integration.sh"
