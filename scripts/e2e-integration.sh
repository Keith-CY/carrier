#!/usr/bin/env bash
# End-to-end integration test: boots daemon + gateway and exercises full command flow.
# Usage: bash scripts/e2e-integration.sh
# Requires: Go toolchain, Bun
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cleanup_pids=()

cleanup() {
  echo "[e2e] Cleaning up..."
  for pid in "${cleanup_pids[@]}"; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT

# Find available port
find_port() {
  python3 -c "import socket; s=socket.socket(); s.bind(('',0)); print(s.getsockname()[1]); s.close()"
}

DAEMON_PORT=$(find_port)
GATEWAY_PORT=$(find_port)
PAIR_CODE="e2e-test-$(date +%s)"
TMPDIR="$(mktemp -d)"

echo "[e2e] Daemon port: $DAEMON_PORT, Gateway port: $GATEWAY_PORT"
echo "[e2e] Temp dir: $TMPDIR"

# Build and start daemon
echo "[e2e] Building daemon..."
cd "$repo_root/daemon"
go build -o "$TMPDIR/agentd" ./cmd/agentd

echo "[e2e] Starting daemon on port $DAEMON_PORT..."
CARRIER_SERVER_PORT="$DAEMON_PORT" \
CARRIER_SERVER_HOST="127.0.0.1" \
CARRIER_DEV_MODE=1 \
  "$TMPDIR/agentd" &
DAEMON_PID=$!
cleanup_pids+=("$DAEMON_PID")

# Wait for daemon to be ready
echo "[e2e] Waiting for daemon..."
for i in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:$DAEMON_PORT/healthz" >/dev/null 2>&1; then
    echo "[e2e] Daemon is ready"
    break
  fi
  if [[ $i -eq 30 ]]; then
    echo "[e2e] ERROR: Daemon failed to start" >&2
    exit 1
  fi
  sleep 0.5
done

# Install gateway deps and start gateway
echo "[e2e] Starting gateway on port $GATEWAY_PORT..."
cd "$repo_root/gateway"
if [[ ! -d node_modules ]]; then
  bun install --frozen-lockfile --no-progress
fi

CARRIER_GATEWAY_PORT="$GATEWAY_PORT" \
CARRIER_GATEWAY_HOST="127.0.0.1" \
CARRIER_DAEMON_URL="http://127.0.0.1:$DAEMON_PORT" \
SESSION_DATA_DIR="$TMPDIR" \
  bun run src/server.ts &
GATEWAY_PID=$!
cleanup_pids+=("$GATEWAY_PID")

# Wait for gateway
echo "[e2e] Waiting for gateway..."
for i in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:$GATEWAY_PORT/healthz" >/dev/null 2>&1; then
    echo "[e2e] Gateway is ready"
    break
  fi
  if [[ $i -eq 30 ]]; then
    echo "[e2e] ERROR: Gateway failed to start" >&2
    exit 1
  fi
  sleep 0.5
done

# Helper to send commands
send_cmd() {
  local input="$1"
  curl -sf -X POST "http://127.0.0.1:$GATEWAY_PORT/command" \
    -H "Content-Type: application/json" \
    -d "{\"input\": \"$input\"}"
}

ERRORS=0
assert_ok() {
  local desc="$1"
  local response="$2"
  if echo "$response" | grep -q '"result":"ok"'; then
    echo "[e2e] PASS: $desc"
  else
    echo "[e2e] FAIL: $desc"
    echo "  Response: $response"
    ERRORS=$((ERRORS + 1))
  fi
}

assert_error() {
  local desc="$1"
  local response="$2"
  if echo "$response" | grep -q '"result":"error"'; then
    echo "[e2e] PASS: $desc (expected error)"
  else
    echo "[e2e] FAIL: $desc (expected error, got ok)"
    echo "  Response: $response"
    ERRORS=$((ERRORS + 1))
  fi
}

echo ""
echo "[e2e] === Running E2E Tests ==="

# Register a pairing code with the daemon
echo "[e2e] Registering pairing code with daemon..."
curl -sf -X POST "http://127.0.0.1:$DAEMON_PORT/api/v1/pairing/codes" \
  -H "Content-Type: application/json" \
  -d "{\"code\": \"$PAIR_CODE\", \"ttlSeconds\": 300}" >/dev/null

# Test: /pair
RESP=$(send_cmd "telegram test-chat req-1 /pair $PAIR_CODE")
assert_ok "/pair" "$RESP"
SESSION_TOKEN=$(echo "$RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('sessionToken',''))" 2>/dev/null || echo "")

if [[ -z "$SESSION_TOKEN" ]]; then
  echo "[e2e] ERROR: No session token received from /pair" >&2
  exit 1
fi
echo "[e2e] Session token: ${SESSION_TOKEN:0:20}..."

# Test: /agents
RESP=$(send_cmd "telegram test-chat req-2 $SESSION_TOKEN /agents")
assert_ok "/agents" "$RESP"

# Test: /status
RESP=$(send_cmd "telegram test-chat req-3 $SESSION_TOKEN /status")
assert_ok "/status" "$RESP"

# Test: /install (openclaw agent)
RESP=$(send_cmd "telegram test-chat req-4 $SESSION_TOKEN /install openclaw")
assert_ok "/install openclaw" "$RESP"

# Test: /status after install
RESP=$(send_cmd "telegram test-chat req-5 $SESSION_TOKEN /status openclaw")
assert_ok "/status after install" "$RESP"

# Test: /logs
RESP=$(send_cmd "telegram test-chat req-6 $SESSION_TOKEN /logs")
assert_ok "/logs" "$RESP"

# Test: unknown command (should error)
RESP=$(send_cmd "telegram test-chat req-7 $SESSION_TOKEN /unknown")
assert_error "unknown command" "$RESP"

echo ""
if [[ $ERRORS -gt 0 ]]; then
  echo "[e2e] $ERRORS test(s) failed!"
  exit 1
else
  echo "[e2e] All tests passed!"
fi
