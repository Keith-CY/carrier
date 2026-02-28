#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TMP_DIR="${ROOT_DIR}/.tmp/coverage-gate"
mkdir -p "${TMP_DIR}"

STRICT_MODE="${COVERAGE_STRICT_100:-0}"

threshold_shared="${COVERAGE_THRESHOLD_SHARED:-100.0}"
threshold_baseagent="${COVERAGE_THRESHOLD_BASEAGENT:-83.0}"
# Daemon module now includes lower-covered support packages (for example
# credentialstore), so keep the gate aligned with the current module baseline.
threshold_daemon="${COVERAGE_THRESHOLD_DAEMON:-79.5}"
threshold_gateway="${COVERAGE_THRESHOLD_GATEWAY:-69.0}"

if [[ "${STRICT_MODE}" == "1" ]]; then
  threshold_shared="100.0"
  threshold_baseagent="100.0"
  threshold_daemon="100.0"
  threshold_gateway="100.0"
fi

run_gate() {
  local module="$1"
  local threshold="$2"
  local module_dir="${ROOT_DIR}/${module}"
  local profile="${TMP_DIR}/${module}.out"
  local log_file="${TMP_DIR}/${module}.log"
  local cache_dir="${ROOT_DIR}/.cache/go-build-coverage-${module}"

  echo "[coverage] running ${module} (threshold: ${threshold}%)"
  mkdir -p "${cache_dir}"
  if ! (
    cd "${module_dir}"
    GOCACHE="${cache_dir}" go test ./... -coverprofile="${profile}" >"${log_file}" 2>&1
  ); then
    echo "[coverage] ${module} tests failed" >&2
    cat "${log_file}" >&2
    return 1
  fi

  local total
  total="$(
    cd "${module_dir}"
    GOCACHE="${cache_dir}" go tool cover -func="${profile}" | awk '/^total:/ {print $NF}' | tr -d '%'
  )"
  if [[ -z "${total}" ]]; then
    echo "[coverage] failed to parse total coverage for ${module}" >&2
    return 1
  fi

  printf "[coverage] %-10s => %5.1f%%\n" "${module}" "${total}"
  if ! awk -v actual="${total}" -v target="${threshold}" 'BEGIN { exit (actual + 0 >= target + 0) ? 0 : 1 }'; then
    echo "[coverage] ${module} is below threshold: ${total}% < ${threshold}%" >&2
    return 1
  fi
  return 0
}

failures=0

run_gate "shared" "${threshold_shared}" || failures=$((failures + 1))
run_gate "baseagent" "${threshold_baseagent}" || failures=$((failures + 1))
run_gate "daemon" "${threshold_daemon}" || failures=$((failures + 1))
run_gate "gateway" "${threshold_gateway}" || failures=$((failures + 1))

if [[ "${failures}" -ne 0 ]]; then
  echo "[coverage] gate failed with ${failures} failing module(s)" >&2
  exit 1
fi

echo "[coverage] gate passed"
