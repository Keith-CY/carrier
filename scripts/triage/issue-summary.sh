#!/usr/bin/env bash
set -euo pipefail

limit="${ISSUE_SUMMARY_LIMIT:-500}"

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Error: required command not found: $cmd" >&2
    exit 1
  fi
}

require_cmd gh
require_cmd jq

if ! gh auth status >/dev/null 2>&1; then
  echo "Error: gh is not authenticated. Run: gh auth login" >&2
  exit 1
fi

issues_json="$(gh issue list --state open --limit "$limit" --json number,assignees,labels)"
total_open="$(printf '%s' "$issues_json" | jq 'length')"
unassigned_open="$(printf '%s' "$issues_json" | jq '[.[] | select((.assignees | length) == 0)] | length')"
top_assignees="$(
  printf '%s' "$issues_json" | jq -r '
    [ .[] | .assignees[]?.login ]
    | group_by(.)
    | map({name: .[0], count: length})
    | sort_by(-.count, .name)
    | .[:10]
    | .[]
    | "\(.count)\t\(.name)"
  '
)"
top_labels="$(
  printf '%s' "$issues_json" | jq -r '
    [ .[] | .labels[]?.name ]
    | group_by(.)
    | map({name: .[0], count: length})
    | sort_by(-.count, .name)
    | .[:10]
    | .[]
    | "\(.count)\t\(.name)"
  '
)"

echo "Issue Summary (open issues)"
echo "Generated (UTC): $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "Scan limit: $limit"
echo
echo "=== Total Open Issues ==="
echo "$total_open"
echo
echo "=== Open Unassigned Issues ==="
echo "$unassigned_open"
echo
echo "=== Top Assignees by Open Count ==="
if [[ -n "$top_assignees" ]]; then
  printf "%-7s %s\n" "count" "assignee"
  while IFS=$'\t' read -r count assignee; do
    printf "%-7s %s\n" "$count" "$assignee"
  done <<<"$top_assignees"
else
  echo "(none)"
fi
echo
echo "=== Top Labels by Open Count ==="
if [[ -n "$top_labels" ]]; then
  printf "%-7s %s\n" "count" "label"
  while IFS=$'\t' read -r count label; do
    printf "%-7s %s\n" "$count" "$label"
  done <<<"$top_labels"
else
  echo "(none)"
fi
