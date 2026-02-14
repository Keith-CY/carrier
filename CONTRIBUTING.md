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
