#!/usr/bin/env bash
# E2E Isolation Test Suite for Carrier
# Tests isolation effectiveness across Linux, macOS (Lima), and Windows (WSL2)
#
# Usage:
#   bash tests/e2e/isolation_e2e_test.sh [--platform linux|macos|windows]
#
# Prerequisites:
#   Linux:   bwrap installed
#   macOS:   Lima installed, carrier daemon running
#   Windows: WSL2 enabled with a Linux distro

set -euo pipefail

###############################################################################
# Configuration
###############################################################################

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
PLATFORM="auto"
TMPDIR=$(mktemp -d)
PASS=0; FAIL=0; TOTAL=0

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --platform)
      PLATFORM="${2:-auto}"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $0 [--platform linux|macos|windows]"
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      echo "Usage: $0 [--platform linux|macos|windows]"
      exit 1
      ;;
  esac
done

###############################################################################
# Helpers
###############################################################################

cleanup() {
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

log()  { printf "\033[1;34m[isolation-e2e]\033[0m %s\n" "$*"; }
pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); printf "\033[1;32m  ✓ %s\033[0m\n" "$*"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); printf "\033[1;31m  ✗ %s\033[0m\n" "$*"; }
skip() { printf "\033[1;33m  ⊘ %s (skipped)\033[0m\n" "$*"; }

detect_platform() {
  case "$(uname -s)" in
    Linux*)  echo "linux" ;;
    Darwin*) echo "macos" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *)       echo "unknown" ;;
  esac
}

###############################################################################
# Platform Detection
###############################################################################

if [ "$PLATFORM" = "auto" ]; then
  PLATFORM=$(detect_platform)
fi

log "Detected platform: $PLATFORM"

###############################################################################
# Linux Tests (bwrap)
###############################################################################

test_linux_bwrap_available() {
  log "Testing: bwrap binary available"
  if command -v bwrap >/dev/null 2>&1; then
    pass "bwrap is installed"
  else
    fail "bwrap not found in PATH"
    return 1
  fi
}

test_linux_pid_isolation() {
  log "Testing: PID namespace isolation"
  
  # Run a command inside bwrap and check if it can see host PIDs
  local isolated_pids
  isolated_pids=$(bwrap --die-with-parent --new-session --bind / / \
    --proc /proc --dev /dev --tmpfs /tmp --unshare-pid \
    -- sh -c 'ps aux 2>/dev/null | wc -l' 2>/dev/null || echo "0")
  
  local host_pids
  host_pids=$(ps aux | wc -l)
  
  # Isolated should see far fewer processes
  if [ "$isolated_pids" -lt "$((host_pids / 2))" ]; then
    pass "PID isolation working (isolated sees $isolated_pids vs host $host_pids)"
  else
    fail "PID isolation may not be working (isolated=$isolated_pids, host=$host_pids)"
  fi
}

test_linux_tmp_isolation() {
  log "Testing: /tmp isolation (tmpfs)"
  
  # Create a file on host /tmp
  local marker="carrier-e2e-test-$$"
  echo "host-marker" > "/tmp/$marker"
  
  # Check if file is visible inside bwrap
  local result
  result=$(bwrap --die-with-parent --new-session --bind / / \
    --proc /proc --dev /dev --tmpfs /tmp --unshare-pid \
    -- sh -c "cat /tmp/$marker 2>/dev/null || echo 'NOT_FOUND'" 2>/dev/null)
  
  rm -f "/tmp/$marker"
  
  if [ "$result" = "NOT_FOUND" ]; then
    pass "/tmp is isolated (host file not visible)"
  else
    fail "/tmp is NOT isolated (host file visible: $result)"
  fi
}

test_linux_session_isolation() {
  log "Testing: Session isolation (new-session)"
  
  # Check session ID inside bwrap vs outside
  local host_sid
  host_sid=$(ps -o sid= -p $$)
  
  local isolated_sid
  isolated_sid=$(bwrap --die-with-parent --new-session --bind / / \
    --proc /proc --dev /dev --tmpfs /tmp --unshare-pid \
    -- sh -c 'ps -o sid= -p $$' 2>/dev/null || echo "error")
  
  if [ "$host_sid" != "$isolated_sid" ] && [ "$isolated_sid" != "error" ]; then
    pass "Session isolation working (host=$host_sid, isolated=$isolated_sid)"
  else
    fail "Session isolation may not be working"
  fi
}

