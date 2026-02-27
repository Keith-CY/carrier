#!/usr/bin/env bash
# End-to-end integration test: boots carrier daemon + gateway and exercises full command flow.
# Usage: bash scripts/e2e-integration.sh
# Requires: Go toolchain, curl, python3
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
TMPDIR="$(mktemp -d)"

echo "[e2e] Daemon port: $DAEMON_PORT, Gateway port: $GATEWAY_PORT"
echo "[e2e] Temp dir: $TMPDIR"

# Build carrier binary
echo "[e2e] Building carrier binary..."
cd "$repo_root"
go build -o "$TMPDIR/carrier" ./cmd/carrier

echo "[e2e] Starting daemon on port $DAEMON_PORT..."
CARRIER_SERVER_PORT="$DAEMON_PORT" \
CARRIER_SERVER_HOST="127.0.0.1" \
CARRIER_DEV_MODE=1 \
  "$TMPDIR/carrier" daemon &
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

# Start gateway
echo "[e2e] Starting gateway on port $GATEWAY_PORT..."
CARRIER_GATEWAY_PORT="$GATEWAY_PORT" \
CARRIER_GATEWAY_HOST="127.0.0.1" \
CARRIER_DAEMON_BASE_URL="http://127.0.0.1:$DAEMON_PORT" \
SESSION_DATA_DIR="$TMPDIR" \
  "$TMPDIR/carrier" gateway &
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
  local session_token="${2:-}"
  if [[ -n "$session_token" ]]; then
    curl -sf -X POST "http://127.0.0.1:$GATEWAY_PORT/command" \
      -H "Content-Type: application/json" \
      -d "{\"input\": \"$input\", \"sessionToken\": \"$session_token\"}"
    return
  fi
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

assert_error_code() {
  local desc="$1"
  local response="$2"
  local expected_code="$3"
  if echo "$response" | grep -q '"result":"error"' && echo "$response" | grep -q "\"errorCode\":\"$expected_code\""; then
    echo "[e2e] PASS: $desc (expected error code: $expected_code)"
  else
    echo "[e2e] FAIL: $desc"
    echo "  Expected error code: $expected_code"
    echo "  Response: $response"
    ERRORS=$((ERRORS + 1))
  fi
}

echo ""
echo "[e2e] === Running E2E Tests ==="

# Read current pairing code from daemon
echo "[e2e] Fetching pairing code from daemon..."
PAIR_CODE=$(curl -sf "http://127.0.0.1:$DAEMON_PORT/api/v1/pairing/codes" | python3 -c 'import json,sys; data=json.load(sys.stdin); codes=data.get("codes") or []; print((codes[0] or {}).get("code","") if codes else "")')
if [[ -z "$PAIR_CODE" ]]; then
  echo "[e2e] ERROR: No pairing code available from daemon" >&2
  exit 1
fi
echo "[e2e] Pairing code: ${PAIR_CODE:0:20}..."

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
RESP=$(send_cmd "telegram test-chat req-2 /agents" "$SESSION_TOKEN")
assert_ok "/agents" "$RESP"

# Test: /status
RESP=$(send_cmd "telegram test-chat req-3 /status" "$SESSION_TOKEN")
assert_ok "/status" "$RESP"

# Test: /install requires host binding in chat mode
RESP=$(send_cmd "telegram test-chat req-4 /install openclaw" "$SESSION_TOKEN")
assert_error_code "/install openclaw blocked in chat" "$RESP" "E_HOST_BINDING_REQUIRED"

# Test: /status after install attempt
RESP=$(send_cmd "telegram test-chat req-5 /status openclaw" "$SESSION_TOKEN")
assert_ok "/status after install attempt" "$RESP"

# Test: /logs
RESP=$(send_cmd "telegram test-chat req-6 /logs openclaw" "$SESSION_TOKEN")
assert_ok "/logs openclaw" "$RESP"

# Test: unknown command (should error)
RESP=$(send_cmd "telegram test-chat req-7 /unknown" "$SESSION_TOKEN")
assert_error "unknown command" "$RESP"

echo ""
if [[ $ERRORS -gt 0 ]]; then
  echo "[e2e] $ERRORS test(s) failed!"
  exit 1
else
  echo "[e2e] All tests passed!"
fi
