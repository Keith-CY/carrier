# Flaky Check Rerun Runbook

Source issue: #607

This runbook defines when rerun is acceptable and when code changes are required.

## Minimal Triage Flow

```bash
gh run list --repo Keith-CY/carrier --branch <branch-name> --limit 10
gh run view <run-id> --repo Keith-CY/carrier
gh run view <run-id> --repo Keith-CY/carrier --log-failed
gh run rerun <run-id> --repo Keith-CY/carrier --failed
```

## Rerun Allowed Checklist

- Failure signature is non-deterministic (timeout/network/infra flake).
- No codepath regression evidence in failed logs.
- Same commit has at least one prior pass on the same check.
- Rerun scope is limited to failed jobs.

## Must-Fix Checklist

- Deterministic test assertion failure.
- Type check/build error tied to changed files.
- Repeated failure with identical stack trace across reruns.
- Security or permission policy failure.

If any Must-Fix item matches, push a code/workflow fix instead of repeated reruns.
