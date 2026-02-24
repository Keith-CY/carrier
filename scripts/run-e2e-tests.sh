#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
gateway_root="$repo_root/gateway"
e2e_backend="${CI_E2E_BACKEND:-auto}"
gateway_pattern="${CI_E2E_TEST_PATTERN:-integration.test.ts}"
e2e_integration_script="$repo_root/scripts/e2e-integration.sh"

cd "$repo_root"

echo "Running end-to-end test suite..."

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
  if [[ ! -x "$e2e_integration_script" ]]; then
    echo "Error: E2E integration runner not available."
    echo "Expected executable at ${e2e_integration_script}"
    exit 1
  fi

  echo "Running repository integration E2E suite."
  "$e2e_integration_script"
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
        echo "Warning: bun is unavailable; falling back to repository integration backend."
        run_carrier_e2e
      else
        echo "Error: neither bun (gateway backend) nor supported fallback backend is available."
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
