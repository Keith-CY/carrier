#!/usr/bin/env bash
# E2E test suite for Carrier daemon.
# Builds the unified carrier binary, starts daemon mode on a random port, exercises the HTTP API.
# Targets ≥40% of system paths.
#
# Usage:  bash tests/e2e/e2e_test.sh

set -euo pipefail

###############################################################################
# Helpers
###############################################################################

PASS=0; FAIL=0; TOTAL=0
CLEANUP_PIDS=()

cleanup() {
  for pid in "${CLEANUP_PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

TMPDIR=$(mktemp -d)

log()  { printf "\033[1;34m[e2e]\033[0m %s\n" "$*"; }
pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); printf "\033[1;32m  ✓ %s\033[0m\n" "$*"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); printf "\033[1;31m  ✗ %s\033[0m\n" "$*"; }

assert_status() {
  local desc="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then pass "$desc"; else fail "$desc (expected=$expected actual=$actual)"; fi
}

assert_contains() {
  local desc="$1" haystack="$2" needle="$3"
  if echo "$haystack" | grep -qF "$needle"; then pass "$desc"; else fail "$desc (missing '$needle')"; fi
}

# HTTP helpers - write body to $TMPDIR/body.txt, status to $TMPDIR/status.txt
do_curl() {
  local auth_header="$1"; shift
  local method="$1"; shift
  local path="$1"; shift

  local status
  status=$(curl -s -o "$TMPDIR/body.txt" -w '%{http_code}' \
    -X "$method" \
    ${auth_header:+-H "$auth_header"} \
    -H "Content-Type: application/json" \
    "$@" \
    "http://127.0.0.1:${DAEMON_PORT}${path}")
  echo "$status" > "$TMPDIR/status.txt"
}

api() {
  local method="$1"; shift
  local path="$1"; shift
  do_curl "Authorization: Bearer $API_TOKEN" "$method" "$path" "$@"
}

api_noauth() {
  local method="$1"; shift
  local path="$1"; shift
  do_curl "" "$method" "$path" "$@"
}

api_badauth() {
  local method="$1"; shift
  local path="$1"; shift
  do_curl "Authorization: Bearer wrong-token-xxx" "$method" "$path" "$@"
}

get_status() { cat "$TMPDIR/status.txt"; }
get_body()   { cat "$TMPDIR/body.txt"; }

###############################################################################
# Build
###############################################################################

export PATH="/tmp/go/bin:$PATH"

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

log "Building carrier..."
go build -o "$TMPDIR/carrier" ./cmd/carrier
log "Build OK"

###############################################################################
# Start daemon
###############################################################################

DAEMON_PORT=$(shuf -i 10000-60000 -n1)
API_TOKEN="e2e-test-token-$(date +%s)"

export CARRIER_DEV_MODE=1
export CARRIER_SERVER_HOST="127.0.0.1"
export CARRIER_SERVER_PORT="$DAEMON_PORT"
export CARRIER_SERVER_API_TOKEN="$API_TOKEN"
export CARRIER_LOG_LEVEL="debug"
export CARRIER_LOG_FORMAT="text"
export OPENAI_API_KEY="sk-fake-e2e-test-key-00000000000000000000"

log "Starting daemon on port ${DAEMON_PORT}..."
"$TMPDIR/carrier" daemon > "$TMPDIR/daemon.log" 2>&1 &
DAEMON_PID=$!
CLEANUP_PIDS+=("$DAEMON_PID")

for _i in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:${DAEMON_PORT}/readyz" >/dev/null 2>&1; then break; fi
  sleep 0.2
done
if ! curl -sf "http://127.0.0.1:${DAEMON_PORT}/readyz" >/dev/null 2>&1; then
  log "Daemon failed to start:"; cat "$TMPDIR/daemon.log"; exit 1
fi
log "Daemon ready (pid=$DAEMON_PID)"

###############################################################################
# 1. Health endpoints (no auth needed)
###############################################################################

log "=== Health endpoints ==="

api_noauth GET "/healthz"
assert_status "GET /healthz → 200" "200" "$(get_status)"

