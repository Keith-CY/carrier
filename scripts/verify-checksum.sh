#!/usr/bin/env bash
# Verify SHA-256 checksums from a checksum file.
# Each line must be: <hex-checksum>  <filename>
# Usage: ./scripts/verify-checksum.sh <checksum-file>
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: verify-checksum.sh <checksum-file>" >&2
  exit 1
fi

CHECKSUM_FILE="$1"

if [[ ! -f "$CHECKSUM_FILE" ]]; then
  echo "Error: checksum file not found: $CHECKSUM_FILE" >&2
  exit 1
fi

FAIL=0
LINE_NUM=0

while IFS= read -r line; do
  LINE_NUM=$((LINE_NUM + 1))

  # Normalize whitespace and skip empty/comment lines
  trimmed="$(echo "$line" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
  if [[ -z "$trimmed" || "$trimmed" == \#* ]]; then
    continue
  fi

  # Extract checksum and filename
  checksum="$(echo "$trimmed" | awk '{print $1}')"
  filename="$(echo "$trimmed" | awk '{print $2}')"

  if [[ -z "$checksum" ]]; then
    echo "Line $LINE_NUM: missing checksum" >&2
    FAIL=1
    continue
  fi

  if [[ -z "$filename" ]]; then
    echo "Line $LINE_NUM: missing filename" >&2
    FAIL=1
    continue
  fi

  # Validate checksum is hex
  if ! echo "$checksum" | grep -qE '^[0-9a-fA-F]+$'; then
    echo "Line $LINE_NUM: invalid checksum format: $checksum" >&2
    FAIL=1
    continue
  fi

  if [[ ! -f "$filename" ]]; then
    echo "Line $LINE_NUM: file not found: $filename" >&2
    FAIL=1
    continue
  fi

  actual="$(sha256sum "$filename" | awk '{print $1}')"
  if [[ "$actual" != "$checksum" ]]; then
    echo "FAIL: $filename (expected $checksum, got $actual)" >&2
    FAIL=1
  else
    echo "OK: $filename"
  fi
done < "$CHECKSUM_FILE"

exit $FAIL
