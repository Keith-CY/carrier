#!/usr/bin/env bash
#
# generate-checksums.sh - Generate SHA256 sidecar files for release artifacts
#
# USAGE:
#   ./scripts/generate-checksums.sh <artifacts_directory>
#
# DESCRIPTION:
#   For each file in the specified directory, generates a `.sha256` sidecar file
#   containing the SHA256 checksum in the format: `<hash>  <filename>`
#
# INTEGRATION WITH RELEASE WORKFLOW:
#   This script should be called in .github/workflows/release.yml after artifact
#   generation. Example step:
#
#     - name: Generate SHA256 checksums
#       run: |
#         chmod +x scripts/generate-checksums.sh
#         ./scripts/generate-checksums.sh dist/
#
# NOTES:
#   - Uses `sha256sum` on Linux or `shasum -a 256` on macOS
#   - Skips files that already have `.sha256` sidecar files
#   - Skips existing `.sha256` files themselves
#

set -euo pipefail

# Detect the SHA256 command based on OS
if command -v sha256sum &> /dev/null; then
    SHA_CMD="sha256sum"
elif command -v shasum &> /dev/null; then
    SHA_CMD="shasum -a 256"
else
    echo "ERROR: Neither sha256sum nor shasum found. Cannot generate checksums." >&2
    exit 1
fi

# Check if directory argument is provided
if [ $# -ne 1 ]; then
    echo "USAGE: $0 <artifacts_directory>" >&2
    exit 1
fi

ARTIFACTS_DIR="$1"

# Validate directory exists
if [ ! -d "$ARTIFACTS_DIR" ]; then
    echo "ERROR: Directory '$ARTIFACTS_DIR' does not exist." >&2
    exit 1
fi

# Change to artifacts directory
cd "$ARTIFACTS_DIR"

echo "Generating SHA256 checksums in: $ARTIFACTS_DIR"
echo "Using command: $SHA_CMD"
echo ""

# Counter for generated checksums
COUNT=0

# Process each file (non-recursively)
for file in *; do
    # Skip if not a regular file
    [ -f "$file" ] || continue
    
    # Skip .sha256 files themselves
    [[ "$file" == *.sha256 ]] && continue
    
    # Skip if .sha256 sidecar already exists
    if [ -f "${file}.sha256" ]; then
        echo "SKIP: ${file} (checksum already exists)"
        continue
    fi
    
    # Generate checksum
    echo "Generating: ${file}.sha256"
    $SHA_CMD "$file" > "${file}.sha256"
    
    COUNT=$((COUNT + 1))
done

echo ""
echo "Generated $COUNT SHA256 checksum file(s)."
