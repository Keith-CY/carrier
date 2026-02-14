# Issue Triage Skill (Carrier)

## Purpose
Guide agents to periodically check open issues and pick up work that needs attention.

## Triage Process
1. List open issues: `gh issue list --repo Keith-CY/carrier --state open --json number,title,assignees,labels,createdAt`
2. Categorize each issue:
   - **Assigned to me**: check if work has started (existing branch/PR). If not, evaluate complexity.
   - **Unassigned**: evaluate if lightweight enough to self-assign.

## Lightweight vs Heavy
**Lightweight (auto-handle):**
- Documentation updates, README changes
- Template additions (issue templates, PR templates)
- Small bug fixes with clear scope
- Configuration changes
- Adding tests for existing code

**Heavy (report only, don't auto-handle):**
- New features requiring design decisions
- Architecture changes
- Multi-file refactors spanning daemon + gateway
- Anything touching security-critical paths without clear spec

## Handling Lightweight Issues
1. Self-assign: `gh issue edit <number> --repo Keith-CY/carrier --add-assignee @me`
2. Create worktree: `git worktree add /tmp/carrier-issue-<N> -b codex/issue-<N>-<description> origin/main` (where `<description>` is a short kebab-case summary of the issue title, e.g., `codex/issue-42-fix-readme-typo`)
3. Implement changes, run tests
4. Push and create PR: `gh pr create --repo Keith-CY/carrier --base main`
5. **Never push directly to main or develop**
6. **Never self-merge** — wait for maintainer review

## Auto-Triage Cadence
A cron job runs every 15 minutes to scan issues and act on lightweight ones.
