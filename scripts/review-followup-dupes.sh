#!/usr/bin/env bash
# Detect duplicate open [review-followup] issues that reference the same PR.
# Usage: ./scripts/review-followup-dupes.sh
# Report-only — does not modify any issues.

set -euo pipefail

REPO="${GITHUB_REPOSITORY:-Keith-CY/carrier}"

echo "Scanning for duplicate [review-followup] issues..."
echo ""

issues=$(gh issue list --repo "$REPO" --state open --search "[review-followup] in:title" \
  --json number,title,body --limit 200)

# Extract PR number from each issue and group
declare -A pr_issues

while IFS= read -r issue; do
  number=$(echo "$issue" | jq -r '.number')
  title=$(echo "$issue" | jq -r '.title')
  body=$(echo "$issue" | jq -r '.body')

  pr_num=$(echo "$title" | sed -n 's/.*PR #\([0-9]\{1,\}\).*/\1/p' | head -1)
  if [[ -z "$pr_num" ]]; then
    pr_num=$(echo "$body" | sed -n 's|.*/pull/\([0-9]\{1,\}\).*|\1|p' | head -1)
  fi

  if [[ -n "$pr_num" ]]; then
    if [[ -n "${pr_issues[$pr_num]:-}" ]]; then
      pr_issues[$pr_num]="${pr_issues[$pr_num]}"$'\n'"  #${number}: ${title}"
    else
      pr_issues[$pr_num]="  #${number}: ${title}"
    fi
  fi
done < <(echo "$issues" | jq -c '.[]')

found=0
for pr_num in "${!pr_issues[@]}"; do
  count=$(echo "${pr_issues[$pr_num]}" | wc -l)
  if [[ "$count" -gt 1 ]]; then
    echo "PR #${pr_num} has ${count} follow-up issues:"
    echo "${pr_issues[$pr_num]}"
    echo ""
    found=1
  fi
done

if [[ "$found" -eq 0 ]]; then
  echo "No duplicate review-followup issues found."
fi
