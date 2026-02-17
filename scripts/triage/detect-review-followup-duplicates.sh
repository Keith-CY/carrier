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

groups_json="$(
  printf '%s' "$issues_json" | jq '
    def trim:
      gsub("^\\s+|\\s+$"; "");
    def normalize:
      ascii_downcase
      | gsub("`"; " ")
      | gsub("\\[[^\\]]*\\]\\([^\\)]*\\)"; " ")
      | gsub("[^a-z0-9]+"; " ")
      | gsub("\\s+"; " ")
      | trim;
    def title_core:
      (.title // "")
      | sub("(?i)^\\[review-followup\\]\\s*"; "")
      | sub("(?i)^pr\\s*#[0-9]+\\s*:\\s*"; "")
      | trim;
    def suggestion_text:
      (.body // "")
      | split("## Suggestion")
      | if length > 1 then .[1] else "" end
      | split("\n## ")
      | .[0]
      | gsub("(?m)^- \\[ \\]\\s*"; "")
      | trim;
    def nbs_marker:
      try ((.body // "") | capture("<!--\\s*(?<key>nbs:[^>]+)\\s*-->").key) catch "";

    map(select(((.title // "") | ascii_downcase) | contains("review-followup")))
    | map(
        . + {
          source_pr: (try ((.title // "") | capture("(?i)PR\\s*#(?<pr>[0-9]+)").pr) catch ""),
          nbs_key: nbs_marker,
          suggestion_raw: suggestion_text,
          suggestion_key: (suggestion_text | normalize),
          title_key: (title_core | normalize)
        }
      )
    | map(
        . + {
          criterion: (
            if .nbs_key != "" then "nbs_marker"
            elif .suggestion_key != "" then "normalized_suggestion"
            else "normalized_title"
            end
          ),
          duplicate_key: (
            if .nbs_key != "" then .nbs_key
            elif .suggestion_key != "" then .suggestion_key
            else .title_key
            end
          )
        }
      )
    | sort_by([.criterion, .duplicate_key, .number])
    | group_by(.criterion + "::" + .duplicate_key)
    | map(select(length > 1))
    | map({
        criterion: .[0].criterion,
        match_key: .[0].duplicate_key,
        count: length,
        issues: map({
          number,
          title,
          url,
          source_pr,
          snippet: (
            if .suggestion_raw != "" then .suggestion_raw else .title end
            | if length > 180 then (.[:177] + "...") else . end
          )
        })
      })
    | sort_by([-.count, .criterion, .match_key])
  '
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
