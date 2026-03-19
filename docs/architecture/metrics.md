# Carrier Metrics Harness

This document defines the **PR metrics harness** for the Carrier repository.

The goal is to provide a lightweight, repeatable gate/report that runs on every PR and leaves one consolidated metrics comment.

## Scope

Current harness sections:

1. **Commit Size** (soft gate)
2. **WebUI Bundle Size** (hard gate)
3. **Code Readability** (hard gate on changed source files)

The workflow is implemented in:

- `.github/workflows/metrics-harness.yml`

## 1) Commit Size (soft gate)

- **Metric**: additions + deletions per commit in PR range
- **Limit**: `≤ 500` lines / commit
- **Behavior**: does **not** fail the workflow; reported in PR comment

Rationale: helps keep review granularity healthy without blocking emergency or refactor PRs.

## 2) WebUI Bundle Size (hard gate)

Measured after `bash scripts/build-webui.sh` using files under `webui/static/assets`.

- **JS bundle gzip**: `≤ 350 KB`
- **Initial JS gzip** (script tags referenced by `webui/static/index.html`): `≤ 180 KB`

If either threshold is exceeded, the metrics workflow fails.

## 3) Code Readability (hard gate)

Checks **changed source files in the PR diff** across:

- `baseagent`, `cmd`, `codeagent`, `daemon`, `gateway`, `shared`, `webui/src`, `scripts`

File types:

- `.go`, `.ts`, `.tsx`, `.js`, `.cjs`, `.mjs`, `.sh`

Rules:

- Every checked source file must be `≤ 500` lines
- Violations are reported per file in the PR metrics comment
- Any violation fails the metrics workflow

This is intentionally strict for changed code so new/modified files stay readable without forcing an immediate full-repo refactor.

## PR Comment Contract

The metrics workflow writes/updates a single PR comment with marker:

- `<!-- carrier-pr-metrics-harness -->`

This keeps the PR timeline clean and avoids duplicate bot comments.

## Future Extensions

Planned additions (optional, not yet hard-gated):

- command latency probes (P50/P95) for selected control-plane flows
- packaged binary size trend section from `e2e-packaged-binary` artifacts
- visual acceptance timing rollup alongside screenshot links
