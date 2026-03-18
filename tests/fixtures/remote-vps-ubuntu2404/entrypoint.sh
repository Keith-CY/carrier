#!/usr/bin/env bash
set -euo pipefail

: "${AUTHORIZED_KEY:?AUTHORIZED_KEY is required}"

mkdir -p /home/ubuntu/.ssh
printf '%s\n' "${AUTHORIZED_KEY}" >/home/ubuntu/.ssh/authorized_keys
chown -R ubuntu:ubuntu /home/ubuntu/.ssh
chmod 700 /home/ubuntu/.ssh
chmod 600 /home/ubuntu/.ssh/authorized_keys

export LANG=C.UTF-8
export LC_ALL=C.UTF-8

exec /usr/sbin/sshd -D -e -p "${SSH_PORT:-22}"
