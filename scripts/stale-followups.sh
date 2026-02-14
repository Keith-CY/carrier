#!/usr/bin/env bash
# Report stale review-followup issues whose referenced PR is already merged.
# Usage: ./scripts/stale-followups.sh [--days N]
#
# Options:
#   --days N   Only report issues older than N days (default: 7)
#
# Example:
#   ./scripts/stale-followups.sh --days 14

set -euo pipefail

THRESHOLD_DAYS=7

while [[ $# -gt 0 ]]; do
  case "$1" in
    --days)
      THRESHOLD_DAYS="$2"
      shift 2
      ;;
    *)
      echo "Usage: $0 [--days N]" >&2
      exit 1
      ;;
  esac
done

REPO="${GITHUB_REPOSITORY:-Keith-CY/carrier}"
THRESHOLD_DATE=$(date -u -d "-${THRESHOLD_DAYS} days" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || \
                 date -u -v-"${THRESHOLD_DAYS}"d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || \
                 echo "")

echo "Scanning for stale [review-followup] issues (older than ${THRESHOLD_DAYS} days)..."
echo ""

# Fetch open issues with [review-followup] prefix
issues=$(gh issue list --repo "$REPO" --state open --search "[review-followup] in:title" \
  --json number,title,createdAt,body --limit 100)

found=0
while IFS= read -r issue; do
  number=$(echo "$issue" | jq -r '.number')
  title=$(echo "$issue" | jq -r '.title')
  created=$(echo "$issue" | jq -r '.createdAt')
  body=$(echo "$issue" | jq -r '.body')

  # Check age threshold
  if [[ -n "$THRESHOLD_DATE" ]] && [[ "$created" > "$THRESHOLD_DATE" ]]; then
    continue
  fi

  # Extract PR number from title (e.g., "PR #47:") or body (e.g., "/pull/47")
  pr_num=$(echo "$title" | sed -n 's/.*PR #\([0-9]\{1,\}\).*/\1/p' | head -1)
  if [[ -z "$pr_num" ]]; then
    pr_num=$(echo "$body" | sed -n 's|.*/pull/\([0-9]\{1,\}\).*|\1|p' | head -1)
  fi

  if [[ -z "$pr_num" ]]; then
    continue
  fi

  # Check if the referenced PR is merged
  pr_state=$(gh pr view "$pr_num" --repo "$REPO" --json state -q '.state' 2>/dev/null || echo "UNKNOWN")

  if [[ "$pr_state" == "MERGED" ]]; then
    echo "  #${number}: ${title}"
    echo "    Created: ${created}"
    echo "    Referenced PR #${pr_num}: MERGED"
    echo ""
    found=1
  fi
done < <(echo "$issues" | jq -c '.[]')

if [[ "$found" -eq 0 ]]; then
  echo "No stale review-followup issues found."
fi
