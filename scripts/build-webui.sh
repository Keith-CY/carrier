#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEBUI_DIR="${ROOT_DIR}/webui"
OUT_DIR="${WEBUI_DIR}/static"

if ! command -v bun >/dev/null 2>&1; then
  echo "ERROR: bun is required to build WebUI assets." >&2
  exit 1
fi

if [[ ! -f "${WEBUI_DIR}/package.json" ]]; then
  echo "ERROR: missing ${WEBUI_DIR}/package.json" >&2
  exit 1
fi

rm -rf "${OUT_DIR}/assets"
rm -f "${OUT_DIR}/index.html"
find "${OUT_DIR}" -maxdepth 1 -type f \( -name '*.js' -o -name '*.min.js' \) -delete

(
  cd "${WEBUI_DIR}"
  if [[ ! -d node_modules ]]; then
    bun install --frozen-lockfile
  fi
  bun run build
)

echo "Built WebUI assets with Vite shell."
