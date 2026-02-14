# Contributing

## Branching and Worktree Policy

All new development must start from `origin/main` and use a dedicated branch/worktree.

Recommended flow:

```bash
git fetch origin
git worktree add -b codex/<topic> ../carrier-<topic> origin/main
cd ../carrier-<topic>
```

Branch naming:
- Use `codex/` prefix for development branches.

## Pull Request Policy

Do not push feature work directly to `main`.

Use PR flow:

```bash
git add .
git commit -m "<message>"
git push -u origin codex/<topic>
gh pr create --base main --head codex/<topic>
```

For this repository we use Merge Queue:
- Open the queue from the GitHub PR page after CI is green.
- Do not use direct push-to-main flows except via merge queue/release process.

## Scope Policy

Phase milestones define delivery scope and must be respected in all planning and review.

- Phase 1: Deliver the full lifecycle for OpenClaw only.
- Every pull request must include a milestone label: `Phase 1` or `Phase 2`.
- Candidate agents are blocked until Phase 1 exit criteria pass.

## Skills Directory

- Before proposing changes, read the `skills/` directory and follow any instructions there (especially `skills/pr-review/SKILL.md` and `skills/review-followup/SKILL.md`).

## Testing Policy

GitHub Actions is the source of truth for test status.

- Required: CI must pass on PR before merge.
- Local tests are optional for faster iteration, but merge decisions are based on CI.

To reduce CI churn, we enforce pre-push tests locally:
- Enable the repo hook path once per clone:
  - `git config core.hooksPath .githooks`
- The pre-push hook runs `./scripts/run-all-tests.sh`, which includes daemon tests, gateway tests/checks, and end-to-end hook.

Current CI checks:
- Daemon Go tests
- Gateway TypeScript type check

### Required CI Check Names for PR Readiness

Before merging, the following required checks must be green:

- **Daemon Tests** — runs `go test ./...` in `daemon/` (job: `daemon-tests`)
- **Gateway Type Check** — runs `bun run check` in `gateway/` (job: `gateway-check`)
- **End-to-End Tests** — runs `./scripts/run-e2e-tests.sh` (job: `e2e-tests`, push-to-main only)

All required checks must pass before a PR can enter the merge queue.

## Label Glossary

The following labels are used for planning and priority triage:

- **`1h`** — Task estimated to take ≤1 hour of focused work. Use for small, well-scoped changes (e.g., add a single test, fix a typo, update one doc section).
- **`1h-Hotfix`** — Urgent fix that should take ≤1 hour and requires immediate attention. Use for production-impacting bugs or security patches (e.g., fix a nil-pointer crash in the start path).
- **`1h-Decomposition`** — A task whose scope needs to be broken down into ≤1h sub-tasks before work begins. Use when an issue is directionally clear but too large to implement in one shot (e.g., "refactor lifecycle service" → split into file-split, test, and doc sub-issues).
- **`Phase 1`** — Work required for Phase 1 delivery (OpenClaw full lifecycle). All Phase 1 PRs must be completed before candidate agent work begins.

