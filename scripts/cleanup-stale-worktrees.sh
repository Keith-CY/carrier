#!/usr/bin/env bash
set -euo pipefail

# Remove stale git worktrees under /tmp older than a threshold.
# Default threshold: 3 hours (180 minutes)
#
# Usage:
#   scripts/cleanup-stale-worktrees.sh
#   scripts/cleanup-stale-worktrees.sh --minutes 240
#   scripts/cleanup-stale-worktrees.sh --dry-run

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
THRESHOLD_MIN=180
DRY_RUN=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --minutes)
      THRESHOLD_MIN="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

THRESHOLD_SEC=$((THRESHOLD_MIN * 60))
NOW_EPOCH="$(date +%s)"

current_path=""
current_branch=""

declare -a REMOVE_ITEMS=()
declare -a KEEP_ITEMS=()

flush_entry() {
  if [[ -z "$current_path" ]]; then
    return
  fi

  # Keep the main worktree always.
  if [[ "$current_path" == "$REPO_ROOT" ]]; then
    KEEP_ITEMS+=("$current_path [$current_branch] keep: main worktree")
    current_path=""
    current_branch=""
    return
  fi

  # Scope safety: only clean temporary worktrees.
  if [[ "$current_path" != /tmp/* ]]; then
    KEEP_ITEMS+=("$current_path [$current_branch] keep: outside /tmp")
    current_path=""
    current_branch=""
    return
  fi

  if [[ ! -d "$current_path" ]]; then
    KEEP_ITEMS+=("$current_path [$current_branch] keep: path missing")
    current_path=""
    current_branch=""
    return
  fi

  # Last commit timestamp in this worktree.
  commit_ts="$(git -C "$current_path" log -1 --format=%ct 2>/dev/null || echo 0)"

  # Last file modification timestamp (excluding .git metadata), shallow scan for speed.
  # Use python3 for portability across GNU/BSD environments.
  fs_ts="$(python3 - "$current_path" <<'PY'
import os
import sys

root = sys.argv[1]
max_depth = 3
best = 0

for dirpath, dirnames, filenames in os.walk(root, topdown=True):
    rel = os.path.relpath(dirpath, root)
    depth = 0 if rel == '.' else rel.count(os.sep) + 1

    dirnames[:] = [d for d in dirnames if d != '.git']
    if depth >= max_depth:
        dirnames[:] = []

    for name in filenames:
        path = os.path.join(dirpath, name)
        try:
            mtime = int(os.path.getmtime(path))
        except OSError:
            continue
        if mtime > best:
            best = mtime

print(best)
PY
  )"
  fs_ts="${fs_ts:-0}"

  recent_ts="$commit_ts"
  if (( fs_ts > recent_ts )); then
    recent_ts="$fs_ts"
  fi

  age_sec=$((NOW_EPOCH - recent_ts))
  age_min=$((age_sec / 60))

  if (( age_sec > THRESHOLD_SEC )); then
    REMOVE_ITEMS+=("$current_path [$current_branch] age~${age_min}m")
  else
    KEEP_ITEMS+=("$current_path [$current_branch] keep: recent ${age_min}m")
  fi

  current_path=""
  current_branch=""
}

while IFS= read -r line; do
  if [[ -z "$line" ]]; then
    flush_entry
    continue
  fi

  case "$line" in
    worktree\ *)
      current_path="${line#worktree }"
      ;;
    branch\ refs/heads/*)
      current_branch="${line#branch refs/heads/}"
      ;;
  esac
done < <(git -C "$REPO_ROOT" worktree list --porcelain)

flush_entry

echo "Threshold: ${THRESHOLD_MIN} minutes"
echo

echo "Candidates to remove (${#REMOVE_ITEMS[@]}):"
for item in "${REMOVE_ITEMS[@]}"; do
  echo "- $item"
done

echo
echo "Kept (${#KEEP_ITEMS[@]}):"
for item in "${KEEP_ITEMS[@]}"; do
  echo "- $item"
done

if [[ "$DRY_RUN" == true ]]; then
  echo
  echo "Dry run only. No worktree removed."
  exit 0
fi

for item in "${REMOVE_ITEMS[@]}"; do
  path="${item%% \[*}"
  git -C "$REPO_ROOT" worktree remove --force "$path"
done

echo
echo "Done. Removed ${#REMOVE_ITEMS[@]} stale worktree(s)."