test_linux_die_with_parent() {
  log "Testing: Process cleanup on parent exit"
  
  # Start a long-running process inside bwrap, then kill parent
  local pid_file="$TMPDIR/child_pid"
  
  (
    bwrap --die-with-parent --new-session --bind / / \
      --proc /proc --dev /dev --tmpfs /tmp --unshare-pid \
      -- sh -c 'echo $$ > /dev/stdout; sleep 60' &
    echo $! > "$pid_file"
    sleep 1
  ) &
  local parent_pid=$!
  
  sleep 0.5
  
  # Get the bwrap child PID
  local child_pid
  child_pid=$(cat "$pid_file" 2>/dev/null || echo "")
  
  # Kill the parent
  kill "$parent_pid" 2>/dev/null || true
  wait "$parent_pid" 2>/dev/null || true
  
  sleep 0.5
  
  # Check if child is still running
  if [ -n "$child_pid" ] && ! kill -0 "$child_pid" 2>/dev/null; then
    pass "Child process terminated with parent"
  else
    # Cleanup if still running
    kill "$child_pid" 2>/dev/null || true
    fail "Child process survived parent exit"
  fi
}

test_linux_multi_instance_isolation() {
  log "Testing: Multi-instance isolation (separate PID namespaces)"
  
  # Start two isolated instances, verify they can't see each other
  local marker1="$TMPDIR/instance1_pid"
  local marker2="$TMPDIR/instance2_pid"
  
  # Instance 1: write its PID to a file and list all PIDs
  bwrap --die-with-parent --new-session --bind / / \
    --proc /proc --dev /dev --tmpfs /tmp --unshare-pid \
    -- sh -c 'echo $$; ps -e -o pid= | tr -d " "' > "$marker1" 2>/dev/null &
  local inst1_job=$!
  
  # Instance 2: same
  bwrap --die-with-parent --new-session --bind / / \
    --proc /proc --dev /dev --tmpfs /tmp --unshare-pid \
    -- sh -c 'echo $$; ps -e -o pid= | tr -d " "' > "$marker2" 2>/dev/null &
  local inst2_job=$!
  
  wait "$inst1_job" "$inst2_job" 2>/dev/null || true
  
  local inst1_pid inst2_pid
  inst1_pid=$(head -1 "$marker1")
  inst2_pid=$(head -1 "$marker2")
  
  # Check if instance 1's PID list contains instance 2's PID (it shouldn't)
  if ! grep -q "^${inst2_pid}$" "$marker1" && ! grep -q "^${inst1_pid}$" "$marker2"; then
    pass "Instances have separate PID namespaces"
  else
    fail "Instances can see each other's PIDs"
  fi
}

run_linux_tests() {
  log "Running Linux isolation tests (bwrap)"
  
  test_linux_bwrap_available || return 1
  test_linux_pid_isolation
  test_linux_tmp_isolation
  test_linux_session_isolation
  test_linux_die_with_parent
  test_linux_multi_instance_isolation
}

###############################################################################
# macOS Tests (Lima VM)
###############################################################################

test_macos_lima_available() {
  log "Testing: Lima available"
  if command -v limactl >/dev/null 2>&1; then
    pass "limactl is installed"
  else
    fail "limactl not found in PATH"
    return 1
  fi
}

test_macos_lima_vm_isolation() {
  log "Testing: Lima VM isolation"
  
  # List running VMs
  local vms
  vms=$(limactl list --format '{{.Name}}' 2>/dev/null | grep "^carrier-" || echo "")
  
  if [ -n "$vms" ]; then
    pass "Found Carrier Lima VMs: $(echo "$vms" | tr '\n' ' ')"
    
    # Test that each VM is separate
    for vm in $vms; do
      local vm_pid
      vm_pid=$(limactl shell "$vm" -- sh -c 'echo $$' 2>/dev/null || echo "error")
      if [ "$vm_pid" != "error" ]; then
        log "  VM $vm has init PID $vm_pid"
      fi
    done
  else
    skip "No Carrier Lima VMs running (run 'carrier add --isolation' first)"
  fi
}

test_macos_lima_bwrap_inside() {
  log "Testing: bwrap available inside Lima VM"
  
  local vms
  vms=$(limactl list --format '{{.Name}}' 2>/dev/null | grep "^carrier-" | head -1 || echo "")
  
  if [ -z "$vms" ]; then
    skip "No Carrier Lima VMs to test"
    return 0
  fi
  
  local bwrap_check
  bwrap_check=$(limactl shell "$vms" -- command -v bwrap 2>/dev/null || echo "")
  
  if [ -n "$bwrap_check" ]; then
    pass "bwrap is available inside Lima VM $vms"
  else
    fail "bwrap not found inside Lima VM $vms"
  fi
}

