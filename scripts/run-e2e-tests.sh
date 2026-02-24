#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
gateway_root="$repo_root/gateway"
e2e_backend="${CI_E2E_BACKEND:-auto}"
gateway_pattern="${CI_E2E_TEST_PATTERN:-integration.test.ts}"
carrier_binary=""
tmp_workspace=""

cd "$repo_root"

echo "Running end-to-end test suite..."

cleanup_tmp_workspace() {
  if [[ -n "$tmp_workspace" && -d "$tmp_workspace" ]]; then
    rm -rf "$tmp_workspace"
  fi
}

trap cleanup_tmp_workspace EXIT

is_gateway_project() {
  [[ -d "$gateway_root" && -f "$gateway_root/package.json" ]]
}

run_gateway_e2e() {
  if ! command -v bun >/dev/null 2>&1; then
    echo "Error: bun is required to run repository E2E tests."
    echo "Install bun (1.x) in CI or run with the repository's supported toolchain."
    exit 1
  fi

  echo "Running gateway integration tests from repository sources."
  echo "Pattern: ${gateway_pattern}"

  (
    cd "$gateway_root"
    if [[ ! -d node_modules ]]; then
      bun install --no-progress --frozen-lockfile
    fi
    if [[ -n "${gateway_pattern}" ]]; then
      bun test "${gateway_pattern}"
    else
      bun test
    fi
  )
}

run_carrier_e2e() {
  local carrier_cmd="carrier"

  if ! command -v carrier >/dev/null 2>&1; then
    tmp_workspace="$(mktemp -d)"
    carrier_binary="$tmp_workspace/carrier"

    echo "carrier CLI not in PATH; building from source."
    (
      cd "$repo_root"
      go build -o "$carrier_binary" ./cmd/carrier
    )
    carrier_cmd="$carrier_binary"
  else
    carrier_cmd="carrier"
  fi

  if [[ ! -x "$carrier_cmd" ]]; then
    echo "Error: carrier CLI is not available."
    exit 1
  fi

  echo "Running carrier CLI E2E suite."
  "$carrier_cmd" test e2e --report test-results/
}

case "${e2e_backend}" in
  gateway)
    if ! is_gateway_project; then
      echo "Error: CI_E2E_BACKEND=gateway requires ${gateway_root}/package.json."
      echo "This repository does not appear to contain a Bun-based gateway package."
      exit 1
    fi
    run_gateway_e2e
    ;;
  carrier)
    run_carrier_e2e
    ;;
  auto)
    if is_gateway_project; then
      if command -v bun >/dev/null 2>&1; then
        run_gateway_e2e
      elif command -v carrier >/dev/null 2>&1; then
        echo "Warning: bun is unavailable; falling back to carrier CLI e2e backend."
        run_carrier_e2e
      else
        echo "Error: neither bun (gateway backend) nor carrier CLI is available."
        exit 1
      fi
    else
      run_carrier_e2e
    fi
    ;;
  *)
    echo "Error: unsupported CI_E2E_BACKEND value: ${e2e_backend}"
    echo "Supported values: auto, gateway, carrier"
    exit 1
    ;;
esac

exit 0
