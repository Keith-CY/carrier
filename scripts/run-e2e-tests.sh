#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
e2e_backend="${CI_E2E_BACKEND:-auto}"
e2e_integration_script="$repo_root/scripts/e2e-integration.sh"

cd "$repo_root"

echo "Running end-to-end test suite..."

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
    echo "Error: CI_E2E_BACKEND=gateway is no longer supported."
    echo "Gateway is now a Go top-level module; use CI_E2E_BACKEND=carrier (or auto)."
    exit 1
    ;;
  carrier)
    run_carrier_e2e
    ;;
  auto)
    run_carrier_e2e
    ;;
  *)
    echo "Error: unsupported CI_E2E_BACKEND value: ${e2e_backend}"
    echo "Supported values: auto, carrier"
    exit 1
    ;;
esac

exit 0
