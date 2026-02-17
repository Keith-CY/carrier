# Carrier Project Agent Rules

## Pull Request Merge Policy
- Use `auto-merge` with `squash` for all project pull requests by default.

## Post-Merge Review Follow-up
- Use the internal skill: `skills/review-followup/SKILL.md`.
- Every non-blocking review suggestion must be marked with the fixed keyword prefix: `NBS:`.
- Write one `NBS:` suggestion per line.
- After merge, automation creates one follow-up issue per `NBS:` line.

## Open PR Review Memory Rule
- When reviewing open PRs, first check PR ownership and review state:
  - If the PR is my own and it already contains any `BS:` findings, post an update review addressing or re-evaluating those findings.
  - If the PR is not mine and I have already reviewed it with no remaining updates, skip re-review.
  - If the PR is not mine and there are still unresolved or new findings, review it again.
- For every review, leave findings as inline code comments tied to file/line locations whenever possible.
- Preserve existing `BS:` and `NBS:` labeling rules.
- Review outcome rule:
  - If any `BS:` finding is proposed, submit review with `request changes`.
  - If no `BS:` findings are proposed, submit `approve`.
