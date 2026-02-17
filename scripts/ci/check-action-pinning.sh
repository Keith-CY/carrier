#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
workflow_dir="$repo_root/.github/workflows"

if [[ ! -d "$workflow_dir" ]]; then
  echo "No workflow directory found: $workflow_dir"
  exit 0
fi

errors=0

echo "Checking third-party GitHub Action refs are pinned to immutable SHAs..."

while IFS= read -r match; do
  file="${match%%:*}"
  rest="${match#*:}"
  line_no="${rest%%:*}"
  content="${rest#*:}"

  value="$(printf '%s' "$content" \
    | sed -E 's/^[[:space:]-]*uses:[[:space:]]*//' \
    | sed -E 's/[[:space:]]+#.*$//' \
    | tr -d '"' \
    | tr -d "'")"

  [[ -z "$value" ]] && continue
  [[ "$value" == ./* ]] && continue
  [[ "$value" == *\$\{\{* ]] && continue
  [[ "$value" != *"@"* ]] && continue

  action="${value%@*}"
  ref="${value##*@}"
  owner="${action%%/*}"

  # Only enforce immutable SHA pinning for third-party actions.
  if [[ "$owner" == "actions" ]]; then
    continue
  fi

  if [[ ! "$ref" =~ ^[0-9a-f]{40}$ ]]; then
    echo "ERROR: $file:$line_no uses mutable ref: $value"
    errors=$((errors + 1))
  fi
done < <(grep -R -nE '^[[:space:]]*-?[[:space:]]*uses:[[:space:]]*' "$workflow_dir" 2>/dev/null || true)

if [[ "$errors" -gt 0 ]]; then
  echo "Found $errors mutable third-party action ref(s)."
  exit 1
fi

echo "Action pinning check passed."
