#!/usr/bin/env bash
set -euo pipefail

cd /workspace

mkdir -p "${CARRIER_VISUAL_ACCEPTANCE_DIR%/screenshots}" "$CARRIER_VISUAL_ACCEPTANCE_DIR"

if [[ ! -d /workspace/webui/e2e/node_modules || ! -x /workspace/webui/e2e/node_modules/.bin/playwright ]]; then
  echo "[visual-acceptance] Installing WebUI E2E dependencies..."
  (cd /workspace/webui/e2e && npm ci)
fi

echo "[visual-acceptance] Starting local control plane and running screenshot spec..."
bash /workspace/scripts/e2e-control-plane-bootstrap.sh bash -lc '
  set -euo pipefail
  cd /workspace/webui/e2e
  bunx playwright test tests/fullstack-visual-acceptance.spec.ts -c playwright.fullstack.config.ts
'
