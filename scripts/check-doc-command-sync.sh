#!/usr/bin/env bash
# Check that command snippets in README.md and CONTRIBUTING.md stay in sync.
# Extracts fenced code blocks tagged with <!-- sync-id: <id> --> markers
# and verifies identical content across files.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
README="$REPO_ROOT/README.md"
CONTRIBUTING="$REPO_ROOT/CONTRIBUTING.md"

if [ ! -f "$README" ] || [ ! -f "$CONTRIBUTING" ]; then
  echo "ERROR: README.md or CONTRIBUTING.md not found."
  exit 1
fi

# Extract sync blocks: lines between <!-- sync-id: X --> and next <!-- /sync -->
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

readme_blocks=$(extract_sync_blocks "$README")
contrib_blocks=$(extract_sync_blocks "$CONTRIBUTING")

# Get unique sync IDs from both files
readme_ids=$(echo "$readme_blocks" | cut -d'|' -f1 | sort -u)
contrib_ids=$(echo "$contrib_blocks" | cut -d'|' -f1 | sort -u)

all_ids=$(printf '%s\n%s\n' "$readme_ids" "$contrib_ids" | sort -u | grep -v '^$' || true)

if [ -z "$all_ids" ]; then
  echo "No sync markers found. Nothing to check."
  exit 0
fi

drift=0
for id in $all_ids; do
  readme_content=$(echo "$readme_blocks" | grep "^${id}|" | cut -d'|' -f2-)
  contrib_content=$(echo "$contrib_blocks" | grep "^${id}|" | cut -d'|' -f2-)

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

echo ""
if [ "$drift" -gt 0 ]; then
  echo "$drift sync block(s) drifted."
  exit 1
else
  echo "All sync blocks are aligned."
fi
