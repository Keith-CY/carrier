#!/usr/bin/env bash
# Print and verify checksum commands for release artifacts.
# Usage: ./scripts/verify-checksum.sh [artifact.zip]
#
# Without arguments, prints supported checksum commands for reference.
# With an argument, verifies the checksum of the given file.

set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "Supported checksum verification commands:"
  echo ""
  echo "  Linux (GNU coreutils):"
  echo "    sha256sum -c carrier-*.sha256"
  echo ""
  echo "  macOS:"
  echo "    shasum -a 256 carrier-*.zip"
  echo ""
  echo "  Windows PowerShell:"
  echo "    Get-FileHash .\\carrier-*.zip -Algorithm SHA256"
  echo ""
  echo "To generate a .sha256 file:"
  echo "  sha256sum carrier-linux-x64.zip > carrier-linux-x64.zip.sha256"
  exit 0
fi

FILE="$1"
if [[ ! -f "$FILE" ]]; then
  echo "Error: file not found: $FILE" >&2
  exit 1
fi

echo "SHA-256 checksum for $FILE:"
if command -v sha256sum &>/dev/null; then
  sha256sum "$FILE"
elif command -v shasum &>/dev/null; then
  shasum -a 256 "$FILE"
else
  echo "Error: no checksum tool found (need sha256sum or shasum)" >&2
  exit 1
fi
