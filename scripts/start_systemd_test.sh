#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
START_SCRIPT="$SCRIPT_DIR/start.sh"
PASS=0
FAIL=0

pass() { echo "PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

TMP_BIN_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_BIN_DIR"' EXIT

cat > "$TMP_BIN_DIR/openclaw" <<'EOF'
#!/bin/sh
echo "mock openclaw $*"
EOF
chmod +x "$TMP_BIN_DIR/openclaw"

run_start() {
  PATH="$TMP_BIN_DIR:$PATH" DRY_RUN=1 "$START_SCRIPT" "$@"
}

echo "Testing scripts/start.sh systemd unit targets..."

OUT_USER="$(run_start --systemd --user 2>&1 || true)"
if echo "$OUT_USER" | grep -q 'WantedBy=default.target'; then
  pass "--systemd --user emits WantedBy=default.target"
else
  fail "--systemd --user did not emit WantedBy=default.target"
fi

if echo "$OUT_USER" | grep -q '\[DRY_RUN\] systemctl --user enable openclaw-gateway.service' && \
   echo "$OUT_USER" | grep -q '\[DRY_RUN\] systemctl --user start openclaw-gateway.service'; then
  pass "--systemd --user dry-run shows enable/start sequence"
else
  fail "--systemd --user dry-run missing enable/start sequence"
fi

OUT_SYSTEM="$(run_start --systemd 2>&1 || true)"
if echo "$OUT_SYSTEM" | grep -q 'WantedBy=multi-user.target'; then
  pass "--systemd emits WantedBy=multi-user.target"
else
  fail "--systemd did not emit WantedBy=multi-user.target"
fi

if echo "$OUT_SYSTEM" | grep -q '\[DRY_RUN\] systemctl enable openclaw-gateway.service' && \
   echo "$OUT_SYSTEM" | grep -q '\[DRY_RUN\] systemctl start openclaw-gateway.service'; then
  pass "--systemd dry-run shows enable/start sequence"
else
  fail "--systemd dry-run missing enable/start sequence"
fi

echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
