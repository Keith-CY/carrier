#!/usr/bin/env bash
# Generate test coverage report for the daemon package.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DAEMON_DIR="${REPO_ROOT}/daemon"
COVERAGE_DIR="${REPO_ROOT}/docs"
PROFILE="${DAEMON_DIR}/coverage.out"

echo "Running tests with coverage..."
cd "${DAEMON_DIR}"
go test -coverprofile="${PROFILE}" ./...

echo ""
echo "=== Coverage Summary ==="
go tool cover -func="${PROFILE}" | tail -1

echo ""
echo "Detailed per-function coverage:"
go tool cover -func="${PROFILE}"

# Write report
REPORT="${COVERAGE_DIR}/coverage-report.md"
mkdir -p "${COVERAGE_DIR}"

TOTAL=$(go tool cover -func="${PROFILE}" | tail -1 | awk '{print $NF}')
DATE=$(date -u +%Y-%m-%d)

cat > "${REPORT}" <<EOF
# Daemon Test Coverage Report

**Date:** ${DATE}
**Total Coverage:** ${TOTAL}

## Per-Package Coverage

\`\`\`
$(go tool cover -func="${PROFILE}")
\`\`\`

## How to Regenerate

\`\`\`bash
./scripts/coverage.sh
\`\`\`
EOF

echo ""
echo "Report written to ${REPORT}"

# Cleanup
rm -f "${PROFILE}"
