#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
README="$REPO_ROOT/README.md"
CONTRIBUTING="$REPO_ROOT/CONTRIBUTING.md"
CLI_DOC="$REPO_ROOT/docs/carrier-cli.md"
MAIN_GO="$REPO_ROOT/cmd/carrier/main.go"

if [ ! -f "$README" ] || [ ! -f "$CONTRIBUTING" ] || [ ! -f "$CLI_DOC" ] || [ ! -f "$MAIN_GO" ]; then
  echo "ERROR: required files missing (README.md, CONTRIBUTING.md, docs/carrier-cli.md, cmd/carrier/main.go)."
  exit 1
fi

extract_sync_blocks() {
  local file="$1"
  local current_id=""
  while IFS= read -r line; do
    if [[ "$line" =~ \<!--\ sync-id:\ ([a-zA-Z0-9_-]+)\ --\> ]]; then
      current_id="${BASH_REMATCH[1]}"
    elif [[ "$line" =~ \<!--\ /sync\ --\> ]]; then
      current_id=""
    elif [ -n "$current_id" ]; then
      echo "$current_id|$line"
    fi
  done < "$file"
}

check_sync_markers() {
  local readme_blocks contrib_blocks readme_ids contrib_ids all_ids
  readme_blocks="$(extract_sync_blocks "$README" || true)"
  contrib_blocks="$(extract_sync_blocks "$CONTRIBUTING" || true)"

  readme_ids="$(echo "$readme_blocks" | cut -d'|' -f1 | sort -u 2>/dev/null || true)"
  contrib_ids="$(echo "$contrib_blocks" | cut -d'|' -f1 | sort -u 2>/dev/null || true)"
  all_ids="$(printf '%s\n%s\n' "$readme_ids" "$contrib_ids" | sort -u | grep -v '^$' || true)"

  if [ -z "$all_ids" ]; then
    echo "No sync markers found. Skipping marker drift check."
    return 0
  fi

  local drift=0
  for id in $all_ids; do
    local readme_content contrib_content
    readme_content="$(echo "$readme_blocks" | grep "^${id}|" | cut -d'|' -f2- || true)"
    contrib_content="$(echo "$contrib_blocks" | grep "^${id}|" | cut -d'|' -f2- || true)"

    if [ -z "$readme_content" ] && [ -n "$contrib_content" ]; then
      echo "DRIFT: sync-id '$id' found in CONTRIBUTING.md but missing from README.md"
      drift=$((drift + 1))
    elif [ -n "$readme_content" ] && [ -z "$contrib_content" ]; then
      echo "DRIFT: sync-id '$id' found in README.md but missing from CONTRIBUTING.md"
      drift=$((drift + 1))
    elif [ "$readme_content" != "$contrib_content" ]; then
      echo "DRIFT: sync-id '$id' content differs between README.md and CONTRIBUTING.md"
      drift=$((drift + 1))
    else
      echo "OK: sync-id '$id'"
    fi
  done

  if [ "$drift" -gt 0 ]; then
    echo "Marker drift found: $drift block(s)."
    return 1
  fi
  return 0
}

normalize_spaces() {
  awk '{$1=$1; print}'
}

command_signature() {
  local command="$1"
  read -r -a parts <<< "$command"
  local out=()
  local index=0
  local token
  for token in "${parts[@]}"; do
    [ -z "$token" ] && continue
    if [[ "$token" == "<"* || "$token" == "["* ]]; then
      break
    fi
    if [[ "$token" == --* || "$token" == -* ]]; then
      if [ "$index" -ge 2 ]; then
        break
      fi
    fi
    out+=("$token")
    index=$((index + 1))
    if [ "${#out[@]}" -ge 6 ]; then
      break
    fi
  done
  if [ "${#out[@]}" -eq 0 ]; then
    return 0
  fi
  printf '%s\n' "${out[*]}"
}

extract_usage_commands() {
  awk '
    /^const usage = `/ {in_usage=1; next}
    in_usage && /^`$/ {in_usage=0; next}
    in_usage && /^  carrier/ {
      line=$0
      sub(/^  /, "", line)
      sub(/[[:space:]]{2,}.*/, "", line)
      print line
    }
  ' "$MAIN_GO"
}

extract_doc_commands() {
  grep -hoE '`carrier[^`]*`' "$README" "$CLI_DOC" | tr -d '`'
}

build_doc_prefixes() {
  local command="$1"
  read -r -a parts <<< "$command"
  local prefix=()
  local i token
  for ((i = 0; i < ${#parts[@]} && i < 6; i++)); do
    token="${parts[$i]}"
    if [[ "$token" == "<"* || "$token" == "["* ]]; then
      break
    fi
    prefix+=("$token")
    printf '%s\n' "${prefix[*]}"
  done
}

check_command_surface() {
  local required_file observed_file missing=0
  required_file="$(mktemp)"
  observed_file="$(mktemp)"

  while IFS= read -r line; do
    line="$(printf '%s\n' "$line" | normalize_spaces)"
    [ -z "$line" ] && continue
    command_signature "$line"
  done < <(extract_usage_commands) | sort -u > "$required_file"

  while IFS= read -r line; do
    line="$(printf '%s\n' "$line" | normalize_spaces)"
    [ -z "$line" ] && continue
    build_doc_prefixes "$line"
  done < <(extract_doc_commands) | sort -u > "$observed_file"

  echo "Checking command surface coverage..."
  while IFS= read -r required; do
    if [ -z "$required" ]; then
      continue
    fi
    if ! grep -Fxq "$required" "$observed_file"; then
      echo "MISSING: '$required' not found in README/docs command references"
      missing=$((missing + 1))
    fi
  done < "$required_file"

  if [ "$missing" -gt 0 ]; then
    echo "Command surface drift found: $missing missing command signature(s)."
    rm -f "$required_file" "$observed_file"
    return 1
  fi
  echo "Command surface coverage is aligned with docs."
  rm -f "$required_file" "$observed_file"
  return 0
}

marker_status=0
surface_status=0

check_sync_markers || marker_status=$?
check_command_surface || surface_status=$?

echo ""
if [ "$marker_status" -ne 0 ] || [ "$surface_status" -ne 0 ]; then
  echo "Documentation sync checks failed."
  exit 1
fi
echo "All documentation sync checks passed."
