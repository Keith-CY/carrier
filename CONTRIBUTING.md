# Contributing

## Branching and Worktree Policy

All new development must start from `origin/main` and use a dedicated branch/worktree.

Recommended flow:

```bash
git fetch origin
git worktree add -b codex/<topic> ../carrier-<topic> origin/main
cd ../carrier-<topic>
```

Branch naming convention:
- Use `codex/` prefix for all development branches.
- Pattern: `codex/issue-<id>-<short-slug>` for issue-driven work.
- Pattern: `codex/<topic>` for ad-hoc maintenance or exploration.
- **Never push directly to `main`.**

### Branch Naming Examples

| Issue type | Branch name |
|------------|-------------|
| Docs update (#270) | `codex/issue-270-branch-naming` |
| Test addition (#245) | `codex/issue-245-reload-failure-tests` |
| Script/chore (#323) | `codex/issue-323-review-followup-dupes` |
| Ad-hoc refactor | `codex/refactor-session-store` |

Keep slugs short and lowercase with hyphens. Avoid special characters.

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

- **Shell script changes:** Run `shellcheck scripts/*.sh` locally to catch portability/quoting issues before CI does.

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
## ShellCheck (Local Lint)

Run ShellCheck across all repository scripts to match CI behavior:

```bash
./scripts/run-shellcheck.sh
```

The script discovers all `scripts/**/*.sh` files and exits non-zero on any lint error.


### Inspecting and Rerunning Failed CI

Use `gh` to quickly inspect failures and rerun jobs without leaving the terminal:

```bash
# List recent workflow runs for your PR
gh run list --branch <your-branch> --limit 5

# View logs for the failed step
gh run view <run-id> --log-failed

# Rerun only failed jobs (requires write access)
gh run rerun <run-id> --failed
```

> **Note:** Rerunning requires write/maintain permissions. If you lack access, push a fixup commit to trigger a new run.

### Transient GitHub API Failures

When `gh pr checks` or `gh run view` fails unexpectedly, check these common causes:

**DNS/connectivity issues:**

```bash
# Verify connectivity
curl -s https://api.github.com/zen
# Retry after a short wait
sleep 5 && gh pr checks <pr-number>
```

**GitHub API rate limits:**

```bash
# Check remaining quota
gh api rate_limit --jq '.rate | "\(.remaining)/\(.limit) (resets \(.reset | todate))"'
```

If rate-limited, wait for the reset time or authenticate with a token that has higher limits.

**Expired or missing auth session:**

```bash
# Check auth status
gh auth status
# Re-authenticate if needed
gh auth login
```
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
- `Security` — vulnerability fixes or hardening (see also [`docs/security-install-integrity.md`](./docs/security-install-integrity.md) for install/upgrade verification)
- `Refactor` — internal restructuring without behavior change
- `Docs` — documentation additions or updates
- `Tests` — new or improved test coverage
- `CI` — build/CI pipeline changes

**Example entry:**

```markdown
### Features

- **WebSocket support** — add real-time event streaming to gateway ([#999](https://github.com/Keith-CY/carrier/pull/999))
```

## Re-Review Rule (Head SHA vs Review SHA)

After new commits are pushed to a PR, determine whether re-review is needed by comparing the current head SHA against the SHA that was last reviewed.

**Fetch the current PR head SHA:**

```bash
gh pr view <PR_NUMBER> --repo Keith-CY/carrier --json headRefOid --jq '.headRefOid'
```

**Fetch the SHA of the last reviewed commit:**

```bash
gh pr view <PR_NUMBER> --repo Keith-CY/carrier --json latestReviews \
  --jq '.latestReviews[0].commit.oid'
```

**Decision flow:**

1. If `headRefOid == latestReviews[].commit.oid` → **skip** (already reviewed at this commit).
2. If they differ → **re-review required** (new commits since last approval).

This rule applies to both manual review sweeps and automated review triggers.

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
## PR Check Health Summary

To get a quick overview of open PR check statuses:

```bash
bash scripts/pr-check-summary.sh
```

The script prints counts for green, pending, and failing PRs, and lists PR numbers for non-green entries. It uses `gh` JSON output and performs no write/mutation actions.
