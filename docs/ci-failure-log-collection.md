# CI Failure Log Collection (gh CLI Quick Guide)

Source issue: #672

Use these commands during PR triage to collect failed GitHub Actions logs quickly.

## 1) Find runs for a PR branch

```bash
gh run list \
  --repo Keith-CY/carrier \
  --branch <branch-name> \
  --limit 10
```

## 2) Inspect failed jobs for one run

```bash
gh run view <run-id> \
  --repo Keith-CY/carrier
```

## 3) Fetch failed-step logs only

```bash
gh run view <run-id> \
  --repo Keith-CY/carrier \
  --log-failed
```

## 4) Optional: filter by workflow name first

```bash
gh run list \
  --repo Keith-CY/carrier \
  --workflow "CI" \
  --branch <branch-name> \
  --limit 10
```

Use this output in PR comments when escalating CI failures.
