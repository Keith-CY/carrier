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

(
  cd "${WEBUI_DIR}"
  if [[ ! -d node_modules ]]; then
    bun install --frozen-lockfile
  fi
  bun run build
)
rm -f \
  "${OUT_DIR}/app.js" \
  "${OUT_DIR}/react.production.min.js" \
  "${OUT_DIR}/react-dom.production.min.js" \
  "${OUT_DIR}/remote-control-islands.js" \
  "${OUT_DIR}/remote-chat-island.js" \
  "${OUT_DIR}/remote-observability-island.js"

echo "Built WebUI assets with Vite shell."
