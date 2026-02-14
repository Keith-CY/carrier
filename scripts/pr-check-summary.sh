#!/usr/bin/env bash
# Print a concise status summary of open PR checks (green/pending/failing).
# Usage: bash scripts/pr-check-summary.sh
set -euo pipefail

REPO="${CARRIER_REPO:-Keith-CY/carrier}"

prs=$(gh pr list --repo "$REPO" --state open --json number,headRefName,statusCheckRollup)

green=0
pending=0
failing=0
pending_list=()
failing_list=()

count=$(echo "$prs" | jq 'length')

for i in $(seq 0 $((count - 1))); do
  pr_number=$(echo "$prs" | jq -r ".[$i].number")
  checks=$(echo "$prs" | jq -r ".[$i].statusCheckRollup")

  if [ "$checks" = "null" ] || [ "$checks" = "[]" ]; then
    pending=$((pending + 1))
    pending_list+=("$pr_number")
    continue
  fi

  has_fail=false
  has_pending=false

  for conclusion in $(echo "$checks" | jq -r '.[].conclusion // "PENDING"'); do
    case "$conclusion" in
      SUCCESS|NEUTRAL|SKIPPED) ;;
      PENDING|""|null) has_pending=true ;;
      *) has_fail=true ;;
    esac
  done

  if $has_fail; then
    failing=$((failing + 1))
    failing_list+=("$pr_number")
  elif $has_pending; then
    pending=$((pending + 1))
    pending_list+=("$pr_number")
  else
    green=$((green + 1))
  fi
done

echo "Open PR Check Summary ($REPO)"
echo "=============================="
echo "  Green:   $green"
echo "  Pending: $pending"
echo "  Failing: $failing"
echo ""

if [ ${#pending_list[@]} -gt 0 ]; then
  echo "Pending PRs: ${pending_list[*]}"
fi
if [ ${#failing_list[@]} -gt 0 ]; then
  echo "Failing PRs: ${failing_list[*]}"
fi
