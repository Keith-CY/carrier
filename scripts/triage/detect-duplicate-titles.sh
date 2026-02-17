#!/usr/bin/env bash
# Detect duplicate open issue titles (case-insensitive, normalized punctuation).
# Usage: bash scripts/triage/detect-duplicate-titles.sh
# Requires: gh CLI with authentication
set -euo pipefail

issues="$(gh issue list --state open --json number,title,url --limit 500 2>/dev/null)" || {
  echo "ERROR: failed to fetch issues via gh CLI" >&2
  exit 1
}

# Normalize and group by title
duplicates="$(echo "$issues" | python3 -c "
import json, sys, re
from collections import defaultdict

data = json.load(sys.stdin)
groups = defaultdict(list)
for issue in data:
    title = issue['title'].strip().lower()
    title = re.sub(r'[^a-z0-9 ]', ' ', title)
    title = re.sub(r'\s+', ' ', title).strip()
    groups[title].append(issue)

found = False
for title, items in sorted(groups.items()):
    if len(items) > 1:
        found = True
        print(f'Duplicate: \"{title}\"')
        for item in items:
            print(f'  #{item[\"number\"]}: {item[\"title\"]}  ({item[\"url\"]})')
        print()

if not found:
    print('No duplicates found.')
")"

echo "$duplicates"
