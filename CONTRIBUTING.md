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
## Stale Follow-Up Detection

To find stale `[review-followup]` issues whose referenced PR is already merged:

```bash
./scripts/stale-followups.sh          # default: older than 7 days
./scripts/stale-followups.sh --days 14 # custom threshold
```

The script is report-only and does not modify any issues.

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