api_noauth GET "/readyz"
assert_status "GET /readyz → 200" "200" "$(get_status)"

###############################################################################
# 2. Auth
###############################################################################

log "=== Auth ==="

api_noauth GET "/api/agents"
assert_status "No token → 401" "401" "$(get_status)"

api_badauth GET "/api/agents"
assert_status "Wrong token → 401" "401" "$(get_status)"

api GET "/api/agents"
assert_status "Valid token → 200" "200" "$(get_status)"

###############################################################################
# 3. List agents
###############################################################################

log "=== List agents ==="
api GET "/api/agents"
assert_status "GET /api/agents → 200" "200" "$(get_status)"
assert_contains "Response contains openclaw" "$(get_body)" "openclaw"

###############################################################################
# 4. Full lifecycle: install → start → status → logs → stop → uninstall
###############################################################################

AGENT_ID="openclaw"

log "=== Full lifecycle ($AGENT_ID) ==="

# Install
api POST "/api/v1/agents/$AGENT_ID/install"
assert_status "Install → 200" "200" "$(get_status)"
assert_contains "Install response" "$(get_body)" "installed"

# Status after install
api GET "/api/v1/agents/$AGENT_ID/status"
assert_status "Status after install → 200" "200" "$(get_status)"
assert_contains "installState=installed" "$(get_body)" '"installed"'
assert_contains "runtimeState=stopped" "$(get_body)" '"stopped"'

# Start
api POST "/api/v1/agents/$AGENT_ID/start"
assert_status "Start → 200" "200" "$(get_status)"

# Wait for the dev mode process to start
sleep 2

# Status after start
api GET "/api/v1/agents/$AGENT_ID/status"
assert_status "Status after start → 200" "200" "$(get_status)"
assert_contains "runtimeState=running" "$(get_body)" '"running"'

# Logs
api GET "/api/v1/agents/$AGENT_ID/logs"
assert_status "Logs → 200" "200" "$(get_status)"
assert_contains "Logs has lines" "$(get_body)" "lines"

# Logs with tail
api GET "/api/v1/agents/$AGENT_ID/logs?tail=5"
assert_status "Logs with tail → 200" "200" "$(get_status)"

# Stop
api POST "/api/v1/agents/$AGENT_ID/stop"
assert_status "Stop → 200" "200" "$(get_status)"
sleep 1

# Status after stop
api GET "/api/v1/agents/$AGENT_ID/status"
assert_status "Status after stop → 200" "200" "$(get_status)"
assert_contains "runtimeState=stopped" "$(get_body)" '"stopped"'

# Uninstall
api POST "/api/v1/agents/$AGENT_ID/uninstall"
assert_status "Uninstall → 200" "200" "$(get_status)"

# Status after uninstall
api GET "/api/v1/agents/$AGENT_ID/status"
assert_status "Status after uninstall → 200" "200" "$(get_status)"
assert_contains "installState=not_installed" "$(get_body)" '"not_installed"'

###############################################################################
# 5. Error paths
###############################################################################

log "=== Error paths ==="

# Unknown agent
api GET "/api/v1/agents/nonexistent-xyz/status"
assert_status "Unknown agent → 404" "404" "$(get_status)"

# Start uninstalled
api POST "/api/v1/agents/$AGENT_ID/start"
S=$(get_status)
if [ "$S" != "200" ]; then pass "Start uninstalled → error ($S)"; else fail "Start uninstalled should fail"; fi

# Path traversal
api POST "/api/install" -d '{"agentId":"../bad"}'
assert_status "Path traversal ID → 400" "400" "$(get_status)"

# Empty agent ID
api POST "/api/install" -d '{"agentId":""}'
assert_status "Empty agent ID → 400" "400" "$(get_status)"

# Slash in ID
api POST "/api/install" -d '{"agentId":"bad/slash"}'
assert_status "Slash in ID → 400" "400" "$(get_status)"

# Method not allowed
api DELETE "/api/v1/agents/$AGENT_ID/install"
S=$(get_status)
if [ "$S" = "405" ] || [ "$S" = "404" ]; then pass "Wrong method → $S"; else fail "Wrong method → $S"; fi

