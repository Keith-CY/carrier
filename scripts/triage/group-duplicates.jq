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
