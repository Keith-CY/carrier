#!/usr/bin/env bash
# Check that issue templates and contributing docs contain an "Acceptance Criteria" heading.
# Usage: bash scripts/ci/check-acceptance-criteria.sh
set -euo pipefail

errors=0

for file in .github/ISSUE_TEMPLATE/*.md; do
  [[ -f "$file" ]] || continue
  if ! grep -qiE '^#+\s*Acceptance\s+Criteria' "$file"; then
    echo "ERROR: $file is missing an 'Acceptance Criteria' heading"
    errors=$((errors + 1))
  fi
done

if [[ "$errors" -gt 0 ]]; then
  echo "Found $errors template(s) missing Acceptance Criteria."
  exit 1
fi

echo "Acceptance Criteria heading check passed."