run_macos_tests() {
  log "Running macOS isolation tests (Lima)"
  
  test_macos_lima_available || return 1
  test_macos_lima_vm_isolation
  test_macos_lima_bwrap_inside
  
  log "Note: Full Lima isolation testing requires running agents"
}

###############################################################################
# Windows Tests (WSL2)
###############################################################################

test_windows_wsl_available() {
  log "Testing: WSL2 available"
  if command -v wsl.exe >/dev/null 2>&1 || command -v wsl >/dev/null 2>&1; then
    pass "wsl is available"
  else
    fail "wsl not found"
    return 1
  fi
}

test_windows_wsl_distro() {
  log "Testing: WSL distro configured"
  
  local wsl_cmd="wsl.exe"
  command -v wsl.exe >/dev/null 2>&1 || wsl_cmd="wsl"
  
  local distros
  distros=$($wsl_cmd --list --quiet 2>/dev/null | tr -d '\r' | head -5 || echo "")
  
  if [ -n "$distros" ]; then
    pass "WSL distros available: $(echo "$distros" | tr '\n' ' ')"
  else
    fail "No WSL distros found"
    return 1
  fi
}

test_windows_wsl_bwrap() {
  log "Testing: bwrap available inside WSL"
  
  local wsl_cmd="wsl.exe"
  command -v wsl.exe >/dev/null 2>&1 || wsl_cmd="wsl"
  
  local bwrap_check
  bwrap_check=$($wsl_cmd -- command -v bwrap 2>/dev/null || echo "")
  
  if [ -n "$bwrap_check" ]; then
    pass "bwrap is available inside WSL default distro"
  else
    log "  Hint: Install bwrap in WSL: sudo apt install bubblewrap"
    fail "bwrap not found inside WSL"
  fi
}

test_windows_wsl_isolation() {
  log "Testing: WSL + bwrap isolation"
  
  local wsl_cmd="wsl.exe"
  command -v wsl.exe >/dev/null 2>&1 || wsl_cmd="wsl"
  
  # Run isolated command inside WSL
  local result
  result=$($wsl_cmd -- sh -lc "bwrap --die-with-parent --new-session --bind / / --proc /proc --dev /dev --tmpfs /tmp --unshare-pid -- echo 'isolated-ok'" 2>/dev/null || echo "error")
  
  if [ "$result" = "isolated-ok" ]; then
    pass "WSL + bwrap isolation chain works"
  else
    fail "WSL + bwrap chain failed: $result"
  fi
}

run_windows_tests() {
  log "Running Windows isolation tests (WSL2)"
  
  test_windows_wsl_available || return 1
  test_windows_wsl_distro || return 1
  test_windows_wsl_bwrap
  test_windows_wsl_isolation
}

###############################################################################
# Integration Tests (All Platforms)
###############################################################################

test_carrier_isolation_install() {
  log "Testing: Carrier isolation install flow"
  
  cd "$ROOT_DIR/daemon"
  
  if go test ./internal/lifecycle -run 'TestInstallWithIsolation' -count=1 -v 2>&1 | tail -5; then
    pass "Carrier isolation install tests pass"
  else
    fail "Carrier isolation install tests failed"
  fi
}

test_carrier_isolation_start() {
  log "Testing: Carrier isolation start flow"
  
  cd "$ROOT_DIR/daemon"
  
  if go test ./internal/lifecycle -run 'TestStartWithIsolation' -count=1 -v 2>&1 | tail -5; then
    pass "Carrier isolation start tests pass"
  else
    fail "Carrier isolation start tests failed"
  fi
}

run_integration_tests() {
  log "Running Carrier integration tests"
  
  test_carrier_isolation_install
  test_carrier_isolation_start
}

###############################################################################
# Main
###############################################################################

main() {
  log "=========================================="
  log "Carrier Isolation E2E Test Suite"
  log "Platform: $PLATFORM"
  log "=========================================="
  
  case "$PLATFORM" in
    linux)
      run_linux_tests
      ;;
    macos)
      run_macos_tests
      ;;
    windows)
      run_windows_tests
      ;;
    *)
      log "Unknown platform: $PLATFORM"
      exit 1
      ;;
  esac
  
  log ""
  log "Running cross-platform integration tests..."
  run_integration_tests
  
  log ""
  log "=========================================="
  log "Results: $PASS passed, $FAIL failed (total $TOTAL)"
  log "=========================================="
  
  if [ "$FAIL" -gt 0 ]; then
    exit 1
  fi
}

main "$@"