###############################################################################
# 6. Multi-instance
###############################################################################

log "=== Multi-instance ==="

# Install base
api POST "/api/v1/agents/$AGENT_ID/install"
assert_status "Install base → 200" "200" "$(get_status)"

# Install named instance
api POST "/api/v1/agents/$AGENT_ID/install" -d "{\"agentId\":\"$AGENT_ID\",\"instance_name\":\"second\"}"
assert_status "Install named instance → 200" "200" "$(get_status)"
assert_contains "Has instance_id" "$(get_body)" "instance_id"

INSTANCE_ID=$(grep -o '"instance_id":"[^"]*"' "$TMPDIR/body.txt" | cut -d'"' -f4)
if [ -n "$INSTANCE_ID" ]; then
  pass "Got instance_id=$INSTANCE_ID"

  # Start instance
  api POST "/api/v1/agents/$INSTANCE_ID/start"
  assert_status "Start instance → 200" "200" "$(get_status)"
  sleep 2

  # Status
  api GET "/api/v1/agents/$INSTANCE_ID/status"
  assert_status "Instance status → 200" "200" "$(get_status)"
  assert_contains "Instance running" "$(get_body)" '"running"'

  # Base agent should still be installed but stopped
  api GET "/api/v1/agents/$AGENT_ID/status"
  assert_contains "Base still installed" "$(get_body)" '"installed"'

  # Stop instance
  api POST "/api/v1/agents/$INSTANCE_ID/stop"
  assert_status "Stop instance → 200" "200" "$(get_status)"
  sleep 0.5

  # Uninstall instance
  api POST "/api/v1/agents/$INSTANCE_ID/uninstall"
  assert_status "Uninstall instance → 200" "200" "$(get_status)"
else
  fail "Could not extract instance_id"
fi

###############################################################################
# 7. Uninstall running agent
###############################################################################

log "=== Uninstall running agent ==="

api POST "/api/v1/agents/$AGENT_ID/start"
assert_status "Start for uninstall test → 200" "200" "$(get_status)"
sleep 2

api GET "/api/v1/agents/$AGENT_ID/status"
assert_contains "Running before uninstall" "$(get_body)" '"running"'

api POST "/api/v1/agents/$AGENT_ID/uninstall"
S=$(get_status)
if [ "$S" = "200" ]; then
  pass "Uninstall running → 200 (auto-stopped)"
  api GET "/api/v1/agents/$AGENT_ID/status"
  assert_contains "After uninstall → not_installed" "$(get_body)" '"not_installed"'
elif [ "$S" = "409" ]; then
  pass "Uninstall running → 409 (must stop first)"
  api POST "/api/v1/agents/$AGENT_ID/stop"
  sleep 0.5
  api POST "/api/v1/agents/$AGENT_ID/uninstall"
  assert_status "Uninstall after stop → 200" "200" "$(get_status)"
else
  fail "Uninstall running → unexpected $S"
fi

###############################################################################
# 8. Concurrent / idempotency
###############################################################################

log "=== Concurrent / idempotency ==="

api POST "/api/v1/agents/$AGENT_ID/install"
api POST "/api/v1/agents/$AGENT_ID/start"
sleep 2

# Double start → conflict
api POST "/api/v1/agents/$AGENT_ID/start"
assert_status "Double start → 409" "409" "$(get_status)"

api POST "/api/v1/agents/$AGENT_ID/stop"
sleep 0.5

# Double stop → conflict
api POST "/api/v1/agents/$AGENT_ID/stop"
assert_status "Double stop → 409" "409" "$(get_status)"

api POST "/api/v1/agents/$AGENT_ID/uninstall"

###############################################################################
# 9. V1 REST routes + legacy routes
###############################################################################

log "=== V1 + legacy parity ==="

# V1 list
api GET "/api/v1/agents"
assert_status "GET /api/v1/agents → 200" "200" "$(get_status)"

