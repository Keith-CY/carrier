#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="${ROOT_DIR}/webui/src"
OUT_DIR="${ROOT_DIR}/webui/static"

if ! command -v bun >/dev/null 2>&1; then
  echo "ERROR: bun is required to build WebUI assets." >&2
  exit 1
fi

entries=(
  "app"
  "remote-control-islands"
  "remote-chat-island"
  "remote-observability-island"
)

for entry in "${entries[@]}"; do
  src_file="${SRC_DIR}/${entry}.ts"
  out_file="${OUT_DIR}/${entry}.js"
  if [[ ! -f "${src_file}" ]]; then
    echo "ERROR: missing source file ${src_file}" >&2
    exit 1
  fi

  bun build "${src_file}" \
    --target browser \
    --outfile "${out_file}"

done

echo "Built WebUI assets from TypeScript sources."
