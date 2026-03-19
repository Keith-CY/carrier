#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
cleanup_containers=()
tmpdir="$(mktemp -d)"

cleanup() {
  echo "[e2e-local] Cleaning up..."
  for container in "${cleanup_containers[@]:-}"; do
    docker rm -f "$container" >/dev/null 2>&1 || true
  done
  rm -rf "$tmpdir"
}
trap cleanup EXIT

find_port() {
  python3 -c "import socket; s=socket.socket(); s.bind(('', 0)); print(s.getsockname()[1]); s.close()"
}

wait_for_ssh() {
  local key_path="$1"
  local port="$2"
  for _ in $(seq 1 60); do
    if ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "$key_path" -p "$port" carrier@127.0.0.1 true >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "[e2e-local] ERROR: timeout waiting for remote SSH on port $port" >&2
  return 1
}

if [[ "${CARRIER_E2E_REMOTE_FIXTURE:-0}" == "1" ]]; then
  echo "[e2e-local] Starting remote Ubuntu fixture..."
  remote_ssh_port="${CARRIER_E2E_REMOTE_SSH_PORT:-$(find_port)}"
  remote_host_name="${CARRIER_E2E_REMOTE_HOST_NAME:-ubuntu-demo}"
  remote_container_name="carrier-e2e-remote-${remote_ssh_port}"
  remote_image_tag="carrier-remote-vps-e2e:latest"
  remote_ssh_dir="$tmpdir/ssh"
  remote_ssh_key="$remote_ssh_dir/id_ed25519"
  mkdir -p "$remote_ssh_dir"
  ssh-keygen -q -t ed25519 -N '' -f "$remote_ssh_key" >/dev/null
  remote_authorized_key="$(cat "$remote_ssh_key.pub")"
  docker build -t "$remote_image_tag" -f "$repo_root/tests/fixtures/remote-vps/Dockerfile" "$repo_root" >/dev/null
  docker run -d --name "$remote_container_name" -p "${remote_ssh_port}:22" -e AUTHORIZED_KEY="$remote_authorized_key" "$remote_image_tag" >/dev/null
  cleanup_containers+=("$remote_container_name")
  wait_for_ssh "$remote_ssh_key" "$remote_ssh_port"

  export CARRIER_E2E_REMOTE_HOST_NAME="$remote_host_name"
  export CARRIER_E2E_REMOTE_HOST_HOST="127.0.0.1"
  export CARRIER_E2E_REMOTE_HOST_PORT="$remote_ssh_port"
  export CARRIER_E2E_REMOTE_HOST_USER="carrier"
  export CARRIER_E2E_REMOTE_HOST_KEY_PATH="$remote_ssh_key"
fi

bash "$repo_root/scripts/e2e-control-plane-bootstrap.sh" --playwright-fullstack "$@"