# V1 all statuses
api GET "/api/v1/agents/status"
assert_status "GET /api/v1/agents/status → 200" "200" "$(get_status)"
assert_contains "Has statuses" "$(get_body)" "statuses"

# Legacy install
api POST "/api/install" -d "{\"agentId\":\"$AGENT_ID\"}"
assert_status "Legacy install → 200" "200" "$(get_status)"

# Legacy status
api GET "/api/status/$AGENT_ID"
assert_status "Legacy status → 200" "200" "$(get_status)"

# Legacy start
api POST "/api/start" -d "{\"agentId\":\"$AGENT_ID\"}"
assert_status "Legacy start → 200" "200" "$(get_status)"
sleep 1

# Legacy logs
api GET "/api/logs/$AGENT_ID"
assert_status "Legacy logs → 200" "200" "$(get_status)"

# Legacy stop
api POST "/api/stop" -d "{\"agentId\":\"$AGENT_ID\"}"
assert_status "Legacy stop → 200" "200" "$(get_status)"
sleep 0.5

# V1 uninstall
api POST "/api/v1/agents/$AGENT_ID/uninstall"
assert_status "V1 uninstall → 200" "200" "$(get_status)"

###############################################################################
# 10. Pairing
###############################################################################

log "=== Pairing ==="

api GET "/api/pairing/codes"
assert_status "GET pairing codes → 200" "200" "$(get_status)"
assert_contains "Has codes" "$(get_body)" "codes"

api POST "/api/pairing/verify-consume" -d '{"code":"invalid-code"}'
assert_status "Invalid pair code → 400" "400" "$(get_status)"

###############################################################################
# 11. Diagnosis
###############################################################################

log "=== Diagnosis ==="

api POST "/api/v1/agents/$AGENT_ID/install"
api POST "/api/v1/agents/$AGENT_ID/diagnose"
assert_status "Diagnose → 200" "200" "$(get_status)"
assert_contains "Has artifactRef" "$(get_body)" "artifactRef"

api POST "/api/v1/diagnosis/handoffs" -d "{\"agentId\":\"$AGENT_ID\",\"consent\":true,\"actor\":\"e2e\"}"
S=$(get_status)
if [ "$S" = "200" ] || [ "$S" = "500" ]; then
  pass "Diagnosis handoff endpoint responds ($S)"
else
  fail "Diagnosis handoff → unexpected $S"
fi

# Handoff with missing agentId
api POST "/api/v1/diagnosis/handoffs" -d '{"agentId":"","consent":true,"actor":"e2e"}'
assert_status "Handoff empty agentId → 400" "400" "$(get_status)"

api POST "/api/v1/agents/$AGENT_ID/uninstall"

###############################################################################
# 12. Upgrade
###############################################################################

log "=== Upgrade ==="

api POST "/api/v1/agents/$AGENT_ID/install"
api POST "/api/v1/agents/$AGENT_ID/upgrade"
S=$(get_status)
if [ "$S" = "200" ] || [ "$S" = "400" ] || [ "$S" = "500" ]; then
  pass "Upgrade endpoint responds ($S)"
else
  fail "Upgrade unexpected $S"
fi
api POST "/api/v1/agents/$AGENT_ID/uninstall"

###############################################################################
# 13. Invalid instance_name
###############################################################################

log "=== Invalid instance_name ==="

api POST "/api/v1/agents/$AGENT_ID/install"
api POST "/api/v1/agents/$AGENT_ID/install" -d "{\"agentId\":\"$AGENT_ID\",\"instance_name\":\"../bad\"}"
S=$(get_status)
if [ "$S" != "200" ]; then pass "Bad instance_name → $S"; else fail "Bad instance_name accepted"; fi
api POST "/api/v1/agents/$AGENT_ID/uninstall"

###############################################################################
# Summary
###############################################################################

echo ""
log "════════════════════════════════════════"
log " Results: $PASS passed, $FAIL failed, $TOTAL total"
log "════════════════════════════════════════"

if [ "$FAIL" -gt 0 ]; then
  log "Daemon log (last 30 lines):"
  tail -30 "$TMPDIR/daemon.log" || true
  exit 1
fi
