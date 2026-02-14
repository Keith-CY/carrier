# Review Follow-up Skill (Carrier)

## Purpose
Standardize how reviewers write non-blocking suggestions so automation can convert them into post-merge issues.

## Fixed keyword
Use this exact prefix for each non-blocking suggestion:

`NBS:`

NBS = Non-Blocking Suggestion.

## Format rules
- Put **one suggestion per line**.
- Each suggestion line must start with `NBS:`.
- Keep each suggestion actionable and specific.
- If there are multiple suggestions, write multiple `NBS:` lines.

## Example
```text
NBS: Add focused unit tests for invalid consent flag parsing.
NBS: Document default retention TTL in README M3 section.
```

## Blocking vs non-blocking
- Blocking: use normal review comments / change requests.
- Non-blocking: use `NBS:` lines so they can be auto-tracked after merge.

## Automation contract
After PR merge, GitHub Action `review-nbs-followup` scans PR reviews/comments for `NBS:` lines and creates one issue per suggestion.
