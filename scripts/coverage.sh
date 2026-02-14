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

# Generate HTML coverage report
HTML_REPORT="${DAEMON_DIR}/coverage.html"
go tool cover -html="${PROFILE}" -o "${HTML_REPORT}"
echo ""
echo "HTML report written to ${HTML_REPORT}"

# Write report
REPORT="${COVERAGE_DIR}/coverage-report.md"
mkdir -p "${COVERAGE_DIR}"

TOTAL=$(go tool cover -func="${PROFILE}" | tail -1 | awk '{print $NF}')
DATE=$(date -u +%Y-%m-%d)

# Identify per-package coverage
PKG_COVERAGE=$(go tool cover -func="${PROFILE}" | grep -v "^total:" | \
  awk -F'\t' '{split($1,a,":"); pkg=a[1]; pct=$NF; gsub(/%/,"",pct); if(pct+0>=0) pkgs[pkg]+=pct; counts[pkg]++} END{for(p in pkgs) printf "%s\t%.1f%%\n", p, pkgs[p]/counts[p]}' | \
  sort -t$'\t' -k2 -n)

# Top 5 coverage gaps (lowest coverage packages)
TOP_GAPS=$(echo "$PKG_COVERAGE" | head -5)

cat > "${REPORT}" <<EOF
# Daemon Test Coverage Report

**Date:** ${DATE}
**Total Coverage:** ${TOTAL}

## Per-Package Coverage

\`\`\`
${PKG_COVERAGE}
\`\`\`

## Top Coverage Gaps

The following packages have the lowest coverage and would benefit from additional tests:

\`\`\`
${TOP_GAPS}
\`\`\`

## How to Regenerate

\`\`\`bash
./scripts/coverage.sh
\`\`\`

The script also generates an HTML report at \`daemon/coverage.html\` for interactive browsing.

CI can optionally run this script and post coverage results as a PR comment.
EOF

echo ""
echo "Report written to ${REPORT}"

# Cleanup profile (keep HTML for manual browsing)
rm -f "${PROFILE}"
