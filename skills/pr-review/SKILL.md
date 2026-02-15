# PR Review Skill (Carrier)

## Purpose
Guide agents to review pull requests like a human collaborator — with inline code comments on specific lines, not just summary approvals.

## Review Process
1. Check CI first via `gh pr checks <number> --repo Keith-CY/carrier`.
2. **Only perform formal code review after all required CI checks are green.**
3. If CI is pending or failing:
   - Do not submit Approve/Request Changes review yet.
   - Leave at most a short status comment that review is waiting on green CI.
   - Re-check the same PR after CI completes, even if there is no new commit.
4. Fetch the PR diff via `gh pr diff <number> --repo Keith-CY/carrier`.
5. Read the full diff before commenting.
6. Submit a review using the GitHub Review API with inline `comments` array (path, line, side, body).
7. Apply decision rules strictly:
   - If there is any **BS** (Blocking Suggestion), submit **Request Changes**.
   - If there are only **NBS** (Non-Blocking Suggestions), leave `NBS:` comments per format and keep review non-blocking.
   - If there is no BS, submit **Approve**.

## What to Check
- **Security**: command injection, credential leaks, unsafe file operations
- **Error handling**: missing error checks, silent failures
- **Naming**: consistency with existing conventions in the codebase
- **Test coverage**: new logic should have tests
- **Consistency**: alignment with patterns in existing PRs (e.g., `recordAudit` for audit events)
- **Dependencies**: accidental inclusion of `node_modules` or build artifacts

## Comment Style
- **BS (Blocking Suggestion)**:
  - Use clear blocking language and expected fix.
  - If any BS exists in the review, overall decision must be **Request Changes**.
- **NBS (Non-Blocking Suggestion)**:
  - Prefix each non-blocking suggestion with `NBS:` per `skills/review-followup/SKILL.md`.
  - One suggestion per `NBS:` line.
- If no BS exists, final decision should be **Approve**.
- Reference related PRs/issues when relevant (e.g., "This conflicts with the pattern in PR #14").

## Build Verification
CI is a hard gate for review decisions:
- Check CI status first: `gh pr checks <number> --repo Keith-CY/carrier`
- Do not submit final review decisions until required checks are green.
- If CI is pending/failing, queue re-check and revisit when CI completes (even without new commits).

Optional local verification when needed:
- **Daemon (Go)**: `cd daemon && go test ./...`
- **Gateway (TypeScript)**: `cd gateway && bun run check`

## Automation Trigger
This skill is intended to be executed by repository automation (for example cron-driven sweeps). Keep cadence details in the automation configuration/prompt as the source of truth.
