#!/usr/bin/env bash
# Validate required local tools before running repository scripts.
# Usage: bash scripts/check-tools.sh
set -euo pipefail

missing=0

check_tool() {
  local tool="$1"
  local required="$2"
  local hint="$3"

  if command -v "$tool" &>/dev/null; then
    version=$("$tool" --version 2>&1 | head -1)
    echo "  ✓ $tool — $version"
  else
    if [ "$required" = "required" ]; then
      echo "  ✗ $tool — MISSING (required)"
      echo "    Install: $hint"
      missing=$((missing + 1))
    else
      echo "  - $tool — not found (optional)"
      echo "    Install: $hint"
    fi
  fi
}

echo "Checking local tools..."
echo ""

check_tool "bash" "required" "Pre-installed on most systems; update via package manager"
check_tool "gh" "required" "https://cli.github.com/ or: brew install gh"
check_tool "jq" "required" "https://jqlang.github.io/jq/ or: apt install jq / brew install jq"
check_tool "go" "required" "https://go.dev/dl/ — match version in daemon/go.mod"
check_tool "bun" "required" "https://bun.sh/ or: curl -fsSL https://bun.sh/install | bash"
check_tool "shellcheck" "optional" "https://www.shellcheck.net/ or: apt install shellcheck / brew install shellcheck"

echo ""
if [ "$missing" -gt 0 ]; then
  echo "ERROR: $missing required tool(s) missing. Install them before continuing."
  exit 1
else
  echo "All required tools are available."
fi
