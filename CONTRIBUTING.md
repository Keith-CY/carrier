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

CI enforcement scripts:

- `scripts/ci/check-action-pinning.sh` — verify third-party actions are pinned to SHAs
- `scripts/ci/check-acceptance-criteria.sh` — verify issue templates have Acceptance Criteria headings

Run actionlint locally:

```bash
# Install: https://github.com/rhysd/actionlint
actionlint
```

Run markdown link check locally:

```bash
npm install -g markdown-link-check@3.12.2
find . -name '*.md' -not -path './node_modules/*' -not -path './.git/*' | \
  xargs -I {} markdown-link-check --quiet --retry {}
```

Run duplicate issue title detection:

```bash
bash scripts/triage/detect-duplicate-titles.sh
```

Run triage classifier:

```bash
python3 scripts/triage/classify-issues.py
```

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

## Targeted test runs

Use these from repository root:

```bash
# Daemon tests only
cd daemon && go test ./...

# Gateway check + tests only
cd gateway && bun install --no-progress && bun run check && bun test

# Full local validation (daemon + gateway + e2e hook)
./scripts/run-all-tests.sh
```

## CI failure first-response runbook

When a PR check fails, triage in this order:

```bash
# 1) See all checks on the PR
gh pr checks <pr-number>

# 2) See required checks only
gh pr checks <pr-number> --required

# 3) Find recent workflow runs for the branch
gh run list --branch <branch-name> --limit 20

# 4) Inspect failing logs from a specific run
gh run view <run-id> --log-failed
```

Typical local re-run commands after log review:

```bash
cd daemon && go test ./...
cd gateway && bun install --no-progress && bun run check && bun test
./scripts/run-e2e-tests.sh
```

Notes:
- Use `gh pr checks <pr-number> --required` to separate blocking checks from informational/non-required checks.
- Docs-only pull requests may not trigger the default CI workflow because `.md` and `docs/**` are excluded there.

## Installer source of truth and update workflow

Canonical installer command source in this repository:
- `catalog/openclaw.manifest.json` (`runtime.install.command` and `runtime.upgrade.command`)

Do not duplicate installer command strings across scripts/docs as independent sources of truth.
If installer behavior changes, update the manifest first, then update dependent documentation/workflows.

Recommended verification sequence (from repository root):

```bash
# Verify manifest remains loadable
go test ./daemon/internal/manifest -run TestLoadFileAcceptsCatalogManifest -count=1

# Verify lifecycle install/upgrade flows still align with manifest commands
go test ./daemon/internal/lifecycle -run 'TestLifecycleInstallStartStop|TestLifecycleUpgradeCreatesBackupAndBumpsVersion' -count=1

# Verify release checksum generation path is still wired
rg -n 'sha256sum "\\${package_file}" > "\\${package_file}\\.sha256"' .github/workflows/release.yml
```

## Installer pre-merge checklist (docs-only guard)

For PRs that touch installer behavior, confirm before merge:
- [ ] Only canonical installer command definitions were edited in `catalog/openclaw.manifest.json` (single-source update).
- [ ] Any release/checksum documentation still matches `.github/workflows/release.yml` output path `dist/<archive>.zip.sha256`.
- [ ] Verification commands above were run from repo root and output is clean.
- [ ] If command strings changed, dependent docs reference the manifest path instead of re-stating command literals.

## Label glossary

Common labels and when to apply them:

- `P0`: Must-fix now for current delivery gate. Use for release/security/availability blockers.
- `P1`: Important next priority after P0. Use for high-impact work that is not an immediate gate blocker.
- `P2`: Maintainability/performance/developer-experience improvements.
- `P3`: Nice-to-have cleanup/polish.
- `Phase 1`: Work that must stay within Phase 1 scope and acceptance criteria.
- `enhancement`: Net-new capability or scope expansion.
- `documentation`: Documentation-only or documentation-heavy updates.
- `review-followup`: Use this as the issue title prefix (`[review-followup]`) for post-review non-blocking follow-up items.

Example label combination for prioritization:
- `Phase 1` + `P1` + `enhancement` means "important Phase 1 feature work, but behind P0 blockers."

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
