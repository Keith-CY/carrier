#!/usr/bin/env bash
set -euo pipefail

repo="${1:-Keith-CY/carrier}"
limit="${LIMIT:-200}"

if ! command -v gh >/dev/null 2>&1; then
  echo "Error: gh CLI is required." >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq is required." >&2
  exit 1
fi

issues_json="$(gh issue list --repo "$repo" --state open --limit "$limit" --json number,title,createdAt,labels,assignees)"

rows="$(printf '%s' "$issues_json" | jq -c '
  [ .[]
    | select((.assignees | length) == 0)
    | (if ([.labels[].name] | index("P0")) != null then "P0"
       elif ([.labels[].name] | index("P1")) != null then "P1"
       else "none" end) as $priority
    | (((now - (.createdAt | fromdateiso8601)) / 86400) | floor) as $age_days
    | (if $age_days < 7 then "<7d"
       elif $age_days <= 30 then "7-30d"
       else ">30d" end) as $age_bucket
    | {
        number,
        title,
        priority: $priority,
        age_days,
        age_bucket
      }
  ]
')"

count="$(printf '%s' "$rows" | jq 'length')"

echo "Unassigned Open Issue Report"
echo "Repository: $repo"
echo "Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo

if [[ "$count" -eq 0 ]]; then
  echo "No unassigned open issues found."
  exit 0
fi

for priority in P0 P1 none; do
  echo "## Priority: $priority"
  for bucket in "<7d" "7-30d" ">30d"; do
    echo "### Age: $bucket"
    lines="$(printf '%s' "$rows" | jq -r --arg p "$priority" --arg b "$bucket" '
      .[]
      | select(.priority == $p and .age_bucket == $b)
      | "- #\(.number) [\(.priority)] (\(.age_days)d) \(.title)"
    ')"
    if [[ -z "$lines" ]]; then
      echo "- (none)"
    else
      printf '%s\n' "$lines"
    fi
    echo
  done
done
