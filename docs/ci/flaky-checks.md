# Flaky Check Triage Playbook

Source issue: #641

## Step-by-Step

1. Identify the failing check and run ID.
2. Read failed-step logs (`gh run view --log-failed`).
3. Classify as flaky vs deterministic.
4. Rerun failed jobs only when flaky criteria are met.
5. Push a fix when failure is deterministic.

## Classify Failure Type

Flaky signals:
- transient network timeout
- runner provisioning hiccup
- non-reproducible timing race

Deterministic signals:
- same assertion/file line fails repeatedly
- type check fails in changed module
- permission/policy check fails consistently

## Example: Daemon Test Failure

```bash
gh pr checks <pr-number> --repo Keith-CY/carrier
gh run view <run-id> --repo Keith-CY/carrier --log-failed
```

If log shows stable Go test failure under `daemon/...`, push a test/code fix.

## Example: Gateway Check Failure

```bash
gh run view <run-id> --repo Keith-CY/carrier --log-failed
```

If log shows repeatable Go test failure under `daemon/internal/gateway/...`, push a code fix.
