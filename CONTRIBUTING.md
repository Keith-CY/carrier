# Contributing

## Branching and Worktree Policy

All new development must start from `origin/main` and use a dedicated branch/worktree.

```bash
git fetch origin
git worktree add -b codex/<topic> ../carrier-<topic> origin/main
cd ../carrier-<topic>
```

Branch naming:

- Use `codex/` prefix for development branches.
- Suggested pattern for issue-driven work: `codex/issue-<id>-<short-slug>`.
- Never push feature work directly to `main`.

## Pull Request Policy

Submit all feature updates via PR.

```bash
git add .
git commit -m "<message>"
git push -u origin codex/<topic>
gh pr create --base main --head codex/<topic>
```

Merge policy:

- Use Merge Queue.
- Use auto-merge with squash by default.

## Scope Policy

Phase milestones define delivery scope and must be respected in planning/review.

- Phase 1: deliver full OpenClaw lifecycle only.
- Every PR must include milestone label: `Phase 1` or `Phase 2`.
- Candidate agents remain blocked until Phase 1 exit criteria pass.

## Skills Directory

Before proposing changes, read `skills/` and follow repository instructions (especially `skills/pr-review/SKILL.md` and `skills/review-followup/SKILL.md`).

## Triage Helper Scripts

From repo root, these scripts provide read-only issue triage summaries.

### 1) Open issue summary

```bash
./scripts/triage/issue-summary.sh
```

Expected output sections:
- `Total Open Issues`
- `Open Unassigned Issues`
- `Top Assignees by Open Count`
- `Top Labels by Open Count`

Optional env var:
- `ISSUE_SUMMARY_LIMIT` (default `500`)

### 2) Duplicate detector for review-followup issues

```bash
./scripts/triage/detect-review-followup-duplicates.sh
```

Matching priority (clear criteria):
1. `nbs_marker` (exact hidden marker in issue body comment)
2. `normalized_suggestion` (normalized text from `## Suggestion`)
3. `normalized_title` (normalized title after stripping `[review-followup]` and `PR #...:` prefix)

Optional env vars:
- `ISSUE_DUP_STATE` (`open`/`closed`/`all`, default `open`)
- `ISSUE_DUP_LIMIT` (default `500`)

Deterministic output format example:

```text
Group 1 (2 issues)
criterion: normalized_suggestion
match_key: add focused unit tests for invalid consent flag parsing
- #901 (PR #882): [review-followup] PR #882: Add focused unit tests...
  https://github.com/Keith-CY/carrier/issues/901
  snippet: Add focused unit tests for invalid consent flag parsing.
- #905 (PR #882): [review-followup] PR #882: Add focused unit tests...
  https://github.com/Keith-CY/carrier/issues/905
  snippet: Add focused unit tests for invalid consent flag parsing.
```

## Testing Policy

GitHub Actions is the source of truth for merge readiness.

- Required: CI must pass before merge.
- Local runs are strongly recommended before pushing.

Enable pre-push hook once per clone:

```bash
git config core.hooksPath .githooks
```

The hook executes `./scripts/run-all-tests.sh`.

## CI Troubleshooting Docs

- First response playbook: `docs/ci/first-response-playbook.md`
- Flaky check triage: `docs/ci/flaky-checks.md`
- Flaky rerun runbook: `docs/ci/flaky-rerun-runbook.md`
- Failure log collection: `docs/ci-failure-log-collection.md`
- Release artifact retention: `docs/ci/release-artifact-retention.md`

## CI-only PR Check Surface

For PRs touching only `.github/workflows/**` and `scripts/ci/**`:

- CI runs minimal guard checks.
- Daemon/gateway test suites are skipped intentionally.

For PRs touching daemon/gateway code:

- Full required checks run.

## Updating Pinned GitHub Action SHAs Safely

Third-party actions must use immutable commit SHAs.

Before:

```yaml
uses: oven-sh/setup-bun@v2
```

After:

```yaml
uses: oven-sh/setup-bun@3d267786b128fe76c2f16a390aa2448b815359f3
```

Safe update workflow:

1. Resolve the exact upstream ref you want.
2. Replace mutable third-party refs with commit SHAs.
3. Re-run pinning guard locally:
   ```bash
   bash scripts/ci/check-action-pinning.sh
   ```
4. Push and verify CI guard is green.

CI enforcement script:

- `scripts/ci/check-action-pinning.sh`

## Checklist for PRs Closing P0/P1 Issues

- Include `Fixes #<issue>` or `Closes #<issue>` in PR body.
- List exact validation commands run.
- Include concise validation evidence.
- Add rollback/safety note if behavior changed.
- Confirm milestone label (`Phase 1`/`Phase 2`).
- Do not merge with unresolved deterministic CI failures.

Example:

```text
Fixes #425
Validation:
- cd gateway && bun run check
- cd gateway && bun test src/server.test.ts
- bash scripts/ci/check-action-pinning.sh
```

## Issue Quality Checklist

Template:

```text
Problem:
In scope:
Out of scope:
Acceptance Criteria:
- [ ] measurable behavior/result
- [ ] concrete file/path/surface
Risk/Rollback (optional):
```

Before (weak):

```text
Improve CI docs.
```

After (testable):

```text
Problem: Contributors cannot quickly collect failed CI logs.
In scope: Add docs/ci-failure-log-collection.md with gh run list/view examples.
Out of scope: workflow changes.
Acceptance Criteria:
- [ ] includes gh run list command for PR branch
- [ ] includes gh run view --log-failed example
- [ ] linked from CONTRIBUTING.md
Risk/Rollback: docs-only, revert if guidance is incorrect.
```

## CODEOWNERS Review Note

Security-sensitive paths are covered in `.github/CODEOWNERS`.

When touching covered files:

- Request CODEOWNERS review early.
- Avoid bundling unrelated refactors.
- Keep workflow/security changes small and auditable.

## Triage Utilities

- Duplicate review-followup detector: `bun scripts/triage/detect-review-followup-duplicates.ts`
- Unassigned issue report: `bash scripts/triage/unassigned-report.sh`
- Triage runbook: `docs/triage.md`

## Review Convention (Non-Blocking Suggestions)

For non-blocking review suggestions, use fixed prefix `NBS:`.

Rules:

- One suggestion per line.
- Each line starts with `NBS:`.

Example:

```text
NBS: Add edge-case test for missing request_id.
NBS: Clarify fallback behavior in README.
```

Post-merge automation creates follow-up issues from `NBS:` lines.
