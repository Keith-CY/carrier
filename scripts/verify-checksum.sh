#!/usr/bin/env bash
# Verify SHA-256 checksum of a file against a checksum file.
# Usage: bash scripts/verify-checksum.sh <target-file> <checksum-file>
set -euo pipefail

if [ $# -lt 2 ]; then
  echo "Usage: $0 <target-file> <checksum-file>"
  exit 1
fi

TARGET="$1"
CHECKSUM_FILE="$2"

if [ ! -f "$TARGET" ]; then
  echo "ERROR: Target file not found: $TARGET"
  exit 1
fi

if [ ! -f "$CHECKSUM_FILE" ]; then
  echo "ERROR: Checksum file not found: $CHECKSUM_FILE"
  exit 1
fi

# Compute actual hash
ACTUAL_HASH=$(sha256sum "$TARGET" | awk '{print $1}')

# Extract expected hash — match by basename to handle paths
TARGET_BASENAME=$(basename "$TARGET")
EXPECTED_LINE=$(grep -F "$TARGET_BASENAME" "$CHECKSUM_FILE" | head -1)

if [ -z "$EXPECTED_LINE" ]; then
  echo "ERROR: No checksum entry found for '$TARGET_BASENAME' in $CHECKSUM_FILE"
  exit 1
fi

EXPECTED_HASH=$(echo "$EXPECTED_LINE" | awk '{print $1}')

if [ "$ACTUAL_HASH" = "$EXPECTED_HASH" ]; then
  echo "OK: $TARGET_BASENAME checksum verified ($ACTUAL_HASH)"
  exit 0
else
  echo "FAIL: checksum mismatch for $TARGET_BASENAME"
  echo "  Expected: $EXPECTED_HASH"
  echo "  Actual:   $ACTUAL_HASH"
  exit 1
fi
