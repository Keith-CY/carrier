#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
e2e_backend="${CI_E2E_BACKEND:-auto}"
gateway_pattern="${CI_E2E_TEST_PATTERN:-integration.test.ts}"

cd "$repo_root"

echo "Running end-to-end test suite..."
run_gateway_e2e() {
  if ! command -v bun >/dev/null 2>&1; then
    echo "Error: bun is required to run repository E2E tests."
    echo "Install bun (1.x) in CI or run with the repository's supported toolchain."
    exit 1
  fi

  echo "Running gateway integration tests from repository sources."
  echo "Pattern: ${gateway_pattern}"

  (
    cd "$repo_root/gateway"
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
  if ! command -v carrier >/dev/null 2>&1; then
    echo "Error: carrier CLI is not available in PATH."
    echo "Expected e2e command: carrier test e2e --report test-results/"
    exit 1
  fi

  echo "Falling back to carrier CLI E2E suite."
  carrier test e2e --report test-results/
}

case "${e2e_backend}" in
  gateway)
    if [[ ! -d "$repo_root/gateway" ]]; then
      echo "Error: CI_E2E_BACKEND=gateway but gateway directory is missing."
      exit 1
    fi
    run_gateway_e2e
    ;;
  carrier)
    run_carrier_e2e
    ;;
  auto)
    if [[ -d "$repo_root/gateway" ]]; then
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
