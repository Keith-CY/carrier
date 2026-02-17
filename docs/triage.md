# Triage Runbook

Source issues: #632 #639 #668

## Unassigned Issue Report (Read-only)

Run:

```bash
bash scripts/triage/unassigned-report.sh
```

Output includes:

- issue number
- title
- age in days
- priority label (`P0` / `P1` / `none`)
- grouping by priority and age bucket (`<7d`, `7-30d`, `>30d`)

Example output snippet:

```text
## Priority: P1
### Age: 7-30d
- #425 [P1] (12d) [L3][Runbook] Document pairing lifecycle and troubleshooting matrix
```

## Duplicate `[review-followup]` Detector (Read-only)

Run:

```bash
bun scripts/triage/detect-review-followup-duplicates.ts
```

The script:

- lists open `[review-followup]` issues
- normalizes titles
- reports likely duplicate groups
- does not create/edit/close any issue

## Review-followup Duplicate Triage

### Deterministic Keep/Close Rule

1. Keep the oldest open issue in the duplicate group.
2. Close newer duplicates.
3. Cross-link both directions (kept issue references closed duplicates; closed issue points to kept issue).

### Close Comment Template

```text
Closing as duplicate of #<kept-issue-number>.
This issue has overlapping scope and title after normalization under the review-followup triage rule.
Keeping #<kept-issue-number> as the canonical thread for tracking and updates.
```

### Example Workflow

1. Run duplicate detector.
2. Group shows `#101`, `#134`, `#152` as same normalized title.
3. Keep `#101` (oldest), close `#134` and `#152` using the template above.
