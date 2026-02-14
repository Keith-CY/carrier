# PR Review Skill (Carrier)

## Purpose
Guide agents to review pull requests like a human collaborator — with inline code comments on specific lines, not just summary approvals.

## Review Process
1. Fetch the PR diff via `gh pr diff <number> --repo Keith-CY/carrier`
2. Read the full diff before commenting
3. Submit a review using the GitHub Review API with inline `comments` array (path, line, side, body)
4. Approve if code is correct; request changes if blocking issues found

## What to Check
- **Security**: command injection, credential leaks, unsafe file operations
- **Error handling**: missing error checks, silent failures
- **Naming**: consistency with existing conventions in the codebase
- **Test coverage**: new logic should have tests
- **Consistency**: alignment with patterns in existing PRs (e.g., `recordAudit` for audit events)
- **Dependencies**: accidental inclusion of `node_modules` or build artifacts

## Comment Style
- **Blocking issues**: use normal review comments or request changes
- **Non-blocking suggestions**: prefix with `NBS:` per `skills/review-followup/SKILL.md`
- One suggestion per `NBS:` line
- Reference related PRs/issues when relevant (e.g., "This conflicts with the pattern in PR #14")

## Build Verification
Before approving, confirm all CI checks have passed:
- Check CI status: `gh pr checks <number> --repo Keith-CY/carrier`
- **Daemon (Go)**: `cd daemon && go test ./...`
- **Gateway (TypeScript)**: `cd gateway && bun run check`

## Auto-Review Cadence
A cron job runs every 15 minutes to check for unreviewed open PRs and submit reviews automatically.
