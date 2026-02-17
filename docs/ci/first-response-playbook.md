# CI First-Response Playbook

Source issue: #675

Quick map from high-frequency failures to first-response commands.

## 1) Failing Unit Tests

```bash
gh pr checks <pr-number> --repo Keith-CY/carrier
gh run list --repo Keith-CY/carrier --branch <branch-name> --limit 10
gh run view <run-id> --repo Keith-CY/carrier --log-failed
```

First response:
- isolate failing suite/module
- reproduce locally
- push focused fix

## 2) Flaky Checks / Reruns

```bash
gh run view <run-id> --repo Keith-CY/carrier --log-failed
gh run rerun <run-id> --repo Keith-CY/carrier --failed
```

First response:
- rerun failed jobs only when failure is non-deterministic
- avoid rerun loops for deterministic failures

## 3) Workflow Permission Failures

```bash
gh run view <run-id> --repo Keith-CY/carrier --log-failed
```

First response:
- verify workflow/job permissions block in the failing workflow
- confirm token scope matches operation (`contents`, `pull-requests`, `actions`)

## 4) Pinned Action SHA Drift

```bash
bash scripts/ci/check-action-pinning.sh
```

First response:
- replace mutable refs for third-party actions with commit SHAs
- validate with pinning guard before pushing