See also the [Scope Policy](#scope-policy) section for milestone rules.

## Validation by Change Type

Before opening or updating a PR, run the minimum local validation for your change type:

- **Daemon code changes:**
  ```bash
  cd daemon
  go test ./...
  ```

- **Gateway code changes:**
  ```bash
  cd gateway
  bun install
  bun run check
  bun test
  ```

- **Docs-only changes:** Verify markdown links render correctly in GitHub preview. No CI test run is required (CI skips docs-only changes via `paths-ignore`).

- **CI/workflow changes:** Test the workflow logic locally where possible. For GitHub Actions changes, verify YAML syntax with a linter (e.g., `actionlint`).

These are minimum checks. The full local validation suite is available via `./scripts/run-all-tests.sh`.
## CI Troubleshooting

When a required CI check fails on your PR, reproduce the failure locally with the commands below.

**Daemon tests (with race detector):**

```bash
cd daemon
go test -race ./...
```

**Daemon lint (golangci-lint):**

```bash
cd daemon
golangci-lint run ./...
```

**Gateway type check:**

```bash
cd gateway
bun install
bun run check
```

**Gateway tests:**

```bash
cd gateway
bun test
```

**Expected tool versions:**
- Go: see `daemon/go.mod` (`go` directive)
- Bun: see `gateway/package.json` or CI workflow (`bun-version`)

These are the same commands CI runs. Fix failures locally before pushing updates.
## Cleanup Stale Worktrees and Branches

When using the issue-driven worktree workflow (`git worktree add …`), stale worktrees and branches accumulate over time. Use the commands below to clean up.

**List current worktrees:**

```bash
git worktree list
```

**Remove a stale worktree** (after its PR is merged):

```bash
git worktree remove /tmp/carrier-issue-<number>
```

If the directory was already deleted, prune the worktree records:

```bash
git worktree prune
```

**Delete merged local branches:**

```bash
git branch --merged main | grep -v '^\*\|main' | xargs -r git branch -d
```

**Sync before starting new work:**

```bash
git checkout main && git pull origin main
```

> **Tip:** Branch names follow the pattern `codex/issue-<number>-<short-desc>`. Avoid `git branch -D` (force delete) unless you are certain the branch has been merged or abandoned.
## Stale Follow-Up Detection

To find stale `[review-followup]` issues whose referenced PR is already merged:

```bash
./scripts/stale-followups.sh          # default: older than 7 days
./scripts/stale-followups.sh --days 14 # custom threshold
```

The script is report-only and does not modify any issues.

## Changelog Maintenance

This project maintains a [`CHANGELOG.md`](./CHANGELOG.md) following [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

**When to add entries:** Add a changelog entry when your PR is opened (not at merge time). The reviewer may suggest wording changes.

**Who updates release headings:** Maintainers move entries from `[Unreleased]` to a versioned heading during the release process.

**Entry format:** Use the category headers already in `CHANGELOG.md` and follow this format:

```markdown
### <Category>

- **Short description** — one-line summary of the change ([#PR](https://github.com/Keith-CY/carrier/pull/PR))
```

**Categories used in this repo:**
- `Features` — new capabilities
- `Bug Fixes` — corrections to existing behavior
- `Security` — vulnerability fixes or hardening
- `Refactor` — internal restructuring without behavior change
- `Docs` — documentation additions or updates
- `Tests` — new or improved test coverage
- `CI` — build/CI pipeline changes

**Example entry:**

```markdown
### Features

- **WebSocket support** — add real-time event streaming to gateway ([#999](https://github.com/Keith-CY/carrier/pull/999))
```

## Review Convention (Non-Blocking Suggestions)

For review comments, use the fixed keyword `NBS:` for each non-blocking suggestion.

Rules:
- one suggestion per line
- each line starts with `NBS:`

Example:
```text
NBS: Add edge-case test for missing request_id.
NBS: Clarify fallback behavior in README.
```

Post-merge automation parses `NBS:` lines and creates one follow-up issue per suggestion.

### Review Wording: Blocking vs Non-Blocking

Use consistent language to distinguish blocking feedback from non-blocking suggestions:

- **BS (blocking suggestion):** The PR should not merge until this is addressed. Write it as a normal review comment or request-changes review.
- **NBS (non-blocking suggestion):** A nice-to-have improvement that can be addressed in a follow-up. Prefix each line with `NBS:`.

**Examples:**

Blocking comment (normal review):
```text
This function can panic on nil input — add a nil guard before merge.
```

Non-blocking suggestions:
```text
NBS: Consider extracting the retry logic into a shared helper.
NBS: The error message could include the agent ID for easier debugging.
```

Non-blocking suggestions tagged with `NBS:` are automatically converted to follow-up issues by the `review-nbs-followup` automation after the PR merges.
## Quick CI Inspection and Rerun Workflow

When a CI check fails on your PR, use these `gh` commands to inspect and rerun without leaving the terminal.

**1. List recent workflow runs for your PR branch:**

```bash
gh run list --branch "$(git branch --show-current)" --limit 5
```

**2. View logs for the failed run:**

```bash
gh run view <run-id> --log-failed
```

**3. Rerun only the failed jobs:**

```bash
gh run rerun <run-id> --failed
```

**4. Rerun the entire workflow (all jobs):**

```bash
gh run rerun <run-id>
```

**Permissions note:** You need write access to the repository to trigger reruns. If the button is unavailable, ask a maintainer to rerun or push an update to your PR branch to trigger a fresh run.
## Local ShellCheck Lint

Run shellcheck across all repository shell scripts:

```bash
bash scripts/run-shellcheck.sh
```

The script discovers `scripts/**/*.sh` files, runs shellcheck on each, and exits non-zero on any findings. Requires `shellcheck` to be installed locally.
## GitHub CLI Preflight and Troubleshooting

Before running review or triage automation scripts locally, verify your `gh` setup:

### Preflight checklist

```bash
# 1. Confirm authentication
gh auth status

# 2. Verify required scopes (need repo, read:org at minimum)
gh auth status -t 2>&1 | grep -i scopes

# 3. Confirm repo context
gh repo view Keith-CY/carrier --json nameWithOwner -q '.nameWithOwner'
```

### Common failures and fixes

**401 Unauthorized / 403 Forbidden:**
- Run `gh auth login` to re-authenticate.
- If using a PAT, ensure it has `repo` scope. Fine-grained tokens need "Issues" and "Pull requests" read/write.

**"Could not resolve to a Repository":**
- You are targeting the wrong repo. Always pass `--repo Keith-CY/carrier` explicitly when running outside a local clone.

**Rate limiting (HTTP 403 with `rate limit` message):**
- Check remaining quota: `gh api rate_limit -q '.rate.remaining'`
- Wait for reset or authenticate with a different token.

These checks apply to both PR review (`gh pr view`, `gh pr checks`) and issue triage (`gh issue list`, `gh issue view`) workflows.
## Selective Test Runs (Quick Start)

For faster local validation, run only the tests relevant to your change instead of the full suite.

### Daemon (Go)

```bash
# Full suite
cd daemon && go test ./...

# Single package
cd daemon && go test ./internal/lifecycle/...

# Single test by name
cd daemon && go test ./internal/memory -run TestXxx

# With verbose output
cd daemon && go test -v ./internal/manifest/...
```

### Gateway (TypeScript / Bun)

```bash
# Full type check + tests
cd gateway && bun install && bun run check && bun test

# Type check only (fast)
cd gateway && bun run check

# Run tests only
cd gateway && bun test

# Single test file
cd gateway && bun test src/index.test.ts
```

### When to run the full suite

Before opening or updating a PR, run `./scripts/run-all-tests.sh` from the repo root at least once to ensure full CI parity. Selective runs are for iteration speed during development.
## Conflict Resolution (DIRTY PRs)

When `gh pr view --json mergeStateStatus` returns `DIRTY`, the PR has merge conflicts with `main`. Resolve them with the following flow:

### Step-by-step

```bash
# 1. Switch to your PR branch
git checkout codex/my-feature

# 2. Fetch latest main
git fetch origin main

# 3. Rebase onto main (preferred) or merge
git rebase origin/main
# If you prefer merge: git merge origin/main

# 4. Resolve conflicts in marked files
#    Edit files, then:
git add <resolved-files>
git rebase --continue   # or git commit if using merge

# 5. Run local checks before pushing
cd daemon && go test ./...
cd ../gateway && bun install && bun run check && bun test

# 6. Force-push the rebased branch
git push --force-with-lease origin codex/my-feature
```

### Important notes

- **Never push directly to `main`** to resolve conflicts.
- After force-push, verify CI checks pass on the updated PR before requesting re-review.
- If conflicts are complex, consider asking the original author for help.
## Multiline Review Comments with --body-file

For multiline review comments or PR reviews, use `--body-file` instead of inline escaped newlines. This is more reliable for automation and avoids shell quoting issues.

### Preparing a review body file

```bash
cat > /tmp/review.md << 'EOF'
Overall the change looks good. Two non-blocking suggestions:

NBS: Extract the retry logic into a shared helper for reuse across providers.
NBS: The error message could include the agent ID for easier debugging.
EOF
```

### Submitting a review with --body-file

```bash
# Submit an approving review with multiline body
gh pr review <PR_NUMBER> --repo Keith-CY/carrier --approve --body-file /tmp/review.md

# Submit a comment-only review (no approval/rejection)
gh pr review <PR_NUMBER> --repo Keith-CY/carrier --comment --body-file /tmp/review.md
```

### Submitting a PR comment with --body-file

```bash
gh pr comment <PR_NUMBER> --repo Keith-CY/carrier --body-file /tmp/review.md
```

### Why --body-file over inline --body

- Avoids shell escaping issues with newlines, quotes, and special characters.
- Keeps `NBS:` formatting intact (one suggestion per line).
- Easier to review/edit before submitting.
- More reliable in scripts and CI automation.
## PR Checks Status Interpretation Guide

When reviewing a PR or running `gh pr checks`, you will see various status values. Here is what each means and what action to take:

| Status | Meaning | Action |
|--------|---------|--------|
| `pass` | Check completed successfully | ✅ No action needed |
| `pending` | Check is still running | ⏳ Wait for completion |
| `fail` | Check failed | 🔴 Fix the failure before merge |
| `skipping` | Check was skipped (e.g., path filter) | ✅ Non-blocking; does not prevent merge |
| `cancelled` | Check was cancelled | 🔄 Re-run the check or investigate |
## PR Merge State Quick Reference

When checking merge readiness with `gh pr view --json mergeStateStatus`, you may see these values:

| Status | Meaning | Typical cause | Suggested action |
|--------|---------|---------------|------------------|
| `CLEAN` | Ready to merge | All checks pass, no conflicts, reviews satisfied | Merge or enable auto-merge |
| `BLOCKED` | Cannot merge yet | Missing required reviews or failing checks | Check `gh pr checks` and request reviews |
| `DIRTY` | Has merge conflicts | Branch diverged from `main` | Rebase or merge `main` into your branch (see [Conflict Resolution](#conflict-resolution-dirty-prs)) |
| `UNKNOWN` | State not yet computed | GitHub is still calculating | Wait a few seconds and re-check |

### Quick check command

```bash
# View all checks for a PR
gh pr checks <PR_NUMBER> --repo Keith-CY/carrier
```

### Merge readiness

A PR is merge-ready when:
- All **required** checks show `pass` (or `skipping` for path-filtered jobs).
- No required check shows `fail` or `cancelled`.
- `pending` checks must complete before merge.

See the [Required CI Check Names](#required-ci-check-names-for-pr-readiness) section for the list of required checks in this repo.
## Auto-Merge Prerequisites

When using `gh pr merge --auto --squash`, the merge will only proceed once all prerequisites are met. If it shows `BLOCKED`, check the following:

### Checklist

1. **Required reviews satisfied** — At least one approving review (no outstanding request-changes).
2. **Required checks passing** — All required CI checks (`daemon-tests`, `gateway-check`) must be `pass` or `skipping`.
3. **Branch up to date** — If branch protection requires "up to date with base", rebase or merge `main` into your branch.
4. **No merge conflicts** — `mergeStateStatus` must not be `DIRTY`.

### Commands

```bash
# Enable auto-merge on a PR
gh pr merge <PR_NUMBER> --repo Keith-CY/carrier --auto --squash

# Check current CI status
gh pr checks <PR_NUMBER> --repo Keith-CY/carrier

# Check merge state
gh pr view <PR_NUMBER> --repo Keith-CY/carrier --json mergeStateStatus -q '.mergeStateStatus'
```

### Common "checks pending" case

If auto-merge shows `BLOCKED` immediately after enabling, it usually means CI checks are still running. This is expected — auto-merge will proceed automatically once all required checks pass. Monitor with `gh pr checks`.
## Issue Decomposition Guide

When splitting large roadmap issues (e.g., `[L2]` / `[Round-1]`) into lightweight follow-up tasks, use the following checklist.

### Splitting checklist

Before creating a sub-issue, verify:
1. **Independence** — Can it be implemented and reviewed without waiting for other sub-issues?
2. **Risk** — Is the change low-risk (no architectural decisions, no breaking changes)?
3. **Review size** — Will the PR be reviewable in one sitting (ideally <200 lines)?
4. **Rollback safety** — Can the change be reverted without affecting other work?

### Good lightweight follow-up examples

| Type | Example title |
|------|---------------|
| Docs | `docs(contributing): add selective test-run quickstart` |
| Test | `test(daemon): add reload-failure state-preservation regression tests` |
| Refactor | `refactor(gateway): extract rate-limit config into separate module` |

### Anti-patterns (too broad for lightweight assignment)

- "Refactor the entire lifecycle service" — too many files, needs design discussion.
- "Add provider support for WhatsApp" — new feature, not a follow-up.
- "Redesign the session store interface" — architectural, affects multiple consumers.

When in doubt, ask: "Can someone implement this in ≤1 hour with no design ambiguity?" If not, decompose further.
gh pr view <PR_NUMBER> --repo Keith-CY/carrier --json mergeStateStatus -q '.mergeStateStatus'
```
