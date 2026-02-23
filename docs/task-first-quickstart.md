# Task-First Quickstart and Troubleshooting

This guide is task-oriented.
It is the fastest path from clean machine to a working first session.

## Task 1: 5-Minute First Setup

Prerequisites:
- Go 1.23+ (matches `daemon/go.mod` / `toolchain go1.23.0`)
- Bun 1.0+ (required by gateway validation scripts)
- Repository cloned

Commands:
```bash
./scripts/run-all-tests.sh
```

Expected output:
- daemon tests pass
- gateway checks pass

Rollback notes:
- If any check fails, do not continue with release/install workflows.
- Move to Task 3 (common failure triage) before retrying.

## Task 2: Connect Provider and Verify Health

Prerequisites:
- daemon running
- pairing code available

Commands:
```text
carrier add openclaw
/pair <code>
/agents
/start openclaw
/status openclaw
```

Expected output:
- `carrier add openclaw` completes install/start for OpenClaw
- `/pair` returns success
- `/status openclaw` reports running/healthy

Rollback notes:
- If startup fails, run `/stop openclaw` and then `/diagnose openclaw`.
- If chat reports `E_INSTALL_GUI_ONLY`, use `carrier add <agent_id>` in TUI/WebUI; chat install is intentionally blocked.
- Keep diagnose artifacts with request IDs before retrying.

## Task 3: Fix Common Failures Quickly

Use this decision table first:

| Signal | Action |
|---|---|
| `E_PAIR_CODE_INVALID` | Request a new pair code and retry `/pair` immediately |
| `E_SESSION_REQUIRED` | Re-run `/pair <code>` before non-pair commands |
| start fails with missing env | Set required env vars and retry `/start` |
| port conflict | stop conflicting process, then retry `/start` |

Detailed references:
- Pairing failures: [`docs/runbooks/pairing-lifecycle.md`](./runbooks/pairing-lifecycle.md)
- CI/test failures: [`docs/ci/first-response-playbook.md`](./ci/first-response-playbook.md)

Rollback notes:
- After one corrected retry, if failure repeats, escalate with request ID and logs.

## Task 4: Safe Upgrade and Rollback

Prerequisites:
- agent currently installed
- maintenance window confirmed

Commands:
```text
/status openclaw
/upgrade openclaw
/status openclaw
```

Expected output:
- upgrade succeeds and returns a version bump
- post-upgrade status is healthy

Rollback notes:
- If upgrade fails, use captured pre-upgrade backup guidance in error output.
- Follow runbook: [`docs/runbooks/go-live-rollback.md`](./runbooks/go-live-rollback.md).
