#!/usr/bin/env bash
set -euo pipefail

state="${ISSUE_DUP_STATE:-open}"   # open | closed | all
limit="${ISSUE_DUP_LIMIT:-500}"

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

issues_json="$(
  gh issue list \
    --state "$state" \
    --search "review-followup in:title" \
    --limit "$limit" \
    --json number,title,url,body
)"

script_dir="$(cd "$(dirname "$0")" && pwd)"
groups_json="$(
  printf '%s' "$issues_json" | jq -f "$script_dir/group-duplicates.jq"
)"

echo "Review-Followup Duplicate Candidates"
echo "Generated (UTC): $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "State: $state"
echo "Scan limit: $limit"
echo
echo "Matching priority:"
echo "1) nbs_marker (exact hidden marker in issue body comment)"
echo "2) normalized_suggestion (from ## Suggestion block)"
echo "3) normalized_title (title after stripping [review-followup] and PR prefix)"
echo

group_count="$(printf '%s' "$groups_json" | jq 'length')"
if [[ "$group_count" -eq 0 ]]; then
  echo "No potential duplicates found."
  exit 0
fi

printf '%s' "$groups_json" | jq -r '
  to_entries[]
  | "Group \(.key + 1) (\(.value.count) issues)\n"
    + "criterion: \(.value.criterion)\n"
    + "match_key: \(.value.match_key)\n"
    + (
      .value.issues
      | map(
          "- #\(.number) (PR #\((if .source_pr != \"\" then .source_pr else \"?\" end))): \(.title)\n  \(.url)\n  snippet: \(.snippet)"
        )
      | join("\n")
    )
    + "\n"
'
