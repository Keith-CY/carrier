#!/usr/bin/env bash
# Verify required local tooling before running repo automation scripts.
# Usage: ./scripts/preflight-check.sh

set -euo pipefail

errors=0

check_tool() {
  local tool="$1"
  local install_hint="$2"
  if command -v "$tool" &>/dev/null; then
    echo "  ✅ $tool ($(command -v "$tool"))"
  else
    echo "  ❌ $tool — not found"
    echo "     Install: $install_hint"
    errors=$((errors + 1))
  fi
}

echo "Preflight check: verifying required tools..."
echo ""

check_tool "gh" "https://cli.github.com/ or: brew install gh"
check_tool "jq" "https://jqlang.github.io/jq/ or: brew install jq / apt install jq"
check_tool "bun" "https://bun.sh/ or: curl -fsSL https://bun.sh/install | bash"
check_tool "go" "https://go.dev/dl/ or: brew install go"

echo ""

if [[ "$errors" -gt 0 ]]; then
  echo "❌ $errors tool(s) missing. Install them and re-run."
  exit 1
else
  echo "✅ All required tools are present."
  exit 0
fi
